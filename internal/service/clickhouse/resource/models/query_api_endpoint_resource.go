package models

import "github.com/hashicorp/terraform-plugin-framework/types"

type QueryAPIEndpointResourceModel struct {
	ID             types.String `tfsdk:"id"`
	ServiceID      types.String `tfsdk:"service_id"`
	Name           types.String `tfsdk:"name"`
	SQL            types.String `tfsdk:"sql"`
	Database       types.String `tfsdk:"database"`
	Parameters     types.Map    `tfsdk:"parameters"`
	APIKeyIDs      types.Set    `tfsdk:"api_key_ids"`
	Roles          types.Set    `tfsdk:"roles"`
	AllowedOrigins types.Set    `tfsdk:"allowed_origins"`
	URL            types.String `tfsdk:"url"`
}
