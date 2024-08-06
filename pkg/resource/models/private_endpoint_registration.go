package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type PrivateEndpointRegistration struct {
	CloudProvider types.String `tfsdk:"cloud_provider"`
	Description   types.String `tfsdk:"description"`
	EndpointId    types.String `tfsdk:"id"`
	Region        types.String `tfsdk:"region"`
}
