package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type IPAccessModel struct {
	Source      types.String `tfsdk:"source"`
	Description types.String `tfsdk:"description"`
}
