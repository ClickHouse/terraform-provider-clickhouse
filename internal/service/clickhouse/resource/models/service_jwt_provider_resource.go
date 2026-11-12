package models

import "github.com/hashicorp/terraform-plugin-framework/types"

// ServiceJWTProviderResourceModel is the Terraform state model for the
// clickhouse_service_jwt_provider resource.
type ServiceJWTProviderResourceModel struct {
	ServiceID  types.String `tfsdk:"service_id"`
	Name       types.String `tfsdk:"name"`
	Issuer     types.String `tfsdk:"issuer"`
	Audience   types.String `tfsdk:"audience"`
	JWKSURL    types.String `tfsdk:"jwks_url"`
	RolesClaim types.String `tfsdk:"roles_claim"`
}
