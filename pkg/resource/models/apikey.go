package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ApiKey struct {
	ID             types.String `tfsdk:"id"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	ExpirationDate types.String `tfsdk:"expiration_date"`
	Name           types.String `tfsdk:"name"`
	Roles          types.List   `tfsdk:"roles"`
}
