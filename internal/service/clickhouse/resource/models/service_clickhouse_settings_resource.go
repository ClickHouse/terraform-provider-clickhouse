package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ServiceClickhouseSettingsResourceModel is the Terraform state model for the
// clickhouse_service_clickhouse_settings resource.
type ServiceClickhouseSettingsResourceModel struct {
	ID        types.String `tfsdk:"id"`
	ServiceID types.String `tfsdk:"service_id"`
	Settings  types.Map    `tfsdk:"settings"`
}
