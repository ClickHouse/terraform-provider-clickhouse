package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/gojuno/minimock/v3"
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

func allowedVpcEndpointMap(endpoints ...api.PostgresAllowedVpcEndpoint) types.Map {
	m, diags := allowedVpcEndpointsToMap(context.Background(), endpoints)
	if diags.HasError() {
		panic(diags)
	}
	return m
}

func privateEndpointModel() models.PostgresPrivateEndpointResourceModel {
	return models.PostgresPrivateEndpointResourceModel{
		PostgresID:          types.StringValue(testPrivateEndpointPostgresID),
		State:               types.StringValue("available"),
		EndpointServiceName: types.StringValue(availablePrivateEndpoint.EndpointServiceName),
		PrivateHostname:     types.StringValue(availablePrivateEndpoint.PrivateHostname),
	}
}

func TestPostgresPrivateEndpointCreate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		waitErr error
	}{
		{name: "waits until available"},
		{name: "wait fails", waitErr: errors.New("did not reach the expected state")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := api.NewClientMock(minimock.NewController(t))
			r := &PostgresPrivateEndpointResource{client: client}
			s := resourceSchema(ctx, t, r)

			client.CreatePostgresPrivateEndpointMock.Expect(ctx, testPrivateEndpointPostgresID).
				Return(&api.PostgresPrivateEndpoint{State: "creating"}, nil)
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
		})
	}
}

func TestPostgresPrivateEndpointReadNotFound(t *testing.T) {
	ctx := context.Background()
	client := api.NewClientMock(minimock.NewController(t))
	r := &PostgresPrivateEndpointResource{client: client}
	s := resourceSchema(ctx, t, r)
	client.GetPostgresPrivateEndpointMock.Return(nil, errors.New("status: 404, body: not found"))

	state := encodeState(ctx, t, s, privateEndpointModel())
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

	state := encodeState(ctx, t, s, privateEndpointModel())
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
		require.Equal(t, endpoints, *body.AllowedVpcEndpoints, "sent sorted by ID")
		return &api.PostgresPrivateEndpoint{State: "available", AllowedVpcEndpoints: endpoints}, nil
	})

	plan := encodeState(ctx, t, s, models.PostgresPrivateEndpointAttachmentResourceModel{
		PostgresID:          types.StringValue(testPrivateEndpointPostgresID),
		AllowedVpcEndpoints: allowedVpcEndpointMap(endpoints[1], endpoints[0]),
	})
	resp := resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s.Schema, Raw: plan.Raw}}, &resp)
	require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)

	var got models.PostgresPrivateEndpointAttachmentResourceModel
	require.False(t, resp.State.Get(ctx, &got).HasError())
	require.True(t, got.AllowedVpcEndpoints.Equal(allowedVpcEndpointMap(endpoints...)))
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
				AllowedVpcEndpoints: allowedVpcEndpointMap(api.PostgresAllowedVpcEndpoint{VpcEndpointId: "vpce-0123456789abcdef0"}),
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
