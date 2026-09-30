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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

const (
	savedQueryServiceID = "22222222-2222-2222-2222-222222222222"
	savedQueryID        = "33333333-3333-3333-3333-333333333333"
)

func TestSavedQueryResourceMetadata(t *testing.T) {
	r := NewSavedQueryResource()
	resp := resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "clickhouse"}, &resp)
	if got, want := resp.TypeName, "clickhouse_saved_query"; got != want {
		t.Fatalf("type name = %q; want %q", got, want)
	}
}

func TestSavedQueryResourceSchema(t *testing.T) {
	ctx := context.Background()
	_, resp := savedQuerySchema(t)
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid schema implementation: %v", diags)
	}

	wantAttributes := []string{"id", "service_id", "name", "sql", "database", "parameters"}
	if len(resp.Schema.Attributes) != len(wantAttributes) {
		t.Fatalf("attribute count = %d; want %d: %#v", len(resp.Schema.Attributes), len(wantAttributes), resp.Schema.Attributes)
	}
	for _, name := range wantAttributes {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing %q", name)
		}
	}

	id, ok := resp.Schema.Attributes["id"].(schema.StringAttribute)
	if !ok || !id.Computed || len(id.PlanModifiers) != 1 {
		t.Errorf("id must be computed with UseStateForUnknown: %#v", id)
	}
	serviceID, ok := resp.Schema.Attributes["service_id"].(schema.StringAttribute)
	if !ok || !serviceID.Required || len(serviceID.PlanModifiers) != 1 || len(serviceID.Validators) != 1 {
		t.Errorf("service_id must be required with UUID validation and replacement: %#v", serviceID)
	}
	name, ok := resp.Schema.Attributes["name"].(schema.StringAttribute)
	if !ok || !name.Required || len(name.Validators) != 1 {
		t.Errorf("name must be required with one length validator: %#v", name)
	}
	assertStringValidatorAccepts(t, name.Validators[0], " ")
	assertStringValidatorRejects(t, name.Validators[0], "")

	for _, attribute := range []string{"sql", "database"} {
		stringAttribute, ok := resp.Schema.Attributes[attribute].(schema.StringAttribute)
		if !ok || !stringAttribute.Required || len(stringAttribute.Validators) != 0 {
			t.Errorf("%s must be required without local validators: %#v", attribute, stringAttribute)
		}
	}
	parameters, ok := resp.Schema.Attributes["parameters"].(schema.MapAttribute)
	if !ok || !parameters.Optional || !parameters.Computed || parameters.Default == nil || parameters.ElementType != types.StringType {
		t.Errorf("parameters must be optional/computed with an empty string map default: %#v", parameters)
	}
}

func TestSavedQueryResourceConfigure(t *testing.T) {
	ctx := context.Background()

	t.Run("nil provider data is accepted", func(t *testing.T) {
		r := NewSavedQueryResource().(*SavedQueryResource)
		resp := resource.ConfigureResponse{}
		r.Configure(ctx, resource.ConfigureRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Configure diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("unexpected provider data is rejected", func(t *testing.T) {
		r := NewSavedQueryResource().(*SavedQueryResource)
		resp := resource.ConfigureResponse{}
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: "not provider data"}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("Configure should reject unexpected provider data")
		}
		if got := resp.Diagnostics.Errors()[0].Summary(); got != "Unexpected provider data" {
			t.Errorf("diagnostic summary = %q", got)
		}
	})

	t.Run("missing API is rejected", func(t *testing.T) {
		r := NewSavedQueryResource().(*SavedQueryResource)
		resp := resource.ConfigureResponse{}
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: &service.ProviderData{}}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("Configure should reject missing API")
		}
		if got := resp.Diagnostics.Errors()[0].Summary(); got != "ClickHouse Cloud API not configured" {
			t.Errorf("diagnostic summary = %q", got)
		}
	})

	t.Run("stores configured API", func(t *testing.T) {
		r := NewSavedQueryResource().(*SavedQueryResource)
		client := api.NewClientMock(minimock.NewController(t))
		resp := resource.ConfigureResponse{}
		r.Configure(ctx, resource.ConfigureRequest{ProviderData: &service.ProviderData{API: client}}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Configure diagnostics: %v", resp.Diagnostics)
		}
		if r.client != client {
			t.Fatal("Configure did not store the configured API")
		}
	})
}

func assertStringValidatorAccepts(t *testing.T, v validator.String, value string) {
	t.Helper()
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("name"),
		ConfigValue: types.StringValue(value),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Errorf("validator rejected %q: %v", value, resp.Diagnostics)
	}
}

func assertStringValidatorRejects(t *testing.T, v validator.String, value string) {
	t.Helper()
	resp := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("name"),
		ConfigValue: types.StringValue(value),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Errorf("validator accepted %q", value)
	}
}

func savedQuerySchema(t *testing.T) (*SavedQueryResource, resource.SchemaResponse) {
	t.Helper()
	r := NewSavedQueryResource().(*SavedQueryResource)
	resp := resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	return r, resp
}

func savedQueryModel(t *testing.T) models.SavedQueryResourceModel {
	t.Helper()
	parameters, diags := types.MapValueFrom(
		context.Background(),
		types.StringType,
		map[string]string{"date": "2026-08-13"},
	)
	if diags.HasError() {
		t.Fatalf("parameters: %v", diags)
	}

	return models.SavedQueryResourceModel{
		ID:         types.StringNull(),
		ServiceID:  types.StringValue(savedQueryServiceID),
		Name:       types.StringValue("daily-actives"),
		SQL:        types.StringValue("SELECT count() FROM events WHERE date = {date:Date}"),
		Database:   types.StringValue("analytics"),
		Parameters: parameters,
	}
}

func savedQueryRequest() api.SavedQueryRequest {
	return api.SavedQueryRequest{
		Name:       "daily-actives",
		SQL:        "SELECT count() FROM events WHERE date = {date:Date}",
		Database:   "analytics",
		Parameters: map[string]string{"date": "2026-08-13"},
	}
}

func savedQueryResponse() *api.SavedQuery {
	return &api.SavedQuery{
		SavedQueryRequest: savedQueryRequest(),
		ID:                savedQueryID,
	}
}

func TestSavedQueryResourceCreate(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "false")
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryRequest()).
		Return(savedQueryResponse(), nil)

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	assertBetaWarning(t, resp.Diagnostics)

	var state models.SavedQueryResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.ID.ValueString() != savedQueryID {
		t.Errorf("id = %q; want %q", state.ID.ValueString(), savedQueryID)
	}
	if state.Parameters.IsNull() || len(state.Parameters.Elements()) != 1 {
		t.Errorf("parameters = %v; want one entry", state.Parameters)
	}
}

func TestSavedQueryResourceUpdate(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "false")
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	model.Name = types.StringValue("weekly-actives")
	model.SQL = types.StringValue("SELECT count() FROM events")
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	request := savedQueryRequest()
	request.Name = "weekly-actives"
	request.SQL = "SELECT count() FROM events"
	response := savedQueryResponse()
	response.Name = request.Name
	response.SQL = request.SQL
	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		UpdateSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID, request).
		Return(response, nil)

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	assertBetaWarning(t, resp.Diagnostics)

	var state models.SavedQueryResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.Name.ValueString() != "weekly-actives" || state.SQL.ValueString() != request.SQL {
		t.Errorf("state = %+v; want updated query", state)
	}
}

func TestSavedQueryResourceRead(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "false")
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	model.Name = types.StringValue("stale-name")
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID).
		Return(savedQueryResponse(), nil)

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	if len(resp.Diagnostics) != 0 {
		t.Fatalf("Read diagnostics: %v; want no warnings", resp.Diagnostics)
	}

	var got models.SavedQueryResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Name.ValueString() != "daily-actives" {
		t.Errorf("name = %q; want daily-actives", got.Name.ValueString())
	}
}

func TestSavedQueryResourceReadRemovesMissingQuery(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID).
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

func TestSavedQueryResourceReadNormalizesEmptyParameters(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	response := savedQueryResponse()
	response.Parameters = nil
	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID).
		Return(response, nil)

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var got models.SavedQueryResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Parameters.IsNull() || got.Parameters.IsUnknown() || len(got.Parameters.Elements()) != 0 {
		t.Errorf("parameters = %v; want known empty map", got.Parameters)
	}
}

func TestSavedQueryResourceReadReportsUnexpectedError(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID).
		Return(nil, errors.New("status: 500, body: internal server error"))

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Read should report unexpected errors")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Error reading saved query" {
		t.Errorf("diagnostic summary = %q", got)
	}
}

func TestSavedQueryResourceDeleteIgnoresNotFound(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		DeleteSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID).
		Return(errors.New("status: 404, body: not found"))

	resp := resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %v", resp.Diagnostics)
	}
}

func TestSavedQueryResourceDelete(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		DeleteSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID).
		Return(nil)

	resp := resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %v", resp.Diagnostics)
	}
}

func TestSavedQueryResourceDeleteReportsUnexpectedError(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		DeleteSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID).
		Return(errors.New("status: 500, body: internal server error"))

	resp := resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Delete should report unexpected errors")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Error deleting saved query" {
		t.Errorf("diagnostic summary = %q", got)
	}
}

func TestSavedQueryResourceUpdateReportsNameConflict(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		UpdateSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID, savedQueryRequest()).
		Return(nil, errors.New("status: 409, body: saved query name already exists"))

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan}, &resp)
	if len(resp.Diagnostics.Errors()) != 1 {
		t.Fatalf("diagnostics = %v; want one error", resp.Diagnostics)
	}
	if got := resp.Diagnostics.Errors()[0]; got.Summary() != "Error updating saved query" || !strings.Contains(got.Detail(), "saved query name already exists") {
		t.Errorf("diagnostic = %v; want name conflict", got)
	}
}

func TestSavedQueryResourceDeleteRejectsConflict(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.ID = types.StringValue(savedQueryID)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		DeleteSavedQueryMock.
		Expect(ctx, savedQueryServiceID, savedQueryID).
		Return(errors.New("status: 409, body: user-owned saved query"))

	resp := resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	assertIncompatibleSavedQueryDiagnostic(t, resp.Diagnostics.Errors())
}

func assertIncompatibleSavedQueryDiagnostic(t *testing.T, diagnostics diag.Diagnostics) {
	t.Helper()
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %v; want one error", diagnostics)
	}
	if got := diagnostics[0].Summary(); got != "Saved query cannot be managed by this resource" {
		t.Errorf("diagnostic summary = %q", got)
	}
}

func TestSavedQueryResourceImport(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "false")
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	resp := resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	r.ImportState(ctx, resource.ImportStateRequest{
		ID: savedQueryServiceID + ":" + savedQueryID,
	}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diagnostics: %v", resp.Diagnostics)
	}
	assertBetaWarning(t, resp.Diagnostics)

	for attribute, want := range map[string]string{
		"service_id": savedQueryServiceID,
		"id":         savedQueryID,
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

func TestSavedQueryResourceImportRejectsInvalidID(t *testing.T) {
	for _, id := range []string{
		savedQueryID,
		savedQueryServiceID + "/" + savedQueryID,
		"not-a-uuid:" + savedQueryID,
		savedQueryServiceID + ":not-a-uuid",
		savedQueryServiceID + ":" + savedQueryID + ":extra",
	} {
		t.Run(strings.ReplaceAll(id, "/", "_"), func(t *testing.T) {
			ctx := context.Background()
			r, schemaResp := savedQuerySchema(t)
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

func assertBetaWarning(t *testing.T, diagnostics diag.Diagnostics) {
	t.Helper()
	if len(diagnostics) != 1 || diagnostics[0].Severity() != diag.SeverityWarning {
		t.Fatalf("diagnostics = %v; want one warning", diagnostics)
	}
	if got := diagnostics[0].Summary(); got != "Beta Resource" {
		t.Errorf("warning summary = %q; want Beta Resource", got)
	}
}

func TestSavedQueryResourceCreateUsesEmptyOptionalParameters(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := savedQuerySchema(t)
	model := savedQueryModel(t)
	model.Parameters = types.MapNull(types.StringType)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	request := savedQueryRequest()
	request.Parameters = map[string]string{}
	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateSavedQueryMock.
		Expect(ctx, savedQueryServiceID, request).
		Return(savedQueryResponse(), nil)

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
}
