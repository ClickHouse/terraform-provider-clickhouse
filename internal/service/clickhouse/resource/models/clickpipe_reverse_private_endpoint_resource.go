package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// CustomPrivateDNSMappingModel describes a custom private DNS mapping.
type CustomPrivateDNSMappingModel struct {
	PrivateDNSName types.String `tfsdk:"private_dns_name"`
	TargetID       types.String `tfsdk:"target_id"`
}

func (m CustomPrivateDNSMappingModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"private_dns_name": types.StringType,
			"target_id":        types.StringType,
		},
	}
}

func (m CustomPrivateDNSMappingModel) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"private_dns_name": m.PrivateDNSName,
		"target_id":        m.TargetID,
	})
}

// PrivateDNSMappingModel describes a read-only private DNS mapping reported by a reverse private endpoint.
type PrivateDNSMappingModel struct {
	PrivateDNSName  types.String `tfsdk:"private_dns_name"`
	InternalDNSName types.String `tfsdk:"internal_dns_name"`
}

func (m PrivateDNSMappingModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"private_dns_name":  types.StringType,
			"internal_dns_name": types.StringType,
		},
	}
}

func (m PrivateDNSMappingModel) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"private_dns_name":  m.PrivateDNSName,
		"internal_dns_name": m.InternalDNSName,
	})
}

// DNSTargetModel describes a read-only DNS target reported by a reverse private endpoint.
type DNSTargetModel struct {
	ID              types.String `tfsdk:"id"`
	Kind            types.String `tfsdk:"kind"`
	InternalDNSName types.String `tfsdk:"internal_dns_name"`
}

func (m DNSTargetModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":                types.StringType,
			"kind":              types.StringType,
			"internal_dns_name": types.StringType,
		},
	}
}

func (m DNSTargetModel) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"id":                m.ID,
		"kind":              m.Kind,
		"internal_dns_name": m.InternalDNSName,
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
	EndpointID                 types.String `tfsdk:"endpoint_id"`
	DNSNames                   types.List   `tfsdk:"dns_names"`
	PrivateDNSNames            types.List   `tfsdk:"private_dns_names"`
	PrivateDNSMappings         types.List   `tfsdk:"private_dns_mappings"`
	DNSTargets                 types.List   `tfsdk:"dns_targets"`
	Status                     types.String `tfsdk:"status"`
}

// ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel describes custom private DNS mappings for a reverse private endpoint.
type ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel struct {
	ID                       types.String `tfsdk:"id"`
	ServiceID                types.String `tfsdk:"service_id"`
	ReversePrivateEndpointID types.String `tfsdk:"reverse_private_endpoint_id"`
	Mapping                  types.List   `tfsdk:"mapping"`
}
