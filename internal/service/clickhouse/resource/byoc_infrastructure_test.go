package resource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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

func TestByocInfrastructureResourceVpcCidrConflictsWithByoVpc(t *testing.T) {
	ctx := context.Background()
	_, schemaResp := byocInfraSchema(t)
	attrSchema, ok := schemaResp.Schema.Attributes["vpc_cidr_range"].(schema.StringAttribute)
	if !ok {
		t.Fatal("vpc_cidr_range is not a string attribute")
	}

	validate := func(t *testing.T, model models.ByocInfrastructureResourceModel) bool {
		t.Helper()
		cfg := tfsdk.Plan{Schema: schemaResp.Schema}
		if diags := cfg.Set(ctx, &model); diags.HasError() {
			t.Fatalf("set config: %v", diags)
		}
		req := validator.StringRequest{
			Path:        path.Root("vpc_cidr_range"),
			ConfigValue: model.VpcCidrRange,
			Config:      tfsdk.Config{Schema: schemaResp.Schema, Raw: cfg.Raw},
		}
		resp := &validator.StringResponse{}
		for _, v := range attrSchema.Validators {
			v.ValidateString(ctx, req, resp)
		}
		return resp.Diagnostics.HasError()
	}

	managedOnly := byocInfraModel()
	managedOnly.VpcCidrRange = types.StringValue("10.0.0.0/16")
	if validate(t, managedOnly) {
		t.Error("vpc_cidr_range alone must pass validation")
	}

	byoVpcVariants := map[string]func(*models.ByocInfrastructureResourceModel){
		"vpc_id": func(m *models.ByocInfrastructureResourceModel) {
			m.VpcID = types.StringValue("vpc-123")
		},
		"private_subnet_ids": func(m *models.ByocInfrastructureResourceModel) {
			m.PrivateSubnetIDs = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("subnet-1")})
		},
		"public_subnet_ids": func(m *models.ByocInfrastructureResourceModel) {
			m.PublicSubnetIDs = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("subnet-2")})
		},
		"gcp_pod_cidr_range_names": func(m *models.ByocInfrastructureResourceModel) {
			m.GcpPodCidrRangeNames = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("pods")})
		},
		"gcp_shared_vpc_host_project_id": func(m *models.ByocInfrastructureResourceModel) {
			m.GcpSharedVpcHostProjectID = types.StringValue("host-project")
		},
	}
	for attrName, set := range byoVpcVariants {
		t.Run(attrName, func(t *testing.T) {
			model := byocInfraModel()
			model.VpcCidrRange = types.StringValue("10.0.0.0/16")
			set(&model)
			if !validate(t, model) {
				t.Errorf("vpc_cidr_range together with %s must fail validation", attrName)
			}
		})
	}
}

func TestByocInfrastructureResourceTagsLimitedTo50(t *testing.T) {
	ctx := context.Background()
	_, schemaResp := byocInfraSchema(t)
	attrSchema, ok := schemaResp.Schema.Attributes["tags"].(schema.MapAttribute)
	if !ok {
		t.Fatal("tags is not a map attribute")
	}

	validate := func(t *testing.T, size int) bool {
		t.Helper()
		elems := map[string]attr.Value{}
		for i := 0; i < size; i++ {
			elems[fmt.Sprintf("key-%d", i)] = types.StringValue("value")
		}
		model := byocInfraModel()
		model.Tags = types.MapValueMust(types.StringType, elems)
		cfg := tfsdk.Plan{Schema: schemaResp.Schema}
		if diags := cfg.Set(ctx, &model); diags.HasError() {
			t.Fatalf("set config: %v", diags)
		}
		req := validator.MapRequest{
			Path:        path.Root("tags"),
			ConfigValue: model.Tags,
			Config:      tfsdk.Config{Schema: schemaResp.Schema, Raw: cfg.Raw},
		}
		resp := &validator.MapResponse{}
		for _, v := range attrSchema.Validators {
			v.ValidateMap(ctx, req, resp)
		}
		return resp.Diagnostics.HasError()
	}

	if validate(t, 50) {
		t.Error("50 tags must pass validation")
	}
	if !validate(t, 51) {
		t.Error("51 tags must fail validation")
	}
}

func byocRunStringModifier(t *testing.T, args byocStringModifierArgs) types.String {
	t.Helper()
	ctx := context.Background()
	_, schemaResp := byocInfraSchema(t)

	stateData := tfsdk.State{Schema: schemaResp.Schema}
	if diags := stateData.Set(ctx, &args.stateModel); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	configData := tfsdk.State{Schema: schemaResp.Schema}
	if diags := configData.Set(ctx, &args.configModel); diags.HasError() {
		t.Fatalf("set config: %v", diags)
	}

	req := planmodifier.StringRequest{
		Path:        args.path,
		Config:      tfsdk.Config{Schema: schemaResp.Schema, Raw: configData.Raw},
		ConfigValue: types.StringNull(),
		Plan:        tfsdk.Plan{Schema: schemaResp.Schema, Raw: configData.Raw},
		PlanValue:   types.StringUnknown(),
		State:       stateData,
		StateValue:  args.stateValue,
	}
	resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
	args.modifier.PlanModifyString(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("modifier diagnostics: %v", resp.Diagnostics)
	}
	return resp.PlanValue
}

type byocStringModifierArgs struct {
	modifier    planmodifier.String
	path        path.Path
	stateValue  types.String
	stateModel  models.ByocInfrastructureResourceModel
	configModel models.ByocInfrastructureResourceModel
}

func TestByocVpcCidrUseStateForUnknownModifier(t *testing.T) {
	stateModel := byocInfraModel()
	stateModel.ID = types.StringValue(byocInfraID)
	stateModel.State = types.StringValue(api.ByocStateReady)
	stateModel.CloudProvider = types.StringValue("aws")
	stateModel.VpcCidrRange = types.StringValue("10.0.0.0/16")

	run := func(t *testing.T, configModel models.ByocInfrastructureResourceModel) types.String {
		t.Helper()
		return byocRunStringModifier(t, byocStringModifierArgs{
			modifier:    byocVpcCidrUseStateForUnknown(),
			path:        path.Root("vpc_cidr_range"),
			stateValue:  types.StringValue("10.0.0.0/16"),
			stateModel:  stateModel,
			configModel: configModel,
		})
	}

	t.Run("managed VPC keeps the prior CIDR", func(t *testing.T) {
		if got := run(t, byocInfraModel()); got.ValueString() != "10.0.0.0/16" {
			t.Errorf("plan value = %v; want the prior state CIDR", got)
		}
	})

	byoVpcVariants := map[string]func(*models.ByocInfrastructureResourceModel){
		"vpc_id": func(m *models.ByocInfrastructureResourceModel) {
			m.VpcID = types.StringValue("vpc-123")
		},
		"private_subnet_ids": func(m *models.ByocInfrastructureResourceModel) {
			m.PrivateSubnetIDs = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("subnet-1")})
		},
		"public_subnet_ids": func(m *models.ByocInfrastructureResourceModel) {
			m.PublicSubnetIDs = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("subnet-2")})
		},
		"gcp_pod_cidr_range_names": func(m *models.ByocInfrastructureResourceModel) {
			m.GcpPodCidrRangeNames = types.ListValueMust(types.StringType, []attr.Value{types.StringValue("pods")})
		},
		"gcp_shared_vpc_host_project_id": func(m *models.ByocInfrastructureResourceModel) {
			m.GcpSharedVpcHostProjectID = types.StringValue("host-project")
		},
	}
	for attrName, set := range byoVpcVariants {
		t.Run(attrName+" leaves the CIDR unknown", func(t *testing.T) {
			configModel := byocInfraModel()
			set(&configModel)
			if got := run(t, configModel); !got.IsUnknown() {
				t.Errorf("plan value = %v; the recorded CIDR must not leak into a BYO-VPC create", got)
			}
		})
	}
}

func byocRunListModifier(t *testing.T, args byocListModifierArgs) types.List {
	t.Helper()
	ctx := context.Background()
	_, schemaResp := byocInfraSchema(t)

	stateData := tfsdk.State{Schema: schemaResp.Schema}
	if diags := stateData.Set(ctx, &args.stateModel); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	configData := tfsdk.State{Schema: schemaResp.Schema}
	if diags := configData.Set(ctx, &args.configModel); diags.HasError() {
		t.Fatalf("set config: %v", diags)
	}

	req := planmodifier.ListRequest{
		Path:        args.path,
		Config:      tfsdk.Config{Schema: schemaResp.Schema, Raw: configData.Raw},
		ConfigValue: types.ListNull(types.StringType),
		Plan:        tfsdk.Plan{Schema: schemaResp.Schema, Raw: configData.Raw},
		PlanValue:   types.ListUnknown(types.StringType),
		State:       stateData,
		StateValue:  args.stateValue,
	}
	resp := &planmodifier.ListResponse{PlanValue: req.PlanValue}
	args.modifier.PlanModifyList(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("modifier diagnostics: %v", resp.Diagnostics)
	}
	return resp.PlanValue
}

type byocListModifierArgs struct {
	modifier    planmodifier.List
	path        path.Path
	stateValue  types.List
	stateModel  models.ByocInfrastructureResourceModel
	configModel models.ByocInfrastructureResourceModel
}

func TestByocByoVpcUseStateForUnknownModifier(t *testing.T) {
	subnets := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("subnet-1")})

	stateModel := byocInfraModel()
	stateModel.ID = types.StringValue(byocInfraID)
	stateModel.State = types.StringValue(api.ByocStateReady)
	stateModel.CloudProvider = types.StringValue("aws")
	stateModel.VpcID = types.StringValue("vpc-123")
	stateModel.PrivateSubnetIDs = subnets

	runString := func(t *testing.T, configModel models.ByocInfrastructureResourceModel) types.String {
		t.Helper()
		return byocRunStringModifier(t, byocStringModifierArgs{
			modifier:    byocByoVpcStringUseStateForUnknown(),
			path:        path.Root("vpc_id"),
			stateValue:  types.StringValue("vpc-123"),
			stateModel:  stateModel,
			configModel: configModel,
		})
	}
	runList := func(t *testing.T, configModel models.ByocInfrastructureResourceModel) types.List {
		t.Helper()
		return byocRunListModifier(t, byocListModifierArgs{
			modifier:    byocByoVpcListUseStateForUnknown(),
			path:        path.Root("private_subnet_ids"),
			stateValue:  subnets,
			stateModel:  stateModel,
			configModel: configModel,
		})
	}

	t.Run("unchanged topology keeps the prior values", func(t *testing.T) {
		if got := runString(t, byocInfraModel()); got.ValueString() != "vpc-123" {
			t.Errorf("vpc_id plan = %v; want the prior state value", got)
		}
		if got := runList(t, byocInfraModel()); !got.Equal(subnets) {
			t.Errorf("private_subnet_ids plan = %v; want the prior state value", got)
		}
	})

	t.Run("configured vpc_cidr_range leaves the BYO fields unknown", func(t *testing.T) {
		configModel := byocInfraModel()
		configModel.VpcCidrRange = types.StringValue("10.0.0.0/16")
		if got := runString(t, configModel); !got.IsUnknown() {
			t.Errorf("vpc_id plan = %v; stale BYO wiring must not leak into a managed-VPC create", got)
		}
		if got := runList(t, configModel); !got.IsUnknown() {
			t.Errorf("private_subnet_ids plan = %v; stale BYO wiring must not leak into a managed-VPC create", got)
		}
	})

	t.Run("changed region leaves the BYO fields unknown", func(t *testing.T) {
		configModel := byocInfraModel()
		configModel.RegionID = types.StringValue("eu-west-1")
		if got := runString(t, configModel); !got.IsUnknown() {
			t.Errorf("vpc_id plan = %v; region-scoped wiring must not survive a region change", got)
		}
		if got := runList(t, configModel); !got.IsUnknown() {
			t.Errorf("private_subnet_ids plan = %v; region-scoped wiring must not survive a region change", got)
		}
	})

	t.Run("changed account leaves the BYO fields unknown", func(t *testing.T) {
		configModel := byocInfraModel()
		configModel.AccountID = types.StringValue("210987654321")
		if got := runString(t, configModel); !got.IsUnknown() {
			t.Errorf("vpc_id plan = %v; account-scoped wiring must not survive an account change", got)
		}
		if got := runList(t, configModel); !got.IsUnknown() {
			t.Errorf("private_subnet_ids plan = %v; account-scoped wiring must not survive an account change", got)
		}
	})

	t.Run("different configured vpc_id leaves dependent fields unknown", func(t *testing.T) {
		configModel := byocInfraModel()
		configModel.VpcID = types.StringValue("vpc-456")
		if got := runList(t, configModel); !got.IsUnknown() {
			t.Errorf("private_subnet_ids plan = %v; subnets of the old VPC must not be restored for a new vpc_id", got)
		}
	})
}

func TestByocPscSubnetUseStateForUnknownModifier(t *testing.T) {
	stateModel := byocInfraModel()
	stateModel.ID = types.StringValue(byocInfraID)
	stateModel.State = types.StringValue(api.ByocStateReady)
	stateModel.CloudProvider = types.StringValue("gcp")
	stateModel.EnablePrivateLink = types.BoolValue(true)
	stateModel.GcpPscSubnetID = types.StringValue("psc-subnet")

	run := func(t *testing.T, configModel models.ByocInfrastructureResourceModel) types.String {
		t.Helper()
		return byocRunStringModifier(t, byocStringModifierArgs{
			modifier:    byocPscSubnetUseStateForUnknown(),
			path:        path.Root("gcp_psc_subnet_id"),
			stateValue:  types.StringValue("psc-subnet"),
			stateModel:  stateModel,
			configModel: configModel,
		})
	}

	t.Run("disabling private link plans the subnet null", func(t *testing.T) {
		configModel := byocInfraModel()
		configModel.EnablePrivateLink = types.BoolValue(false)
		if got := run(t, configModel); !got.IsNull() {
			t.Errorf("plan value = %v; want null because the API clears the subnet", got)
		}
	})

	t.Run("unset private link keeps the prior subnet", func(t *testing.T) {
		if got := run(t, byocInfraModel()); got.ValueString() != "psc-subnet" {
			t.Errorf("plan value = %v; want the prior state subnet", got)
		}
	})

	t.Run("enabled private link keeps the prior subnet", func(t *testing.T) {
		configModel := byocInfraModel()
		configModel.EnablePrivateLink = types.BoolValue(true)
		if got := run(t, configModel); got.ValueString() != "psc-subnet" {
			t.Errorf("plan value = %v; want the prior state subnet", got)
		}
	})
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

func TestByocInfrastructureResourceCreateKeepsPostWaitStateWhenRefreshFails(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	model := byocInfraModel()
	model.EnablePrivateLink = types.BoolValue(true)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	getCalls := 0
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.ValidateByocInfrastructureMock.
		Return(passingByocValidation(), nil)
	client.CreateByocInfrastructureMock.
		Return(&api.ByocInfrastructure{Id: byocInfraID, State: api.ByocStateProvisioning}, nil)
	client.GetByocInfrastructureMock.
		Set(func(_ context.Context, byocId string) (*api.ByocInfrastructureDetails, error) {
			getCalls++
			switch getCalls {
			case 1:
				return byocInfraDetails(api.ByocStateProvisioning), nil
			case 2:
				return byocInfraDetails(api.ByocStateReady), nil
			default:
				return nil, errors.New("status: 500, body: transient")
			}
		})
	client.GetByocInfrastructureTagsMock.
		Return(map[string]string{}, nil)
	client.WaitForByocInfrastructureStateMock.
		Return(nil)
	client.UpdateByocInfrastructureMock.
		Expect(ctx, byocInfraID, api.ByocInfrastructureUpdateRequest{
			EnablePrivateLink: boolPtr(true),
		}).
		Return(&api.ByocInfrastructure{Id: byocInfraID, State: api.ByocStateReady}, nil)
	r.client = client

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	if getCalls != 3 {
		t.Fatalf("GetByocInfrastructure calls = %d; want 3", getCalls)
	}

	var state models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.State.ValueString() != api.ByocStateReady {
		t.Errorf("state = %q; want %q despite the failed refresh", state.State.ValueString(), api.ByocStateReady)
	}
	if !state.EnablePrivateLink.ValueBool() {
		t.Error("enable_private_link must record the applied patch value despite the failed refresh")
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

func TestByocInfrastructureResourceReadKeepsTerminatingInfra(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(byocInfraDetails(api.ByocStateTerminating), nil)
	client.GetByocInfrastructureTagsMock.
		Expect(ctx, byocInfraID).
		Return(map[string]string{}, nil)
	r.client = client

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("terminating infrastructure must stay in state until it is gone")
	}

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.State.ValueString() != api.ByocStateTerminating {
		t.Errorf("state = %q; want %q", got.State.ValueString(), api.ByocStateTerminating)
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

func TestByocInfrastructureResourceUpdateKeepsPlanWhenRefreshFails(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	model := byocInfraModel()
	model.ID = types.StringValue(byocInfraID)
	// state has no UseStateForUnknown, so real update plans carry it unknown.
	model.State = types.StringUnknown()
	model.CloudProvider = types.StringValue("aws")
	model.DisplayName = types.StringValue("renamed-byoc")
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.UpdateByocInfrastructureMock.
		Expect(ctx, byocInfraID, api.ByocInfrastructureUpdateRequest{
			DisplayName: strPtr("renamed-byoc"),
		}).
		Return(&api.ByocInfrastructure{Id: byocInfraID, State: api.ByocStateReady}, nil)
	client.GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(nil, errors.New("status: 500, body: transient"))
	r.client = client

	resp := resource.UpdateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("a successful update must set state even when the refresh fails")
	}

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.DisplayName.ValueString() != "renamed-byoc" {
		t.Errorf("display_name = %q; want the plan value renamed-byoc", got.DisplayName.ValueString())
	}
	if got.State.IsUnknown() || got.State.ValueString() != api.ByocStateReady {
		t.Errorf("state = %v; want the prior state value %q, never unknown", got.State, api.ByocStateReady)
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

func TestByocPostCreatePatchSendsPrivateLinkWithPscSubnet(t *testing.T) {
	config := models.ByocInfrastructureResourceModel{
		GcpPscSubnetID:    types.StringValue("psc-subnet"),
		EnablePrivateLink: types.BoolValue(true),
	}
	details := byocInfraDetails(api.ByocStateReady)

	patch, hasPatch, diags := byocPostCreatePatch(&config, details)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
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

func TestByocPostCreatePatchRejectsPscSubnetWithoutPrivateLink(t *testing.T) {
	// Unknown interpolations bypass ValidateConfig, so the resolved values
	// must be re-checked instead of forcing enablePrivateLink = true.
	for name, enablePrivateLink := range map[string]types.Bool{
		"omitted":        types.BoolNull(),
		"explicit false": types.BoolValue(false),
	} {
		t.Run(name, func(t *testing.T) {
			config := models.ByocInfrastructureResourceModel{
				GcpPscSubnetID:    types.StringValue("psc-subnet"),
				EnablePrivateLink: enablePrivateLink,
			}

			_, hasPatch, diags := byocPostCreatePatch(&config, byocInfraDetails(api.ByocStateReady))
			if !diags.HasError() {
				t.Fatal("expected an error for gcp_psc_subnet_id without enable_private_link = true")
			}
			if hasPatch {
				t.Error("expected no patch when the PSC subnet invariant fails")
			}
		})
	}
}

func TestByocUpdateRequestSendsPrivateLinkWithPscSubnet(t *testing.T) {
	state := byocInfraModel()
	state.EnablePrivateLink = types.BoolValue(true)
	plan := byocInfraModel()
	plan.EnablePrivateLink = types.BoolValue(true)
	plan.GcpPscSubnetID = types.StringValue("psc-subnet")

	patch, hasPatch, diags := byocUpdateRequestFromModels(context.Background(), &plan, &state)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if !hasPatch {
		t.Fatal("expected a patch for the added PSC subnet")
	}
	if patch.GcpPscSubnetId == nil || *patch.GcpPscSubnetId != "psc-subnet" {
		t.Errorf("gcpPscSubnetId = %v; want psc-subnet", patch.GcpPscSubnetId)
	}
	if patch.EnablePrivateLink == nil || !*patch.EnablePrivateLink {
		t.Error("enablePrivateLink must be true alongside gcpPscSubnetId")
	}
}

func TestByocUpdateRequestRejectsPscSubnetWithoutPrivateLink(t *testing.T) {
	state := byocInfraModel()
	state.EnablePrivateLink = types.BoolValue(false)
	plan := byocInfraModel()
	plan.EnablePrivateLink = types.BoolValue(false)
	plan.GcpPscSubnetID = types.StringValue("psc-subnet")

	_, hasPatch, diags := byocUpdateRequestFromModels(context.Background(), &plan, &state)
	if !diags.HasError() {
		t.Fatal("expected an error for gcp_psc_subnet_id without enable_private_link = true")
	}
	if hasPatch {
		t.Error("expected no patch when the PSC subnet invariant fails")
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

func TestByocWriteOnlyAttributesReplaceWithoutImportMarker(t *testing.T) {
	ctx := context.Background()
	_, schemaResp := byocInfraSchema(t)
	raw := byocInfraState(t, schemaResp).Raw
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: raw}
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw}

	stringModifier := byocStringRequiresReplaceUnlessAdopted()

	nullStateReq := planmodifier.StringRequest{
		State:      state,
		Plan:       plan,
		StateValue: types.StringNull(),
		PlanValue:  types.StringValue("external-id-1"),
	}
	nullStateResp := &planmodifier.StringResponse{PlanValue: nullStateReq.PlanValue}
	stringModifier.PlanModifyString(ctx, nullStateReq, nullStateResp)
	if !nullStateResp.RequiresReplace {
		t.Error("configuring a value over a null state without the import marker must require replacement")
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
	listNullStateReq := planmodifier.ListRequest{
		State:      state,
		Plan:       plan,
		StateValue: types.ListNull(types.StringType),
		PlanValue:  types.ListValueMust(types.StringType, []attr.Value{types.StringValue("subnet-1")}),
	}
	listNullStateResp := &planmodifier.ListResponse{PlanValue: listNullStateReq.PlanValue}
	listModifier.PlanModifyList(ctx, listNullStateReq, listNullStateResp)
	if !listNullStateResp.RequiresReplace {
		t.Error("configuring a list over a null state without the import marker must require replacement")
	}
}

// fakeByocPrivateState stands in for the framework's private state, whose
// concrete type lives in an internal package and cannot be constructed here.
func TestByocRequiresReplaceIfKnown(t *testing.T) {
	ctx := context.Background()
	_, schemaResp := byocInfraSchema(t)
	raw := byocInfraState(t, schemaResp).Raw
	state := tfsdk.State{Schema: schemaResp.Schema, Raw: raw}
	plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw}

	stringModifier := byocStringRequiresReplaceIfKnown()

	unknownReq := planmodifier.StringRequest{
		State:      state,
		Plan:       plan,
		StateValue: types.StringNull(),
		PlanValue:  types.StringUnknown(),
	}
	unknownResp := &planmodifier.StringResponse{PlanValue: unknownReq.PlanValue}
	stringModifier.PlanModifyString(ctx, unknownReq, unknownResp)
	if unknownResp.RequiresReplace {
		t.Error("an unknown planned value must not force replacement")
	}

	knownReq := planmodifier.StringRequest{
		State:      state,
		Plan:       plan,
		StateValue: types.StringValue("10.0.0.0/16"),
		PlanValue:  types.StringValue("10.1.0.0/16"),
	}
	knownResp := &planmodifier.StringResponse{PlanValue: knownReq.PlanValue}
	stringModifier.PlanModifyString(ctx, knownReq, knownResp)
	if !knownResp.RequiresReplace {
		t.Error("a known planned value differing from state must force replacement")
	}

	listModifier := byocListRequiresReplaceIfKnown()
	listUnknownReq := planmodifier.ListRequest{
		State:      state,
		Plan:       plan,
		StateValue: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("subnet-1")}),
		PlanValue:  types.ListUnknown(types.StringType),
	}
	listUnknownResp := &planmodifier.ListResponse{PlanValue: listUnknownReq.PlanValue}
	listModifier.PlanModifyList(ctx, listUnknownReq, listUnknownResp)
	if listUnknownResp.RequiresReplace {
		t.Error("an unknown planned list must not force replacement")
	}
}

type fakeByocPrivateState struct {
	data map[string][]byte
}

func (f *fakeByocPrivateState) GetKey(_ context.Context, key string) ([]byte, diag.Diagnostics) {
	return f.data[key], nil
}

func (f *fakeByocPrivateState) SetKey(_ context.Context, key string, value []byte) diag.Diagnostics {
	f.data[key] = value
	return nil
}

func TestByocImportMarkerRoundTrip(t *testing.T) {
	ctx := context.Background()
	private := &fakeByocPrivateState{data: map[string][]byte{}}

	var diags diag.Diagnostics
	if byocWasImported(ctx, private, &diags) {
		t.Error("expected false before the import marker is written")
	}

	byocMarkImported(ctx, private, &diags)
	if len(private.data[byocImportedPrivateKey]) == 0 {
		t.Fatalf("expected ImportState marker under %q, got keys %v", byocImportedPrivateKey, private.data)
	}

	if !byocWasImported(ctx, private, &diags) {
		t.Error("expected true after byocMarkImported wrote the marker")
	}
	if diags.HasError() {
		t.Errorf("unexpected diags: %v", diags)
	}
}

func TestByocDetailsMapByoVpcFieldsToResourceState(t *testing.T) {
	ctx := context.Background()

	details := byocInfraDetails(api.ByocStateReady)
	details.IsByoVpc = boolPtr(true)
	details.ByoVpcId = strPtr("vpc-123")
	details.ByoVpcPrivateSubnetIds = []string{"subnet-1", "subnet-2"}
	details.ByoVpcPodCidrRangeNames = []string{"pods-a"}
	details.ByoVpcSharedVpcHostProjectId = strPtr("host-project")

	model := models.ByocInfrastructureResourceModel{}
	if diags := applyByocDetailsToResourceState(ctx, details, nil, &model); diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}

	if got := model.VpcID; !got.Equal(types.StringValue("vpc-123")) {
		t.Errorf("VpcID = %v, want vpc-123", got)
	}
	if got := model.GcpSharedVpcHostProjectID; !got.Equal(types.StringValue("host-project")) {
		t.Errorf("GcpSharedVpcHostProjectID = %v, want host-project", got)
	}
	wantSubnets := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("subnet-1"), types.StringValue("subnet-2")})
	if got := model.PrivateSubnetIDs; !got.Equal(wantSubnets) {
		t.Errorf("PrivateSubnetIDs = %v, want %v", got, wantSubnets)
	}
	wantRanges := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("pods-a")})
	if got := model.GcpPodCidrRangeNames; !got.Equal(wantRanges) {
		t.Errorf("GcpPodCidrRangeNames = %v, want %v", got, wantRanges)
	}

	managed := models.ByocInfrastructureResourceModel{}
	if diags := applyByocDetailsToResourceState(ctx, byocInfraDetails(api.ByocStateReady), nil, &managed); diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if !managed.VpcID.IsNull() || !managed.GcpSharedVpcHostProjectID.IsNull() ||
		!managed.PrivateSubnetIDs.IsNull() || !managed.GcpPodCidrRangeNames.IsNull() {
		t.Error("managed-VPC details must map the BYO-VPC attributes to null")
	}
}

func TestByocPostCreatePatchNoopWhenNothingConfigured(t *testing.T) {
	config := models.ByocInfrastructureResourceModel{}
	_, hasPatch, diags := byocPostCreatePatch(&config, byocInfraDetails(api.ByocStateReady))
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if hasPatch {
		t.Error("expected no patch when no toggles are configured")
	}
}
