package models

import "github.com/hashicorp/terraform-plugin-framework/types"

type SavedQueryResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ServiceID  types.String `tfsdk:"service_id"`
	Name       types.String `tfsdk:"name"`
	SQL        types.String `tfsdk:"sql"`
	Database   types.String `tfsdk:"database"`
	Parameters types.Map    `tfsdk:"parameters"`
}
