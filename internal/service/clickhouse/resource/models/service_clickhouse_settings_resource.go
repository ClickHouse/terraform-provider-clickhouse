package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ServiceClickHouseSettingsResourceModel is the Terraform state model for the
// clickhouse_service_clickhouse_settings resource.
type ServiceClickHouseSettingsResourceModel struct {
	ID        types.String `tfsdk:"id"`
	ServiceID types.String `tfsdk:"service_id"`
	Settings  types.Map    `tfsdk:"settings"`
}
