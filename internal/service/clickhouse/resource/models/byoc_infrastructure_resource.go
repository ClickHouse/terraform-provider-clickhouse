package models

import "github.com/hashicorp/terraform-plugin-framework/types"

type ByocInfrastructureResourceModel struct {
	ID                        types.String `tfsdk:"id"`
	State                     types.String `tfsdk:"state"`
	CloudProvider             types.String `tfsdk:"cloud_provider"`
	RegionID                  types.String `tfsdk:"region_id"`
	AccountID                 types.String `tfsdk:"account_id"`
	DisplayName               types.String `tfsdk:"display_name"`
	ExternalID                types.String `tfsdk:"external_id"`
	TenantID                  types.String `tfsdk:"tenant_id"`
	ServicePrincipalClientID  types.String `tfsdk:"service_principal_client_id"`
	VpcCidrRange              types.String `tfsdk:"vpc_cidr_range"`
	AvailabilityZoneSuffixes  types.List   `tfsdk:"availability_zone_suffixes"`
	VpcID                     types.String `tfsdk:"vpc_id"`
	PrivateSubnetIDs          types.List   `tfsdk:"private_subnet_ids"`
	PublicSubnetIDs           types.List   `tfsdk:"public_subnet_ids"`
	GcpPodCidrRangeNames      types.List   `tfsdk:"gcp_pod_cidr_range_names"`
	GcpSharedVpcHostProjectID types.String `tfsdk:"gcp_shared_vpc_host_project_id"`
	EnablePrivateLink         types.Bool   `tfsdk:"enable_private_link"`
	EnablePrivateLoadBalancer types.Bool   `tfsdk:"enable_private_load_balancer"`
	EnablePublicLoadBalancer  types.Bool   `tfsdk:"enable_public_load_balancer"`
	GcpPscSubnetID            types.String `tfsdk:"gcp_psc_subnet_id"`
	Tags                      types.Map    `tfsdk:"tags"`
	IsByoVpc                  types.Bool   `tfsdk:"is_byo_vpc"`
	SkipPreflightValidation   types.Bool   `tfsdk:"skip_preflight_validation"`
}
