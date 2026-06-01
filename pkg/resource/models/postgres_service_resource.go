package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	Tags   types.Set    `tfsdk:"tags"`

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

// PostgresServiceTagModel is one entry of the tags set.
//
// Shape mirrors server's ResourceTagV1 ({ key: string; value?: string }).
// Value is a nullable string at the Terraform layer; the API client maps
// types.StringNull() to api.Tag{Value: ""} which the server treats as
// "value omitted."
type PostgresServiceTagModel struct {
	Key   types.String `tfsdk:"key"`
	Value types.String `tfsdk:"value"`
}

// PostgresServiceTagObjectType is the attr.Type for a single tag entry.
// Centralized so the schema, the planning code, and the syncState code all
// agree on the same nested type definition.
func PostgresServiceTagObjectType() attr.Type {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"key":   types.StringType,
			"value": types.StringType,
		},
	}
}
