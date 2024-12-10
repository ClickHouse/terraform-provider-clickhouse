package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type TableResourceModel struct {
	QueryAPIEndpoint types.String `tfsdk:"query_api_endpoint"`
	Name             types.String `tfsdk:"name"`
	Columns          types.Set    `tfsdk:"column"`
}

type TableColumnModel struct {
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}
