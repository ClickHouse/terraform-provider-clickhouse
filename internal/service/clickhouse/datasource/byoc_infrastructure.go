package datasource

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

//go:embed descriptions/byoc_infrastructure.md
var byocInfrastructureDataSourceDescription string

var _ datasource.DataSource = &byocInfrastructureDataSource{}

func NewByocInfrastructureDataSource() datasource.DataSource { return &byocInfrastructureDataSource{} }

type byocInfrastructureDataSource struct{ client api.Client }

type byocInfrastructureDataSourceModel struct {
	ID                           types.String `tfsdk:"id"`
	State                        types.String `tfsdk:"state"`
	CloudProvider                types.String `tfsdk:"cloud_provider"`
	RegionID                     types.String `tfsdk:"region_id"`
	AccountID                    types.String `tfsdk:"account_id"`
	DisplayName                  types.String `tfsdk:"display_name"`
	EnablePrivateLink            types.Bool   `tfsdk:"enable_private_link"`
	EnablePrivateLoadBalancer    types.Bool   `tfsdk:"enable_private_load_balancer"`
	EnablePublicLoadBalancer     types.Bool   `tfsdk:"enable_public_load_balancer"`
	GcpPscSubnetID               types.String `tfsdk:"gcp_psc_subnet_id"`
	VpcCidrRange                 types.String `tfsdk:"vpc_cidr_range"`
	VpcAvailabilityZoneList      types.List   `tfsdk:"vpc_availability_zone_list"`
	IsByoVpc                     types.Bool   `tfsdk:"is_byo_vpc"`
	ByoVpcID                     types.String `tfsdk:"byo_vpc_id"`
	ByoVpcPrivateSubnetIDs       types.List   `tfsdk:"byo_vpc_private_subnet_ids"`
	ByoVpcPodCidrRangeNames      types.List   `tfsdk:"byo_vpc_pod_cidr_range_names"`
	ByoVpcSharedVpcHostProjectID types.String `tfsdk:"byo_vpc_shared_vpc_host_project_id"`
	Tags                         types.Map    `tfsdk:"tags"`
	PrivateEndpointConfig        types.Object `tfsdk:"private_endpoint_config"`
}

func byocPrivateEndpointConfigObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"endpoint_name":        types.StringType,
			"private_dns_hostname": types.StringType,
		},
	}
}

// byocInfrastructureDetailsToModel maps the API read-back onto the data source
// model. Extracted from Read so the mapping is independently testable. Absent
// optional fields become null; nil string slices become known empty lists.
func byocInfrastructureDetailsToModel(ctx context.Context, details *api.ByocInfrastructureDetails, tags map[string]string, model *byocInfrastructureDataSourceModel) diag.Diagnostics {
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
	model.ByoVpcID = types.StringPointerValue(details.ByoVpcId)
	model.ByoVpcSharedVpcHostProjectID = types.StringPointerValue(details.ByoVpcSharedVpcHostProjectId)

	var d diag.Diagnostics
	model.VpcAvailabilityZoneList, d = stringSliceToListValue(details.VpcAvailabilityZoneList)
	diags.Append(d...)
	model.ByoVpcPrivateSubnetIDs, d = stringSliceToListValue(details.ByoVpcPrivateSubnetIds)
	diags.Append(d...)
	model.ByoVpcPodCidrRangeNames, d = stringSliceToListValue(details.ByoVpcPodCidrRangeNames)
	diags.Append(d...)

	if tags == nil {
		tags = map[string]string{}
	}
	tagsValue, d := types.MapValueFrom(ctx, types.StringType, tags)
	diags.Append(d...)
	model.Tags = tagsValue

	return diags
}

func stringSliceToListValue(items []string) (types.List, diag.Diagnostics) {
	elems := make([]attr.Value, 0, len(items))
	for _, it := range items {
		elems = append(elems, types.StringValue(it))
	}
	return types.ListValue(types.StringType, elems)
}

func byocPrivateEndpointConfigToObjectValue(cfg *api.ByocInfrastructurePrivateEndpointConfig) (types.Object, diag.Diagnostics) {
	if cfg == nil {
		return types.ObjectNull(byocPrivateEndpointConfigObjectType().AttrTypes), nil
	}
	return types.ObjectValue(byocPrivateEndpointConfigObjectType().AttrTypes, map[string]attr.Value{
		"endpoint_name":        types.StringValue(cfg.EndpointName),
		"private_dns_hostname": types.StringValue(cfg.PrivateDnsHostname),
	})
}

func (d *byocInfrastructureDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError("ClickHouse Cloud API not configured",
			"This data source requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).")
		return
	}
	d.client = providerData.API
}

func (d *byocInfrastructureDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_byoc_infrastructure"
}

func (d *byocInfrastructureDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: byocInfrastructureDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the BYOC infrastructure.",
				Required:    true,
			},
			"state": schema.StringAttribute{
				Description: "Provisioning state of the infrastructure (e.g. 'infra-provisioning', 'infra-ready', 'infra-terminating').",
				Computed:    true,
			},
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider the infrastructure runs in ('aws', 'gcp' or 'azure').",
				Computed:    true,
			},
			"region_id": schema.StringAttribute{
				Description: "Region the infrastructure is deployed in (e.g. 'us-east-1').",
				Computed:    true,
			},
			"account_id": schema.StringAttribute{
				Description: "Customer cloud account the infrastructure is deployed into: AWS account ID, GCP project ID or Azure subscription ID.",
				Computed:    true,
			},
			"display_name": schema.StringAttribute{
				Description: "Human readable name of the infrastructure.",
				Computed:    true,
			},
			"enable_private_link": schema.BoolAttribute{
				Description: "Whether private link connectivity (AWS PrivateLink, GCP Private Service Connect or Azure Private Link) is enabled.",
				Computed:    true,
			},
			"enable_private_load_balancer": schema.BoolAttribute{
				Description: "Whether the internal (VPC-private) load balancer is enabled.",
				Computed:    true,
			},
			"enable_public_load_balancer": schema.BoolAttribute{
				Description: "Whether the internet-facing load balancer is enabled.",
				Computed:    true,
			},
			"gcp_psc_subnet_id": schema.StringAttribute{
				Description: "GCP only: subnet used for the Private Service Connect NAT. Null unless private link is enabled on a GCP infrastructure.",
				Computed:    true,
			},
			"vpc_cidr_range": schema.StringAttribute{
				Description: "CIDR range of the infrastructure VPC. Null for BYO-VPC infrastructures.",
				Computed:    true,
			},
			"vpc_availability_zone_list": schema.ListAttribute{
				Description: "Availability zones the infrastructure VPC spans.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"is_byo_vpc": schema.BoolAttribute{
				Description: "Whether the infrastructure runs inside a customer-managed VPC instead of a ClickHouse-managed one.",
				Computed:    true,
			},
			"byo_vpc_id": schema.StringAttribute{
				Description: "ID of the customer-managed VPC. Null unless 'is_byo_vpc' is true.",
				Computed:    true,
			},
			"byo_vpc_private_subnet_ids": schema.ListAttribute{
				Description: "Private subnet IDs of the customer-managed VPC. Empty unless 'is_byo_vpc' is true.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"byo_vpc_pod_cidr_range_names": schema.ListAttribute{
				Description: "GCP only: names of the secondary ranges used for pod IPs in the customer-managed VPC.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"byo_vpc_shared_vpc_host_project_id": schema.StringAttribute{
				Description: "GCP only: host project of the shared VPC, when the customer-managed VPC is a shared VPC.",
				Computed:    true,
			},
			"tags": schema.MapAttribute{
				Description: "Tags applied to the cloud resources of the infrastructure.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"private_endpoint_config": schema.SingleNestedAttribute{
				Description: "Private link endpoint service of the infrastructure. Null unless private link is enabled.",
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					"endpoint_name": schema.StringAttribute{
						Description: "Name of the endpoint service: AWS VPC endpoint service name, GCP Private Service Connect service attachment or Azure Private Link service alias.",
						Computed:    true,
					},
					"private_dns_hostname": schema.StringAttribute{
						Description: "Private DNS hostname of the VPC the infrastructure runs in.",
						Computed:    true,
					},
				},
			},
		},
	}
}

func (d *byocInfrastructureDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	utils.BetaWarning("clickhouse_byoc_infrastructure", &resp.Diagnostics)

	var cfg byocInfrastructureDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	byocId := cfg.ID.ValueString()

	details, err := d.client.GetByocInfrastructure(ctx, byocId)
	if err != nil {
		resp.Diagnostics.AddError("Error reading BYOC infrastructure",
			fmt.Sprintf("Could not read BYOC infrastructure %s: %s", byocId, err.Error()))
		return
	}

	tags, err := d.client.GetByocInfrastructureTags(ctx, byocId)
	if err != nil {
		resp.Diagnostics.AddError("Error reading BYOC infrastructure tags",
			fmt.Sprintf("Could not read tags of BYOC infrastructure %s: %s", byocId, err.Error()))
		return
	}

	resp.Diagnostics.Append(byocInfrastructureDetailsToModel(ctx, details, tags, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var endpointConfig *api.ByocInfrastructurePrivateEndpointConfig
	if details.EnablePrivateLink != nil && *details.EnablePrivateLink {
		endpointConfig, err = d.client.GetByocInfrastructurePrivateEndpointConfig(ctx, byocId)
		// The endpoint service only exists once provisioning finished; a 404
		// while the infra is still provisioning is a null config, not an error.
		if err != nil && !api.IsNotFound(err) {
			resp.Diagnostics.AddError("Error reading BYOC infrastructure private endpoint config",
				fmt.Sprintf("Could not read private endpoint config of BYOC infrastructure %s: %s", byocId, err.Error()))
			return
		}
	}
	endpointObj, diags := byocPrivateEndpointConfigToObjectValue(endpointConfig)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg.PrivateEndpointConfig = endpointObj

	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
