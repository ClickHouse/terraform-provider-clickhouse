//go:build alpha

package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type User struct {
	ServiceID          types.String `tfsdk:"service_id"`
	Name               types.String `tfsdk:"name"`
	PasswordSha256Hash types.String `tfsdk:"password_sha256_hash"`
}
