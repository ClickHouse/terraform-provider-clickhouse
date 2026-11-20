package resource

import (
	"context"
	"fmt"
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

func warningDetailContains(diags diag.Diagnostics, substr string) bool {
	for _, d := range diags.Warnings() {
		if strings.Contains(d.Detail(), substr) {
			return true
		}
	}
	return false
}

func TestClickPipeResource_ModifyPlan_TableResync(t *testing.T) {
	ctx := context.Background()

	t.Run("changing a mapping without resync_tables_on_change is rejected and points to it", func(t *testing.T) {
		state := postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users")
		plan := postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users")
		setPostgresMappingAttr(ctx, t, &plan, "partition_by_expr", types.StringValue("toYYYYMM(created_at)"))

		diags := driveClickPipeModifyPlan(ctx, t, state, plan)

		assert.True(t, errorDetailContains(diags, "Cannot modify partition_by_expr"), "got: %v", diags.Errors())
		assert.True(t, errorDetailContains(diags, "Set resync_tables_on_change = true"), "got: %v", diags.Errors())
	})

	t.Run("changing a mapping with resync_tables_on_change warns instead", func(t *testing.T) {
		state := postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users")
		plan := postgresPlanWithExcludedColumns(ctx, []string{"status", "email"}, "users")
		plan.ResyncTablesOnChange = types.BoolValue(true)

		diags := driveClickPipeModifyPlan(ctx, t, state, plan)

		assert.False(t, errorDetailContains(diags, "Cannot modify"), "got: %v", diags.Errors())
		assert.True(t, warningDetailContains(diags, "excluded_columns changed for table mapping 'public.users'"), "got: %v", diags.Warnings())
	})

	t.Run("target_table cannot change even with resync_tables_on_change", func(t *testing.T) {
		state := postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users")
		plan := postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users_renamed")
		plan.ResyncTablesOnChange = types.BoolValue(true)

		diags := driveClickPipeModifyPlan(ctx, t, state, plan)

		assert.True(t, errorDetailContains(diags, "Cannot modify target_table"), "got: %v", diags.Errors())
	})

	t.Run("MySQL mapping change with resync_tables_on_change warns", func(t *testing.T) {
		state := mysqlPlanForModifyPlan()
		plan := mysqlPlanForModifyPlan()
		plan.ResyncTablesOnChange = types.BoolValue(true)
		setMySQLMappingAttr(ctx, t, &plan, "table_engine", types.StringValue("Null"))

		diags := driveClickPipeModifyPlan(ctx, t, state, plan)

		assert.False(t, errorDetailContains(diags, "Cannot modify"), "got: %v", diags.Errors())
		assert.True(t, warningDetailContains(diags, "table_engine changed"), "got: %v", diags.Warnings())
	})
}

func TestTableMappingChange_NullAndEmptyCollectionsMatch(t *testing.T) {
	prior := models.ClickPipePostgresTableMappingModel{
		TargetTable:     types.StringValue("users"),
		ExcludedColumns: types.SetNull(types.StringType),
		SortingKeys:     types.ListValueMust(types.StringType, []attr.Value{}),
	}
	planned := prior
	planned.ExcludedColumns = types.SetValueMust(types.StringType, []attr.Value{})
	planned.SortingKeys = types.ListNull(types.StringType)

	assert.Empty(t, postgresTableMappingChange(prior, planned))

	planned.SortingKeys = stringListValue([]string{"id"})
	assert.Equal(t, "sorting_keys", postgresTableMappingChange(prior, planned))
}

// withResyncPartitionByExpr returns the fixture pipe with its single mapping
// partitioned, as the API returns it after the resync.
func withResyncPartitionByExpr(pipe *api.ClickPipe, expr string) *api.ClickPipe {
	pipe.Source.Postgres.Mappings[0].PartitionByExpr = &expr
	return pipe
}

func TestClickPipeResource_Update_ResyncsChangedTableMapping(t *testing.T) {
	ctx := context.Background()
	const expr = "toYYYYMM(created_at)"
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users")
	plan.ResyncTablesOnChange = types.BoolValue(true)
	setPostgresMappingAttr(ctx, t, &plan, "partition_by_expr", types.StringValue(expr))

	mc := minimock.NewController(t)
	syncPipe := withResyncPartitionByExpr(postgresAPIPipe(api.ClickPipeRunningState, "users"), expr)
	mock, calls := pauseEditClientMock(mc, syncPipe, nil)

	var captured api.ClickPipeUpdate
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
		*calls = append(*calls, "update")
		captured = update
		return withResyncPartitionByExpr(postgresAPIPipe(api.ClickPipePausedState, "users"), expr), nil
	})
	expectSyncRead(mock, calls, syncPipe)

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())

	require.NotNil(t, captured.Source)
	require.NotNil(t, captured.Source.Postgres)
	pg := captured.Source.Postgres
	require.Len(t, pg.TableMappingsToResync, 1)
	resynced := pg.TableMappingsToResync[0]
	assert.Equal(t, "users", resynced.SourceTable)
	assert.Equal(t, "users", resynced.TargetTable)
	require.NotNil(t, resynced.PartitionByExpr)
	assert.Equal(t, expr, *resynced.PartitionByExpr)
	assert.Empty(t, pg.TableMappingsToAdd, "a resynced table must not also be added")
	assert.Empty(t, pg.TableMappingsToRemove, "a resynced table must not also be removed")
	assert.Empty(t, pg.Host, "a mappings-only edit must not re-send the connection (#617)")

	// The API only accepts table mapping edits on a paused pipe.
	assert.Equal(t, []string{"state:" + api.ClickPipeStateStop, "wait", "update", "state:" + api.ClickPipeStateStart, "wait", "get"}, *calls)
}

// Turning resync_tables_on_change on, with no mapping change, is provider-side
// only: nothing must be sent to the API.
func TestClickPipeResource_Update_ResyncFlagOnlyChangeSendsNothing(t *testing.T) {
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users")
	plan.ResyncTablesOnChange = types.BoolValue(true)

	mc := minimock.NewController(t)
	syncPipe := postgresAPIPipe(api.ClickPipeRunningState, "users")
	mock := api.NewClientMock(mc)
	mock.WaitForClickPipeStateMock.Set(func(_ context.Context, _, _ string, _ func(string) bool, _ time.Duration) (*api.ClickPipe, error) {
		return syncPipe, nil
	})
	mock.GetClickPipeMock.Return(syncPipe, nil)

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())

	var got models.ClickPipeResourceModel
	require.False(t, resp.State.Get(ctx, &got).HasError())
	assert.True(t, got.ResyncTablesOnChange.ValueBool(), "the configured value must be kept in state")
}

func mongoDBResyncMapping(collection string, persistDeleted types.Bool) models.ClickPipeMongoDBTableMappingModel {
	m := mongoDBTableMappingModel(persistDeleted)
	m.SourceCollection = types.StringValue(collection)
	return m
}

func mongoDBMappingSet(t *testing.T, mappings ...models.ClickPipeMongoDBTableMappingModel) types.Set {
	t.Helper()
	values := make([]attr.Value, len(mappings))
	for i, m := range mappings {
		values[i] = m.ObjectValue()
	}
	return types.SetValueMust(models.ClickPipeMongoDBTableMappingModel{}.ObjectType(), values)
}

func TestMongoDBTableMappingDeltas(t *testing.T) {
	ctx := context.Background()
	stateSet := mongoDBMappingSet(t,
		mongoDBResyncMapping("orders", types.BoolNull()),
		mongoDBResyncMapping("carts", types.BoolValue(true)),
	)
	planSet := mongoDBMappingSet(t,
		mongoDBResyncMapping("orders", types.BoolValue(true)),
		mongoDBResyncMapping("users", types.BoolNull()),
	)

	t.Run("with resync", func(t *testing.T) {
		var diagnostics diag.Diagnostics
		toAdd, toRemove, toResync := mongoDBTableMappingDeltas(ctx, &diagnostics, planSet, stateSet, true)
		require.False(t, diagnostics.HasError(), "%v", diagnostics.Errors())

		require.Len(t, toAdd, 1)
		assert.Equal(t, "users", toAdd[0].SourceCollection)

		require.Len(t, toRemove, 1)
		assert.Equal(t, "carts", toRemove[0].SourceCollection)
		assert.Nil(t, toRemove[0].MongoConfig, "a removal must not carry mongoConfig")

		// The resync replaces the whole mapping, so its settings are sent.
		require.Len(t, toResync, 1)
		assert.Equal(t, "orders", toResync[0].SourceCollection)
		require.NotNil(t, toResync[0].MongoConfig)
		assert.True(t, *toResync[0].MongoConfig.PersistDeletedDocuments)
	})

	t.Run("without resync a changed mapping is not sent", func(t *testing.T) {
		var diagnostics diag.Diagnostics
		_, _, toResync := mongoDBTableMappingDeltas(ctx, &diagnostics, planSet, stateSet, false)
		require.False(t, diagnostics.HasError(), "%v", diagnostics.Errors())
		assert.Empty(t, toResync)
	})
}

func TestValidateMongoDBResyncColumns(t *testing.T) {
	ctx := context.Background()
	columnType := models.ClickPipeMongoDBColumnModel{}.ObjectType()
	structured := func(columns types.List) types.Object {
		return models.ClickPipeMongoDBStructuredIngestionModel{
			DropUnexpectedValues: types.BoolNull(),
			Columns:              columns,
			ExcludedColumns:      types.SetNull(types.StringType),
			SortingKeys:          types.ListNull(types.StringType),
			PartitionByExpr:      types.StringNull(),
		}.ObjectValue()
	}
	someColumns := types.ListValueMust(columnType, []attr.Value{
		models.ClickPipeMongoDBColumnModel{Name: types.StringValue("total"), Type: types.StringValue("Float64")}.ObjectValue(),
	})

	for name, tc := range map[string]struct {
		structuredIngestion types.Object
		wantError           bool
	}{
		"structured ingestion off":         {structuredIngestion: types.ObjectNull(models.ClickPipeMongoDBStructuredIngestionModel{}.ObjectType().AttrTypes)},
		"columns set":                      {structuredIngestion: structured(someColumns)},
		"columns unknown (to be inferred)": {structuredIngestion: structured(types.ListUnknown(columnType)), wantError: true},
		"columns null":                     {structuredIngestion: structured(types.ListNull(columnType)), wantError: true},
	} {
		t.Run(name, func(t *testing.T) {
			mapping := mongoDBResyncMapping("orders", types.BoolNull())
			mapping.StructuredIngestion = tc.structuredIngestion
			var diagnostics diag.Diagnostics
			validateMongoDBResyncColumns(ctx, &diagnostics, "shop.orders", mapping)
			assert.Equal(t, tc.wantError, diagnostics.HasError(), fmt.Sprint(diagnostics.Errors()))
		})
	}
}
