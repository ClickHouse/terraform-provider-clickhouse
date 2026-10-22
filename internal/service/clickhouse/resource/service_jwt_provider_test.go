package resource

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

const (
	jwtProviderServiceID = "44444444-4444-4444-4444-444444444444"
	jwtProviderName      = "my-oidc-provider"
)

func jwtProviderSchema(t *testing.T) (*ServiceJWTProviderResource, resource.SchemaResponse) {
	t.Helper()
	r := NewServiceJWTProviderResource().(*ServiceJWTProviderResource)
	resp := resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	return r, resp
}

func jwtProviderModel() models.ServiceJWTProviderResourceModel {
	return models.ServiceJWTProviderResourceModel{
		ServiceID:  types.StringValue(jwtProviderServiceID),
		Name:       types.StringValue(jwtProviderName),
		Issuer:     types.StringValue("https://auth.example.com"),
		Audience:   types.StringValue("my-service"),
		JWKSURL:    types.StringValue("https://auth.example.com/.well-known/jwks.json"),
		RolesClaim: types.StringValue("clickhouse:roles"),
	}
}

func jwtProviderResponse() *api.JWTProvider {
	rolesClaim := "clickhouse:roles"
	return &api.JWTProvider{
		Name:       jwtProviderName,
		Issuer:     "https://auth.example.com",
		Audience:   "my-service",
		JWKSURL:    "https://auth.example.com/.well-known/jwks.json",
		RolesClaim: &rolesClaim,
	}
}

func jwtProviderState(t *testing.T, schemaResp resource.SchemaResponse, model models.ServiceJWTProviderResourceModel) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(context.Background(), &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	return state
}

func jwtProviderPlan(t *testing.T, schemaResp resource.SchemaResponse, model models.ServiceJWTProviderResourceModel) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}
	return plan
}

func readJWTProviderState(t *testing.T, state tfsdk.State) models.ServiceJWTProviderResourceModel {
	t.Helper()
	var model models.ServiceJWTProviderResourceModel
	if diags := state.Get(context.Background(), &model); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	return model
}

func jwtProviderStringValidators(t *testing.T, attribute string) []validator.String {
	t.Helper()
	_, resp := jwtProviderSchema(t)
	stringAttribute, ok := resp.Schema.Attributes[attribute].(schema.StringAttribute)
	if !ok {
		t.Fatalf("%s is not a string attribute", attribute)
	}
	return stringAttribute.Validators
}

// jwtProviderValidatorsReject runs every validator of an attribute, the way
// the framework does, so a value passes only if all of them accept it.
func jwtProviderValidatorsReject(validators []validator.String, value string) bool {
	for _, v := range validators {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("attribute"),
			ConfigValue: types.StringValue(value),
		}, resp)
		if resp.Diagnostics.HasError() {
			return true
		}
	}
	return false
}

func TestServiceJWTProviderResourceMetadata(t *testing.T) {
	r := NewServiceJWTProviderResource()
	resp := resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "clickhouse"}, &resp)
	if got, want := resp.TypeName, "clickhouse_service_jwt_provider"; got != want {
		t.Fatalf("type name = %q; want %q", got, want)
	}
}

func TestServiceJWTProviderResourceSchema(t *testing.T) {
	ctx := context.Background()
	_, resp := jwtProviderSchema(t)
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid schema implementation: %v", diags)
	}

	wantAttributes := []string{"service_id", "name", "issuer", "audience", "jwks_url", "roles_claim"}
	if len(resp.Schema.Attributes) != len(wantAttributes) {
		t.Fatalf("attribute count = %d; want %d", len(resp.Schema.Attributes), len(wantAttributes))
	}
	for _, name := range wantAttributes {
		attribute, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("schema missing string attribute %q", name)
		}
		wantRequired := name != "roles_claim"
		if attribute.Required != wantRequired || attribute.Optional == wantRequired || attribute.Computed {
			t.Errorf("%s: required=%t optional=%t computed=%t; want required=%t", name, attribute.Required, attribute.Optional, attribute.Computed, wantRequired)
		}
		wantReplace := name == "service_id" || name == "name"
		if got := len(attribute.PlanModifiers) == 1; got != wantReplace {
			t.Errorf("%s: has RequiresReplace = %t; want %t", name, got, wantReplace)
		}
	}
}

func TestServiceJWTProviderResourceValidators(t *testing.T) {
	tooLong := "https://auth.example.com/" + strings.Repeat("a", 240)
	cases := []struct {
		attribute string
		accept    []string
		reject    []string
	}{
		{
			attribute: "service_id",
			accept:    []string{jwtProviderServiceID},
			reject:    []string{"not-a-uuid"},
		},
		{
			attribute: "name",
			accept:    []string{"my-oidc-provider", "a", "chx", "my-cloud", strings.Repeat("a", 59)},
			reject: []string{
				"", "Upper", "has space", "under_score", strings.Repeat("a", 60),
				"cloud", "clickhouse", "ch", "internal", "system", "default", "clickhouse-prod", "ch-1",
			},
		},
		{
			attribute: "issuer",
			accept:    []string{"https://auth.example.com", "my-issuer"},
			reject:    []string{"", " https://auth.example.com", "has space", strings.Repeat("a", 257)},
		},
		{
			attribute: "audience",
			accept:    []string{"my-service"},
			reject:    []string{"", "my service", "my-service\n"},
		},
		{
			attribute: "jwks_url",
			accept:    []string{"https://auth.example.com/.well-known/jwks.json"},
			reject:    []string{"", "http://auth.example.com/jwks", "https://", "https://auth.example.com/ jwks", tooLong},
		},
		{
			attribute: "roles_claim",
			accept:    []string{"clickhouse:roles", "roles"},
			reject:    []string{"", "sub", "iss", "has space"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.attribute, func(t *testing.T) {
			validators := jwtProviderStringValidators(t, tc.attribute)
			for _, value := range tc.accept {
				if jwtProviderValidatorsReject(validators, value) {
					t.Errorf("rejected %q", value)
				}
			}
			for _, value := range tc.reject {
				if !jwtProviderValidatorsReject(validators, value) {
					t.Errorf("accepted %q", value)
				}
			}
		})
	}
}

func TestServiceJWTProviderResourceCreate(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := jwtProviderSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateJWTProviderMock.
		Expect(ctx, jwtProviderServiceID, *jwtProviderResponse()).
		Return(jwtProviderResponse(), nil)

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: jwtProviderPlan(t, schemaResp, jwtProviderModel())}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}

	if got := readJWTProviderState(t, resp.State); got != jwtProviderModel() {
		t.Errorf("state = %+v; want %+v", got, jwtProviderModel())
	}
}

func TestServiceJWTProviderResourceCreateWithoutRolesClaim(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := jwtProviderSchema(t)

	model := jwtProviderModel()
	model.RolesClaim = types.StringNull()
	request := *jwtProviderResponse()
	request.RolesClaim = nil

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateJWTProviderMock.
		Expect(ctx, jwtProviderServiceID, request).
		Return(&request, nil)

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: jwtProviderPlan(t, schemaResp, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	if got := readJWTProviderState(t, resp.State).RolesClaim; !got.IsNull() {
		t.Errorf("roles_claim = %v; want null", got)
	}
}

func TestServiceJWTProviderResourceCreateErrors(t *testing.T) {
	for _, tc := range []struct {
		name        string
		err         error
		wantSummary string
		wantDetail  string
	}{
		{
			name:        "existing provider points at import",
			err:         errors.New(`status: 409, body: {"error":"CONFLICT: a JWT provider named \"my-oidc-provider\" already exists"}`),
			wantSummary: "JWT provider already exists",
			wantDetail:  "terraform import clickhouse_service_jwt_provider.<name> " + jwtProviderServiceID + ":" + jwtProviderName,
		},
		{
			name:        "missing entitlement explains the tier requirement",
			err:         errors.New(`status: 403, body: {"error":"FORBIDDEN: JWT authentication providers is not available"}`),
			wantSummary: "Error creating JWT provider: forbidden",
			wantDetail:  "Enterprise tier",
		},
		{
			name:        "invalid JWKS keeps the server message",
			err:         errors.New(`status: 400, body: {"error":"BAD_REQUEST: Could not reach the JWKS URL"}`),
			wantSummary: "Error creating JWT provider",
			wantDetail:  "Could not reach the JWKS URL",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r, schemaResp := jwtProviderSchema(t)

			mc := minimock.NewController(t)
			r.client = api.NewClientMock(mc).CreateJWTProviderMock.Return(nil, tc.err)

			resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			r.Create(ctx, resource.CreateRequest{Plan: jwtProviderPlan(t, schemaResp, jwtProviderModel())}, &resp)

			errs := resp.Diagnostics.Errors()
			if len(errs) != 1 {
				t.Fatalf("diagnostics = %v; want one error", resp.Diagnostics)
			}
			if got := errs[0].Summary(); got != tc.wantSummary {
				t.Errorf("summary = %q; want %q", got, tc.wantSummary)
			}
			if got := errs[0].Detail(); !strings.Contains(got, tc.wantDetail) {
				t.Errorf("detail = %q; want it to contain %q", got, tc.wantDetail)
			}
		})
	}
}

func TestServiceJWTProviderResourceReadRefreshesDrift(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := jwtProviderSchema(t)

	response := jwtProviderResponse()
	response.Audience = "changed-outside-terraform"
	response.RolesClaim = nil

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetJWTProviderMock.
		Expect(ctx, jwtProviderServiceID, jwtProviderName).
		Return(response, nil)

	state := jwtProviderState(t, schemaResp, jwtProviderModel())
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	got := readJWTProviderState(t, resp.State)
	if got.Audience.ValueString() != "changed-outside-terraform" || !got.RolesClaim.IsNull() {
		t.Errorf("state = %+v; want refreshed audience and null roles_claim", got)
	}
}

func TestServiceJWTProviderResourceReadRemovesMissingProvider(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := jwtProviderSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetJWTProviderMock.
		Expect(ctx, jwtProviderServiceID, jwtProviderName).
		Return(nil, errors.New("status: 404, body: not found"))

	state := jwtProviderState(t, schemaResp, jwtProviderModel())
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("Read should remove a missing provider from state")
	}
}

func TestServiceJWTProviderResourceUpdate(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := jwtProviderSchema(t)

	model := jwtProviderModel()
	model.Audience = types.StringValue("another-service")
	model.RolesClaim = types.StringNull()
	update := api.JWTProviderUpdate{
		Issuer:   "https://auth.example.com",
		Audience: "another-service",
		JWKSURL:  "https://auth.example.com/.well-known/jwks.json",
	}
	response := &api.JWTProvider{
		Name:     jwtProviderName,
		Issuer:   update.Issuer,
		Audience: update.Audience,
		JWKSURL:  update.JWKSURL,
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		UpdateJWTProviderMock.
		Expect(ctx, jwtProviderServiceID, jwtProviderName, update).
		Return(response, nil)

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: jwtProviderPlan(t, schemaResp, model)}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if got := readJWTProviderState(t, resp.State); got != model {
		t.Errorf("state = %+v; want %+v", got, model)
	}
}

func TestServiceJWTProviderResourceDelete(t *testing.T) {
	for _, tc := range []struct {
		name      string
		err       error
		wantError bool
	}{
		{name: "deletes", err: nil},
		{name: "ignores an already deleted provider", err: errors.New("status: 404, body: not found")},
		{name: "reports other errors", err: errors.New("status: 500, body: internal server error"), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r, schemaResp := jwtProviderSchema(t)

			mc := minimock.NewController(t)
			r.client = api.NewClientMock(mc).
				DeleteJWTProviderMock.
				Expect(ctx, jwtProviderServiceID, jwtProviderName).
				Return(tc.err)

			state := jwtProviderState(t, schemaResp, jwtProviderModel())
			resp := resource.DeleteResponse{State: state}
			r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
			if got := resp.Diagnostics.HasError(); got != tc.wantError {
				t.Errorf("has error = %t; want %t: %v", got, tc.wantError, resp.Diagnostics)
			}
		})
	}
}

func TestServiceJWTProviderResourceImport(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := jwtProviderSchema(t)

	resp := resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	r.ImportState(ctx, resource.ImportStateRequest{ID: jwtProviderServiceID + ":" + jwtProviderName}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diagnostics: %v", resp.Diagnostics)
	}

	for attribute, want := range map[string]string{
		"service_id": jwtProviderServiceID,
		"name":       jwtProviderName,
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

func TestServiceJWTProviderResourceImportRejectsInvalidID(t *testing.T) {
	for _, id := range []string{
		jwtProviderName,
		jwtProviderServiceID,
		jwtProviderServiceID + "/" + jwtProviderName,
		"not-a-uuid:" + jwtProviderName,
		jwtProviderServiceID + ":Not_Valid",
		jwtProviderServiceID + ":" + jwtProviderName + ":extra",
	} {
		t.Run(id, func(t *testing.T) {
			ctx := context.Background()
			r, schemaResp := jwtProviderSchema(t)

			resp := resource.ImportStateResponse{
				State: tfsdk.State{
					Schema: schemaResp.Schema,
					Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
				},
			}
			r.ImportState(ctx, resource.ImportStateRequest{ID: id}, &resp)
			if got := resp.Diagnostics.Errors(); len(got) != 1 || got[0].Summary() != "Invalid JWT provider import ID" {
				t.Errorf("diagnostics = %v; want one invalid import ID error", resp.Diagnostics)
			}
		})
	}
}
