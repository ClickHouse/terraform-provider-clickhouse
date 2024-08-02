package clickhouse

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ServiceResourceModel struct {
	ID                              types.String    `tfsdk:"id"`
	Name                            types.String    `tfsdk:"name"`
	Password                        types.String    `tfsdk:"password"`
	PasswordHash                    types.String    `tfsdk:"password_hash"`
	DoubleSha1PasswordHash          types.String    `tfsdk:"double_sha1_password_hash"`
	Endpoints                       types.List      `tfsdk:"endpoints"`
	CloudProvider                   types.String    `tfsdk:"cloud_provider"`
	Region                          types.String    `tfsdk:"region"`
	Tier                            types.String    `tfsdk:"tier"`
	IdleScaling                     types.Bool      `tfsdk:"idle_scaling"`
	IpAccessList                    []IpAccessModel `tfsdk:"ip_access"`
	MinTotalMemoryGb                types.Int64     `tfsdk:"min_total_memory_gb"`
	MaxTotalMemoryGb                types.Int64     `tfsdk:"max_total_memory_gb"`
	NumReplicas                     types.Int64     `tfsdk:"num_replicas"`
	IdleTimeoutMinutes              types.Int64     `tfsdk:"idle_timeout_minutes"`
	IAMRole                         types.String    `tfsdk:"iam_role"`
	LastUpdated                     types.String    `tfsdk:"last_updated"`
	PrivateEndpointConfig           types.Object    `tfsdk:"private_endpoint_config"`
	PrivateEndpointIds              types.List      `tfsdk:"private_endpoint_ids"`
	EncryptionKey                   types.String    `tfsdk:"encryption_key"`
	EncryptionAssumedRoleIdentifier types.String    `tfsdk:"encryption_assumed_role_identifier"`
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
		!m.EncryptionAssumedRoleIdentifier.Equal(b.EncryptionAssumedRoleIdentifier) {
		return false
	}

	if len(m.IpAccessList) != len(b.IpAccessList) {
		return false
	}
	for i, ipAccess := range b.IpAccessList {
		if !ipAccess.Equal(b.IpAccessList[i]) {
			return false
		}
	}

	return true
}
