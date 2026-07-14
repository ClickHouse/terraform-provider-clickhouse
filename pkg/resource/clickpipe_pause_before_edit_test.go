package resource

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

// isClickPipeStoppedOrPaused gates the pause-before-edit for CDC table_mappings
// changes and is the state we wait for. It must recognize only the terminal
// paused states (Stopped/Paused), not transitional (Stopping/Pausing) or active
// states, or an edit could be issued against a pipe that is not yet editable.
func TestIsClickPipeStoppedOrPaused(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		state    string
		expected bool
	}{
		"stopped":               {state: api.ClickPipeStoppedState, expected: true},
		"paused":                {state: api.ClickPipePausedState, expected: true},
		"running":               {state: api.ClickPipeRunningState, expected: false},
		"stopping-transitional": {state: api.ClickPipeStoppingState, expected: false},
		"pausing-transitional":  {state: api.ClickPipePausingState, expected: false},
		"snapshot":              {state: api.ClickPipeSnapShotState, expected: false},
		"provisioning":          {state: api.ClickPipeProvisioningState, expected: false},
		"failed":                {state: api.ClickPipeFailedState, expected: false},
		"empty":                 {state: "", expected: false},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := isClickPipeStoppedOrPaused(tc.state); got != tc.expected {
				t.Errorf("isClickPipeStoppedOrPaused(%q) = %v, want %v", tc.state, got, tc.expected)
			}
		})
	}
}

// ============================================================================
// Update-flow orchestration tests for the pause-before-edit fix (issue #497).
//
// These drive ClickPipeResource.Update end to end against a minimock client and
// record the order of API calls, so they pin the contract that matters: the
// pause (stop + wait for Paused) happens BEFORE the PATCH, the reconciliation
// converges the pipe to the plan's declared state afterward, and a failed edit
// never strands a running pipe in Paused.
// ============================================================================

// postgresUpdateModel returns a fully-typed Postgres CDC model encodable against
// the resource schema, with one table mapping per given source table name. State
// and plan for an update are built from this same helper so they differ only in
// what a test varies.
func postgresUpdateModel(ctx context.Context, t *testing.T, tables ...string) models.ClickPipeResourceModel {
	t.Helper()

	m := getPostgresInitialState()
	m.State = types.StringValue(api.ClickPipeRunningState)
	m.Scaling = types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes)
	m.FieldMappings = types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType())
	m.Settings = types.DynamicNull()
	m.Stopped = types.BoolValue(false)
	m.TriggerResync = types.BoolNull()

	mappings := make([]attr.Value, len(tables))
	for i, table := range tables {
		mappings[i] = types.ObjectValueMust(models.ClickPipePostgresTableMappingModel{}.ObjectType().AttrTypes, map[string]attr.Value{
			"source_schema_name":     types.StringValue("public"),
			"source_table":           types.StringValue(table),
			"target_table":           types.StringValue(table),
			"excluded_columns":       types.SetNull(types.StringType),
			"use_custom_sorting_key": types.BoolNull(),
			"sorting_keys":           types.ListNull(types.StringType),
			"table_engine":           types.StringNull(),
			"partition_key":          types.StringNull(),
		})
	}

	var src models.ClickPipeSourceModel
	m.Source.As(ctx, &src, basetypes.ObjectAsOptions{})
	var pg models.ClickPipePostgresSourceModel
	src.Postgres.As(ctx, &pg, basetypes.ObjectAsOptions{})
	pg.TableMappings = types.SetValueMust(models.ClickPipePostgresTableMappingModel{}.ObjectType(), mappings)
	src.Postgres = pg.ObjectValue()
	m.Source = src.ObjectValue()
	return m
}

// postgresAPIPipe returns the API-side view of the fixture pipe in the given
// state, with one mapping per given source table.
func postgresAPIPipe(state string, tables ...string) *api.ClickPipe {
	mappings := make([]api.ClickPipePostgresTableMapping, len(tables))
	for i, table := range tables {
		mappings[i] = api.ClickPipePostgresTableMapping{
			SourceSchemaName: "public",
			SourceTable:      table,
			TargetTable:      table,
		}
	}
	return &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-pipe",
		State: state,
		Source: api.ClickPipeSource{
			Postgres: &api.ClickPipePostgresSource{
				Host:     "postgres.example.com",
				Port:     5432,
				Database: "mydb",
				Settings: &api.ClickPipePostgresSettings{ReplicationMode: "cdc"},
				Mappings: mappings,
			},
		},
		Destination: api.ClickPipeDestination{Database: "default"},
	}
}

// pauseEditClientMock builds a ClientMock whose calls are appended, in order, to
// the returned slice ("state:<command>", "wait", "update", "get"). updateFunc
// controls each UpdateClickPipe result; the other methods succeed, with waits
// resolving to the first state the checker accepts and reads/waits returning
// pipes shaped like apiPipe.
func pauseEditClientMock(mc *minimock.Controller, apiPipe *api.ClickPipe, updateFunc func(callNum int) (*api.ClickPipe, error)) (*api.ClientMock, *[]string) {
	calls := &[]string{}
	updateCalls := 0

	mock := api.NewClientMock(mc)
	mock.ChangeClickPipeStateMock.Set(func(_ context.Context, _, _, command string) (*api.ClickPipe, error) {
		*calls = append(*calls, "state:"+command)
		return nil, nil
	})
	mock.WaitForClickPipeStateMock.Set(func(_ context.Context, _, _ string, checker func(string) bool, _ time.Duration) (*api.ClickPipe, error) {
		*calls = append(*calls, "wait")
		for _, s := range []string{api.ClickPipePausedState, api.ClickPipeRunningState} {
			if checker(s) {
				pipe := *apiPipe
				pipe.State = s
				return &pipe, nil
			}
		}
		return nil, fmt.Errorf("state checker accepted neither Paused nor Running")
	})
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, _ api.ClickPipeUpdate) (*api.ClickPipe, error) {
		*calls = append(*calls, "update")
		updateCalls++
		return updateFunc(updateCalls)
	})
	return mock, calls
}

// expectSyncRead arms GetClickPipe for the end-of-Update state sync. minimock
// treats every armed method as "must be called", so only tests whose flow
// reaches the sync (i.e. the PATCH succeeds) may arm it.
func expectSyncRead(mock *api.ClientMock, calls *[]string, apiPipe *api.ClickPipe) {
	mock.GetClickPipeMock.Set(func(_ context.Context, _, _ string) (*api.ClickPipe, error) {
		*calls = append(*calls, "get")
		return apiPipe, nil
	})
}

// driveClickPipeUpdate encodes the models against the real resource schema and
// invokes Update, returning the response for diagnostic assertions.
func driveClickPipeUpdate(ctx context.Context, t *testing.T, r *ClickPipeResource, stateModel, planModel models.ClickPipeResourceModel) *resource.UpdateResponse {
	t.Helper()

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError(), "building resource schema failed: %v", schemaResp.Diagnostics.Errors())
	sch := schemaResp.Schema

	stateVal := tfsdk.State{Schema: sch}
	require.False(t, stateVal.Set(ctx, &stateModel).HasError(), "encoding prior state failed")
	planVal := tfsdk.Plan{Schema: sch}
	require.False(t, planVal.Set(ctx, &planModel).HasError(), "encoding plan failed")

	req := resource.UpdateRequest{
		State:  stateVal,
		Plan:   planVal,
		Config: tfsdk.Config{Schema: sch, Raw: planVal.Raw},
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	r.Update(ctx, req, resp)
	return resp
}

func TestClickPipeResource_Update_PausesBeforeTableMappingsEdit(t *testing.T) {
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users", "orders")

	mc := minimock.NewController(t)
	// The PATCH response reports Paused: the auto-resume the API performs to
	// validate the edit is not yet observable, which is exactly the window the
	// reconciliation must handle by issuing start.
	syncPipe := postgresAPIPipe(api.ClickPipeRunningState, "users", "orders")
	mock, calls := pauseEditClientMock(mc, syncPipe, func(int) (*api.ClickPipe, error) {
		return postgresAPIPipe(api.ClickPipePausedState, "users", "orders"), nil
	})
	expectSyncRead(mock, calls, syncPipe)

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)

	assert.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())
	assert.Equal(t, []string{"state:stop", "wait", "update", "state:start", "wait", "get"}, *calls,
		"a table_mappings edit on a running CDC pipe must pause (stop + wait) before the PATCH, then converge back to running")
}

func TestClickPipeResource_Update_ResumesWhenEditFails(t *testing.T) {
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users", "orders")

	mc := minimock.NewController(t)
	mock, calls := pauseEditClientMock(mc, postgresAPIPipe(api.ClickPipeRunningState, "users"), func(int) (*api.ClickPipe, error) {
		return nil, errors.New("status: 500, body: internal error")
	})

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)

	assert.True(t, resp.Diagnostics.HasError(), "the failed PATCH must surface an error")
	assert.Equal(t, []string{"state:stop", "wait", "update", "state:start"}, *calls,
		"a pipe paused for an edit that failed must be resumed (plan declares it running)")
}

func TestClickPipeResource_Update_WarnsWhenResumeAfterFailedEditFails(t *testing.T) {
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users", "orders")

	mc := minimock.NewController(t)
	mock, _ := pauseEditClientMock(mc, postgresAPIPipe(api.ClickPipeRunningState, "users"), func(int) (*api.ClickPipe, error) {
		return nil, errors.New("status: 500, body: internal error")
	})
	// Make the recovery start fail too, on top of the shared mock behavior:
	// stop succeeds (so the pause is issued), start errors.
	mock.ChangeClickPipeStateMock.Set(func(_ context.Context, _, _, command string) (*api.ClickPipe, error) {
		if command == api.ClickPipeStateStart {
			return nil, errors.New("status: 503, body: unavailable")
		}
		return nil, nil
	})

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)

	assert.True(t, resp.Diagnostics.HasError())
	foundWarning := false
	for _, d := range resp.Diagnostics.Warnings() {
		if d.Summary() == "ClickPipe may be left paused" {
			foundWarning = true
		}
	}
	assert.True(t, foundWarning, "a failed recovery resume must warn that the pipe may be left paused; got: %v", resp.Diagnostics)
}

func TestClickPipeResource_Update_NoResumeWhenPlanDeclaresStopped(t *testing.T) {
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users", "orders")
	plan.Stopped = types.BoolValue(true)

	mc := minimock.NewController(t)
	mock, calls := pauseEditClientMock(mc, postgresAPIPipe(api.ClickPipeRunningState, "users"), func(int) (*api.ClickPipe, error) {
		return nil, errors.New("status: 500, body: internal error")
	})

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)

	assert.True(t, resp.Diagnostics.HasError())
	assert.Equal(t, []string{"state:stop", "wait", "update"}, *calls,
		"when the plan declares stopped=true, a failed edit must NOT force-start the pipe — paused already matches the user's intent")
}

func TestClickPipeResource_Update_RetriesOnceOnMustBePaused400(t *testing.T) {
	// The plan changes only the name, so the table_mappings pause gate does not
	// fire — but the API still demands a pause (e.g. the gate's knowledge of
	// pause-required fields is stale, or the pipe was resumed out of band).
	// The provider must trust the 400, pause, and retry the PATCH exactly once.
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users")
	plan.Name = types.StringValue("renamed-pipe")

	apiPipe := postgresAPIPipe(api.ClickPipePausedState, "users")
	apiPipe.Name = "renamed-pipe"

	mc := minimock.NewController(t)
	mock, calls := pauseEditClientMock(mc, apiPipe, func(callNum int) (*api.ClickPipe, error) {
		if callNum == 1 {
			return nil, errors.New(`status: 400, body: {"error":"BAD_REQUEST: Postgres ClickPipe must be paused to edit"}`)
		}
		return apiPipe, nil
	})
	expectSyncRead(mock, calls, apiPipe)

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)

	assert.False(t, resp.Diagnostics.HasError(), "the retried update must succeed: %v", resp.Diagnostics.Errors())
	assert.Equal(t, []string{"update", "state:stop", "wait", "update", "state:start", "wait", "get"}, *calls,
		"an unanticipated must-be-paused 400 must trigger pause + exactly one PATCH retry, then converge back to running")
}

// errAlreadyRunning400 is the API's response to a start command that lost the race
// against the post-edit auto-resume (observed live while testing #497).
var errAlreadyRunning400 = errors.New(`status: 400, body: {"requestId":"x","error":"BAD_REQUEST: 3524192a ClickPipe is already running","status":400}`)

func TestClickPipeResource_Update_ToleratesAlreadyRunningOnReconcileStart(t *testing.T) {
	// Live repro: pause → PATCH succeeds (response still reports Paused) → the
	// API auto-resumes the pipe before the reconciliation's start lands → the
	// start 400s with "already running". The pipe is in exactly the desired
	// state, so the apply must succeed.
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users", "orders")
	plan := postgresUpdateModel(ctx, t, "users")

	syncPipe := postgresAPIPipe(api.ClickPipeRunningState, "users")
	mc := minimock.NewController(t)
	mock, calls := pauseEditClientMock(mc, syncPipe, func(int) (*api.ClickPipe, error) {
		return postgresAPIPipe(api.ClickPipePausedState, "users"), nil
	})
	expectSyncRead(mock, calls, syncPipe)
	mock.ChangeClickPipeStateMock.Set(func(_ context.Context, _, _, command string) (*api.ClickPipe, error) {
		*calls = append(*calls, "state:"+command)
		if command == api.ClickPipeStateStart {
			return nil, errAlreadyRunning400
		}
		return nil, nil
	})

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)

	assert.False(t, resp.Diagnostics.HasError(),
		"a start that lost the race against the API's auto-resume must not fail the apply: %v", resp.Diagnostics.Errors())
	assert.Empty(t, resp.Diagnostics.Warnings(), "no spurious warnings for a pipe already in the desired state")
	assert.Equal(t, []string{"state:stop", "wait", "update", "state:start", "wait", "get"}, *calls)
}

func TestClickPipeResource_Update_GuardToleratesAlreadyRunning(t *testing.T) {
	// If the edit fails and the recovery resume finds the pipe already running
	// (e.g. resumed out of band), the failed-edit error must surface but the
	// "may be left paused" warning must not — the pipe is not paused.
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users", "orders")

	mc := minimock.NewController(t)
	mock, calls := pauseEditClientMock(mc, postgresAPIPipe(api.ClickPipeRunningState, "users"), func(int) (*api.ClickPipe, error) {
		return nil, errors.New("status: 500, body: internal error")
	})
	mock.ChangeClickPipeStateMock.Set(func(_ context.Context, _, _, command string) (*api.ClickPipe, error) {
		*calls = append(*calls, "state:"+command)
		if command == api.ClickPipeStateStart {
			return nil, errAlreadyRunning400
		}
		return nil, nil
	})

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)

	assert.True(t, resp.Diagnostics.HasError(), "the failed PATCH must still surface its error")
	for _, d := range resp.Diagnostics.Warnings() {
		assert.NotEqual(t, "ClickPipe may be left paused", d.Summary(),
			"an already-running pipe must not be reported as possibly paused")
	}
	assert.Equal(t, []string{"state:stop", "wait", "update", "state:start"}, *calls)
}
