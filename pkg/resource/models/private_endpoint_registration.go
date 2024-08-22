package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type PrivateEndpointRegistration struct {
	CloudProvider types.String `tfsdk:"cloud_provider"`
	Description   types.String `tfsdk:"description"`
	EndpointId    types.String `tfsdk:"private_endpoint_id"`
	Region        types.String `tfsdk:"region"`

	// TODO remove in 2.0.0
	LegacyEndpointId types.String `tfsdk:"id"`
}
