//go:build alpha

package resource

import (
	upstreamresource "github.com/hashicorp/terraform-plugin-framework/resource"
)

func GetResourceFactories() []func() upstreamresource.Resource {
	return []func() upstreamresource.Resource{
		NewServiceResource,
		NewPrivateEndpointRegistrationResource,
		NewServicePrivateEndpointsAttachmentResource,
		NewServiceTransparentDataEncryptionKeyAssociationResource,
		NewDatabaseResource,
		NewClickPipeResource,
		NewClickPipeReversePrivateEndpointResource,
		NewUserResource,
		NewRoleResource,
		NewGrantRoleResource,
		NewGrantPrivilegeResource,
	}
}
