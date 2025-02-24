//go:build alpha

package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type DatabaseResourceModel struct {
	ServiceID types.String `tfsdk:"service_id"`
	Name      types.String `tfsdk:"name"`
	Comment   types.String `tfsdk:"comment"`
}
