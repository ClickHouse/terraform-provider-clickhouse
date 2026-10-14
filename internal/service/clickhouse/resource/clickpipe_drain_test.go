package resource

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/require"
)

func TestClickPipeDrainUpdate(t *testing.T) {
	for _, tc := range []struct {
		name, initial                                             string
		stopped, priorDrain, drain, timeout, reject, edit, resync bool
	}{
		{name: "drain", initial: api.ClickPipeRunningState, stopped: true, drain: true},
		{name: "cancel drain", initial: api.ClickPipeDrainingState, stopped: true, priorDrain: true},
		{name: "cancel drain after interrupted apply", initial: api.ClickPipeDrainingState, stopped: true},
		{name: "ordinary pause", initial: api.ClickPipeRunningState, stopped: true},
		{name: "resume draining", initial: api.ClickPipeDrainingState, priorDrain: true, drain: true},
		{name: "wait existing drain", initial: api.ClickPipeDrainingState, stopped: true, drain: true},
		{name: "timeout", initial: api.ClickPipeRunningState, stopped: true, drain: true, timeout: true},
		{name: "retry completed drain", initial: api.ClickPipePausedState, stopped: true, priorDrain: true, drain: true},
		{name: "reject ordinary paused", initial: api.ClickPipePausedState, stopped: true, drain: true, reject: true},
		{name: "reject ordinary pausing", initial: api.ClickPipePausingState, stopped: true, drain: true, reject: true},
		{name: "reject concurrent edit", initial: api.ClickPipeRunningState, stopped: true, drain: true, edit: true, reject: true},
		{name: "reject concurrent resync", initial: api.ClickPipeRunningState, stopped: true, drain: true, resync: true, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			state := postgresUpdateModel(ctx, t, "users")
			state.State = types.StringValue(tc.initial)
			state.Stopped = types.BoolValue(tc.initial != api.ClickPipeRunningState)
			state.Drain = types.BoolValue(tc.priorDrain)
			plan := state
			plan.Stopped = types.BoolValue(tc.stopped)
			plan.Drain = types.BoolValue(tc.drain)
			if tc.edit {
				plan.Name = types.StringValue("edited")
			}
			if tc.resync {
				plan.TriggerResync = types.BoolValue(true)
			}
			mock := api.NewClientMock(minimock.NewController(t))
			final := api.ClickPipeRunningState
			if tc.stopped {
				final = api.ClickPipePausedState
			}
			if !tc.reject {
				if tc.initial == api.ClickPipeRunningState || (tc.initial == api.ClickPipeDrainingState && tc.stopped && !tc.drain) {
					if tc.drain {
						mock.DrainClickPipeMock.Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).Return(nil, nil)
					} else {
						mock.ChangeClickPipeStateMock.Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString(), api.ClickPipeStateStop).Return(nil, nil)
					}
				} else if !tc.stopped {
					mock.ChangeClickPipeStateMock.Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString(), api.ClickPipeStateStart).Return(nil, nil)
				}
				mock.WaitForClickPipeStateMock.Set(func(_ context.Context, _, _ string, check func(string) bool, timeout time.Duration) (*api.ClickPipe, error) {
					if tc.drain && tc.stopped {
						require.Equal(t, clickPipeDrainMaxWait, timeout)
					} else {
						require.Equal(t, clickPipeStateChangeMaxWait, timeout)
					}
					require.False(t, check(api.ClickPipeDrainingState))
					require.True(t, check(final))
					if tc.timeout {
						return postgresAPIPipe(api.ClickPipeDrainingState, "users"), errors.New("timeout")
					}
					return postgresAPIPipe(final, "users"), nil
				})
				if !tc.timeout {
					mock.GetClickPipeMock.Return(postgresAPIPipe(final, "users"), nil)
				}
			}
			r := &ClickPipeResource{client: mock}
			response := driveClickPipeUpdate(ctx, t, r, state, plan)
			require.Equal(t, tc.reject || tc.timeout, response.Diagnostics.HasError(), "%v", response.Diagnostics)
			if tc.timeout {
				var saved models.ClickPipeResourceModel
				require.False(t, response.State.Get(ctx, &saved).HasError())
				require.False(t, saved.Drain.ValueBool(), "an incomplete drain must remain a planned change")
				require.True(t, saved.Stopped.ValueBool(), "stopped = false must still trigger a resume")
			}

		})
	}
}

func TestValidateClickPipeDrain(t *testing.T) {
	for _, mode := range []string{"cdc", "cdc_only", "snapshot", "non-postgres"} {
		t.Run(mode, func(t *testing.T) {
			ctx := t.Context()
			plan := postgresUpdateModel(ctx, t, "users")
			plan.Drain = types.BoolValue(true)
			var source models.ClickPipeSourceModel
			require.False(t, plan.Source.As(ctx, &source, basetypes.ObjectAsOptions{}).HasError())
			var pg models.ClickPipePostgresSourceModel
			require.False(t, source.Postgres.As(ctx, &pg, basetypes.ObjectAsOptions{}).HasError())
			var settings models.ClickPipePostgresSettingsModel
			require.False(t, pg.Settings.As(ctx, &settings, basetypes.ObjectAsOptions{}).HasError())
			settings.ReplicationMode = types.StringValue(mode)
			pg.Settings = settings.ObjectValue()
			source.Postgres = pg.ObjectValue()
			if mode == "non-postgres" {
				source.Postgres = types.ObjectNull(pg.ObjectType().AttrTypes)
			}
			plan.Source = source.ObjectValue()
			var diagnostics diag.Diagnostics
			validateClickPipeDrain(ctx, plan, &diagnostics)
			require.Equal(t, mode == "snapshot" || mode == "non-postgres", diagnostics.HasError(), "%v", diagnostics)
		})
	}
}

func TestClickPipeDrainPlansReconciliation(t *testing.T) {
	for _, status := range []string{api.ClickPipeDrainingState, api.ClickPipePausedState, api.ClickPipePausingState, api.ClickPipeRunningState} {
		t.Run(status, func(t *testing.T) {
			ctx := t.Context()
			r := &ClickPipeResource{}
			var schemaResponse resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
			model := postgresUpdateModel(ctx, t, "users")
			model.State = types.StringValue(status)
			state := tfsdk.State{Schema: schemaResponse.Schema}
			require.False(t, state.Set(ctx, model).HasError())
			plan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: state.Raw}
			response := resource.ModifyPlanResponse{Plan: plan}
			r.planStateAttribute(ctx, resource.ModifyPlanRequest{State: state, Plan: plan}, &response)
			require.False(t, response.Diagnostics.HasError())
			var plannedStatus types.String
			require.False(t, response.Plan.GetAttribute(ctx, path.Root("state"), &plannedStatus).HasError())
			require.Equal(t, status == api.ClickPipeDrainingState, plannedStatus.IsUnknown())
		})
	}
}
