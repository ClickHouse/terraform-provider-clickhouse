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
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

func byocRestrictedFactsObject(t *testing.T, mutate func(*models.ByocRestrictedFactsModel)) types.Object {
	t.Helper()
	facts := models.ByocRestrictedFactsModel{
		ClusterName:              types.StringValue("ra-cluster"),
		PlatformNodeGroup:        types.StringNull(),
		AvailabilityZones:        types.ListValueMust(types.StringType, []attr.Value{types.StringValue("us-east-1a"), types.StringValue("us-east-1b"), types.StringValue("us-east-1c")}),
		NamespacePrefix:          types.StringNull(),
		ExecutorRoleArn:          types.StringValue("arn:aws:iam::123456789012:role/executor"),
		NodeRoleArns:             types.ListValueMust(types.StringType, []attr.Value{types.StringValue("arn:aws:iam::123456789012:role/node")}),
		ImageMode:                types.StringValue(api.ByocImageModeDirect),
		ImageRegistry:            types.StringNull(),
		DataBucket:               types.StringValue("data"),
		BackupBucket:             types.StringValue("backup"),
		StorageRoleArn:           types.StringValue("arn:aws:iam::123456789012:role/storage"),
		ScraperEndpoint:          types.StringNull(),
		EnableMonitoring:         types.BoolValue(false),
		EnableSnapshotController: types.BoolNull(),
		EnableAutoscaler:         types.BoolNull(),
	}
	if mutate != nil {
		mutate(&facts)
	}
	obj, diags := types.ObjectValueFrom(context.Background(), models.ByocRestrictedFactsAttrTypes(), facts)
	if diags.HasError() {
		t.Fatalf("build restricted facts: %v", diags)
	}
	return obj
}

func byocRestrictedModel(t *testing.T) models.ByocInfrastructureResourceModel {
	model := byocInfraModel()
	model.ExternalID = types.StringNull()
	model.AccessModel = types.StringValue(api.ByocAccessModelRestricted)
	model.RestrictedFacts = byocRestrictedFactsObject(t, nil)
	return model
}

func byocRestrictedAPIFacts() *api.ByocRestrictedFacts {
	return &api.ByocRestrictedFacts{
		ClusterName:       "ra-cluster",
		AvailabilityZones: []string{"us-east-1a", "us-east-1b", "us-east-1c"},
		ExecutorRoleArn:   strPtr("arn:aws:iam::123456789012:role/executor"),
		NodeRoleArns:      []string{"arn:aws:iam::123456789012:role/node"},
		ImageMode:         api.ByocImageModeDirect,
		DataBucket:        "data",
		BackupBucket:      "backup",
		StorageRoleArn:    "arn:aws:iam::123456789012:role/storage",
		EnableMonitoring:  boolPtr(false),
	}
}

func byocRestrictedDetails(state string) *api.ByocInfrastructureDetails {
	facts := byocRestrictedAPIFacts()
	facts.PlatformNodeGroup = strPtr("system")
	return &api.ByocInfrastructureDetails{
		Id:                byocInfraID,
		State:             state,
		AccountId:         "123456789012",
		RegionId:          "us-east-1",
		CloudProvider:     "aws",
		DisplayName:       "ra-us-east-1",
		EnablePrivateLink: boolPtr(false),
		AccessModel:       api.ByocAccessModelRestricted,
		RestrictedFacts:   facts,
	}
}

func byocRestrictedCreateRequest() api.ByocInfrastructureCreateRequest {
	return api.ByocInfrastructureCreateRequest{
		RegionId:        "us-east-1",
		AccountId:       "123456789012",
		AccessModel:     strPtr(api.ByocAccessModelRestricted),
		RestrictedFacts: byocRestrictedAPIFacts(),
	}
}

func TestByocInfrastructureResourceCreateRestricted(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	model := byocRestrictedModel(t)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	// Unset mocks fail the test when called: a restricted create must not
	// wait for readiness, read tags or patch settings after create.
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.ValidateByocInfrastructureMock.
		Expect(ctx, byocValidateRequestFromCreate(byocRestrictedCreateRequest())).
		Return(passingByocValidation(), nil)
	client.CreateByocInfrastructureMock.
		Expect(ctx, byocRestrictedCreateRequest()).
		Return(&api.ByocInfrastructure{Id: byocInfraID, State: api.ByocStateAwaitingConnector, AccessModel: api.ByocAccessModelRestricted}, nil)
	client.GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(byocRestrictedDetails(api.ByocStateAwaitingConnector), nil)
	r.client = client

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}

	var state models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.State.ValueString() != api.ByocStateAwaitingConnector {
		t.Errorf("state = %q; want %q", state.State.ValueString(), api.ByocStateAwaitingConnector)
	}
	if state.AccessModel.ValueString() != api.ByocAccessModelRestricted {
		t.Errorf("access_model = %q; want restricted", state.AccessModel.ValueString())
	}
	var facts models.ByocRestrictedFactsModel
	if diags := state.RestrictedFacts.As(ctx, &facts, basetypes.ObjectAsOptions{}); diags.HasError() {
		t.Fatalf("read facts: %v", diags)
	}
	if facts.PlatformNodeGroup.ValueString() != "system" {
		t.Errorf("platform_node_group = %v; want the API default read back", facts.PlatformNodeGroup)
	}
	if state.Tags.IsNull() || len(state.Tags.Elements()) != 0 {
		t.Errorf("tags = %v; want known empty map", state.Tags)
	}
}

func TestByocInfrastructureResourceCreateRestrictedConflictSuggestsImport(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	model := byocRestrictedModel(t)
	model.SkipPreflightValidation = types.BoolValue(true)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateByocInfrastructureMock.
		Return(nil, errors.New("status: 409, body: already registered"))

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan, Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: plan.Raw}}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create should surface the conflict")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "terraform import") {
		t.Errorf("detail = %q; want an import hint", detail)
	}
}

func TestByocInfrastructureResourceReadRestrictedSkipsTagsAndFillsFacts(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	// An imported state holds only the ID; Read must fill the registration.
	model := byocInfraModel()
	model.ID = types.StringValue(byocInfraID)
	model.ExternalID = types.StringNull()
	model.AccessModel = types.StringNull()
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(byocRestrictedDetails(api.ByocStateReady), nil)

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.AccessModel.ValueString() != api.ByocAccessModelRestricted {
		t.Errorf("access_model = %v; want restricted", got.AccessModel)
	}
	want, diags := models.ByocRestrictedFactsObjectFromAPI(ctx, byocRestrictedDetails(api.ByocStateReady).RestrictedFacts)
	if diags.HasError() {
		t.Fatalf("build facts: %v", diags)
	}
	if !got.RestrictedFacts.Equal(want) {
		t.Errorf("restricted_facts = %v; want %v", got.RestrictedFacts, want)
	}
}

func TestByocInfrastructureResourceReadStandardDefaultsAccessModel(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	model := byocInfraModel()
	model.ID = types.StringValue(byocInfraID)
	model.AccessModel = types.StringNull()
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetByocInfrastructureMock.Return(byocInfraDetails(api.ByocStateReady), nil)
	client.GetByocInfrastructureTagsMock.Return(map[string]string{}, nil)
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
	if got.AccessModel.ValueString() != api.ByocAccessModelStandard {
		t.Errorf("access_model = %v; an absent API access model must read back as standard", got.AccessModel)
	}
	if !got.RestrictedFacts.IsNull() {
		t.Errorf("restricted_facts = %v; want null for standard", got.RestrictedFacts)
	}
}

func TestByocInfrastructureResourceValidateConfigAccessModel(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)

	cases := []struct {
		name      string
		mutate    func(*models.ByocInfrastructureResourceModel)
		wantError string
	}{
		{
			name:   "valid restricted",
			mutate: func(*models.ByocInfrastructureResourceModel) {},
		},
		{
			name:      "restricted without facts",
			mutate:    func(m *models.ByocInfrastructureResourceModel) { m.RestrictedFacts = types.ObjectNull(models.ByocRestrictedFactsAttrTypes()) },
			wantError: "Missing restricted_facts",
		},
		{
			name:      "restricted with external_id",
			mutate:    func(m *models.ByocInfrastructureResourceModel) { m.ExternalID = types.StringValue("x") },
			wantError: "external_id is not supported",
		},
		{
			name:      "restricted with vpc_cidr_range",
			mutate:    func(m *models.ByocInfrastructureResourceModel) { m.VpcCidrRange = types.StringValue("10.0.0.0/16") },
			wantError: "vpc_cidr_range is not supported",
		},
		{
			name:      "restricted with enable_private_link",
			mutate:    func(m *models.ByocInfrastructureResourceModel) { m.EnablePrivateLink = types.BoolValue(false) },
			wantError: "enable_private_link is not supported",
		},
		{
			name: "restricted with tags",
			mutate: func(m *models.ByocInfrastructureResourceModel) {
				m.Tags = types.MapValueMust(types.StringType, map[string]attr.Value{"team": types.StringValue("data")})
			},
			wantError: "tags is not supported",
		},
		{
			name: "restricted with empty tags",
			mutate: func(m *models.ByocInfrastructureResourceModel) {
				m.Tags = types.MapValueMust(types.StringType, map[string]attr.Value{})
			},
		},
		{
			name: "direct mode without executor role",
			mutate: func(m *models.ByocInfrastructureResourceModel) {
				m.RestrictedFacts = byocRestrictedFactsObject(t, func(f *models.ByocRestrictedFactsModel) { f.ExecutorRoleArn = types.StringNull() })
			},
			wantError: "Missing executor_role_arn",
		},
		{
			name: "direct mode without node roles",
			mutate: func(m *models.ByocInfrastructureResourceModel) {
				m.RestrictedFacts = byocRestrictedFactsObject(t, func(f *models.ByocRestrictedFactsModel) { f.NodeRoleArns = types.ListNull(types.StringType) })
			},
			wantError: "Missing node_role_arns",
		},
		{
			name: "bundle mode without registry",
			mutate: func(m *models.ByocInfrastructureResourceModel) {
				m.RestrictedFacts = byocRestrictedFactsObject(t, func(f *models.ByocRestrictedFactsModel) { f.ImageMode = types.StringValue(api.ByocImageModeBundle) })
			},
			wantError: "Missing image_registry",
		},
		{
			name: "monitoring by default without scraper endpoint",
			mutate: func(m *models.ByocInfrastructureResourceModel) {
				m.RestrictedFacts = byocRestrictedFactsObject(t, func(f *models.ByocRestrictedFactsModel) { f.EnableMonitoring = types.BoolNull() })
			},
			wantError: "Missing scraper_endpoint",
		},
		{
			name: "monitoring with scraper endpoint",
			mutate: func(m *models.ByocInfrastructureResourceModel) {
				m.RestrictedFacts = byocRestrictedFactsObject(t, func(f *models.ByocRestrictedFactsModel) {
					f.EnableMonitoring = types.BoolValue(true)
					f.ScraperEndpoint = types.StringValue("http://scraper:9093/api/v1/write")
				})
			},
		},
		{
			name: "standard with facts",
			mutate: func(m *models.ByocInfrastructureResourceModel) {
				m.AccessModel = types.StringValue(api.ByocAccessModelStandard)
			},
			wantError: "restricted_facts requires access_model",
		},
		{
			name: "omitted access model with facts",
			mutate: func(m *models.ByocInfrastructureResourceModel) {
				m.AccessModel = types.StringNull()
			},
			wantError: "restricted_facts requires access_model",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := byocRestrictedModel(t)
			tc.mutate(&model)
			cfg := byocInfraConfig(t, schemaResp, &model)

			resp := resource.ValidateConfigResponse{}
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: cfg}, &resp)
			if tc.wantError == "" {
				if resp.Diagnostics.HasError() {
					t.Fatalf("ValidateConfig diagnostics: %v", resp.Diagnostics)
				}
				return
			}
			found := false
			for _, d := range resp.Diagnostics.Errors() {
				if strings.Contains(d.Summary(), tc.wantError) {
					found = true
				}
			}
			if !found {
				t.Errorf("errors = %v; want one containing %q", resp.Diagnostics.Errors(), tc.wantError)
			}
		})
	}
}

func TestByocAccessModelRequiresReplace(t *testing.T) {
	ctx := context.Background()
	_, schemaResp := byocInfraSchema(t)
	raw := byocInfraState(t, schemaResp).Raw
	modifier := byocAccessModelRequiresReplace()

	cases := []struct {
		name  string
		state types.String
		plan  types.String
		want  bool
	}{
		{name: "pre-upgrade null state planned standard", state: types.StringNull(), plan: types.StringValue(api.ByocAccessModelStandard), want: false},
		{name: "pre-upgrade null state planned restricted", state: types.StringNull(), plan: types.StringValue(api.ByocAccessModelRestricted), want: true},
		{name: "unchanged", state: types.StringValue(api.ByocAccessModelRestricted), plan: types.StringValue(api.ByocAccessModelRestricted), want: false},
		{name: "standard to restricted", state: types.StringValue(api.ByocAccessModelStandard), plan: types.StringValue(api.ByocAccessModelRestricted), want: true},
		{name: "unknown plan", state: types.StringValue(api.ByocAccessModelStandard), plan: types.StringUnknown(), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.StringRequest{
				State:      tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
				Plan:       tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
				StateValue: tc.state,
				PlanValue:  tc.plan,
			}
			resp := &planmodifier.StringResponse{PlanValue: tc.plan}
			modifier.PlanModifyString(ctx, req, resp)
			if resp.RequiresReplace != tc.want {
				t.Errorf("RequiresReplace = %v; want %v", resp.RequiresReplace, tc.want)
			}
		})
	}
}

func TestByocRestrictedFactsPresenceRequiresReplace(t *testing.T) {
	ctx := context.Background()
	_, schemaResp := byocInfraSchema(t)
	raw := byocInfraState(t, schemaResp).Raw
	modifier := byocRestrictedFactsPresenceRequiresReplace()
	facts := byocRestrictedFactsObject(t, nil)
	changed := byocRestrictedFactsObject(t, func(f *models.ByocRestrictedFactsModel) { f.ClusterName = types.StringValue("other") })
	null := types.ObjectNull(models.ByocRestrictedFactsAttrTypes())

	cases := []struct {
		name        string
		state, plan types.Object
		want        bool
	}{
		{name: "added", state: null, plan: facts, want: true},
		{name: "removed", state: facts, plan: null, want: true},
		{name: "attribute change is left to the nested attribute", state: facts, plan: changed, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.ObjectRequest{
				State:      tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
				Plan:       tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
				StateValue: tc.state,
				PlanValue:  tc.plan,
			}
			resp := &planmodifier.ObjectResponse{PlanValue: tc.plan}
			modifier.PlanModifyObject(ctx, req, resp)
			if resp.RequiresReplace != tc.want {
				t.Errorf("RequiresReplace = %v; want %v", resp.RequiresReplace, tc.want)
			}
		})
	}
}

func TestByocResolveUnknownRestrictedFacts(t *testing.T) {
	ctx := context.Background()
	planned := byocRestrictedFactsObject(t, func(f *models.ByocRestrictedFactsModel) {
		f.PlatformNodeGroup = types.StringUnknown()
		f.EnableAutoscaler = types.BoolUnknown()
		f.AvailabilityZones = types.ListUnknown(types.StringType)
	})
	got, diags := models.ByocResolveUnknownRestrictedFacts(ctx, planned)
	if diags.HasError() {
		t.Fatalf("resolve: %v", diags)
	}
	var facts models.ByocRestrictedFactsModel
	if diags := got.As(ctx, &facts, basetypes.ObjectAsOptions{}); diags.HasError() {
		t.Fatalf("read facts: %v", diags)
	}
	if !facts.PlatformNodeGroup.IsNull() || !facts.EnableAutoscaler.IsNull() || !facts.AvailabilityZones.IsNull() {
		t.Errorf("facts = %+v; want unknowns resolved to null", facts)
	}
	if facts.ClusterName.ValueString() != "ra-cluster" {
		t.Errorf("cluster_name = %v; known values must be kept", facts.ClusterName)
	}
}
