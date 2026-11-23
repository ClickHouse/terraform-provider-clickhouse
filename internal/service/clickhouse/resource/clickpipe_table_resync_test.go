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
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// A change to an existing table mapping is rejected unless its resync_version
// changes too, and a changed resync_version warns about the resync.
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
	postgresMapping := func(t *testing.T, excludedColumns []string, version types.Int64) models.ClickPipeResourceModel {
		m := postgresPlanWithExcludedColumns(ctx, excludedColumns, "users")
		setPostgresMappingAttr(ctx, t, &m, "resync_version", version)
		return m
	}
	mongoDBMapping := func(m models.ClickPipeMongoDBTableMappingModel, version types.Int64) models.ClickPipeMongoDBTableMappingModel {
		m.ResyncVersion = version
		return m
	}

	tests := []struct {
		name string
		// build returns the prior state and the plan.
		build func(t *testing.T) (state, plan models.ClickPipeResourceModel)
		// wantError is a substring of the expected error; when empty, the plan must have no error.
		wantError string
		// wantWarning is a substring of the expected resync warning; when both are empty, no resync is planned.
		wantWarning string
	}{
		{
			name: "postgres mapping change without a resync_version change is rejected",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				state = postgresMapping(t, []string{"status"}, types.Int64Value(1))
				plan = postgresMapping(t, []string{"status"}, types.Int64Value(1))
				setPostgresMappingAttr(ctx, t, &plan, "partition_by_expr", types.StringValue("toYYYYMM(created_at)"))
				return state, plan
			},
			wantError: "Cannot modify partition_by_expr for existing table mapping 'public.users'. Change its resync_version",
		},
		{
			name: "postgres mapping change with a resync_version change warns",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				return postgresMapping(t, []string{"status"}, types.Int64Value(1)),
					postgresMapping(t, []string{"status", "email"}, types.Int64Value(2))
			},
			wantWarning: "resync_version changed for table mapping 'public.users'",
		},
		{
			name: "postgres resync_version change alone resyncs the unchanged table",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				return postgresMapping(t, []string{"status"}, types.Int64Null()),
					postgresMapping(t, []string{"status"}, types.Int64Value(1))
			},
			wantWarning: "resync_version changed for table mapping 'public.users'",
		},
		{
			name: "postgres resync_version removal resyncs nothing",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				return postgresMapping(t, []string{"status"}, types.Int64Value(1)),
					postgresMapping(t, []string{"status"}, types.Int64Null())
			},
		},
		{
			name: "postgres excluded_columns null to empty is not a change",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				state = postgresMapping(t, []string{}, types.Int64Null())
				setPostgresMappingAttr(ctx, t, &state, "excluded_columns", types.SetNull(types.StringType))
				return state, postgresMapping(t, []string{}, types.Int64Null())
			},
		},
		{
			name: "target_table cannot change even on resync",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				state = postgresMapping(t, []string{"status"}, types.Int64Null())
				plan = postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users_renamed")
				setPostgresMappingAttr(ctx, t, &plan, "resync_version", types.Int64Value(1))
				return state, plan
			},
			wantError: "Cannot modify target_table",
		},
		{
			name: "mysql mapping change with a resync_version change warns",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				state, plan = mysqlPlanForModifyPlan(), mysqlPlanForModifyPlan()
				setMySQLMappingAttr(ctx, t, &plan, "table_engine", types.StringValue("Null"))
				setMySQLMappingAttr(ctx, t, &plan, "resync_version", types.Int64Value(1))
				return state, plan
			},
			wantWarning: "resync_version changed for table mapping 'public.users'",
		},
		{
			name: "mongodb mapping change without a resync_version change is rejected",
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
					mongoDBModelWithMappings(t, mongoDBMapping(mongoDBStructuredMapping("replicants", changed), types.Int64Value(1)))
			},
			wantWarning: "resync_version changed for table mapping 'shop.replicants'",
		},
		{
			name: "mongodb enabling structured ingestion on resync requires columns",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel) {
				return mongoDBModelWithMappings(t, mongoDBTableMappingModel(types.BoolNull())),
					mongoDBModelWithMappings(t, mongoDBMapping(mongoDBStructuredMapping("orders", mongoDBStructuredIngestion(unknownColumns)), types.Int64Value(1)))
			},
			wantError: "structured_ingestion.columns must be set to resync collection 'shop.orders'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, plan := tt.build(t)

			diags := driveClickPipeModifyPlan(ctx, t, state, plan)

			if tt.wantError != "" {
				assert.True(t, errorDetailContains(diags, tt.wantError), "got: %v", diags.Errors())
			} else {
				assert.False(t, diags.HasError(), "got: %v", diags.Errors())
			}
			if tt.wantWarning != "" {
				assert.True(t, warningContains(diags, tt.wantWarning), "got: %v", diags.Warnings())
			} else if tt.wantError == "" {
				assert.False(t, warningContains(diags, "resync_version changed"), "no resync must be planned; got: %v", diags.Warnings())
			}
		})
	}
}

// A mapping whose resync_version changed is sent in tableMappingsToResync, in
// a mappings-only PATCH (#617) issued while the pipe is paused.
func TestClickPipeResource_Update_TableResync(t *testing.T) {
	ctx := context.Background()
	const partitionByExpr = "toYYYYMM(created_at)"

	tests := []struct {
		name string
		// build returns the prior state, the plan, and the pipe the API returns.
		build func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe)
		check func(t *testing.T, update api.ClickPipeUpdate)
	}{
		{
			name: "postgres mapping change",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = postgresUpdateModel(ctx, t, "users")
				plan = postgresUpdateModel(ctx, t, "users")
				setPostgresMappingAttr(ctx, t, &plan, "partition_by_expr", types.StringValue(partitionByExpr))
				setPostgresMappingAttr(ctx, t, &plan, "resync_version", types.Int64Value(1))
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
			name: "postgres resync_version change only",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = postgresUpdateModel(ctx, t, "users")
				setPostgresMappingAttr(ctx, t, &state, "resync_version", types.Int64Value(1))
				plan = postgresUpdateModel(ctx, t, "users")
				setPostgresMappingAttr(ctx, t, &plan, "resync_version", types.Int64Value(2))
				return state, plan, postgresAPIPipe(api.ClickPipeRunningState, "users")
			},
			check: func(t *testing.T, update api.ClickPipeUpdate) {
				require.NotNil(t, update.Source)
				pg := update.Source.Postgres
				require.NotNil(t, pg)
				require.Len(t, pg.TableMappingsToResync, 1)
				assert.Equal(t, "users", pg.TableMappingsToResync[0].SourceTable)
				assert.Nil(t, pg.TableMappingsToResync[0].PartitionByExpr)
			},
		},
		{
			name: "mysql mapping change",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = mysqlPlanForModifyPlan()
				state.State = types.StringValue(api.ClickPipeRunningState)
				state.Stopped = types.BoolValue(false)
				plan = state
				setMySQLMappingAttr(ctx, t, &plan, "table_engine", types.StringValue("Null"))
				setMySQLMappingAttr(ctx, t, &plan, "resync_version", types.Int64Value(1))
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
			name: "resync_version removed",
			build: func(t *testing.T) (state, plan models.ClickPipeResourceModel, apiPipe *api.ClickPipe) {
				state = postgresUpdateModel(ctx, t, "users")
				setPostgresMappingAttr(ctx, t, &state, "resync_version", types.Int64Value(1))
				return state, postgresUpdateModel(ctx, t, "users"), postgresAPIPipe(api.ClickPipeRunningState, "users")
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
		})
	}
}

// A Completed pipe cannot be modified, so a resync is rejected before any
// call is made.
func TestClickPipeResource_Update_CompletedPipeRejectsTableResync(t *testing.T) {
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	state.State = types.StringValue(api.ClickPipeCompletedState)
	plan := state
	setPostgresMappingAttr(ctx, t, &plan, "resync_version", types.Int64Value(1))

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: api.NewClientMock(minimock.NewController(t))}, state, plan)

	assert.True(t, errorDetailContains(resp.Diagnostics, "cannot be modified"), "got: %v", resp.Diagnostics.Errors())
}

// resync_version is not stored by the API: a read keeps the configured value,
// and leaves it null for a mapping state does not know (import).
func TestClickPipeResource_syncClickPipeState_ResyncVersion(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		// stateTables are the mappings in state before the read.
		stateTables []string
		version     types.Int64
		expected    types.Int64
	}{
		{name: "configured value is kept", stateTables: []string{"users"}, version: types.Int64Value(3), expected: types.Int64Value(3)},
		{name: "unset stays null", stateTables: []string{"users"}, version: types.Int64Null(), expected: types.Int64Null()},
		{name: "mapping unknown to state is null", stateTables: []string{}, expected: types.Int64Null()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := postgresUpdateModel(ctx, t, tt.stateTables...)
			if len(tt.stateTables) > 0 {
				setPostgresMappingAttr(ctx, t, &state, "resync_version", tt.version)
			}

			mock := api.NewClientMock(minimock.NewController(t))
			mock.GetClickPipeMock.Return(postgresAPIPipe(api.ClickPipeRunningState, "users"), nil)

			require.NoError(t, (&ClickPipeResource{client: mock}).syncClickPipeState(ctx, &state))

			var src models.ClickPipeSourceModel
			require.False(t, state.Source.As(ctx, &src, basetypes.ObjectAsOptions{}).HasError())
			var pg models.ClickPipePostgresSourceModel
			require.False(t, src.Postgres.As(ctx, &pg, basetypes.ObjectAsOptions{}).HasError())
			var mappings []models.ClickPipePostgresTableMappingModel
			require.False(t, pg.TableMappings.ElementsAs(ctx, &mappings, false).HasError())
			require.Len(t, mappings, 1)
			assert.True(t, tt.expected.Equal(mappings[0].ResyncVersion), "expected %v, got %v", tt.expected, mappings[0].ResyncVersion)
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
			name:   "resync_version is not a mapping change",
			change: func(m *models.ClickPipePostgresTableMappingModel) { m.ResyncVersion = types.Int64Value(1) },
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

// Mappings are split into the collections to add, remove and resync. Only a
// changed resync_version resyncs a collection. A removal only identifies its
// collection, while a resync carries the whole mapping, as the API replaces it
// entirely.
func TestMongoDBTableMappingDeltas(t *testing.T) {
	ctx := context.Background()
	mappingType := models.ClickPipeMongoDBTableMappingModel{}.ObjectType()

	orders := mongoDBTableMappingModel(types.BoolNull())
	carts := mongoDBTableMappingModel(types.BoolValue(true))
	carts.SourceCollection = types.StringValue("carts")
	users := mongoDBTableMappingModel(types.BoolNull())
	users.SourceCollection = types.StringValue("users")
	stateSet := types.SetValueMust(mappingType, []attr.Value{orders.ObjectValue(), carts.ObjectValue()})

	collections := func(mappings []api.ClickPipeMongoDBTableMapping) []string {
		names := make([]string, len(mappings))
		for i, m := range mappings {
			names[i] = m.SourceCollection
		}
		return names
	}

	tests := []struct {
		name          string
		ordersVersion types.Int64
		wantResynced  []string
	}{
		{name: "resync_version changed", ordersVersion: types.Int64Value(1), wantResynced: []string{"orders"}},
		{name: "resync_version unchanged", ordersVersion: types.Int64Null(), wantResynced: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plannedOrders := mongoDBTableMappingModel(types.BoolValue(true))
			plannedOrders.ResyncVersion = tt.ordersVersion
			planSet := types.SetValueMust(mappingType, []attr.Value{plannedOrders.ObjectValue(), users.ObjectValue()})

			var diagnostics diag.Diagnostics
			toAdd, toRemove, toResync := mongoDBTableMappingDeltas(ctx, &diagnostics, planSet, stateSet)
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
