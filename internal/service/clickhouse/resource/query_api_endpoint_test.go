package resource

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func TestQueryAPIEndpointResourceSchema(t *testing.T) {
	ctx := context.Background()
	_, resp := queryAPIEndpointSchema(t)
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid schema implementation: %v", diags)
	}

	wantAttributes := []string{
		"id", "service_id", "name", "sql", "database", "parameters",
		"api_key_ids", "roles", "allowed_origins", "url",
	}
	if len(resp.Schema.Attributes) != len(wantAttributes) {
		t.Fatalf("attribute count = %d; want %d: %#v", len(resp.Schema.Attributes), len(wantAttributes), resp.Schema.Attributes)
	}
	for _, name := range wantAttributes {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing %q", name)
		}
	}

	parameters, ok := resp.Schema.Attributes["parameters"].(schema.MapAttribute)
	if !ok || !parameters.Optional || !parameters.Computed || parameters.Default == nil {
		t.Errorf("parameters must be optional with an empty static default: %#v", parameters)
	}
	allowedOrigins, ok := resp.Schema.Attributes["allowed_origins"].(schema.SetAttribute)
	if !ok || !allowedOrigins.Optional || !allowedOrigins.Computed || allowedOrigins.Default == nil {
		t.Errorf("allowed_origins must be optional with an empty static default: %#v", allowedOrigins)
	}
}

const (
	queryAPIEndpointServiceID = "22222222-2222-2222-2222-222222222222"
	queryAPIEndpointID        = "33333333-3333-3333-3333-333333333333"
	queryAPIEndpointAPIKeyID  = "44444444-4444-4444-4444-444444444444"
)

func queryAPIEndpointSchema(t *testing.T) (*QueryAPIEndpointResource, resource.SchemaResponse) {
	t.Helper()
	r := NewQueryAPIEndpointResource().(*QueryAPIEndpointResource)
	resp := resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	return r, resp
}

func queryAPIEndpointModel(t *testing.T) models.QueryAPIEndpointResourceModel {
	t.Helper()
	parameters, diags := types.MapValueFrom(
		context.Background(),
		types.StringType,
		map[string]string{"date": "2026-08-13"},
	)
	if diags.HasError() {
		t.Fatalf("parameters: %v", diags)
	}
	apiKeyIDs, diags := types.SetValueFrom(
		context.Background(),
		types.StringType,
		[]string{queryAPIEndpointAPIKeyID},
	)
	if diags.HasError() {
		t.Fatalf("api key ids: %v", diags)
	}
	roles, diags := types.SetValueFrom(context.Background(), types.StringType, []string{"analytics_reader"})
	if diags.HasError() {
		t.Fatalf("roles: %v", diags)
	}
	allowedOrigins, diags := types.SetValueFrom(context.Background(), types.StringType, []string{"https://example.com"})
	if diags.HasError() {
		t.Fatalf("allowed origins: %v", diags)
	}

	return models.QueryAPIEndpointResourceModel{
		ID:             types.StringNull(),
		ServiceID:      types.StringValue(queryAPIEndpointServiceID),
		Name:           types.StringValue("daily-actives"),
		SQL:            types.StringValue("SELECT count() FROM events WHERE date = {date:Date}"),
		Database:       types.StringValue("analytics"),
		Parameters:     parameters,
		APIKeyIDs:      apiKeyIDs,
		Roles:          roles,
		AllowedOrigins: allowedOrigins,
		URL:            types.StringNull(),
	}
}

func queryAPIEndpointRequest() api.QueryAPIEndpointRequest {
	return api.QueryAPIEndpointRequest{
		Name:           "daily-actives",
		SQL:            "SELECT count() FROM events WHERE date = {date:Date}",
		Database:       "analytics",
		Parameters:     map[string]string{"date": "2026-08-13"},
		APIKeyIDs:      []string{queryAPIEndpointAPIKeyID},
		Roles:          []string{"analytics_reader"},
		AllowedOrigins: []string{"https://example.com"},
	}
}

func queryAPIEndpointResponse(ownerType string) *api.QueryAPIEndpoint {
	return &api.QueryAPIEndpoint{
		QueryAPIEndpointRequest: queryAPIEndpointRequest(),
		ID:                      queryAPIEndpointID,
		URL:                     "https://queries.clickhouse.cloud/run/" + queryAPIEndpointID,
		OwnerType:               ownerType,
	}
}

func TestQueryAPIEndpointResourceCreate(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := queryAPIEndpointSchema(t)
	model := queryAPIEndpointModel(t)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateQueryAPIEndpointMock.
		Expect(ctx, queryAPIEndpointServiceID, queryAPIEndpointRequest()).
		Return(queryAPIEndpointResponse(api.QueryAPIEndpointOwnerType), nil)

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}

	var state models.QueryAPIEndpointResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.ID.ValueString() != queryAPIEndpointID {
		t.Errorf("id = %q; want %q", state.ID.ValueString(), queryAPIEndpointID)
	}
	if state.URL.ValueString() != "https://queries.clickhouse.cloud/run/"+queryAPIEndpointID {
		t.Errorf("url = %q", state.URL.ValueString())
	}
}

func TestQueryAPIEndpointResourceUpdate(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := queryAPIEndpointSchema(t)
	model := queryAPIEndpointModel(t)
	model.ID = types.StringValue(queryAPIEndpointID)
	model.Name = types.StringValue("weekly-actives")
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	request := queryAPIEndpointRequest()
	request.Name = "weekly-actives"
	endpoint := queryAPIEndpointResponse(api.QueryAPIEndpointOwnerType)
	endpoint.Name = "weekly-actives"
	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		UpdateQueryAPIEndpointMock.
		Expect(ctx, queryAPIEndpointServiceID, queryAPIEndpointID, request).
		Return(endpoint, nil)

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}

	var state models.QueryAPIEndpointResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.Name.ValueString() != "weekly-actives" {
		t.Errorf("name = %q; want weekly-actives", state.Name.ValueString())
	}
}

func TestQueryAPIEndpointResourceReadRemovesMissingEndpoint(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := queryAPIEndpointSchema(t)
	model := queryAPIEndpointModel(t)
	model.ID = types.StringValue(queryAPIEndpointID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetQueryAPIEndpointMock.
		Expect(ctx, queryAPIEndpointServiceID, queryAPIEndpointID).
		Return(nil, errors.New("status: 404, body: not found"))

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected resource to be removed from state")
	}
}

func TestQueryAPIEndpointResourceRead(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := queryAPIEndpointSchema(t)
	model := queryAPIEndpointModel(t)
	model.ID = types.StringValue(queryAPIEndpointID)
	model.Name = types.StringValue("stale-name")
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetQueryAPIEndpointMock.
		Expect(ctx, queryAPIEndpointServiceID, queryAPIEndpointID).
		Return(queryAPIEndpointResponse(api.QueryAPIEndpointOwnerType), nil)

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var got models.QueryAPIEndpointResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Name.ValueString() != "daily-actives" {
		t.Errorf("name = %q; want daily-actives", got.Name.ValueString())
	}
}

func TestQueryAPIEndpointResourceDeleteIgnoresNotFound(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := queryAPIEndpointSchema(t)
	model := queryAPIEndpointModel(t)
	model.ID = types.StringValue(queryAPIEndpointID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		DeleteQueryAPIEndpointMock.
		Expect(ctx, queryAPIEndpointServiceID, queryAPIEndpointID).
		Return(errors.New("status: 404, body: not found"))

	resp := resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %v", resp.Diagnostics)
	}
}

func TestQueryAPIEndpointResourceReadRejectsUserOwnedEndpoint(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := queryAPIEndpointSchema(t)
	model := queryAPIEndpointModel(t)
	model.ID = types.StringValue(queryAPIEndpointID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetQueryAPIEndpointMock.
		Expect(ctx, queryAPIEndpointServiceID, queryAPIEndpointID).
		Return(queryAPIEndpointResponse("user"), nil)

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Read should reject a user-owned endpoint")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Query API endpoint cannot be managed by this resource" {
		t.Errorf("diagnostic summary = %q", got)
	}
}

func TestQueryAPIEndpointResourceUpdateRejectsUserOwnedEndpoint(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := queryAPIEndpointSchema(t)
	model := queryAPIEndpointModel(t)
	model.ID = types.StringValue(queryAPIEndpointID)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		UpdateQueryAPIEndpointMock.
		Expect(ctx, queryAPIEndpointServiceID, queryAPIEndpointID, queryAPIEndpointRequest()).
		Return(nil, errors.New("status: 409, body: user-owned saved query"))

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan}, &resp)
	assertUserOwnedQueryAPIEndpointDiagnostic(t, resp.Diagnostics.Errors())
}

func TestQueryAPIEndpointResourceDeleteRejectsUserOwnedEndpoint(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := queryAPIEndpointSchema(t)
	model := queryAPIEndpointModel(t)
	model.ID = types.StringValue(queryAPIEndpointID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		DeleteQueryAPIEndpointMock.
		Expect(ctx, queryAPIEndpointServiceID, queryAPIEndpointID).
		Return(errors.New("status: 409, body: user-owned saved query"))

	resp := resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	assertUserOwnedQueryAPIEndpointDiagnostic(t, resp.Diagnostics.Errors())
}

func assertUserOwnedQueryAPIEndpointDiagnostic(t *testing.T, diagnostics diag.Diagnostics) {
	t.Helper()
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %v; want one error", diagnostics)
	}
	if got := diagnostics[0].Summary(); got != "Query API endpoint cannot be managed by this resource" {
		t.Errorf("diagnostic summary = %q", got)
	}
}

func TestQueryAPIEndpointResourceImport(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := queryAPIEndpointSchema(t)

	resp := resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	r.ImportState(ctx, resource.ImportStateRequest{
		ID: queryAPIEndpointServiceID + ":" + queryAPIEndpointID,
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diagnostics: %v", resp.Diagnostics)
	}

	for attribute, want := range map[string]string{
		"service_id": queryAPIEndpointServiceID,
		"id":         queryAPIEndpointID,
	} {
		var got types.String
		if diags := resp.State.GetAttribute(ctx, path.Root(attribute), &got); diags.HasError() {
			t.Fatalf("read %s: %v", attribute, diags)
		}
		if got.ValueString() != want {
			t.Errorf("%s = %q; want %q", attribute, got.ValueString(), want)
		}
	}
}

func TestQueryAPIEndpointResourceImportRejectsInvalidID(t *testing.T) {
	for _, id := range []string{
		queryAPIEndpointID,
		queryAPIEndpointServiceID + "/" + queryAPIEndpointID,
		"not-a-uuid:" + queryAPIEndpointID,
		queryAPIEndpointServiceID + ":not-a-uuid",
		queryAPIEndpointServiceID + ":" + queryAPIEndpointID + ":extra",
	} {
		t.Run(strings.ReplaceAll(id, "/", "_"), func(t *testing.T) {
			ctx := context.Background()
			r, schemaResp := queryAPIEndpointSchema(t)
			resp := resource.ImportStateResponse{
				State: tfsdk.State{
					Schema: schemaResp.Schema,
					Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
				},
			}

			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, &resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("ImportState should reject an invalid ID")
			}
		})
	}
}

func TestQueryAPIEndpointRequestFromModelUsesEmptyOptionalCollections(t *testing.T) {
	model := queryAPIEndpointModel(t)
	model.Parameters = types.MapNull(types.StringType)
	model.AllowedOrigins = types.SetNull(types.StringType)

	request, diags := queryAPIEndpointRequestFromModel(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("queryAPIEndpointRequestFromModel diagnostics: %v", diags)
	}
	if request.Parameters == nil || len(request.Parameters) != 0 {
		t.Errorf("parameters = %#v; want non-nil empty map", request.Parameters)
	}
	if request.AllowedOrigins == nil || len(request.AllowedOrigins) != 0 {
		t.Errorf("allowed origins = %#v; want non-nil empty slice", request.AllowedOrigins)
	}
}
