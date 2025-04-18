package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

type IPAccessList struct {
	Source      types.String `tfsdk:"source"`
	Description types.String `tfsdk:"description"`
}

func (i IPAccessList) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"source":      types.StringType,
			"description": types.StringType,
		},
	}
}

func (i IPAccessList) ObjectValue() basetypes.ObjectValue {
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

type Endpoints struct {
	NativeSecure types.Object `tfsdk:"nativesecure"`
	HTTPS        types.Object `tfsdk:"https"`
	MySQL        types.Object `tfsdk:"mysql"`
}

func (q Endpoints) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"nativesecure": types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"host": types.StringType,
					"port": types.Int32Type,
				},
			},
			"https": types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"host": types.StringType,
					"port": types.Int32Type,
				},
			},
			"mysql": types.ObjectType{
				AttrTypes: map[string]attr.Type{
					"enabled": types.BoolType,
					"host":    types.StringType,
					"port":    types.Int32Type,
				},
			},
		},
	}
}

func (q Endpoints) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(q.ObjectType().AttrTypes, map[string]attr.Value{
		"nativesecure": q.NativeSecure,
		"https":        q.HTTPS,
		"mysql":        q.MySQL,
	})
}

type Endpoint struct {
	Host types.String `tfsdk:"host"`
	Port types.Int32  `tfsdk:"port"`
}

func (e Endpoint) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"host": types.StringType,
			"port": types.Int32Type,
		},
	}
}

func (e Endpoint) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(e.ObjectType().AttrTypes, map[string]attr.Value{
		"host": e.Host,
		"port": e.Port,
	})
}

type OptionalEndpoint struct {
	Enabled types.Bool   `tfsdk:"enabled"`
	Host    types.String `tfsdk:"host"`
	Port    types.Int32  `tfsdk:"port"`
}

func (e OptionalEndpoint) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"enabled": types.BoolType,
			"host":    types.StringType,
			"port":    types.Int32Type,
		},
	}
}

func (e OptionalEndpoint) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(e.ObjectType().AttrTypes, map[string]attr.Value{
		"enabled": e.Enabled,
		"host":    e.Host,
		"port":    e.Port,
	})
}

type TransparentEncryptionData struct {
	Enabled types.Bool   `tfsdk:"enabled"`
	RoleID  types.String `tfsdk:"role_id"`
}

func (t TransparentEncryptionData) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"enabled": types.BoolType,
			"role_id": types.StringType,
		},
	}
}

func (t TransparentEncryptionData) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(t.ObjectType().AttrTypes, map[string]attr.Value{
		"enabled": t.Enabled,
		"role_id": t.RoleID,
	})
}

type QueryAPIEndpoints struct {
	APIKeyIDs      types.List   `tfsdk:"api_key_ids"`
	Roles          types.List   `tfsdk:"roles"`
	AllowedOrigins types.String `tfsdk:"allowed_origins"`
}

func (q QueryAPIEndpoints) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"api_key_ids":     types.ListType{ElemType: types.StringType},
			"roles":           types.ListType{ElemType: types.StringType},
			"allowed_origins": types.StringType,
		},
	}
}

func (q QueryAPIEndpoints) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(q.ObjectType().AttrTypes, map[string]attr.Value{
		"api_key_ids":     q.APIKeyIDs,
		"roles":           q.Roles,
		"allowed_origins": q.AllowedOrigins,
	})
}

type BackupConfiguration struct {
	BackupPeriodInHours          types.Int32  `tfsdk:"backup_period_in_hours"`
	BackupRetentionPeriodInHours types.Int32  `tfsdk:"backup_retention_period_in_hours"`
	BackupStartTime              types.String `tfsdk:"backup_start_time"`
}

func (b BackupConfiguration) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"backup_period_in_hours":           types.Int32Type,
			"backup_retention_period_in_hours": types.Int32Type,
			"backup_start_time":                types.StringType,
		},
	}
}

func (b BackupConfiguration) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(b.ObjectType().AttrTypes, map[string]attr.Value{
		"backup_period_in_hours":           b.BackupPeriodInHours,
		"backup_retention_period_in_hours": b.BackupRetentionPeriodInHours,
		"backup_start_time":                b.BackupStartTime,
	})
}

type ServiceResourceModel struct {
	ID                              types.String `tfsdk:"id"`
	BYOCID                          types.String `tfsdk:"byoc_id"`
	DataWarehouseID                 types.String `tfsdk:"warehouse_id"`
	IsPrimary                       types.Bool   `tfsdk:"is_primary"`
	ReadOnly                        types.Bool   `tfsdk:"readonly"`
	Name                            types.String `tfsdk:"name"`
	Password                        types.String `tfsdk:"password"`
	PasswordHash                    types.String `tfsdk:"password_hash"`
	DoubleSha1PasswordHash          types.String `tfsdk:"double_sha1_password_hash"`
	Endpoints                       types.Object `tfsdk:"endpoints"`
	CloudProvider                   types.String `tfsdk:"cloud_provider"`
	Region                          types.String `tfsdk:"region"`
	Tier                            types.String `tfsdk:"tier"`
	ReleaseChannel                  types.String `tfsdk:"release_channel"`
	IdleScaling                     types.Bool   `tfsdk:"idle_scaling"`
	IpAccessList                    types.List   `tfsdk:"ip_access"`
	MinTotalMemoryGb                types.Int64  `tfsdk:"min_total_memory_gb"`
	MaxTotalMemoryGb                types.Int64  `tfsdk:"max_total_memory_gb"`
	MinReplicaMemoryGb              types.Int64  `tfsdk:"min_replica_memory_gb"`
	MaxReplicaMemoryGb              types.Int64  `tfsdk:"max_replica_memory_gb"`
	NumReplicas                     types.Int64  `tfsdk:"num_replicas"`
	IdleTimeoutMinutes              types.Int64  `tfsdk:"idle_timeout_minutes"`
	IAMRole                         types.String `tfsdk:"iam_role"`
	PrivateEndpointConfig           types.Object `tfsdk:"private_endpoint_config"`
	EncryptionKey                   types.String `tfsdk:"encryption_key"`
	EncryptionAssumedRoleIdentifier types.String `tfsdk:"encryption_assumed_role_identifier"`
	TransparentEncryptionData       types.Object `tfsdk:"transparent_data_encryption"`
	QueryAPIEndpoints               types.Object `tfsdk:"query_api_endpoints"`
	BackupConfiguration             types.Object `tfsdk:"backup_configuration"`
}

func (m *ServiceResourceModel) Equals(b ServiceResourceModel) bool {
	if !m.ID.Equal(b.ID) ||
		!m.BYOCID.Equal(b.BYOCID) ||
		!m.DataWarehouseID.Equal(b.DataWarehouseID) ||
		!m.ReadOnly.Equal(b.ReadOnly) ||
		!m.IsPrimary.Equal(b.IsPrimary) ||
		!m.Name.Equal(b.Name) ||
		!m.Password.Equal(b.Password) ||
		!m.PasswordHash.Equal(b.PasswordHash) ||
		!m.DoubleSha1PasswordHash.Equal(b.DoubleSha1PasswordHash) ||
		!m.Endpoints.Equal(b.Endpoints) ||
		!m.CloudProvider.Equal(b.CloudProvider) ||
		!m.Region.Equal(b.Region) ||
		!m.Tier.Equal(b.Tier) ||
		!m.ReleaseChannel.Equal(b.ReleaseChannel) ||
		!m.IdleScaling.Equal(b.IdleScaling) ||
		!m.MinTotalMemoryGb.Equal(b.MinTotalMemoryGb) ||
		!m.MaxTotalMemoryGb.Equal(b.MaxTotalMemoryGb) ||
		!m.NumReplicas.Equal(b.NumReplicas) ||
		!m.IdleTimeoutMinutes.Equal(b.IdleTimeoutMinutes) ||
		!m.IAMRole.Equal(b.IAMRole) ||
		!m.PrivateEndpointConfig.Equal(b.PrivateEndpointConfig) ||
		!m.EncryptionKey.Equal(b.EncryptionKey) ||
		!m.EncryptionAssumedRoleIdentifier.Equal(b.EncryptionAssumedRoleIdentifier) ||
		!m.TransparentEncryptionData.Equal(b.TransparentEncryptionData) ||
		!m.IpAccessList.Equal(b.IpAccessList) ||
		!m.QueryAPIEndpoints.Equal(b.QueryAPIEndpoints) ||
		!m.BackupConfiguration.Equal(b.BackupConfiguration) {
		return false
	}

	return true
}
