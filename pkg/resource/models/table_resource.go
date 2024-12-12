package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type TableResourceModel struct {
	QueryAPIEndpoint types.String `tfsdk:"query_api_endpoint"`
	Name             types.String `tfsdk:"name"`
	Engine           types.Object `tfsdk:"engine"`
	Columns          types.Map    `tfsdk:"columns"`
	OrderBy          types.String `tfsdk:"order_by"`
	Settings         types.Map    `tfsdk:"settings"`
	Comment          types.String `tfsdk:"comment"`
}

type Column struct {
	Type         types.String `tfsdk:"type"`
	Nullable     types.Bool   `tfsdk:"nullable"`
	Default      types.String `tfsdk:"default"`
	Materialized types.String `tfsdk:"materialized"`
	Ephemeral    types.Bool   `tfsdk:"ephemeral"`
	Alias        types.String `tfsdk:"alias"`
	Comment      types.String `tfsdk:"comment"`
}

func (c Column) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"type":         types.StringType,
			"nullable":     types.BoolType,
			"default":      types.StringType,
			"materialized": types.StringType,
			"ephemeral":    types.BoolType,
			"alias":        types.StringType,
			"comment":      types.StringType,
		},
	}
}

type Engine struct {
	Name   types.String `tfsdk:"name"`
	Params types.List   `tfsdk:"params"`
}

func (e Engine) AttributeTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name": types.StringType,
		"params": types.ListType{
			ElemType: types.StringType,
		},
	}
}
