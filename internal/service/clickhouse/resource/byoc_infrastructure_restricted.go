package resource

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

const byocAccessModelReplaceDescription = "Requires replacement when the access model changes; a state without a recorded access model (written by an older provider version) is standard and is not replaced."

// byocAccessModelRequiresReplace replaces only on a real change: states
// written before access_model existed hold null, which is standard.
func byocAccessModelRequiresReplace() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			if !isKnown(req.PlanValue) {
				return
			}
			prior := req.StateValue.ValueString()
			if req.StateValue.IsNull() {
				prior = api.ByocAccessModelStandard
			}
			resp.RequiresReplace = prior != req.PlanValue.ValueString()
		},
		byocAccessModelReplaceDescription, byocAccessModelReplaceDescription,
	)
}

const byocFactsPresenceReplaceDescription = "Requires replacement when the registration is added or removed."

// byocRestrictedFactsPresenceRequiresReplace replaces only on null <-> set:
// per-attribute changes are handled by the nested attributes, because the
// parent runs its modifiers before the children resolve their unknowns.
func byocRestrictedFactsPresenceRequiresReplace() planmodifier.Object {
	return objectplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.ObjectRequest, resp *objectplanmodifier.RequiresReplaceIfFuncResponse) {
			if req.PlanValue.IsUnknown() || req.State.Raw.IsNull() {
				return
			}
			resp.RequiresReplace = req.StateValue.IsNull() != req.PlanValue.IsNull()
		},
		byocFactsPresenceReplaceDescription, byocFactsPresenceReplaceDescription,
	)
}

func byocBoolRequiresReplaceIfKnown() planmodifier.Bool {
	return boolplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.BoolRequest, resp *boolplanmodifier.RequiresReplaceIfFuncResponse) {
			resp.RequiresReplace = isKnown(req.PlanValue)
		},
		byocReplaceIfKnownDescription, byocReplaceIfKnownDescription,
	)
}

const byocFactOptionalSuffix = " Omitting it keeps the registered value (the API default on create)."

func byocRequiredFactString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Description: description,
		Required:    true,
		Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
}

// Optional facts are computed so an omitted value adopts the registered one
// (API defaults, or values set when registered outside Terraform and
// imported) instead of forcing a replacement.
func byocOptionalFactString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Description: description + byocFactOptionalSuffix,
		Optional:    true,
		Computed:    true,
		Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
			byocStringRequiresReplaceIfKnown(),
		},
	}
}

func byocOptionalFactList(description string, validators ...validator.List) schema.ListAttribute {
	return schema.ListAttribute{
		Description: description + byocFactOptionalSuffix,
		Optional:    true,
		Computed:    true,
		ElementType: types.StringType,
		Validators:  validators,
		PlanModifiers: []planmodifier.List{
			listplanmodifier.UseStateForUnknown(),
			byocListRequiresReplaceIfKnown(),
		},
	}
}

func byocOptionalFactBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		Description: description + byocFactOptionalSuffix,
		Optional:    true,
		Computed:    true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseStateForUnknown(),
			byocBoolRequiresReplaceIfKnown(),
		},
	}
}

func byocAccessModelSchema() schema.StringAttribute {
	return schema.StringAttribute{
		Description: "Access model of the infrastructure: 'standard' (ClickHouse provisions and operates the data plane through the onboarding role) or 'restricted' (the customer provisions the Kubernetes cluster, IAM and storage; ClickHouse operates it only through the connector the customer installs). Defaults to 'standard'. Changing it requires replacement.",
		Optional:    true,
		Computed:    true,
		Default:     stringdefault.StaticString(api.ByocAccessModelStandard),
		Validators: []validator.String{
			stringvalidator.OneOf(api.ByocAccessModelStandard, api.ByocAccessModelRestricted),
		},
		PlanModifiers: []planmodifier.String{
			byocAccessModelRequiresReplace(),
		},
	}
}

func byocRestrictedFactsSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "Restricted access only, and required then: registration of the customer-provisioned cluster. The API does not accept changes after registration, so changing any attribute requires replacement.",
		Optional:    true,
		PlanModifiers: []planmodifier.Object{
			byocRestrictedFactsPresenceRequiresReplace(),
		},
		Attributes: map[string]schema.Attribute{
			"cluster_name":        byocRequiredFactString("Name of the customer-provisioned EKS cluster."),
			"platform_node_group": byocOptionalFactString("Value of the 'clickhouseGroup' node label that selects the nodes for the ClickHouse platform components. The API defaults to 'system'."),
			"availability_zones": byocOptionalFactList(
				"Availability zones (e.g. ['us-east-1a', 'us-east-1b', 'us-east-1c']) the cluster nodes run in; at least three.",
				listvalidator.SizeAtLeast(3),
				listvalidator.UniqueValues(),
			),
			"namespace_prefix":  byocOptionalFactString("Prefix of the Kubernetes namespaces ClickHouse creates in the cluster."),
			"executor_role_arn": byocOptionalFactString("Direct image mode only, and required then: IAM role ARN of the connector executor, trusted to pull platform images."),
			"node_role_arns": byocOptionalFactList(
				"Direct image mode only, and required then: IAM role ARNs of the cluster nodes, trusted to pull platform images.",
				listvalidator.SizeAtLeast(1),
				listvalidator.UniqueValues(),
			),
			"image_mode": schema.StringAttribute{
				Description: "Where the cluster pulls platform images from: 'direct' (the ClickHouse registry) or 'bundle' (a customer registry mirrored from the ClickHouse image bundle).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(api.ByocImageModeDirect, api.ByocImageModeBundle),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"image_registry":             byocOptionalFactString("Bundle image mode only, and required then: the customer registry platform images are pulled from."),
			"data_bucket":                byocRequiredFactString("S3 bucket for service data."),
			"backup_bucket":              byocRequiredFactString("S3 bucket for service backups."),
			"storage_role_arn":           byocRequiredFactString("IAM role ARN services assume to access the data and backup buckets."),
			"scraper_endpoint":           byocOptionalFactString("Endpoint the connector scraper ships metrics to."),
			"enable_monitoring":          byocOptionalFactBool("Install the monitoring stack in the cluster."),
			"enable_snapshot_controller": byocOptionalFactBool("Install the volume snapshot controller in the cluster."),
			"enable_autoscaler":          byocOptionalFactBool("Install the node autoscaler in the cluster."),
		},
	}
}

// byocStandardOnlyAttributes are rejected for restricted access: the API
// refuses them on create, and the customer owns the network and the cluster.
func byocStandardOnlyAttributes(model *models.ByocInfrastructureResourceModel) []struct {
	name  string
	value knowable
} {
	return []struct {
		name  string
		value knowable
	}{
		{"external_id", model.ExternalID},
		{"tenant_id", model.TenantID},
		{"service_principal_client_id", model.ServicePrincipalClientID},
		{"vpc_cidr_range", model.VpcCidrRange},
		{"availability_zone_suffixes", model.AvailabilityZoneSuffixes},
		{"vpc_id", model.VpcID},
		{"private_subnet_ids", model.PrivateSubnetIDs},
		{"public_subnet_ids", model.PublicSubnetIDs},
		{"gcp_pod_cidr_range_names", model.GcpPodCidrRangeNames},
		{"gcp_shared_vpc_host_project_id", model.GcpSharedVpcHostProjectID},
		{"enable_private_link", model.EnablePrivateLink},
		{"enable_private_load_balancer", model.EnablePrivateLoadBalancer},
		{"enable_public_load_balancer", model.EnablePublicLoadBalancer},
		{"gcp_psc_subnet_id", model.GcpPscSubnetID},
		{"customer_authorized_access", model.CustomerAuthorizedAccess},
	}
}

func byocConfigIsRestricted(model *models.ByocInfrastructureResourceModel) bool {
	return isKnown(model.AccessModel) && api.IsRestrictedAccess(model.AccessModel.ValueString())
}

// byocValidateAccessModel checks the restricted / standard split at plan
// time, so a configuration the API would refuse fails before any call.
func byocValidateAccessModel(ctx context.Context, model *models.ByocInfrastructureResourceModel, diags *diag.Diagnostics) {
	if model.AccessModel.IsUnknown() {
		return
	}
	if !byocConfigIsRestricted(model) {
		if !model.RestrictedFacts.IsNull() {
			diags.AddAttributeError(
				path.Root("restricted_facts"),
				"restricted_facts requires access_model = \"restricted\"",
				"The registration of a customer-provisioned cluster only applies to restricted-access infrastructures. Set access_model = \"restricted\" or remove restricted_facts.",
			)
		}
		return
	}

	if model.RestrictedFacts.IsNull() {
		diags.AddAttributeError(
			path.Root("restricted_facts"),
			"Missing restricted_facts",
			"A restricted-access infrastructure registers a customer-provisioned cluster. Set restricted_facts.",
		)
	}
	for _, attribute := range byocStandardOnlyAttributes(model) {
		if !attribute.value.IsNull() {
			diags.AddAttributeError(
				path.Root(attribute.name),
				fmt.Sprintf("%s is not supported with access_model = \"restricted\"", attribute.name),
				"The customer provisions the network and the cluster of a restricted-access infrastructure, and ClickHouse cannot change cloud resources in the account. Remove the attribute.",
			)
		}
	}
	if isKnown(model.Tags) && len(model.Tags.Elements()) > 0 {
		diags.AddAttributeError(
			path.Root("tags"),
			"tags is not supported with access_model = \"restricted\"",
			"ClickHouse cannot tag cloud resources in a restricted-access account. Tag them in the configuration that provisions them.",
		)
	}

	if !isKnown(model.RestrictedFacts) {
		return
	}
	var facts models.ByocRestrictedFactsModel
	diags.Append(model.RestrictedFacts.As(ctx, &facts, basetypes.ObjectAsOptions{})...)
	if diags.HasError() || !isKnown(facts.ImageMode) {
		return
	}
	factPath := path.Root("restricted_facts")
	switch facts.ImageMode.ValueString() {
	case api.ByocImageModeDirect:
		if facts.ExecutorRoleArn.IsNull() {
			diags.AddAttributeError(factPath.AtName("executor_role_arn"), "Missing executor_role_arn", "Direct image mode pulls platform images with the connector executor role. Set executor_role_arn.")
		}
		if facts.NodeRoleArns.IsNull() {
			diags.AddAttributeError(factPath.AtName("node_role_arns"), "Missing node_role_arns", "Direct image mode pulls platform images with the cluster node roles. Set node_role_arns.")
		}
	case api.ByocImageModeBundle:
		if facts.ImageRegistry.IsNull() {
			diags.AddAttributeError(factPath.AtName("image_registry"), "Missing image_registry", "Bundle image mode pulls platform images from the customer registry. Set image_registry.")
		}
	}
}
