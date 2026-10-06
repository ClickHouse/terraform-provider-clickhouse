package resource

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

const (
	// Provisioning builds a full data plane in the customer account; the
	// console quotes 30-60 minutes, so leave headroom.
	byocCreateWaitSeconds = 90 * 60
	byocDeleteWaitSeconds = 60 * 60
)

var (
	_ resource.Resource                   = (*ByocInfrastructureResource)(nil)
	_ resource.ResourceWithConfigure      = (*ByocInfrastructureResource)(nil)
	_ resource.ResourceWithImportState    = (*ByocInfrastructureResource)(nil)
	_ resource.ResourceWithValidateConfig = (*ByocInfrastructureResource)(nil)
)

// Write-only creation parameters are never returned by the API, so an
// imported state holds null for them. Replacing only when a previously
// recorded value changes lets the owner re-add them after import without
// destroying the infrastructure. Adoption is gated on the import marker in
// private state: a resource created by Terraform also stores null for omitted
// creation-only options, and adding one later must replace — recording it
// without an API call would silently diverge state from the infrastructure.
const byocAdoptDescription = "Requires replacement when the value changes; after import, a value configured over the null imported state is adopted in place without replacement."

const byocImportedPrivateKey = "imported"

// byocPrivateGetter matches the framework's private state accessor so the
// import check stays testable (the concrete type is framework-internal).
type byocPrivateGetter interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
}

type byocPrivateSetter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

func byocWasImported(ctx context.Context, private byocPrivateGetter, diags *diag.Diagnostics) bool {
	data, d := private.GetKey(ctx, byocImportedPrivateKey)
	diags.Append(d...)
	return len(data) > 0
}

func byocMarkImported(ctx context.Context, private byocPrivateSetter, diags *diag.Diagnostics) {
	diags.Append(private.SetKey(ctx, byocImportedPrivateKey, []byte("true"))...)
}

func byocStringRequiresReplaceUnlessAdopted() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		func(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = !req.StateValue.IsNull() || !byocWasImported(ctx, req.Private, &resp.Diagnostics)
		},
		byocAdoptDescription, byocAdoptDescription,
	)
}

func byocListRequiresReplaceUnlessAdopted() planmodifier.List {
	return listplanmodifier.RequiresReplaceIf(
		func(ctx context.Context, req planmodifier.ListRequest, resp *listplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = !req.StateValue.IsNull() || !byocWasImported(ctx, req.Private, &resp.Diagnostics)
		},
		byocAdoptDescription, byocAdoptDescription,
	)
}

const byocVpcCidrModifierDescription = "Uses the prior state value unless BYO-VPC inputs are configured; a recorded managed-VPC CIDR must not leak into a BYO-VPC create."

// byocVpcCidrUseStateForUnknown is UseStateForUnknown except when BYO-VPC
// inputs are configured: during a managed-VPC -> BYO-VPC replacement the old
// CIDR would otherwise be planned as known and sent alongside vpc_id, which
// the API rejects as mutually exclusive.
func byocVpcCidrUseStateForUnknown() planmodifier.String {
	return byocVpcCidrUseStateForUnknownModifier{}
}

type byocVpcCidrUseStateForUnknownModifier struct{}

func (m byocVpcCidrUseStateForUnknownModifier) Description(context.Context) string {
	return byocVpcCidrModifierDescription
}

func (m byocVpcCidrUseStateForUnknownModifier) MarkdownDescription(context.Context) string {
	return byocVpcCidrModifierDescription
}

func (m byocVpcCidrUseStateForUnknownModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, p := range []path.Path{path.Root("vpc_id"), path.Root("gcp_shared_vpc_host_project_id")} {
		var v types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p, &v)...)
		if resp.Diagnostics.HasError() || !v.IsNull() {
			return
		}
	}
	for _, p := range []path.Path{path.Root("private_subnet_ids"), path.Root("public_subnet_ids"), path.Root("gcp_pod_cidr_range_names")} {
		var v types.List
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, p, &v)...)
		if resp.Diagnostics.HasError() || !v.IsNull() {
			return
		}
	}
	resp.PlanValue = req.StateValue
}

const byocByoVpcModifierDescription = "Uses the prior state value unless the replacement changes topology (vpc_cidr_range configured, region_id changed, or a different vpc_id configured); stale BYO-VPC wiring must not leak into the new infrastructure."

// byocByoVpcUseStateForUnknown is UseStateForUnknown except when the plan
// changes topology: a BYO-VPC -> managed replacement, a region change, or a
// switch to a different vpc_id would otherwise restore stale BYO-VPC wiring
// from state and send it with the replacement create.
func byocByoVpcStringUseStateForUnknown() planmodifier.String {
	return byocByoVpcUseStateForUnknownModifier{}
}

func byocByoVpcListUseStateForUnknown() planmodifier.List {
	return byocByoVpcUseStateForUnknownModifier{}
}

type byocByoVpcUseStateForUnknownModifier struct{}

func (m byocByoVpcUseStateForUnknownModifier) Description(context.Context) string {
	return byocByoVpcModifierDescription
}

func (m byocByoVpcUseStateForUnknownModifier) MarkdownDescription(context.Context) string {
	return byocByoVpcModifierDescription
}

type byocByoVpcTopologyArgs struct {
	config tfsdk.Config
	plan   tfsdk.Plan
	state  tfsdk.State
}

func byocByoVpcTopologyChanged(ctx context.Context, args byocByoVpcTopologyArgs, diags *diag.Diagnostics) bool {
	var cidr, configVpcID, stateVpcID, planRegion, stateRegion types.String
	diags.Append(args.config.GetAttribute(ctx, path.Root("vpc_cidr_range"), &cidr)...)
	diags.Append(args.config.GetAttribute(ctx, path.Root("vpc_id"), &configVpcID)...)
	diags.Append(args.state.GetAttribute(ctx, path.Root("vpc_id"), &stateVpcID)...)
	diags.Append(args.plan.GetAttribute(ctx, path.Root("region_id"), &planRegion)...)
	diags.Append(args.state.GetAttribute(ctx, path.Root("region_id"), &stateRegion)...)
	if diags.HasError() {
		return true
	}
	return !cidr.IsNull() ||
		!planRegion.Equal(stateRegion) ||
		(!configVpcID.IsNull() && !configVpcID.Equal(stateVpcID))
}

func (m byocByoVpcUseStateForUnknownModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	if byocByoVpcTopologyChanged(ctx, byocByoVpcTopologyArgs{config: req.Config, plan: req.Plan, state: req.State}, &resp.Diagnostics) {
		return
	}
	resp.PlanValue = req.StateValue
}

func (m byocByoVpcUseStateForUnknownModifier) PlanModifyList(ctx context.Context, req planmodifier.ListRequest, resp *planmodifier.ListResponse) {
	if req.StateValue.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	if byocByoVpcTopologyChanged(ctx, byocByoVpcTopologyArgs{config: req.Config, plan: req.Plan, state: req.State}, &resp.Diagnostics) {
		return
	}
	resp.PlanValue = req.StateValue
}

const byocPscSubnetModifierDescription = "Uses the prior state value unless private link is planned disabled, in which case the API clears the subnet and the plan becomes null."

// byocPscSubnetUseStateForUnknown is UseStateForUnknown except when private
// link is planned disabled: the API clears the subnet then, so preserving the
// recorded value would plan a known value that the read-back contradicts.
func byocPscSubnetUseStateForUnknown() planmodifier.String {
	return byocPscSubnetUseStateForUnknownModifier{}
}

type byocPscSubnetUseStateForUnknownModifier struct{}

func (m byocPscSubnetUseStateForUnknownModifier) Description(context.Context) string {
	return byocPscSubnetModifierDescription
}

func (m byocPscSubnetUseStateForUnknownModifier) MarkdownDescription(context.Context) string {
	return byocPscSubnetModifierDescription
}

func (m byocPscSubnetUseStateForUnknownModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || !req.PlanValue.IsUnknown() || req.ConfigValue.IsUnknown() {
		return
	}
	var configPrivateLink, statePrivateLink types.Bool
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("enable_private_link"), &configPrivateLink)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("enable_private_link"), &statePrivateLink)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// enable_private_link may not have run its own modifiers yet, so resolve
	// its effective planned value the same way: config wins, else prior state.
	effective := configPrivateLink
	if effective.IsNull() {
		effective = statePrivateLink
	}
	if effective.IsUnknown() {
		return
	}
	if !effective.IsNull() && !effective.ValueBool() {
		resp.PlanValue = types.StringNull()
		return
	}
	resp.PlanValue = req.StateValue
}

//go:embed descriptions/byoc_infrastructure.md
var byocInfrastructureResourceDescription string

func NewByocInfrastructureResource() resource.Resource {
	return &ByocInfrastructureResource{}
}

type ByocInfrastructureResource struct {
	client api.Client
}

func (r *ByocInfrastructureResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_byoc_infrastructure"
}

func (r *ByocInfrastructureResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: byocInfrastructureResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique ID of the BYOC infrastructure.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"state": schema.StringAttribute{
				Description: "Provisioning state of the infrastructure (e.g. 'infra-provisioning', 'infra-ready').",
				Computed:    true,
			},
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider the infrastructure runs in ('aws', 'gcp' or 'azure'), derived from 'region_id'.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"region_id": schema.StringAttribute{
				Description: "BYOC region to deploy into (e.g. 'us-east-1').",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"account_id": schema.StringAttribute{
				Description: "Customer cloud account to deploy into: AWS account ID, GCP project ID or Azure subscription ID.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"display_name": schema.StringAttribute{
				Description: "Human readable name of the infrastructure. Defaults to a server-generated name.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"external_id": schema.StringAttribute{
				Description: "AWS only: ExternalID of the onboarding IAM role trust policy. Write-only; not returned by the API.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					byocStringRequiresReplaceUnlessAdopted(),
				},
			},
			"tenant_id": schema.StringAttribute{
				Description: "Azure only: Entra tenant ID of the subscription. Write-only; not returned by the API.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					byocStringRequiresReplaceUnlessAdopted(),
				},
			},
			"service_principal_client_id": schema.StringAttribute{
				Description: "Azure only: client ID of the onboarding service principal. Write-only; not returned by the API.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					byocStringRequiresReplaceUnlessAdopted(),
				},
			},
			"vpc_cidr_range": schema.StringAttribute{
				Description: "CIDR range for the ClickHouse-managed VPC. Mutually exclusive with the BYO-VPC attributes.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(
						path.MatchRoot("vpc_id"),
						path.MatchRoot("private_subnet_ids"),
						path.MatchRoot("public_subnet_ids"),
						path.MatchRoot("gcp_pod_cidr_range_names"),
						path.MatchRoot("gcp_shared_vpc_host_project_id"),
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					byocVpcCidrUseStateForUnknown(),
				},
			},
			"availability_zone_suffixes": schema.ListAttribute{
				Description: "Availability zone suffixes (e.g. ['a', 'b', 'c']) for the ClickHouse-managed VPC. Write-only; the resolved zones are returned by the data source as 'vpc_availability_zone_list'.",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					byocListRequiresReplaceUnlessAdopted(),
				},
			},
			"vpc_id": schema.StringAttribute{
				Description: "BYO-VPC: ID of the customer-managed VPC to deploy into instead of a ClickHouse-managed one.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					byocByoVpcStringUseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"private_subnet_ids": schema.ListAttribute{
				Description: "BYO-VPC: private subnet IDs to deploy into.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					byocByoVpcListUseStateForUnknown(),
					listplanmodifier.RequiresReplace(),
				},
			},
			"public_subnet_ids": schema.ListAttribute{
				Description: "BYO-VPC: public subnet IDs for internet-facing load balancers. Write-only; not returned by the API.",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					byocListRequiresReplaceUnlessAdopted(),
				},
			},
			"gcp_pod_cidr_range_names": schema.ListAttribute{
				Description: "GCP BYO-VPC: names of the secondary ranges used for pod IPs.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.List{
					byocByoVpcListUseStateForUnknown(),
					listplanmodifier.RequiresReplace(),
				},
			},
			"gcp_shared_vpc_host_project_id": schema.StringAttribute{
				Description: "GCP BYO-VPC: host project ID when the customer-managed VPC is a shared VPC.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					byocByoVpcStringUseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enable_private_link": schema.BoolAttribute{
				Description: "Enable private link connectivity (AWS PrivateLink, GCP Private Service Connect or Azure Private Link).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_private_load_balancer": schema.BoolAttribute{
				Description: "Enable the internal (VPC-private) load balancer.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_public_load_balancer": schema.BoolAttribute{
				Description: "Enable the internet-facing load balancer.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"gcp_psc_subnet_id": schema.StringAttribute{
				Description: "GCP only: subnet for the Private Service Connect NAT. Only accepted together with 'enable_private_link = true'.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					byocPscSubnetUseStateForUnknown(),
				},
			},
			"tags": schema.MapAttribute{
				Description: "Tags to apply to the cloud resources of the infrastructure (max 50). Updates replace the full tag set.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Validators: []validator.Map{
					mapvalidator.SizeAtMost(50),
				},
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"is_byo_vpc": schema.BoolAttribute{
				Description: "Whether the infrastructure runs inside a customer-managed VPC.",
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"skip_preflight_validation": schema.BoolAttribute{
				Description: "Skip the preflight cloud permission validation before creating. Defaults to false.",
				Optional:    true,
			},
		},
	}
}

// ValidateConfig rejects gcp_psc_subnet_id without an explicit
// enable_private_link = true: the API only accepts the subnet together with
// private link enabled, and an omitted toggle must not silently flip it on.
// Unknown values pass here and are re-checked with resolved values when the
// patch is built in Create and Update.
func (r *ByocInfrastructureResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config models.ByocInfrastructureResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if config.GcpPscSubnetID.IsNull() || config.GcpPscSubnetID.IsUnknown() || config.EnablePrivateLink.IsUnknown() {
		return
	}
	if !config.EnablePrivateLink.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("gcp_psc_subnet_id"),
			"gcp_psc_subnet_id requires enable_private_link = true",
			"The API only accepts a Private Service Connect subnet together with private link enabled. Set enable_private_link = true explicitly in the configuration.",
		)
	}
}

func (r *ByocInfrastructureResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError(
			"ClickHouse Cloud API not configured",
			"This resource requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).",
		)
		return
	}
	r.client = providerData.API
}

func (r *ByocInfrastructureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning("clickhouse_byoc_infrastructure", &resp.Diagnostics)

	var plan models.ByocInfrastructureResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createRequest, diags := byocCreateRequestFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !plan.SkipPreflightValidation.ValueBool() {
		validation, err := r.client.ValidateByocInfrastructure(ctx, byocValidateRequestFromCreate(createRequest))
		if err != nil {
			resp.Diagnostics.AddError(
				"Error running BYOC preflight validation",
				fmt.Sprintf("Could not run the preflight validation: %s\n\nSet skip_preflight_validation = true to create without the preflight gate.", err.Error()),
			)
			return
		}
		if !validation.Supported {
			resp.Diagnostics.AddWarning(
				"BYOC preflight validation not supported",
				fmt.Sprintf("Preflight validation is not supported for cloud provider %q, so no cloud permissions were verified before creating.", validation.CloudProvider),
			)
		} else if !validation.AllPassed {
			resp.Diagnostics.AddError("BYOC preflight validation failed", formatFailedByocChecks(validation))
			return
		}
	}

	infra, err := r.client.CreateByocInfrastructure(ctx, createRequest)
	if err != nil {
		resp.Diagnostics.AddError("Error creating BYOC infrastructure", err.Error())
		return
	}

	// Persist state before waiting: provisioning takes up to an hour and a
	// timeout must not orphan the created infrastructure.
	if !r.syncByocResourceState(ctx, infra.Id, &plan, &resp.Diagnostics) {
		applyByocSummaryFallback(infra, &plan)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err = r.client.WaitForByocInfrastructureState(ctx, infra.Id, func(state string) bool {
		return state != api.ByocStateProvisioning
	}, byocCreateWaitSeconds)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for BYOC infrastructure to provision",
			fmt.Sprintf("BYOC infrastructure %s did not finish provisioning: %s\n\nProvisioning continues in the background and the infrastructure is tracked in state, but Terraform marks it tainted, so the next apply would destroy and recreate it. To keep it, run 'terraform untaint <resource address>', wait for the infrastructure to reach the %q state (check the ClickHouse Cloud console or the clickhouse_byoc_infrastructure data source), then run terraform apply to finish aligning the configuration.", infra.Id, err.Error(), api.ByocStateReady),
		)
		return
	}

	details, err := r.client.GetByocInfrastructure(ctx, infra.Id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading BYOC infrastructure after create", err.Error())
		return
	}
	if details.State != api.ByocStateReady {
		resp.Diagnostics.AddError(
			"BYOC infrastructure provisioning failed",
			fmt.Sprintf("BYOC infrastructure %s entered state %q instead of %q.", infra.Id, details.State, api.ByocStateReady),
		)
		return
	}

	// Connectivity toggles and the PSC subnet are not creation parameters;
	// apply configured values that differ from the provisioned defaults.
	var config models.ByocInfrastructureResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch, hasPatch, patchDiags := byocPostCreatePatch(&config, details)
	resp.Diagnostics.Append(patchDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if hasPatch {
		if _, err := r.client.UpdateByocInfrastructure(ctx, infra.Id, patch); err != nil {
			resp.Diagnostics.AddError("Error applying BYOC infrastructure settings after create", err.Error())
			return
		}
	}

	// Persist state even when this refresh fails: the pre-wait snapshot
	// (infra-provisioning, pre-patch toggles) must not outlive the create.
	if !r.syncByocResourceState(ctx, infra.Id, &plan, &resp.Diagnostics) {
		plan.State = types.StringValue(details.State)
		if patch.EnablePrivateLink != nil {
			plan.EnablePrivateLink = types.BoolPointerValue(patch.EnablePrivateLink)
		}
		if patch.EnablePrivateLoadBalancer != nil {
			plan.EnablePrivateLoadBalancer = types.BoolPointerValue(patch.EnablePrivateLoadBalancer)
		}
		if patch.EnablePublicLoadBalancer != nil {
			plan.EnablePublicLoadBalancer = types.BoolPointerValue(patch.EnablePublicLoadBalancer)
		}
		if patch.GcpPscSubnetId != nil {
			plan.GcpPscSubnetID = types.StringPointerValue(patch.GcpPscSubnetId)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ByocInfrastructureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ByocInfrastructureResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	details, err := r.client.GetByocInfrastructure(ctx, state.ID.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading BYOC infrastructure", err.Error())
		return
	}
	// Termination is asynchronous (up to an hour); keep tracking until the
	// infrastructure is actually gone so a replacement cannot overlap it.
	if details.State == api.ByocStateTerminated {
		resp.State.RemoveResource(ctx)
		return
	}

	tags, err := r.client.GetByocInfrastructureTags(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading BYOC infrastructure tags", err.Error())
		return
	}

	resp.Diagnostics.Append(applyByocDetailsToResourceState(ctx, details, tags, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ByocInfrastructureResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning("clickhouse_byoc_infrastructure", &resp.Diagnostics)

	var plan, state models.ByocInfrastructureResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	patch, hasPatch, diags := byocUpdateRequestFromModels(ctx, &plan, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if hasPatch {
		if _, err := r.client.UpdateByocInfrastructure(ctx, state.ID.ValueString(), patch); err != nil {
			resp.Diagnostics.AddError("Error updating BYOC infrastructure", err.Error())
			return
		}
	}

	// Persist the plan even when the refresh fails: returning without state
	// after a successful update is a framework error. state is the only
	// computed attribute without UseStateForUnknown (it is volatile; pinning
	// it to the prior value in the plan would make Terraform reject any
	// server-side transition as an inconsistent result), so the plan carries
	// it as unknown — resolve it from prior state before writing.
	if !r.syncByocResourceState(ctx, state.ID.ValueString(), &plan, &resp.Diagnostics) {
		plan.State = state.State
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ByocInfrastructureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ByocInfrastructureResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	byocId := state.ID.ValueString()
	if err := r.client.DeleteByocInfrastructure(ctx, byocId); err != nil {
		if api.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting BYOC infrastructure", err.Error())
		return
	}

	err := r.client.WaitForByocInfrastructureState(ctx, byocId, func(state string) bool {
		return state == api.ByocStateTerminated
	}, byocDeleteWaitSeconds)
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Error waiting for BYOC infrastructure termination",
			fmt.Sprintf("BYOC infrastructure %s did not finish terminating: %s", byocId, err.Error()),
		)
	}
}

func (r *ByocInfrastructureResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning("clickhouse_byoc_infrastructure", &resp.Diagnostics)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	// Unlocks adopting write-only creation parameters over the imported null
	// state; nil only in hand-built test responses.
	if resp.Private != nil {
		byocMarkImported(ctx, resp.Private, &resp.Diagnostics)
	}
}

// syncByocResourceState refreshes the API-returned attributes of the model
// from the details and tags endpoints. Returns false (with diagnostics
// untouched beyond mapping errors) when the refresh could not complete.
func (r *ByocInfrastructureResource) syncByocResourceState(ctx context.Context, byocId string, model *models.ByocInfrastructureResourceModel, diags *diag.Diagnostics) bool {
	details, err := r.client.GetByocInfrastructure(ctx, byocId)
	if err != nil {
		diags.AddWarning("Could not refresh BYOC infrastructure", fmt.Sprintf("Reading BYOC infrastructure %s failed: %s", byocId, err.Error()))
		return false
	}
	tags, err := r.client.GetByocInfrastructureTags(ctx, byocId)
	if err != nil {
		diags.AddWarning("Could not refresh BYOC infrastructure tags", fmt.Sprintf("Reading tags of BYOC infrastructure %s failed: %s", byocId, err.Error()))
		return false
	}
	d := applyByocDetailsToResourceState(ctx, details, tags, model)
	diags.Append(d...)
	return !d.HasError()
}

// applyByocDetailsToResourceState maps the read-back onto the model. Only
// API-returned attributes are touched: write-only creation parameters keep
// their configured values.
func applyByocDetailsToResourceState(ctx context.Context, details *api.ByocInfrastructureDetails, tags map[string]string, model *models.ByocInfrastructureResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.ID = types.StringValue(details.Id)
	model.State = types.StringValue(details.State)
	model.CloudProvider = types.StringValue(details.CloudProvider)
	model.RegionID = types.StringValue(details.RegionId)
	model.AccountID = types.StringValue(details.AccountId)
	model.DisplayName = types.StringValue(details.DisplayName)
	model.EnablePrivateLink = types.BoolPointerValue(details.EnablePrivateLink)
	model.EnablePrivateLoadBalancer = types.BoolPointerValue(details.EnablePrivateLoadBalancer)
	model.EnablePublicLoadBalancer = types.BoolPointerValue(details.EnablePublicLoadBalancer)
	model.GcpPscSubnetID = types.StringPointerValue(details.GcpPscSubnetId)
	model.VpcCidrRange = types.StringPointerValue(details.VpcCidrRange)
	model.IsByoVpc = types.BoolPointerValue(details.IsByoVpc)
	model.VpcID = types.StringPointerValue(details.ByoVpcId)
	model.GcpSharedVpcHostProjectID = types.StringPointerValue(details.ByoVpcSharedVpcHostProjectId)

	var d diag.Diagnostics
	model.PrivateSubnetIDs, d = stringSliceToList(details.ByoVpcPrivateSubnetIds)
	diags.Append(d...)
	model.GcpPodCidrRangeNames, d = stringSliceToList(details.ByoVpcPodCidrRangeNames)
	diags.Append(d...)

	if tags == nil {
		tags = map[string]string{}
	}
	tagsValue, d := types.MapValueFrom(ctx, types.StringType, tags)
	diags.Append(d...)
	model.Tags = tagsValue

	return diags
}

// applyByocSummaryFallback fills the model from the create response and nulls
// the remaining computed attributes so the state can be persisted even when
// the details read-back failed right after creation.
func applyByocSummaryFallback(infra *api.ByocInfrastructure, model *models.ByocInfrastructureResourceModel) {
	model.ID = types.StringValue(infra.Id)
	model.State = types.StringValue(infra.State)
	model.CloudProvider = types.StringValue(infra.CloudProvider)
	model.DisplayName = types.StringValue(infra.DisplayName)
	if model.VpcCidrRange.IsUnknown() {
		model.VpcCidrRange = types.StringNull()
	}
	if model.VpcID.IsUnknown() {
		model.VpcID = types.StringNull()
	}
	if model.GcpSharedVpcHostProjectID.IsUnknown() {
		model.GcpSharedVpcHostProjectID = types.StringNull()
	}
	if model.PrivateSubnetIDs.IsUnknown() {
		model.PrivateSubnetIDs = types.ListNull(types.StringType)
	}
	if model.GcpPodCidrRangeNames.IsUnknown() {
		model.GcpPodCidrRangeNames = types.ListNull(types.StringType)
	}
	if model.EnablePrivateLink.IsUnknown() {
		model.EnablePrivateLink = types.BoolNull()
	}
	if model.EnablePrivateLoadBalancer.IsUnknown() {
		model.EnablePrivateLoadBalancer = types.BoolNull()
	}
	if model.EnablePublicLoadBalancer.IsUnknown() {
		model.EnablePublicLoadBalancer = types.BoolNull()
	}
	if model.GcpPscSubnetID.IsUnknown() {
		model.GcpPscSubnetID = types.StringNull()
	}
	if model.Tags.IsUnknown() {
		model.Tags = types.MapNull(types.StringType)
	}
	if model.IsByoVpc.IsUnknown() {
		model.IsByoVpc = types.BoolNull()
	}
}

func byocCreateRequestFromModel(ctx context.Context, model *models.ByocInfrastructureResourceModel) (api.ByocInfrastructureCreateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	request := api.ByocInfrastructureCreateRequest{
		RegionId:                 model.RegionID.ValueString(),
		AccountId:                model.AccountID.ValueString(),
		ExternalId:               model.ExternalID.ValueStringPointer(),
		TenantId:                 model.TenantID.ValueStringPointer(),
		ServicePrincipalClientId: model.ServicePrincipalClientID.ValueStringPointer(),
	}
	if isKnown(model.VpcID) {
		request.VpcId = model.VpcID.ValueStringPointer()
	}
	if isKnown(model.GcpSharedVpcHostProjectID) {
		request.GcpSharedVpcHostProjectId = model.GcpSharedVpcHostProjectID.ValueStringPointer()
	}
	if isKnown(model.DisplayName) {
		request.DisplayName = model.DisplayName.ValueStringPointer()
	}
	if isKnown(model.VpcCidrRange) {
		request.VpcCidrRange = model.VpcCidrRange.ValueStringPointer()
	}

	var d diag.Diagnostics
	request.AvailabilityZoneSuffixes, d = stringListFromModel(ctx, model.AvailabilityZoneSuffixes)
	diags.Append(d...)
	request.PrivateSubnetIds, d = stringListFromModel(ctx, model.PrivateSubnetIDs)
	diags.Append(d...)
	request.PublicSubnetIds, d = stringListFromModel(ctx, model.PublicSubnetIDs)
	diags.Append(d...)
	request.GcpPodCidrRangeNames, d = stringListFromModel(ctx, model.GcpPodCidrRangeNames)
	diags.Append(d...)

	if isKnown(model.Tags) {
		tags := map[string]string{}
		diags.Append(model.Tags.ElementsAs(ctx, &tags, false)...)
		request.Tags = tags
	}

	return request, diags
}

func byocValidateRequestFromCreate(r api.ByocInfrastructureCreateRequest) api.ByocInfrastructureValidateRequest {
	return api.ByocInfrastructureValidateRequest{
		RegionId:                  r.RegionId,
		AccountId:                 r.AccountId,
		ExternalId:                r.ExternalId,
		TenantId:                  r.TenantId,
		ServicePrincipalClientId:  r.ServicePrincipalClientId,
		VpcCidrRange:              r.VpcCidrRange,
		AvailabilityZoneSuffixes:  r.AvailabilityZoneSuffixes,
		VpcId:                     r.VpcId,
		PrivateSubnetIds:          r.PrivateSubnetIds,
		PublicSubnetIds:           r.PublicSubnetIds,
		GcpPodCidrRangeNames:      r.GcpPodCidrRangeNames,
		GcpSharedVpcHostProjectId: r.GcpSharedVpcHostProjectId,
		Tags:                      r.Tags,
	}
}

// byocRequirePrivateLinkForPscSubnet re-checks the PSC-subnet invariant with
// resolved values: unknown interpolations bypass ValidateConfig, and silently
// forcing enablePrivateLink = true would contradict an explicit false.
func byocRequirePrivateLinkForPscSubnet(enablePrivateLink types.Bool, diags *diag.Diagnostics) bool {
	if isKnown(enablePrivateLink) && enablePrivateLink.ValueBool() {
		return true
	}
	diags.AddAttributeError(
		path.Root("gcp_psc_subnet_id"),
		"gcp_psc_subnet_id requires enable_private_link = true",
		"The API only accepts a Private Service Connect subnet together with private link enabled. Set enable_private_link = true explicitly in the configuration.",
	)
	return false
}

// byocPostCreatePatch returns the update needed to align the provisioned
// infrastructure with explicitly configured toggles and PSC subnet.
func byocPostCreatePatch(config *models.ByocInfrastructureResourceModel, details *api.ByocInfrastructureDetails) (api.ByocInfrastructureUpdateRequest, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	patch := api.ByocInfrastructureUpdateRequest{}
	hasPatch := false

	if isKnown(config.EnablePrivateLink) && boolDiffers(config.EnablePrivateLink.ValueBool(), details.EnablePrivateLink) {
		patch.EnablePrivateLink = config.EnablePrivateLink.ValueBoolPointer()
		hasPatch = true
	}
	if isKnown(config.EnablePrivateLoadBalancer) && boolDiffers(config.EnablePrivateLoadBalancer.ValueBool(), details.EnablePrivateLoadBalancer) {
		patch.EnablePrivateLoadBalancer = config.EnablePrivateLoadBalancer.ValueBoolPointer()
		hasPatch = true
	}
	if isKnown(config.EnablePublicLoadBalancer) && boolDiffers(config.EnablePublicLoadBalancer.ValueBool(), details.EnablePublicLoadBalancer) {
		patch.EnablePublicLoadBalancer = config.EnablePublicLoadBalancer.ValueBoolPointer()
		hasPatch = true
	}
	if isKnown(config.GcpPscSubnetID) && stringDiffers(config.GcpPscSubnetID.ValueString(), details.GcpPscSubnetId) {
		if !byocRequirePrivateLinkForPscSubnet(config.EnablePrivateLink, &diags) {
			return patch, false, diags
		}
		patch.GcpPscSubnetId = config.GcpPscSubnetID.ValueStringPointer()
		patch.EnablePrivateLink = config.EnablePrivateLink.ValueBoolPointer()
		hasPatch = true
	}

	return patch, hasPatch, diags
}

func byocUpdateRequestFromModels(ctx context.Context, plan *models.ByocInfrastructureResourceModel, state *models.ByocInfrastructureResourceModel) (api.ByocInfrastructureUpdateRequest, bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	patch := api.ByocInfrastructureUpdateRequest{}
	hasPatch := false

	if isKnown(plan.DisplayName) && !plan.DisplayName.Equal(state.DisplayName) {
		patch.DisplayName = plan.DisplayName.ValueStringPointer()
		hasPatch = true
	}
	if isKnown(plan.EnablePrivateLink) && !plan.EnablePrivateLink.Equal(state.EnablePrivateLink) {
		patch.EnablePrivateLink = plan.EnablePrivateLink.ValueBoolPointer()
		hasPatch = true
	}
	if isKnown(plan.EnablePrivateLoadBalancer) && !plan.EnablePrivateLoadBalancer.Equal(state.EnablePrivateLoadBalancer) {
		patch.EnablePrivateLoadBalancer = plan.EnablePrivateLoadBalancer.ValueBoolPointer()
		hasPatch = true
	}
	if isKnown(plan.EnablePublicLoadBalancer) && !plan.EnablePublicLoadBalancer.Equal(state.EnablePublicLoadBalancer) {
		patch.EnablePublicLoadBalancer = plan.EnablePublicLoadBalancer.ValueBoolPointer()
		hasPatch = true
	}
	if isKnown(plan.GcpPscSubnetID) && !plan.GcpPscSubnetID.Equal(state.GcpPscSubnetID) {
		if !byocRequirePrivateLinkForPscSubnet(plan.EnablePrivateLink, &diags) {
			return patch, false, diags
		}
		patch.GcpPscSubnetId = plan.GcpPscSubnetID.ValueStringPointer()
		patch.EnablePrivateLink = plan.EnablePrivateLink.ValueBoolPointer()
		hasPatch = true
	}
	if isKnown(plan.Tags) && !plan.Tags.Equal(state.Tags) {
		tags := map[string]string{}
		diags.Append(plan.Tags.ElementsAs(ctx, &tags, false)...)
		if diags.HasError() {
			return patch, false, diags
		}
		patch.Tags = &tags
		hasPatch = true
	}

	return patch, hasPatch, diags
}

func formatFailedByocChecks(validation *api.ByocInfrastructureValidation) string {
	var failed []string
	for _, check := range validation.Checks {
		if check.Allowed {
			continue
		}
		line := fmt.Sprintf("  - %s (%s)", check.Name, check.Action)
		if check.Reason != "" {
			line += ": " + check.Reason
		}
		failed = append(failed, line)
	}
	return fmt.Sprintf(
		"The preflight validation found %d failing cloud permission check(s):\n%s\n\nFix the cloud-side setup and retry, or set skip_preflight_validation = true to create anyway.",
		len(failed), strings.Join(failed, "\n"),
	)
}

type knowable interface {
	IsNull() bool
	IsUnknown() bool
}

func isKnown(v knowable) bool {
	return !v.IsNull() && !v.IsUnknown()
}

func boolDiffers(configured bool, actual *bool) bool {
	return actual == nil || *actual != configured
}

func stringDiffers(configured string, actual *string) bool {
	return actual == nil || *actual != configured
}

func stringListFromModel(ctx context.Context, list types.List) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return nil, diags
	}
	var out []string
	diags.Append(list.ElementsAs(ctx, &out, false)...)
	return out, diags
}

func stringSliceToList(values []string) (types.List, diag.Diagnostics) {
	if values == nil {
		return types.ListNull(types.StringType), nil
	}
	elements := make([]attr.Value, 0, len(values))
	for _, v := range values {
		elements = append(elements, types.StringValue(v))
	}
	return types.ListValue(types.StringType, elements)
}
