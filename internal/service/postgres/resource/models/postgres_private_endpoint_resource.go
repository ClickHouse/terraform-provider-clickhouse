package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// PostgresPrivateEndpointResourceModel is the Terraform plan/state model for
// the clickhouse_postgres_private_endpoint resource.
type PostgresPrivateEndpointResourceModel struct {
	PostgresID          types.String `tfsdk:"postgres_id"`
	SupportedRegions    types.Set    `tfsdk:"supported_regions"`
	State               types.String `tfsdk:"state"`
	EndpointServiceName types.String `tfsdk:"endpoint_service_name"`
	PrivateHostname     types.String `tfsdk:"private_hostname"`
}

// PostgresPrivateEndpointAttachmentResourceModel is the Terraform plan/state
// model for the clickhouse_postgres_private_endpoint_attachment resource.
type PostgresPrivateEndpointAttachmentResourceModel struct {
	PostgresID          types.String `tfsdk:"postgres_id"`
	AllowedVpcEndpoints types.Set    `tfsdk:"allowed_vpc_endpoints"`
}

// PostgresAllowedVpcEndpointModel is one element of allowed_vpc_endpoints.
type PostgresAllowedVpcEndpointModel struct {
	VpcEndpointID types.String `tfsdk:"vpc_endpoint_id"`
	Description   types.String `tfsdk:"description"`
}

func (PostgresAllowedVpcEndpointModel) ObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"vpc_endpoint_id": types.StringType, "description": types.StringType,
	}}
}
