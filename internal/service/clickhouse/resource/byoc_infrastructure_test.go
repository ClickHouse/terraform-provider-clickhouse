package resource

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

const byocInfraID = "44444444-4444-4444-4444-444444444444"

func byocInfraSchema(t *testing.T) (*ByocInfrastructureResource, resource.SchemaResponse) {
	t.Helper()
	r := NewByocInfrastructureResource().(*ByocInfrastructureResource)
	resp := resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	return r, resp
}

func byocInfraModel() models.ByocInfrastructureResourceModel {
	return models.ByocInfrastructureResourceModel{
		RegionID:                 types.StringValue("us-east-1"),
		AccountID:                types.StringValue("123456789012"),
		ExternalID:               types.StringValue("external-id-1"),
		AvailabilityZoneSuffixes: types.ListNull(types.StringType),
		PrivateSubnetIDs:         types.ListNull(types.StringType),
		PublicSubnetIDs:          types.ListNull(types.StringType),
		GcpPodCidrRangeNames:     types.ListNull(types.StringType),
		Tags:                     types.MapNull(types.StringType),
	}
}

func byocInfraCreateRequest() api.ByocInfrastructureCreateRequest {
	return api.ByocInfrastructureCreateRequest{
		RegionId:   "us-east-1",
		AccountId:  "123456789012",
		ExternalId: strPtr("external-id-1"),
	}
}

func byocInfraDetails(state string) *api.ByocInfrastructureDetails {
	return &api.ByocInfrastructureDetails{
		Id:                        byocInfraID,
		State:                     state,
		AccountId:                 "123456789012",
		RegionId:                  "us-east-1",
		CloudProvider:             "aws",
		DisplayName:               "byoc-us-east-1",
		EnablePrivateLink:         boolPtr(false),
		EnablePrivateLoadBalancer: boolPtr(false),
		EnablePublicLoadBalancer:  boolPtr(true),
		VpcCidrRange:              strPtr("10.0.0.0/16"),
		IsByoVpc:                  boolPtr(false),
	}
}

func passingByocValidation() *api.ByocInfrastructureValidation {
	return &api.ByocInfrastructureValidation{
		CloudProvider: "aws",
		Supported:     true,
		AllPassed:     true,
		AnyPassed:     true,
	}
}

func TestByocInfrastructureResourceMetadata(t *testing.T) {
	r := NewByocInfrastructureResource()
	resp := resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "clickhouse"}, &resp)
	if got, want := resp.TypeName, "clickhouse_byoc_infrastructure"; got != want {
		t.Fatalf("type name = %q; want %q", got, want)
	}
}

func TestByocInfrastructureResourceSchema(t *testing.T) {
	ctx := context.Background()
	_, resp := byocInfraSchema(t)
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid schema implementation: %v", diags)
	}
}

func TestByocInfrastructureResourceCreate(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "false")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	model := byocInfraModel()
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	waitCalled := false
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.ValidateByocInfrastructureMock.
		Expect(ctx, byocValidateRequestFromCreate(byocInfraCreateRequest())).
		Return(passingByocValidation(), nil)
	client.CreateByocInfrastructureMock.
		Expect(ctx, byocInfraCreateRequest()).
		Return(&api.ByocInfrastructure{Id: byocInfraID, State: api.ByocStateProvisioning}, nil)
	client.GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(byocInfraDetails(api.ByocStateReady), nil)
	client.GetByocInfrastructureTagsMock.
		Expect(ctx, byocInfraID).
		Return(map[string]string{}, nil)
	client.WaitForByocInfrastructureStateMock.
		Set(func(_ context.Context, byocId string, stateChecker func(string) bool, maxWaitSeconds int) error {
			waitCalled = true
			if byocId != byocInfraID {
				t.Errorf("wait byocId = %q; want %q", byocId, byocInfraID)
			}
			if maxWaitSeconds != byocCreateWaitSeconds {
				t.Errorf("maxWaitSeconds = %d; want %d", maxWaitSeconds, byocCreateWaitSeconds)
			}
			if stateChecker(api.ByocStateProvisioning) {
				t.Error("stateChecker should keep waiting while provisioning")
			}
			if !stateChecker(api.ByocStateReady) {
				t.Error("stateChecker should accept the ready state")
			}
			return nil
		})
	r.client = client

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	assertBetaWarning(t, resp.Diagnostics)
	if !waitCalled {
		t.Error("Create did not wait for the infrastructure to provision")
	}

	var state models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.ID.ValueString() != byocInfraID {
		t.Errorf("id = %q; want %q", state.ID.ValueString(), byocInfraID)
	}
	if state.State.ValueString() != api.ByocStateReady {
		t.Errorf("state = %q; want %q", state.State.ValueString(), api.ByocStateReady)
	}
	if state.CloudProvider.ValueString() != "aws" {
		t.Errorf("cloud_provider = %q; want aws", state.CloudProvider.ValueString())
	}
	if state.ExternalID.ValueString() != "external-id-1" {
		t.Errorf("external_id = %q; want configured value kept", state.ExternalID.ValueString())
	}
	if !state.EnablePublicLoadBalancer.ValueBool() {
		t.Error("enable_public_load_balancer should be synced from the read-back")
	}
	if state.Tags.IsNull() || len(state.Tags.Elements()) != 0 {
		t.Errorf("tags = %v; want known empty map", state.Tags)
	}
}

func TestByocInfrastructureResourceCreateFailsPreflight(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	model := byocInfraModel()
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		ValidateByocInfrastructureMock.
		Expect(ctx, byocValidateRequestFromCreate(byocInfraCreateRequest())).
		Return(&api.ByocInfrastructureValidation{
			CloudProvider: "aws",
			Supported:     true,
			AllPassed:     false,
			AnyPassed:     true,
			Checks: []api.ByocInfrastructureValidationCheck{
				{Name: "create-vpc", Action: "ec2:CreateVpc", Allowed: true},
				{Name: "create-role", Action: "iam:CreateRole", Allowed: false, Reason: "explicit deny in SCP"},
			},
		}, nil)

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw}}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create should fail when preflight checks fail")
	}
	got := resp.Diagnostics.Errors()[0]
	if got.Summary() != "BYOC preflight validation failed" {
		t.Errorf("diagnostic summary = %q", got.Summary())
	}
	for _, want := range []string{"create-role", "iam:CreateRole", "explicit deny in SCP", "skip_preflight_validation"} {
		if !strings.Contains(got.Detail(), want) {
			t.Errorf("diagnostic detail %q missing %q", got.Detail(), want)
		}
	}
	if strings.Contains(got.Detail(), "create-vpc") {
		t.Errorf("diagnostic detail should not list passing checks: %q", got.Detail())
	}
}

func TestByocInfrastructureResourceCreateWarnsWhenValidationUnsupported(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	model := byocInfraModel()
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.ValidateByocInfrastructureMock.
		Expect(ctx, byocValidateRequestFromCreate(byocInfraCreateRequest())).
		Return(&api.ByocInfrastructureValidation{CloudProvider: "azure", Supported: false}, nil)
	client.CreateByocInfrastructureMock.
		Expect(ctx, byocInfraCreateRequest()).
		Return(nil, errors.New("status: 400, body: not onboarded"))
	r.client = client

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw}}, &resp)
	warnings := resp.Diagnostics.Warnings()
	if len(warnings) != 1 || warnings[0].Summary() != "BYOC preflight validation not supported" {
		t.Errorf("warnings = %v; want the unsupported-validation warning", warnings)
	}
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create should surface the create error")
	}
}

func TestByocInfrastructureResourceCreateSkipsPreflight(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	model := byocInfraModel()
	model.SkipPreflightValidation = types.BoolValue(true)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateByocInfrastructureMock.
		Expect(ctx, byocInfraCreateRequest()).
		Return(nil, errors.New("status: 400, body: not onboarded"))

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw}}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create should surface the create error")
	}
	if got := resp.Diagnostics.Errors()[0].Summary(); got != "Error creating BYOC infrastructure" {
		t.Errorf("diagnostic summary = %q", got)
	}
}

func byocInfraConfig(t *testing.T, schemaResp resource.SchemaResponse, model *models.ByocInfrastructureResourceModel) tfsdk.Config {
	t.Helper()
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatalf("set config: %v", diags)
	}
	return tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw}
}

func byocInfraState(t *testing.T, schemaResp resource.SchemaResponse) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	model := byocInfraModel()
	model.ID = types.StringValue(byocInfraID)
	model.State = types.StringValue(api.ByocStateReady)
	model.CloudProvider = types.StringValue("aws")
	model.DisplayName = types.StringValue("byoc-us-east-1")
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	return state
}

func TestByocInfrastructureResourceRead(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(byocInfraDetails(api.ByocStateDegraded), nil)
	client.GetByocInfrastructureTagsMock.
		Expect(ctx, byocInfraID).
		Return(map[string]string{"env": "prod"}, nil)
	r.client = client

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.State.ValueString() != api.ByocStateDegraded {
		t.Errorf("state = %q; want %q", got.State.ValueString(), api.ByocStateDegraded)
	}
	if got.ExternalID.ValueString() != "external-id-1" {
		t.Error("read must not clear write-only creation parameters")
	}
	if len(got.Tags.Elements()) != 1 {
		t.Errorf("tags = %v; want one entry", got.Tags)
	}
}

func TestByocInfrastructureResourceReadRemovesMissingInfra(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
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

func TestByocInfrastructureResourceReadRemovesTerminatedInfra(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(byocInfraDetails(api.ByocStateTerminated), nil)

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Error("expected terminated infrastructure to be removed from state")
	}
}

func TestByocInfrastructureResourceUpdate(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	model := byocInfraModel()
	model.ID = types.StringValue(byocInfraID)
	model.State = types.StringValue(api.ByocStateReady)
	model.CloudProvider = types.StringValue("aws")
	model.DisplayName = types.StringValue("renamed-byoc")
	model.EnablePrivateLink = types.BoolValue(true)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	details := byocInfraDetails(api.ByocStateReady)
	details.DisplayName = "renamed-byoc"
	details.EnablePrivateLink = boolPtr(true)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.UpdateByocInfrastructureMock.
		Expect(ctx, byocInfraID, api.ByocInfrastructureUpdateRequest{
			DisplayName:       strPtr("renamed-byoc"),
			EnablePrivateLink: boolPtr(true),
		}).
		Return(&api.ByocInfrastructure{Id: byocInfraID, State: api.ByocStateReady}, nil)
	client.GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(details, nil)
	client.GetByocInfrastructureTagsMock.
		Expect(ctx, byocInfraID).
		Return(map[string]string{}, nil)
	r.client = client

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.DisplayName.ValueString() != "renamed-byoc" {
		t.Errorf("display_name = %q; want renamed-byoc", got.DisplayName.ValueString())
	}
	if !got.EnablePrivateLink.ValueBool() {
		t.Error("enable_private_link should be true after update")
	}
}

func TestByocInfrastructureResourceDeleteWaitsForTermination(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.DeleteByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(nil)
	client.WaitForByocInfrastructureStateMock.
		Set(func(_ context.Context, byocId string, stateChecker func(string) bool, maxWaitSeconds int) error {
			if byocId != byocInfraID {
				t.Errorf("wait byocId = %q; want %q", byocId, byocInfraID)
			}
			if maxWaitSeconds != byocDeleteWaitSeconds {
				t.Errorf("maxWaitSeconds = %d; want %d", maxWaitSeconds, byocDeleteWaitSeconds)
			}
			if stateChecker(api.ByocStateTerminating) {
				t.Error("stateChecker should keep waiting while terminating")
			}
			if !stateChecker(api.ByocStateTerminated) {
				t.Error("stateChecker should accept the terminated state")
			}
			// The infra record often disappears before reaching the
			// terminated state; the resource must treat that as success.
			return errors.New("status: 404, body: not found")
		})
	r.client = client

	resp := resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %v", resp.Diagnostics)
	}
}

func TestByocInfrastructureResourceDeleteIgnoresNotFound(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		DeleteByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(errors.New("status: 404, body: not found"))

	resp := resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %v", resp.Diagnostics)
	}
}

func TestByocInfrastructureResourceImport(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "false")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	resp := resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	r.ImportState(ctx, resource.ImportStateRequest{ID: byocInfraID}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diagnostics: %v", resp.Diagnostics)
	}
	assertBetaWarning(t, resp.Diagnostics)

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.ID.ValueString() != byocInfraID {
		t.Errorf("id = %q; want %q", got.ID.ValueString(), byocInfraID)
	}
}

func TestByocPostCreatePatchForcesPrivateLinkWithPscSubnet(t *testing.T) {
	config := models.ByocInfrastructureResourceModel{
		GcpPscSubnetID: types.StringValue("psc-subnet"),
	}
	details := byocInfraDetails(api.ByocStateReady)

	patch, hasPatch := byocPostCreatePatch(&config, details)
	if !hasPatch {
		t.Fatal("expected a patch for the configured PSC subnet")
	}
	if patch.GcpPscSubnetId == nil || *patch.GcpPscSubnetId != "psc-subnet" {
		t.Errorf("gcpPscSubnetId = %v; want psc-subnet", patch.GcpPscSubnetId)
	}
	if patch.EnablePrivateLink == nil || !*patch.EnablePrivateLink {
		t.Error("enablePrivateLink must accompany gcpPscSubnetId")
	}
}

func TestByocUpdateRequestForcesPrivateLinkWithPscSubnet(t *testing.T) {
	// The private-link toggle in the plan is known-false via
	// UseStateForUnknown when omitted from config; the PSC subnet patch must
	// still carry enablePrivateLink = true, which ValidateConfig guarantees
	// the configuration agrees with.
	state := byocInfraModel()
	state.EnablePrivateLink = types.BoolValue(false)
	plan := byocInfraModel()
	plan.EnablePrivateLink = types.BoolValue(false)
	plan.GcpPscSubnetID = types.StringValue("psc-subnet")

	patch, hasPatch, diags := byocUpdateRequestFromModels(context.Background(), &plan, &state)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if !hasPatch {
		t.Fatal("expected a patch for the added PSC subnet")
	}
	if patch.EnablePrivateLink == nil || !*patch.EnablePrivateLink {
		t.Error("enablePrivateLink must be true alongside gcpPscSubnetId")
	}
}

func TestByocUpdateRequestEmptyTagsClearsServerTags(t *testing.T) {
	state := byocInfraModel()
	state.Tags = types.MapValueMust(types.StringType, map[string]attr.Value{"env": types.StringValue("prod")})
	plan := byocInfraModel()
	plan.Tags = types.MapValueMust(types.StringType, map[string]attr.Value{})

	patch, hasPatch, diags := byocUpdateRequestFromModels(context.Background(), &plan, &state)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if !hasPatch {
		t.Fatal("expected a patch for the emptied tags")
	}
	if patch.Tags == nil || len(*patch.Tags) != 0 {
		t.Errorf("tags = %v; want pointer to empty map", patch.Tags)
	}
}

func TestByocInfrastructureResourceValidateConfigRejectsPscSubnetWithoutPrivateLink(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)

	for name, enablePrivateLink := range map[string]types.Bool{
		"omitted":        types.BoolNull(),
		"explicit false": types.BoolValue(false),
	} {
		t.Run(name, func(t *testing.T) {
			model := byocInfraModel()
			model.GcpPscSubnetID = types.StringValue("psc-subnet")
			model.EnablePrivateLink = enablePrivateLink
			cfg := byocInfraConfig(t, schemaResp, &model)

			resp := resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: cfg}, &resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("ValidateConfig should reject gcp_psc_subnet_id without enable_private_link = true")
			}
		})
	}

	model := byocInfraModel()
	model.GcpPscSubnetID = types.StringValue("psc-subnet")
	model.EnablePrivateLink = types.BoolValue(true)
	cfg := byocInfraConfig(t, schemaResp, &model)
	resp := resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: cfg}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ValidateConfig diagnostics: %v", resp.Diagnostics)
	}
}

func TestByocWriteOnlyAttributesAdoptedOverNullState(t *testing.T) {
	ctx := context.Background()
	_, schemaResp := byocInfraSchema(t)
	raw := byocInfraState(t, schemaResp).Raw
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: raw}
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw}

	stringModifier := byocStringRequiresReplaceUnlessAdopted()

	adoptReq := planmodifier.StringRequest{
		State:      state,
		Plan:       plan,
		StateValue: types.StringNull(),
		PlanValue:  types.StringValue("external-id-1"),
	}
	adoptResp := &planmodifier.StringResponse{PlanValue: adoptReq.PlanValue}
	stringModifier.PlanModifyString(ctx, adoptReq, adoptResp)
	if adoptResp.RequiresReplace {
		t.Error("configuring a value over a null (imported) state must not require replacement")
	}

	changeReq := planmodifier.StringRequest{
		State:      state,
		Plan:       plan,
		StateValue: types.StringValue("old"),
		PlanValue:  types.StringValue("new"),
	}
	changeResp := &planmodifier.StringResponse{PlanValue: changeReq.PlanValue}
	stringModifier.PlanModifyString(ctx, changeReq, changeResp)
	if !changeResp.RequiresReplace {
		t.Error("changing a previously recorded value must require replacement")
	}

	listModifier := byocListRequiresReplaceUnlessAdopted()
	listAdoptReq := planmodifier.ListRequest{
		State:      state,
		Plan:       plan,
		StateValue: types.ListNull(types.StringType),
		PlanValue:  types.ListValueMust(types.StringType, []attr.Value{types.StringValue("subnet-1")}),
	}
	listAdoptResp := &planmodifier.ListResponse{PlanValue: listAdoptReq.PlanValue}
	listModifier.PlanModifyList(ctx, listAdoptReq, listAdoptResp)
	if listAdoptResp.RequiresReplace {
		t.Error("configuring a list over a null (imported) state must not require replacement")
	}
}

func TestByocPostCreatePatchNoopWhenNothingConfigured(t *testing.T) {
	config := models.ByocInfrastructureResourceModel{}
	_, hasPatch := byocPostCreatePatch(&config, byocInfraDetails(api.ByocStateReady))
	if hasPatch {
		t.Error("expected no patch when no toggles are configured")
	}
}
