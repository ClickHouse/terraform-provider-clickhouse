//go:build alpha

package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type GrantPrivilege struct {
	ServiceID       types.String `tfsdk:"service_id"`
	Privilege       types.String `tfsdk:"privilege_name"`
	Database        types.String `tfsdk:"database_name"`
	Table           types.String `tfsdk:"table_name"`
	Column          types.String `tfsdk:"column_name"`
	GranteeUserName types.String `tfsdk:"grantee_user_name"`
	GranteeRoleName types.String `tfsdk:"grantee_role_name"`
	GrantOption     types.Bool   `tfsdk:"grant_option"`
}
