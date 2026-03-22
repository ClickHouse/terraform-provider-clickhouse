package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type PostgresInstanceResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	CloudProvider    types.String `tfsdk:"cloud_provider"`
	Region           types.String `tfsdk:"region"`
	PostgresVersion  types.String `tfsdk:"postgres_version"`
	Size             types.String `tfsdk:"size"`
	StorageSize      types.Int64  `tfsdk:"storage_size"`
	HAType           types.String `tfsdk:"ha_type"`
	State            types.String `tfsdk:"state"`
	IsPrimary        types.Bool   `tfsdk:"is_primary"`
	Hostname         types.String `tfsdk:"hostname"`
	ConnectionString types.String `tfsdk:"connection_string"`
	Username         types.String `tfsdk:"username"`
	PgConfig         types.Map    `tfsdk:"pg_config"`
	PgBouncerConfig  types.Map    `tfsdk:"pg_bouncer_config"`
	Tags             types.Map    `tfsdk:"tags"`
}

func (m PostgresInstanceResourceModel) Equals(other PostgresInstanceResourceModel) bool {
	if !m.ID.Equal(other.ID) ||
		!m.Name.Equal(other.Name) ||
		!m.CloudProvider.Equal(other.CloudProvider) ||
		!m.Region.Equal(other.Region) ||
		!m.PostgresVersion.Equal(other.PostgresVersion) ||
		!m.Size.Equal(other.Size) ||
		!m.StorageSize.Equal(other.StorageSize) ||
		!m.HAType.Equal(other.HAType) ||
		!m.State.Equal(other.State) ||
		!m.IsPrimary.Equal(other.IsPrimary) ||
		!m.Hostname.Equal(other.Hostname) ||
		!m.ConnectionString.Equal(other.ConnectionString) ||
		!m.Username.Equal(other.Username) ||
		!m.PgConfig.Equal(other.PgConfig) ||
		!m.PgBouncerConfig.Equal(other.PgBouncerConfig) ||
		!m.Tags.Equal(other.Tags) {
		return false
	}

	return true
}
