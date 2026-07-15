package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type OrganizationSettingsResourceModel struct {
	ID               types.String `tfsdk:"id"`
	CoreDumpsEnabled types.Bool   `tfsdk:"core_dumps_enabled"`
}
