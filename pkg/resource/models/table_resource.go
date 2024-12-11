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

type Column struct {
	Name         types.String `tfsdk:"name"`
	Type         types.String `tfsdk:"type"`
	Nullable     types.Bool   `tfsdk:"nullable"`
	Default      types.String `tfsdk:"default"`
	Materialized types.String `tfsdk:"materialized"`
	Ephemeral    types.Bool   `tfsdk:"ephemeral"`
	Alias        types.String `tfsdk:"alias"`
	Codec        types.String `tfsdk:"codec"`
	Comment      types.String `tfsdk:"comment"`
	TTL          types.Object `tfsdk:"ttl"`
}

type TTL struct {
	TimeColumn types.String `tfsdk:"time_column"`
	Interval   types.String `tfsdk:"interval"`
}
