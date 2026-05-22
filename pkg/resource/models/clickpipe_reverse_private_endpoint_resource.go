package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// CustomPrivateDNSMappingModel describes a custom private DNS mapping.
type CustomPrivateDNSMappingModel struct {
	PrivateDNSName types.String `tfsdk:"private_dns_name"`
}

func (m CustomPrivateDNSMappingModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"private_dns_name": types.StringType,
		},
	}
}

func (m CustomPrivateDNSMappingModel) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"private_dns_name": m.PrivateDNSName,
	})
}

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
	GCPServiceAttachment       types.String `tfsdk:"gcp_service_attachment"`
	CustomPrivateDNSMappings   types.List   `tfsdk:"custom_private_dns_mappings"`
	EndpointID                 types.String `tfsdk:"endpoint_id"`
	DNSNames                   types.List   `tfsdk:"dns_names"`
	PrivateDNSNames            types.List   `tfsdk:"private_dns_names"`
	Status                     types.String `tfsdk:"status"`
}
