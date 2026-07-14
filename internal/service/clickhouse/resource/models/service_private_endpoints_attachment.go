package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ServicePrivateEndpointsAttachment struct {
	PrivateEndpointIDs types.List   `tfsdk:"private_endpoint_ids"`
	ServiceID          types.String `tfsdk:"service_id"`
}
