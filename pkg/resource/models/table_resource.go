package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type TableResourceModel struct {
	QueryAPIEndpoint types.String `tfsdk:"query_api_endpoint"`
	Name             types.String `tfsdk:"name"`
	Columns          types.Set    `tfsdk:"column"`
	OrderBy          types.String `tfsdk:"order_by"`
}

type TableColumnModel struct {
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	Nullable types.Bool   `tfsdk:"nullable"`
	Default  types.String `tfsdk:"default"`
	Codec    types.String `tfsdk:"codec"`
}
