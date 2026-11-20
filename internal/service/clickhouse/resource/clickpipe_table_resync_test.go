package resource

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// A change to an existing table mapping is rejected unless the pipe sets
// resync_tables_on_change, and warns about the resync when it does.
func TestClickPipeResource_ModifyPlan_TableResync(t *testing.T) {
	ctx := context.Background()
	unknownColumns := types.ListUnknown(models.ClickPipeMongoDBColumnModel{}.ObjectType())

	warningContains := func(diags diag.Diagnostics, substr string) bool {
		for _, d := range diags.Warnings() {
			if strings.Contains(d.Detail(), substr) {
				return true
			}
		}
		return false
	}

	tests := []struct {
		name string
		// build returns the prior state and the plan, before resync is applied to the plan.
		build  func(t *testing.T) (state, plan models.ClickPipeResourceModel)
		resync bool
		// wantError is a substring of the expected error; when empty, the plan must have no error.
		wantError string
		// wantWarning is a substring of the expected resync warning.
		wantWarning string
	}{
		{
			name: "postgres mapping change without resync_tables_on_change is rejected",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				state = postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users")
				plan = postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users")
				setPostgresMappingAttr(ctx, t, &plan, "partition_by_expr", types.StringValue("toYYYYMM(created_at)"))
				return state, plan
			},
			wantError: "Cannot modify partition_by_expr for existing table mapping 'public.users'. Set resync_tables_on_change = true",
		},
		{
			name: "postgres mapping change with resync_tables_on_change warns",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				return postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users"),
					postgresPlanWithExcludedColumns(ctx, []string{"status", "email"}, "users")
			},
			resync:      true,
			wantWarning: "excluded_columns changed for table mapping 'public.users'",
		},
		{
			name: "postgres excluded_columns null to empty is not a change",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				state = postgresPlanWithExcludedColumns(ctx, []string{}, "users")
				setPostgresMappingAttr(ctx, t, &state, "excluded_columns", types.SetNull(types.StringType))
				return state, postgresPlanWithExcludedColumns(ctx, []string{}, "users")
			},
		},
		{
			name: "target_table cannot change even with resync_tables_on_change",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				return postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users"),
					postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users_renamed")
			},
			resync:    true,
			wantError: "Cannot modify target_table",
		},
		{
			name: "mysql mapping change with resync_tables_on_change warns",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				state, plan = mysqlPlanForModifyPlan(), mysqlPlanForModifyPlan()
				setMySQLMappingAttr(ctx, t, &plan, "table_engine", types.StringValue("Null"))
				return state, plan
			},
			resync:      true,
			wantWarning: "table_engine changed for table mapping 'public.users'",
		},
		{
			name: "mongodb mapping change without resync_tables_on_change is rejected",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				return mongoDBModelWithMappings(t, mongoDBTableMappingModel(types.BoolNull())),
					mongoDBModelWithMappings(t, mongoDBTableMappingModel(types.BoolValue(true)))
			},
			wantError: "Cannot modify persist_deleted_documents",
		},
		{
			// The repair restores the inferred columns before the checks, so they
			// are sent with the resync.
			name: "mongodb structured ingestion change keeps the inferred columns",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				changed := mongoDBStructuredIngestion(unknownColumns)
				changed.SortingKeys = stringListValue([]string{"name"})
				return mongoDBModelWithMappings(t, mongoDBStructuredMapping("replicants", mongoDBStructuredIngestion(mongoDBColumns("name", "String")))),
					mongoDBModelWithMappings(t, mongoDBStructuredMapping("replicants", changed))
			},
			resync:      true,
			wantWarning: "structured_ingestion changed for table mapping 'shop.replicants'",
		},
		{
			name: "mongodb enabling structured ingestion on resync requires columns",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				return mongoDBModelWithMappings(t, mongoDBTableMappingModel(types.BoolNull())),
					mongoDBModelWithMappings(t, mongoDBStructuredMapping("orders", mongoDBStructuredIngestion(unknownColumns)))
			},
			resync:    true,
			wantError: "structured_ingestion.columns must be set to resync collection 'shop.orders'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, plan := tt.build(t)
			plan.ResyncTablesOnChange = types.BoolValue(tt.resync)

			diags := driveClickPipeModifyPlan(ctx, t, state, plan)

			if tt.wantError != "" {
				assert.True(t, errorDetailContains(diags, tt.wantError), "got: %v", diags.Errors())
			} else {
				assert.False(t, diags.HasError(), "got: %v", diags.Errors())
			}
			if tt.wantWarning != "" {
				assert.True(t, warningContains(diags, tt.wantWarning), "got: %v", diags.Warnings())
			}
		})
	}
}

// A changed mapping is sent in tableMappingsToResync, in a mappings-only PATCH
// (#617) issued while the pipe is paused.
func TestClickPipeResource_Update_TableResync(t *testing.T) {
	ctx := context.Background()
	const partitionByExpr = "toYYYYMM(created_at)"

	tests := []struct {
		name string
		// build returns the prior state, the plan before resync is applied, and the pipe the API returns.
		build func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe)
		check func(t *testing.T, update api.ClickPipeUpdate)
	}{
		{
			name: "postgres",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = postgresUpdateModel(ctx, t, "users")
				plan = postgresUpdateModel(ctx, t, "users")
				setPostgresMappingAttr(ctx, t, &plan, "partition_by_expr", types.StringValue(partitionByExpr))
				return state, plan, postgresAPIPipe(api.ClickPipeRunningState, "users")
			},
			check: func(t *testing.T, update api.ClickPipeUpdate) {
				require.NotNil(t, update.Source)
				pg := update.Source.Postgres
				require.NotNil(t, pg)
				require.Len(t, pg.TableMappingsToResync, 1)
				assert.Equal(t, "users", pg.TableMappingsToResync[0].SourceTable)
				assert.Equal(t, "users", pg.TableMappingsToResync[0].TargetTable)
				require.NotNil(t, pg.TableMappingsToResync[0].PartitionByExpr)
				assert.Equal(t, partitionByExpr, *pg.TableMappingsToResync[0].PartitionByExpr)
				assert.Empty(t, pg.TableMappingsToAdd)
				assert.Empty(t, pg.TableMappingsToRemove)
				assert.Empty(t, pg.Host, "a mappings-only edit must not re-send the connection")
			},
		},
		{
			name: "mysql",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = mysqlPlanForModifyPlan()
				state.State = types.StringValue(api.ClickPipeRunningState)
				state.Stopped = types.BoolValue(false)
				plan = state
				setMySQLMappingAttr(ctx, t, &plan, "table_engine", types.StringValue("Null"))
				apiPipe = mysqlAPIResponse(nil)
				apiPipe.Source.MySQL.Mappings[0].SourceSchemaName = "public"
				return state, plan, apiPipe
			},
			check: func(t *testing.T, update api.ClickPipeUpdate) {
				require.NotNil(t, update.Source)
				mysql := update.Source.MySQL
				require.NotNil(t, mysql)
				require.Len(t, mysql.TableMappingsToResync, 1)
				assert.Equal(t, "public", mysql.TableMappingsToResync[0].SourceSchemaName)
				assert.Equal(t, "users", mysql.TableMappingsToResync[0].SourceTable)
				require.NotNil(t, mysql.TableMappingsToResync[0].TableEngine)
				assert.Equal(t, "Null", *mysql.TableMappingsToResync[0].TableEngine)
				assert.Empty(t, mysql.TableMappingsToAdd)
				assert.Empty(t, mysql.TableMappingsToRemove)
				assert.Empty(t, mysql.Host, "a mappings-only edit must not re-send the connection")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, plan, apiPipe := tt.build(t)
			plan.ResyncTablesOnChange = types.BoolValue(true)

			mc := minimock.NewController(t)
			mock, calls := pauseEditClientMock(mc, apiPipe, nil)
			var captured api.ClickPipeUpdate
			mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
				*calls = append(*calls, "update")
				captured = update
				paused := *apiPipe
				paused.State = api.ClickPipePausedState
				return &paused, nil
			})
			expectSyncRead(mock, calls, apiPipe)

			resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
			require.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())

			tt.check(t, captured)
			assert.Equal(t, []string{"state:" + api.ClickPipeStateStop, "wait", "update", "state:" + api.ClickPipeStateStart, "wait", "get"}, *calls,
				"the pipe must be paused for the edit and resumed after it")
		})
	}
}

// Plan changes that mean nothing to the API must neither pause the pipe nor
// send a PATCH: only the final wait and read are armed on the mock, and
// minimock fails on any other call.
func TestClickPipeResource_Update_TableMappingNoOpSendsNothing(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// build returns the prior state, the plan, and the pipe the API returns.
		build func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe)
	}{
		{
			name: "resync_tables_on_change only",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = postgresUpdateModel(ctx, t, "users")
				plan = postgresUpdateModel(ctx, t, "users")
				plan.ResyncTablesOnChange = types.BoolValue(true)
				return state, plan, postgresAPIPipe(api.ClickPipeRunningState, "users")
			},
		},
		{
			name: "resync_tables_on_change only, on a Completed pipe",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = postgresUpdateModel(ctx, t, "users")
				state.State = types.StringValue(api.ClickPipeCompletedState)
				plan = state
				plan.ResyncTablesOnChange = types.BoolValue(true)
				return state, plan, postgresAPIPipe(api.ClickPipeCompletedState, "users")
			},
		},
		{
			name: "postgres excluded_columns null to empty",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = postgresUpdateModel(ctx, t, "users")
				plan = postgresUpdateModel(ctx, t, "users")
				setPostgresMappingAttr(ctx, t, &plan, "excluded_columns", types.SetValueMust(types.StringType, []attr.Value{}))
				return state, plan, postgresAPIPipe(api.ClickPipeRunningState, "users")
			},
		},
		{
			name: "mongodb persist_deleted_documents false to null",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = buildMongoDBPersistDeletedState(types.BoolValue(false))
				state.State = types.StringValue(api.ClickPipeRunningState)
				state.Stopped = types.BoolValue(false)
				state.Scaling = types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes)
				state.FieldMappings = types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType())
				state.Settings = types.DynamicNull()
				state.Destination = getMongoDBInitialState().Destination
				plan = state
				plan.Source = buildMongoDBPersistDeletedState(types.BoolNull()).Source
				return state, plan, mongoDBPersistDeletedAPIResponse(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, plan, apiPipe := tt.build(t)

			mock := api.NewClientMock(minimock.NewController(t))
			mock.WaitForClickPipeStateMock.Set(func(_ context.Context, _, _ string, _ func(string) bool, _ time.Duration) (*api.ClickPipe, error) {
				return apiPipe, nil
			})
			mock.GetClickPipeMock.Return(apiPipe, nil)

			resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
			require.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())

			var got models.ClickPipeResourceModel
			require.False(t, resp.State.Get(ctx, &got).HasError())
			assert.Equal(t, plan.ResyncTablesOnChange.ValueBool(), got.ResyncTablesOnChange.ValueBool(), "the configured resync_tables_on_change must be kept in state")
		})
	}
}

// A Completed pipe only accepts changes that reach no API, so a resync is
// still rejected before any call is made.
func TestClickPipeResource_Update_CompletedPipeRejectsTableResync(t *testing.T) {
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	state.State = types.StringValue(api.ClickPipeCompletedState)
	plan := state
	plan.ResyncTablesOnChange = types.BoolValue(true)
	setPostgresMappingAttr(ctx, t, &plan, "partition_by_expr", types.StringValue("toYYYYMM(created_at)"))

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: api.NewClientMock(minimock.NewController(t))}, state, plan)

	assert.True(t, errorDetailContains(resp.Diagnostics, "cannot be modified"), "got: %v", resp.Diagnostics.Errors())
}

// resync_tables_on_change is not stored by the API: a read keeps the
// configured value, and fills in the default when state has none (import, or
// state written by a provider version without the attribute).
func TestClickPipeResource_syncClickPipeState_ResyncTablesOnChange(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		state    types.Bool
		expected types.Bool
	}{
		{name: "unset defaults to false", state: types.BoolNull(), expected: types.BoolValue(false)},
		{name: "true is kept", state: types.BoolValue(true), expected: types.BoolValue(true)},
		{name: "false is kept", state: types.BoolValue(false), expected: types.BoolValue(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := postgresUpdateModel(ctx, t, "users")
			state.ResyncTablesOnChange = tt.state

			mock := api.NewClientMock(minimock.NewController(t))
			mock.GetClickPipeMock.Return(postgresAPIPipe(api.ClickPipeRunningState, "users"), nil)

			require.NoError(t, (&ClickPipeResource{client: mock}).syncClickPipeState(ctx, &state))
			assert.True(t, tt.expected.Equal(state.ResyncTablesOnChange), "expected %v, got %v", tt.expected, state.ResyncTablesOnChange)
		})
	}
}

// The API stores null and empty lists alike, so switching between them is not
// a change; any other difference names the changed attribute.
func TestSQLTableMappingChange(t *testing.T) {
	prior := models.ClickPipePostgresTableMappingModel{
		TargetTable:     types.StringValue("users"),
		ExcludedColumns: types.SetNull(types.StringType),
		SortingKeys:     types.ListValueMust(types.StringType, []attr.Value{}),
	}

	tests := []struct {
		name     string
		change   func(m *models.ClickPipePostgresTableMappingModel)
		expected string
	}{
		{
			name: "excluded_columns null to empty",
			change: func(m *models.ClickPipePostgresTableMappingModel) {
				m.ExcludedColumns = types.SetValueMust(types.StringType, []attr.Value{})
			},
		},
		{
			name:   "sorting_keys empty to null",
			change: func(m *models.ClickPipePostgresTableMappingModel) { m.SortingKeys = types.ListNull(types.StringType) },
		},
		{
			name:     "sorting_keys set",
			change:   func(m *models.ClickPipePostgresTableMappingModel) { m.SortingKeys = stringListValue([]string{"id"}) },
			expected: "sorting_keys",
		},
		{
			name:     "target_table renamed",
			change:   func(m *models.ClickPipePostgresTableMappingModel) { m.TargetTable = types.StringValue("users_renamed") },
			expected: clickPipeTargetTable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planned := prior
			tt.change(&planned)
			assert.Equal(t, tt.expected, sqlTableMappingChange(prior, planned))
		})
	}
}

// Mappings are split into the collections to add, remove and resync. A
// removal only identifies its collection, while a resync carries the whole
// mapping, as the API replaces it entirely.
func TestMongoDBTableMappingDeltas(t *testing.T) {
	ctx := context.Background()
	mappingType := models.ClickPipeMongoDBTableMappingModel{}.ObjectType()

	orders := mongoDBTableMappingModel(types.BoolNull())
	ordersPersistingDeletes := mongoDBTableMappingModel(types.BoolValue(true))
	carts := mongoDBTableMappingModel(types.BoolValue(true))
	carts.SourceCollection = types.StringValue("carts")
	users := mongoDBTableMappingModel(types.BoolNull())
	users.SourceCollection = types.StringValue("users")

	stateSet := types.SetValueMust(mappingType, []attr.Value{orders.ObjectValue(), carts.ObjectValue()})
	planSet := types.SetValueMust(mappingType, []attr.Value{ordersPersistingDeletes.ObjectValue(), users.ObjectValue()})

	collections := func(mappings []api.ClickPipeMongoDBTableMapping) []string {
		names := make([]string, len(mappings))
		for i, m := range mappings {
			names[i] = m.SourceCollection
		}
		return names
	}

	tests := []struct {
		name         string
		resync       bool
		wantResynced []string
	}{
		{name: "with resync", resync: true, wantResynced: []string{"orders"}},
		{name: "without resync a changed mapping is not sent", resync: false, wantResynced: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diagnostics diag.Diagnostics
			toAdd, toRemove, toResync := mongoDBTableMappingDeltas(ctx, &diagnostics, planSet, stateSet, tt.resync)
			require.False(t, diagnostics.HasError(), "%v", diagnostics.Errors())

			assert.Equal(t, []string{"users"}, collections(toAdd))
			assert.Equal(t, []string{"carts"}, collections(toRemove))
			assert.Equal(t, tt.wantResynced, collections(toResync))
			for _, m := range toRemove {
				assert.Nil(t, m.MongoConfig, "a removal must not carry mongoConfig")
			}
			for _, m := range toResync {
				require.NotNil(t, m.MongoConfig, "a resync must carry the whole mapping")
				assert.True(t, *m.MongoConfig.PersistDeletedDocuments)
			}
		})
	}
}

// Columns are only inferred for added collections, so a resync with
// structured ingestion must provide them.
func TestValidateMongoDBResyncColumns(t *testing.T) {
	ctx := context.Background()
	columnType := models.ClickPipeMongoDBColumnModel{}.ObjectType()

	tests := []struct {
		name                string
		structuredIngestion types.Object
		wantError           bool
	}{
		{
			name:                "structured ingestion off",
			structuredIngestion: types.ObjectNull(models.ClickPipeMongoDBStructuredIngestionModel{}.ObjectType().AttrTypes),
		},
		{
			name:                "columns set",
			structuredIngestion: mongoDBStructuredIngestion(mongoDBColumns("total", "Float64")).ObjectValue(),
		},
		{
			name:                "columns unknown",
			structuredIngestion: mongoDBStructuredIngestion(types.ListUnknown(columnType)).ObjectValue(),
			wantError:           true,
		},
		{
			name:                "columns null",
			structuredIngestion: mongoDBStructuredIngestion(types.ListNull(columnType)).ObjectValue(),
			wantError:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := mongoDBTableMappingModel(types.BoolNull())
			mapping.StructuredIngestion = tt.structuredIngestion

			var diagnostics diag.Diagnostics
			validateMongoDBResyncColumns(ctx, &diagnostics, "shop.orders", mapping)

			assert.Equal(t, tt.wantError, diagnostics.HasError(), "%v", diagnostics.Errors())
		})
	}
}
