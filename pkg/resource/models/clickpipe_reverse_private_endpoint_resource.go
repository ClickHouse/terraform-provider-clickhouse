//go:build alpha

package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ClickPipeReversePrivateEndpointResourceModel describes the resource data model.
type ClickPipeReversePrivateEndpointResourceModel struct {
	ID                         types.String `tfsdk:"id"`
	ServiceID                  types.String `tfsdk:"service_id"`
	Description                types.String `tfsdk:"description"`
	Type                       types.String `tfsdk:"type"`
	VPCEndpointServiceName     types.String `tfsdk:"vpc_endpoint_service_name"`
	VPCResourceConfigurationID types.String `tfsdk:"vpc_resource_configuration_id"`
	VPCResourceShareArn        types.String `tfsdk:"vpc_resource_share_arn"`
	MSKClusterArn              types.String `tfsdk:"msk_cluster_arn"`
	MSKAuthentication          types.String `tfsdk:"msk_authentication"`
	EndpointID                 types.String `tfsdk:"endpoint_id"`
	DNSNames                   types.List   `tfsdk:"dns_names"`
	PrivateDNSNames            types.List   `tfsdk:"private_dns_names"`
	Status                     types.String `tfsdk:"status"`
}
