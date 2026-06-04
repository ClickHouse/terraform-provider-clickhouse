package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// PostgresServiceResourceModel is the Terraform plan/state model for the
// clickhouse_postgres_service resource.
type PostgresServiceResourceModel struct {
	// Identity / immutable.
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	CloudProvider   types.String `tfsdk:"cloud_provider"`
	Region          types.String `tfsdk:"region"`
	PostgresVersion types.String `tfsdk:"postgres_version"`

	// Mutable.
	Size   types.String `tfsdk:"size"`
	HaType types.String `tfsdk:"ha_type"`
	Tags   types.Map    `tfsdk:"tags"`

	// Computed.
	State            types.String `tfsdk:"state"`
	CreatedAt        types.String `tfsdk:"created_at"`
	IsPrimary        types.Bool   `tfsdk:"is_primary"`
	Hostname         types.String `tfsdk:"hostname"`
	Port             types.Int64  `tfsdk:"port"`
	Username         types.String `tfsdk:"username"`
	ConnectionString types.String `tfsdk:"connection_string"`

	// Sensitive / write-only.
	// Currently Computed-only (server always generates).
	Password types.String `tfsdk:"password"`
}
