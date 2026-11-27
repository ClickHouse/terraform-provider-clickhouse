package resource

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
)

func ptrBool(b bool) *bool { return &b }

func TestPostgresDeletionProtectionFromAPI(t *testing.T) {
	cases := []struct {
		name     string
		current  types.Bool
		apiValue *bool
		want     types.Bool
	}{
		{"api true", types.BoolNull(), ptrBool(true), types.BoolValue(true)},
		{"api false, unset stays null", types.BoolNull(), ptrBool(false), types.BoolNull()},
		{"api omitted, unset stays null", types.BoolNull(), nil, types.BoolNull()},
		{"api omitted, unknown becomes null", types.BoolUnknown(), nil, types.BoolNull()},
		{"api omitted, managed true reads as off", types.BoolValue(true), nil, types.BoolValue(false)},
		{"api false, managed false", types.BoolValue(false), ptrBool(false), types.BoolValue(false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := postgresDeletionProtectionFromAPI(tc.current, tc.apiValue); !got.Equal(tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSettlePostgresProtection(t *testing.T) {
	cases := []struct {
		name      string
		planned   types.Bool
		sent      *bool
		reported  *bool
		want      types.Bool
		wantError bool
	}{
		{"nothing sent keeps plan", types.BoolValue(false), nil, nil, types.BoolValue(false), false},
		{"nothing sent, null plan", types.BoolNull(), nil, ptrBool(false), types.BoolNull(), false},
		{"sent true confirmed", types.BoolValue(true), ptrBool(true), ptrBool(true), types.BoolValue(true), false},
		{"sent true not reported", types.BoolValue(true), ptrBool(true), nil, types.BoolNull(), true},
		{"sent true reported false", types.BoolValue(true), ptrBool(true), ptrBool(false), types.BoolValue(false), true},
		{"sent false omitted", types.BoolValue(false), ptrBool(false), nil, types.BoolValue(false), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m models.PostgresServiceResourceModel
			m.DeletionProtection = types.BoolValue(true) // overwritten unless sent is nil and planned unknown
			detail := settlePostgresProtection(tc.planned, tc.sent, tc.reported, &m)
			if (detail != "") != tc.wantError {
				t.Errorf("detail = %q, wantError %v", detail, tc.wantError)
			}
			if !m.DeletionProtection.Equal(tc.want) {
				t.Errorf("state = %v, want %v", m.DeletionProtection, tc.want)
			}
		})
	}
}

func TestBuildPostgresUpdate_DeletionProtection(t *testing.T) {
	ctx := context.Background()
	m := func(dp types.Bool) models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{
			Size:               types.StringValue("c6gd.large"),
			HaType:             types.StringValue("none"),
			StandbyCount:       types.Int64Value(0),
			Tags:               mapTags("env", "prod"),
			DeletionProtection: dp,
		}
	}

	t.Run("turning on sends the flag and re-asserts tags", func(t *testing.T) {
		result, diags := buildPostgresUpdate(ctx, m(types.BoolValue(true)), m(types.BoolNull()))
		require.False(t, diags.HasError(), "%v", diags)
		require.NotNil(t, result.Body)
		require.NotNil(t, result.Body.DeletionProtection)
		require.True(t, *result.Body.DeletionProtection)
		require.False(t, result.TransitionExpected, "the flag is not an instance transition")
		require.NotNil(t, result.Body.Tags)
		require.Equal(t, []api.Tag{{Key: "env", Value: "prod"}}, *result.Body.Tags)
		require.Empty(t, result.Body.Size)
		require.Empty(t, result.Body.HaType)
	})
	t.Run("turning off sends false", func(t *testing.T) {
		result, diags := buildPostgresUpdate(ctx, m(types.BoolValue(false)), m(types.BoolValue(true)))
		require.False(t, diags.HasError(), "%v", diags)
		require.NotNil(t, result.Body)
		require.NotNil(t, result.Body.DeletionProtection)
		require.False(t, *result.Body.DeletionProtection)
	})
	t.Run("removed from config is no change", func(t *testing.T) {
		result, diags := buildPostgresUpdate(ctx, m(types.BoolNull()), m(types.BoolValue(true)))
		require.False(t, diags.HasError(), "%v", diags)
		require.Nil(t, result.Body)
	})
	t.Run("unchanged is no change", func(t *testing.T) {
		result, diags := buildPostgresUpdate(ctx, m(types.BoolValue(true)), m(types.BoolValue(true)))
		require.False(t, diags.HasError(), "%v", diags)
		require.Nil(t, result.Body)
	})
}

func TestReplicaUpdateForbidden_DeletionProtection(t *testing.T) {
	state := gateModel(false)
	plan := gateModel(false)
	plan.DeletionProtection = types.BoolValue(true)
	diags := replicaUpdateForbidden(plan, state)
	require.True(t, diags.HasError())
	require.Contains(t, diags.Errors()[0].Detail(), "deletion_protection")

	require.False(t, replicaUpdateForbidden(state, state).HasError())
}

func TestForbidDeletionProtectionOnReplicaCreate(t *testing.T) {
	replica := func(dp types.Bool) models.PostgresServiceResourceModel {
		m := gateModel(false)
		m.ReadReplicaOf = types.StringValue("pg-primary")
		m.DeletionProtection = dp
		return m
	}
	require.True(t, forbidDeletionProtectionOnReplicaCreate(replica(types.BoolValue(true))).HasError())
	require.False(t, forbidDeletionProtectionOnReplicaCreate(replica(types.BoolValue(false))).HasError())
	require.False(t, forbidDeletionProtectionOnReplicaCreate(replica(types.BoolNull())).HasError())
	require.False(t, forbidDeletionProtectionOnReplicaCreate(replica(types.BoolUnknown())).HasError())

	primary := gateModel(true)
	primary.DeletionProtection = types.BoolValue(true)
	require.False(t, forbidDeletionProtectionOnReplicaCreate(primary).HasError())
}

func TestSyncPostgresState_DeletionProtection(t *testing.T) {
	var state models.PostgresServiceResourceModel
	state.DeletionProtection = types.BoolNull()
	require.False(t, syncPostgresState(context.Background(), &api.Postgres{DeletionProtection: ptrBool(true)}, &state).HasError())
	require.True(t, state.DeletionProtection.Equal(types.BoolValue(true)))
}

func postgresSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	var schemaResp resource.SchemaResponse
	(&PostgresServiceResource{}).Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError(), "%v", schemaResp.Diagnostics)
	return schemaResp
}

func runningPostgres(dp *bool) *api.Postgres {
	return &api.Postgres{
		Id: "pg-1", Name: "n", Provider: "aws", Region: "us-east-1", Size: "m6gd.large",
		PostgresVersion: "18", HaType: "none", State: api.PostgresStateRunning, IsPrimary: true,
		DeletionProtection: dp,
	}
}

func TestPostgresUpdate_EnablesDeletionProtection(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresServiceResource{client: client}
	sch := postgresSchema(t).Schema

	stateModel := gateModel(true)
	stateModel.StandbyCount = types.Int64Value(0)
	stateModel.Password = types.StringValue("ValidPass1234x")
	prior := tfsdk.State{Schema: sch}
	require.False(t, prior.Set(ctx, stateModel).HasError())
	planModel := stateModel
	planModel.DeletionProtection = types.BoolValue(true)
	planned := tfsdk.State{Schema: sch}
	require.False(t, planned.Set(ctx, planModel).HasError())

	client.UpdatePostgresMock.Expect(ctx, "pg-1", api.PostgresUpdate{DeletionProtection: ptrBool(true)}).
		Return(runningPostgres(ptrBool(true)), nil)
	client.GetPostgresMock.Expect(ctx, "pg-1").Return(runningPostgres(ptrBool(true)), nil)
	client.GetPostgresConfigMock.Expect(ctx, "pg-1").Return(&api.PostgresConfig{}, nil)

	resp := resource.UpdateResponse{State: prior}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: planned.Raw},
		Config: tfsdk.Config{Schema: sch, Raw: planned.Raw},
		State:  prior,
	}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	var got models.PostgresServiceResourceModel
	require.False(t, resp.State.Get(ctx, &got).HasError())
	require.True(t, got.DeletionProtection.Equal(types.BoolValue(true)), "%v", got.DeletionProtection)
}

func TestPostgresUpdate_DeletionProtectionNotConfirmed(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresServiceResource{client: client}
	sch := postgresSchema(t).Schema

	stateModel := gateModel(true)
	stateModel.StandbyCount = types.Int64Value(0)
	prior := tfsdk.State{Schema: sch}
	require.False(t, prior.Set(ctx, stateModel).HasError())
	planModel := stateModel
	planModel.DeletionProtection = types.BoolValue(true)
	planned := tfsdk.State{Schema: sch}
	require.False(t, planned.Set(ctx, planModel).HasError())

	// Organization without the feature: the API accepts the PATCH but never reports the field.
	client.UpdatePostgresMock.Return(runningPostgres(nil), nil)
	client.GetPostgresMock.Return(runningPostgres(nil), nil)
	client.GetPostgresConfigMock.Return(&api.PostgresConfig{}, nil)

	resp := resource.UpdateResponse{State: prior}
	r.Update(ctx, resource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: planned.Raw},
		Config: tfsdk.Config{Schema: sch, Raw: planned.Raw},
		State:  prior,
	}, &resp)
	require.True(t, resp.Diagnostics.HasError())
	require.Equal(t, "Postgres Deletion Protection Not Applied", resp.Diagnostics.Errors()[0].Summary())
}

func TestPostgresCreate_EnablesDeletionProtectionAfterRunning(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresServiceResource{client: client}
	sch := postgresSchema(t).Schema

	planModel := gateModel(true)
	planModel.ID = types.StringUnknown()
	planModel.StandbyCount = types.Int64Value(0)
	planModel.Password = types.StringValue("ValidPass1234x")
	planModel.Tags = mapTags("env", "prod")
	planModel.DeletionProtection = types.BoolValue(true)
	planned := tfsdk.State{Schema: sch}
	require.False(t, planned.Set(ctx, planModel).HasError())

	var order []string
	client.CreatePostgresMock.Set(func(_ context.Context, body api.PostgresCreate) (*api.Postgres, string, error) {
		order = append(order, "create")
		return runningPostgres(nil), "", nil
	})
	client.WaitForPostgresStateMock.Set(func(context.Context, string, func(string) bool, int) error {
		order = append(order, "wait")
		return nil
	})
	client.SetPostgresPasswordMock.Set(func(context.Context, string, api.PostgresPassword) (*api.PostgresPassword, error) {
		order = append(order, "password")
		return &api.PostgresPassword{}, nil
	})
	client.UpdatePostgresMock.Set(func(_ context.Context, id string, body api.PostgresUpdate) (*api.Postgres, error) {
		order = append(order, "protect")
		if body.DeletionProtection == nil || !*body.DeletionProtection {
			return nil, fmt.Errorf("want deletionProtection=true, got %v", body.DeletionProtection)
		}
		if body.Tags == nil || len(*body.Tags) != 1 {
			return nil, fmt.Errorf("want tags re-asserted, got %v", body.Tags)
		}
		return runningPostgres(ptrBool(true)), nil
	})
	client.GetPostgresMock.Return(runningPostgres(ptrBool(true)), nil)
	client.GetPostgresConfigMock.Return(&api.PostgresConfig{}, nil)

	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: planned.Raw.Copy()}}
	r.Create(ctx, resource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: planned.Raw},
		Config: tfsdk.Config{Schema: sch, Raw: planned.Raw},
	}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	require.Equal(t, "create,wait,password,protect", strings.Join(order, ","))
	var got models.PostgresServiceResourceModel
	require.False(t, resp.State.Get(ctx, &got).HasError())
	require.True(t, got.DeletionProtection.Equal(types.BoolValue(true)), "%v", got.DeletionProtection)
}

func TestPostgresDelete_DeletionProtected(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresServiceResource{client: client}
	sch := postgresSchema(t).Schema

	prior := tfsdk.State{Schema: sch}
	require.False(t, prior.Set(ctx, gateModel(true)).HasError())
	client.DeletePostgresMock.Expect(ctx, "pg-1").Return(fmt.Errorf("%w: status: 409", api.ErrPostgresDeletionProtected))

	resp := resource.DeleteResponse{State: prior}
	r.Delete(ctx, resource.DeleteRequest{State: prior}, &resp)
	require.True(t, resp.Diagnostics.HasError())
	require.Equal(t, "Postgres Service Is Protected From Deletion", resp.Diagnostics.Errors()[0].Summary())
	require.Contains(t, resp.Diagnostics.Errors()[0].Detail(), "deletion_protection = false")
}
