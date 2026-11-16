package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
)

const testPrivateEndpointPostgresID = "pg-1"

var availablePrivateEndpoint = &api.PostgresPrivateEndpoint{
	State:               api.PostgresPrivateEndpointStateAvailable,
	EndpointServiceName: "com.amazonaws.vpce.us-east-1.vpce-svc-0123456789abcdef0",
	PrivateHostname:     "pg.private.example.com",
	SupportedRegions:    []string{"us-west-2"},
}

func resourceSchema(ctx context.Context, t *testing.T, r resource.Resource) resource.SchemaResponse {
	t.Helper()
	var resp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	return resp
}

func encodeState(ctx context.Context, t *testing.T, s resource.SchemaResponse, model any) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: s.Schema}
	require.False(t, state.Set(ctx, model).HasError())
	return state
}

func emptyState(ctx context.Context, s resource.SchemaResponse) tfsdk.State {
	return tfsdk.State{Schema: s.Schema, Raw: tftypes.NewValue(s.Schema.Type().TerraformType(ctx), nil)}
}

func regionSet(regions ...string) types.Set {
	set, diags := types.SetValueFrom(context.Background(), types.StringType, append([]string{}, regions...))
	if diags.HasError() {
		panic(diags)
	}
	return set
}

func allowedVpcEndpointSet(endpoints ...api.PostgresAllowedVpcEndpoint) types.Set {
	set, diags := allowedVpcEndpointsToSet(context.Background(), endpoints)
	if diags.HasError() {
		panic(diags)
	}
	return set
}

func privateEndpointModel(regions types.Set) models.PostgresPrivateEndpointResourceModel {
	return models.PostgresPrivateEndpointResourceModel{
		PostgresID:          types.StringValue(testPrivateEndpointPostgresID),
		SupportedRegions:    regions,
		State:               types.StringValue("available"),
		EndpointServiceName: types.StringValue(availablePrivateEndpoint.EndpointServiceName),
		PrivateHostname:     types.StringValue(availablePrivateEndpoint.PrivateHostname),
	}
}

func TestPostgresPrivateEndpointCreate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		regions types.Set
		body    api.PostgresPrivateEndpointCreate
		waitErr error
	}{
		{name: "regions omitted", regions: types.SetUnknown(types.StringType)},
		{name: "with regions", regions: regionSet("us-west-2"), body: api.PostgresPrivateEndpointCreate{SupportedRegions: []string{"us-west-2"}}},
		{name: "wait fails", regions: types.SetUnknown(types.StringType), waitErr: errors.New("did not reach the expected state")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := api.NewClientMock(minimock.NewController(t))
			r := &PostgresPrivateEndpointResource{client: client}
			s := resourceSchema(ctx, t, r)

			client.CreatePostgresPrivateEndpointMock.Expect(ctx, testPrivateEndpointPostgresID, tc.body).
				Return(&api.PostgresPrivateEndpoint{State: "creating", SupportedRegions: tc.body.SupportedRegions}, nil)
			client.WaitForPostgresPrivateEndpointMock.Set(func(_ context.Context, id string, done func(*api.PostgresPrivateEndpoint) bool, _ int) error {
				require.Equal(t, testPrivateEndpointPostgresID, id)
				require.False(t, done(&api.PostgresPrivateEndpoint{State: "available"}), "needs the service name")
				require.True(t, done(availablePrivateEndpoint))
				return tc.waitErr
			})
			if tc.waitErr == nil {
				client.GetPostgresPrivateEndpointMock.Expect(ctx, testPrivateEndpointPostgresID).Return(availablePrivateEndpoint, nil)
			}

			plan := encodeState(ctx, t, s, models.PostgresPrivateEndpointResourceModel{
				PostgresID:          types.StringValue(testPrivateEndpointPostgresID),
				SupportedRegions:    tc.regions,
				State:               types.StringUnknown(),
				EndpointServiceName: types.StringUnknown(),
				PrivateHostname:     types.StringUnknown(),
			})
			resp := resource.CreateResponse{State: emptyState(ctx, s)}
			r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s.Schema, Raw: plan.Raw}}, &resp)

			var got models.PostgresPrivateEndpointResourceModel
			require.False(t, resp.State.Get(ctx, &got).HasError())
			require.Equal(t, testPrivateEndpointPostgresID, got.PostgresID.ValueString())
			if tc.waitErr != nil {
				require.True(t, resp.Diagnostics.HasError())
				require.Equal(t, "creating", got.State.ValueString(), "a failed wait keeps the created service in state")
				require.True(t, got.EndpointServiceName.IsNull())
				return
			}
			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			require.Equal(t, availablePrivateEndpoint.EndpointServiceName, got.EndpointServiceName.ValueString())
			require.Equal(t, availablePrivateEndpoint.PrivateHostname, got.PrivateHostname.ValueString())
			require.True(t, got.SupportedRegions.Equal(regionSet("us-west-2")))
		})
	}
}

func TestPostgresPrivateEndpointUpdateRegions(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresPrivateEndpointResource{client: client}
	s := resourceSchema(ctx, t, r)
	none := []string{}
	client.UpdatePostgresPrivateEndpointMock.Expect(ctx, testPrivateEndpointPostgresID, api.PostgresPrivateEndpointUpdate{SupportedRegions: &none}).
		Return(&api.PostgresPrivateEndpoint{State: "available", EndpointServiceName: "svc", PrivateHostname: "h"}, nil)

	prior := encodeState(ctx, t, s, privateEndpointModel(regionSet("us-west-2")))
	planned := encodeState(ctx, t, s, privateEndpointModel(regionSet()))
	resp := resource.UpdateResponse{State: prior}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s.Schema, Raw: planned.Raw}, State: prior}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

	var got models.PostgresPrivateEndpointResourceModel
	require.False(t, resp.State.Get(ctx, &got).HasError())
	require.True(t, got.SupportedRegions.Equal(regionSet()))
}

func TestPostgresPrivateEndpointModifyPlanRejectsOwnRegion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		regions types.Set
		wantErr bool
	}{
		{name: "other region", regions: regionSet("us-west-2")},
		{name: "own region", regions: regionSet("us-west-2", "us-east-1"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := api.NewClientMock(minimock.NewController(t))
			r := &PostgresPrivateEndpointResource{client: client}
			s := resourceSchema(ctx, t, r)
			client.GetPostgresMock.Expect(ctx, testPrivateEndpointPostgresID).Return(&api.Postgres{Id: testPrivateEndpointPostgresID, Region: "us-east-1"}, nil)

			planned := encodeState(ctx, t, s, privateEndpointModel(tc.regions))
			plan := tfsdk.Plan{Schema: s.Schema, Raw: planned.Raw}
			resp := resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: tfsdk.Config{Schema: s.Schema, Raw: planned.Raw}, Plan: plan, State: emptyState(ctx, s)}, &resp)
			require.Equal(t, tc.wantErr, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
		})
	}
}

// Replacing postgres_id with a service in one of the old supported regions
// plans those regions from state when supported_regions is omitted.
func TestPostgresPrivateEndpointModifyPlanIgnoresInheritedRegions(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresPrivateEndpointResource{client: client}
	s := resourceSchema(ctx, t, r)

	configured := encodeState(ctx, t, s, privateEndpointModel(types.SetNull(types.StringType)))
	planned := encodeState(ctx, t, s, privateEndpointModel(regionSet("us-west-2")))
	plan := tfsdk.Plan{Schema: s.Schema, Raw: planned.Raw}
	resp := resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: tfsdk.Config{Schema: s.Schema, Raw: configured.Raw}, Plan: plan, State: emptyState(ctx, s)}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	require.Zero(t, client.GetPostgresAfterCounter(), "omitted regions are not checked")
}

func TestPostgresPrivateEndpointModifyPlanDefersUnknownRegions(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresPrivateEndpointResource{client: client}
	s := resourceSchema(ctx, t, r)
	regions := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("us-west-2"), types.StringUnknown()})

	planned := encodeState(ctx, t, s, privateEndpointModel(regions))
	plan := tfsdk.Plan{Schema: s.Schema, Raw: planned.Raw}
	resp := resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: tfsdk.Config{Schema: s.Schema, Raw: planned.Raw}, Plan: plan, State: emptyState(ctx, s)}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	require.Zero(t, client.GetPostgresAfterCounter(), "the own-region check waits until every region is known")
}

func TestPostgresPrivateEndpointReadNotFound(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresPrivateEndpointResource{client: client}
	s := resourceSchema(ctx, t, r)
	client.GetPostgresPrivateEndpointMock.Return(nil, errors.New("status: 404, body: not found"))

	state := encodeState(ctx, t, s, privateEndpointModel(regionSet()))
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
	require.True(t, resp.State.Raw.IsNull())
}

func TestPostgresPrivateEndpointDeleteWaitsUntilGone(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresPrivateEndpointResource{client: client}
	s := resourceSchema(ctx, t, r)
	client.DeletePostgresPrivateEndpointMock.Expect(ctx, testPrivateEndpointPostgresID).Return(nil)
	client.WaitForPostgresPrivateEndpointMock.Set(func(_ context.Context, _ string, done func(*api.PostgresPrivateEndpoint) bool, _ int) error {
		require.False(t, done(&api.PostgresPrivateEndpoint{State: "deleting"}))
		require.True(t, done(nil))
		return nil
	})

	state := encodeState(ctx, t, s, privateEndpointModel(regionSet()))
	resp := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
}

func TestPostgresPrivateEndpointAttachmentApply(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresPrivateEndpointAttachmentResource{client: client}
	s := resourceSchema(ctx, t, r)
	endpoints := []api.PostgresAllowedVpcEndpoint{
		{VpcEndpointId: "vpce-0123456789abcdef0", Description: "app"},
		{VpcEndpointId: "vpce-0123456789abcdef1", Description: ""},
	}
	client.UpdatePostgresPrivateEndpointMock.Set(func(_ context.Context, id string, body api.PostgresPrivateEndpointUpdate) (*api.PostgresPrivateEndpoint, error) {
		require.Equal(t, testPrivateEndpointPostgresID, id)
		require.Nil(t, body.SupportedRegions, "the attachment must not touch the regions")
		require.ElementsMatch(t, endpoints, *body.AllowedVpcEndpoints)
		// The API returns endpoints sorted by ID, whatever order they were sent in.
		return &api.PostgresPrivateEndpoint{State: "available", AllowedVpcEndpoints: endpoints}, nil
	})

	plan := encodeState(ctx, t, s, models.PostgresPrivateEndpointAttachmentResourceModel{
		PostgresID:          types.StringValue(testPrivateEndpointPostgresID),
		AllowedVpcEndpoints: allowedVpcEndpointSet(endpoints[1], endpoints[0]),
	})
	resp := resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s.Schema, Raw: plan.Raw}}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

	var got models.PostgresPrivateEndpointAttachmentResourceModel
	require.False(t, resp.State.Get(ctx, &got).HasError())
	require.True(t, got.AllowedVpcEndpoints.Equal(allowedVpcEndpointSet(endpoints...)))
}

func TestPostgresPrivateEndpointAttachmentDelete(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		wantErr bool
	}{
		{name: "removes every endpoint"},
		{name: "private endpoint already gone", err: errors.New("status: 404, body: not found")},
		{name: "API error", err: errors.New("status: 400, body: bad"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := api.NewClientMock(minimock.NewController(t))
			r := &PostgresPrivateEndpointAttachmentResource{client: client}
			s := resourceSchema(ctx, t, r)
			none := []api.PostgresAllowedVpcEndpoint{}
			client.UpdatePostgresPrivateEndpointMock.Expect(ctx, testPrivateEndpointPostgresID, api.PostgresPrivateEndpointUpdate{AllowedVpcEndpoints: &none}).
				Return(&api.PostgresPrivateEndpoint{}, tc.err)

			state := encodeState(ctx, t, s, models.PostgresPrivateEndpointAttachmentResourceModel{
				PostgresID:          types.StringValue(testPrivateEndpointPostgresID),
				AllowedVpcEndpoints: allowedVpcEndpointSet(api.PostgresAllowedVpcEndpoint{VpcEndpointId: "vpce-0123456789abcdef0"}),
			})
			resp := resource.DeleteResponse{State: state}
			r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
			require.Equal(t, tc.wantErr, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
		})
	}
}

func TestPostgresVpcEndpointValidation(t *testing.T) {
	for id, valid := range map[string]bool{
		"vpce-0123456789abcdef0": true,
		"vpce-0123456789ABCDEF0": false,
		"vpce-0123456789abcdef":  false,
		"0123456789abcdef01234":  false,
	} {
		require.Equal(t, valid, postgresVpcEndpointIDRegexp.MatchString(id), id)
	}
	for description, valid := range map[string]bool{
		"":              true,
		"a":             true,
		"app":           true,
		"prod vpc":      true,
		" app":          true,
		"multi\nline a": false,
		"tab\tinside":   false,
		" app":          false,
		"app\n":         false,
		"app\x00":       false,
		"\tapp":         false,
	} {
		require.Equal(t, valid, postgresVpcEndpointDescriptionRegexp.MatchString(description), "%q", description)
	}
}
