package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

type Endpoint struct {
	Protocol types.String `tfsdk:"protocol"`
	Host     types.String `tfsdk:"host"`
	Port     types.Int64  `tfsdk:"port"`
}

func (e Endpoint) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"protocol": types.StringType,
			"host":     types.StringType,
			"port":     types.Int64Type,
		},
	}
}

func (e Endpoint) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(e.ObjectType().AttrTypes, map[string]attr.Value{
		"protocol": e.Protocol,
		"host":     e.Host,
		"port":     e.Port,
	})
}

type IpAccessList struct {
	Source      types.String `tfsdk:"source"`
	Description types.String `tfsdk:"description"`
}

func (i IpAccessList) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"source":      types.StringType,
			"description": types.StringType,
		},
	}
}

func (i IpAccessList) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(i.ObjectType().AttrTypes, map[string]attr.Value{
		"source":      i.Source,
		"description": i.Description,
	})
}

type PrivateEndpointConfig struct {
	EndpointServiceID  types.String `tfsdk:"endpoint_service_id"`
	PrivateDNSHostname types.String `tfsdk:"private_dns_hostname"`
}

func (p PrivateEndpointConfig) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"endpoint_service_id":  types.StringType,
			"private_dns_hostname": types.StringType,
		},
	}
}

func (p PrivateEndpointConfig) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(p.ObjectType().AttrTypes, map[string]attr.Value{
		"endpoint_service_id":  p.EndpointServiceID,
		"private_dns_hostname": p.PrivateDNSHostname,
	})
}

type ServiceResourceModel struct {
	ID                              types.String `tfsdk:"id"`
	Name                            types.String `tfsdk:"name"`
	Password                        types.String `tfsdk:"password"`
	PasswordHash                    types.String `tfsdk:"password_hash"`
	DoubleSha1PasswordHash          types.String `tfsdk:"double_sha1_password_hash"`
	Endpoints                       types.List   `tfsdk:"endpoints"`
	CloudProvider                   types.String `tfsdk:"cloud_provider"`
	Region                          types.String `tfsdk:"region"`
	Tier                            types.String `tfsdk:"tier"`
	IdleScaling                     types.Bool   `tfsdk:"idle_scaling"`
	IpAccessList                    types.List   `tfsdk:"ip_access"`
	MinTotalMemoryGb                types.Int64  `tfsdk:"min_total_memory_gb"`
	MaxTotalMemoryGb                types.Int64  `tfsdk:"max_total_memory_gb"`
	NumReplicas                     types.Int64  `tfsdk:"num_replicas"`
	IdleTimeoutMinutes              types.Int64  `tfsdk:"idle_timeout_minutes"`
	IAMRole                         types.String `tfsdk:"iam_role"`
	LastUpdated                     types.String `tfsdk:"last_updated"`
	PrivateEndpointConfig           types.Object `tfsdk:"private_endpoint_config"`
	PrivateEndpointIds              types.List   `tfsdk:"private_endpoint_ids"`
	EncryptionKey                   types.String `tfsdk:"encryption_key"`
	EncryptionAssumedRoleIdentifier types.String `tfsdk:"encryption_assumed_role_identifier"`
}

func (m *ServiceResourceModel) Equals(b ServiceResourceModel) bool {
	if !m.ID.Equal(b.ID) ||
		!m.Name.Equal(b.Name) ||
		!m.Password.Equal(b.Password) ||
		!m.PasswordHash.Equal(b.PasswordHash) ||
		!m.DoubleSha1PasswordHash.Equal(b.DoubleSha1PasswordHash) ||
		!m.Endpoints.Equal(b.Endpoints) ||
		!m.CloudProvider.Equal(b.CloudProvider) ||
		!m.Region.Equal(b.Region) ||
		!m.Tier.Equal(b.Tier) ||
		!m.IdleScaling.Equal(b.IdleScaling) ||
		!m.MinTotalMemoryGb.Equal(b.MinTotalMemoryGb) ||
		!m.MaxTotalMemoryGb.Equal(b.MaxTotalMemoryGb) ||
		!m.NumReplicas.Equal(b.NumReplicas) ||
		!m.IdleTimeoutMinutes.Equal(b.IdleTimeoutMinutes) ||
		!m.IAMRole.Equal(b.IAMRole) ||
		!m.LastUpdated.Equal(b.LastUpdated) ||
		!m.PrivateEndpointConfig.Equal(b.PrivateEndpointConfig) ||
		!m.PrivateEndpointIds.Equal(b.PrivateEndpointIds) ||
		!m.EncryptionKey.Equal(b.EncryptionKey) ||
		!m.EncryptionAssumedRoleIdentifier.Equal(b.EncryptionAssumedRoleIdentifier) ||
		!m.IpAccessList.Equal(b.IpAccessList) {
		return false
	}

	return true
}
