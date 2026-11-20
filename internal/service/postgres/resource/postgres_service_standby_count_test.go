package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
)

func TestValidateStandbyCount(t *testing.T) {
	cases := []struct {
		name    string
		haType  types.String
		count   types.Int64
		wantErr bool
	}{
		{"none 0", types.StringValue("none"), types.Int64Value(0), false},
		{"none 1", types.StringValue("none"), types.Int64Value(1), true},
		{"async 1", types.StringValue("async"), types.Int64Value(1), false},
		{"async 0", types.StringValue("async"), types.Int64Value(0), true},
		{"async 2", types.StringValue("async"), types.Int64Value(2), true},
		{"sync 1", types.StringValue("sync"), types.Int64Value(1), false},
		{"sync 2", types.StringValue("sync"), types.Int64Value(2), false},
		{"sync 0", types.StringValue("sync"), types.Int64Value(0), true},
		{"count omitted", types.StringValue("async"), types.Int64Null(), false},
		{"count unknown", types.StringValue("async"), types.Int64Unknown(), false},
		{"ha_type unknown", types.StringUnknown(), types.Int64Value(2), false},
		{"ha_type omitted", types.StringNull(), types.Int64Value(2), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diags := validateStandbyCount(tc.haType, tc.count)
			if diags.HasError() != tc.wantErr {
				t.Errorf("HasError = %v, want %v (%v)", diags.HasError(), tc.wantErr, diags)
			}
		})
	}
}

func TestPlanStandbyCount(t *testing.T) {
	m := func(ha types.String, count types.Int64) models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{HaType: ha, StandbyCount: count}
	}
	sync := types.StringValue("sync")
	async := types.StringValue("async")
	state := func(ha types.String, count types.Int64) *models.PostgresServiceResourceModel {
		s := m(ha, count)
		return &s
	}
	cases := []struct {
		name         string
		config, plan models.PostgresServiceResourceModel
		state        *models.PostgresServiceResourceModel
		want         types.Int64
	}{
		{"configured value wins", m(sync, types.Int64Value(1)), m(sync, types.Int64Value(1)), state(sync, types.Int64Value(2)), types.Int64Value(1)},
		{"create takes the ha_type default", m(sync, types.Int64Null()), m(sync, types.Int64Unknown()), nil, types.Int64Value(2)},
		{"create without ha_type stays unknown", m(types.StringNull(), types.Int64Null()), m(types.StringUnknown(), types.Int64Unknown()), nil, types.Int64Unknown()},
		{"unchanged ha_type keeps the prior count", m(sync, types.Int64Null()), m(sync, types.Int64Unknown()), state(sync, types.Int64Value(1)), types.Int64Value(1)},
		{"changed ha_type takes the new default", m(async, types.Int64Null()), m(async, types.Int64Unknown()), state(sync, types.Int64Value(2)), types.Int64Value(1)},
		{"state from an older provider stays unknown", m(async, types.Int64Null()), m(async, types.Int64Unknown()), state(async, types.Int64Null()), types.Int64Unknown()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := planStandbyCount(tc.config, tc.plan, tc.state); !got.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildPostgresUpdate_StandbyCount(t *testing.T) {
	m := func(ha string, count int64) models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{
			Size:         types.StringValue("c6gd.large"),
			HaType:       types.StringValue(ha),
			StandbyCount: types.Int64Value(count),
			Tags:         mapTags(),
		}
	}
	cases := []struct {
		name        string
		plan, state models.PostgresServiceResourceModel
	}{
		{"count-only change", m("sync", 1), m("sync", 2)},
		{"ha_type and count change", m("sync", 1), m("async", 1)},
	}
	t.Run("unknown count is not a change", func(t *testing.T) {
		plan := m("sync", 2)
		plan.StandbyCount = types.Int64Unknown()
		state := m("sync", 2)
		state.StandbyCount = types.Int64Null()
		result, diags := buildPostgresUpdate(context.Background(), plan, state)
		if diags.HasError() || result.Body != nil {
			t.Fatalf("expected no PATCH, got %#v (%v)", result.Body, diags)
		}
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, diags := buildPostgresUpdate(context.Background(), tc.plan, tc.state)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if result.Body == nil || result.Body.HaType != "sync" || result.Body.StandbyCount == nil || *result.Body.StandbyCount != 1 {
				t.Fatalf("expected haType=sync standbyCount=1 body, got %#v", result.Body)
			}
			if !result.TransitionExpected {
				t.Error("an HA change must signal TransitionExpected=true")
			}
		})
	}
}

func TestBuildPostgresMatchPredicate_StandbyCount(t *testing.T) {
	one, two := 1, 2
	predicate := buildPostgresMatchPredicate(&api.PostgresUpdate{HaType: "sync", StandbyCount: &one})
	if !predicate(&api.Postgres{State: api.PostgresStateRunning, HaType: "sync", StandbyCount: &one}) {
		t.Error("should match once the server reports the PATCHed count")
	}
	if predicate(&api.Postgres{State: api.PostgresStateRunning, HaType: "sync", StandbyCount: &two}) {
		t.Error("must NOT match while standbyCount is still the pre-PATCH value")
	}
	if predicate(&api.Postgres{State: api.PostgresStateRunning, HaType: "sync"}) {
		t.Error("must NOT match while standbyCount is absent")
	}
}

func TestSyncPostgresState_StandbyCount(t *testing.T) {
	one := 1
	cases := []struct {
		name string
		pg   api.Postgres
		want types.Int64
	}{
		{"server value", api.Postgres{HaType: "sync", StandbyCount: &one}, types.Int64Value(1)},
		{"omitted falls back to the ha_type default", api.Postgres{HaType: "sync"}, types.Int64Value(2)},
		{"omitted with no ha_type", api.Postgres{}, types.Int64Value(0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var state models.PostgresServiceResourceModel
			if diags := syncPostgresState(context.Background(), &tc.pg, &state); diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !state.StandbyCount.Equal(tc.want) {
				t.Errorf("got %v, want %v", state.StandbyCount, tc.want)
			}
		})
	}
}

// A state written before standby_count existed plans the count as unknown;
// Update must read it back even when nothing else changes.
func TestPostgresUpdateResolvesUnknownStandbyCount(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresServiceResource{client: client}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	model := gateModel(true)
	model.HaType = types.StringValue("sync")
	prior := tfsdk.State{Schema: schemaResp.Schema}
	require.False(t, prior.Set(ctx, model).HasError())
	model.StandbyCount = types.Int64Unknown()
	planned := tfsdk.State{Schema: schemaResp.Schema}
	require.False(t, planned.Set(ctx, model).HasError())
	one := 1
	client.GetPostgresMock.Expect(ctx, "pg-1").Return(&api.Postgres{
		Id: "pg-1", Name: "n", Provider: "aws", Region: "us-east-1", Size: "m6gd.large",
		PostgresVersion: "18", HaType: "sync", StandbyCount: &one, State: api.PostgresStateRunning, IsPrimary: true,
	}, nil)
	client.GetPostgresConfigMock.Expect(ctx, "pg-1").Return(&api.PostgresConfig{}, nil)
	resp := resource.UpdateResponse{State: prior}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: planned.Raw},
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: prior.Raw},
		State:  prior,
	}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	var got models.PostgresServiceResourceModel
	require.False(t, resp.State.Get(ctx, &got).HasError())
	require.True(t, got.StandbyCount.Equal(types.Int64Value(1)), "%v", got.StandbyCount)
}
