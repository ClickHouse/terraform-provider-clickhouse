//go:build alpha

package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type GrantRole struct {
	ServiceID       types.String `tfsdk:"service_id"`
	RoleName        types.String `tfsdk:"role_name"`
	GranteeUserName types.String `tfsdk:"grantee_user_name"`
	GranteeRoleName types.String `tfsdk:"grantee_role_name"`
	AdminOption     types.Bool   `tfsdk:"admin_option"`
}
