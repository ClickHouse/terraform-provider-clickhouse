package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ServiceTransparentDataEncryptionKeyAssociation struct {
	ServiceID types.String `tfsdk:"service_id"`
	KeyID     types.String `tfsdk:"key_id"`
}
