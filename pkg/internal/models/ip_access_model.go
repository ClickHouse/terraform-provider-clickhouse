package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type IpAccessModel struct {
	Source      types.String `tfsdk:"source"`
	Description types.String `tfsdk:"description"`
}
