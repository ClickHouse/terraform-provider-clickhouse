package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ServiceUpgradeWindowResourceModel is the Terraform state model for the
// clickhouse_service_upgrade_window resource.
type ServiceUpgradeWindowResourceModel struct {
	ID           types.String `tfsdk:"id"`
	ServiceID    types.String `tfsdk:"service_id"`
	Weekday      types.Int64  `tfsdk:"weekday"`
	StartHourUtc types.Int64  `tfsdk:"start_hour_utc"`
	Duration     types.Int64  `tfsdk:"duration"`
}
