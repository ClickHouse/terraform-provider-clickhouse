package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ApiKeyResourceModel struct {
	ID           types.String `tfsdk:"id"`
	KeyID        types.String `tfsdk:"key_id"`
	Name         types.String `tfsdk:"name"`
	State        types.String `tfsdk:"state"`
	ExpireAt     types.String `tfsdk:"expire_at"`
	KeySuffix    types.String `tfsdk:"key_suffix"`
	KeySecret    types.String `tfsdk:"key_secret"`
	IpAccessList types.List   `tfsdk:"ip_access"`
}
