package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type PrivateEndpointAttachment struct {
	PrivateEndpointIds types.List   `tfsdk:"private_endpoint_ids"`
	ServiceId          types.String `tfsdk:"service_id"`
}
