package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ClickPipeSSHKeyResourceModel describes the standalone SSH key resource data model.
type ClickPipeSSHKeyResourceModel struct {
	ID              types.String `tfsdk:"id"`
	ServiceID       types.String `tfsdk:"service_id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Host            types.String `tfsdk:"host"`
	Port            types.Int64  `tfsdk:"port"`
	Username        types.String `tfsdk:"username"`
	PublicKey       types.String `tfsdk:"public_key"`
	Status          types.String `tfsdk:"status"`
	StatusMessage   types.String `tfsdk:"status_message"`
	LastValidatedAt types.String `tfsdk:"last_validated_at"`
	CreatedAt       types.String `tfsdk:"created_at"`
	UpdatedAt       types.String `tfsdk:"updated_at"`
}
