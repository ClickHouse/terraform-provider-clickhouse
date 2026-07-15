//go:build !alpha

package resource

import (
	upstreamresource "github.com/hashicorp/terraform-plugin-framework/resource"
)

func GetResourceFactories() []func() upstreamresource.Resource {
	return []func() upstreamresource.Resource{
		NewClickPipeCdcInfrastructureResource,
		NewClickPipeResource,
		NewClickPipeReversePrivateEndpointCustomPrivateDNSResource,
		NewClickPipeReversePrivateEndpointResource,
		NewInvitationResource,
		NewOrganizationSettingsResource,
		NewPostgresServiceResource,
		NewPrivateEndpointRegistrationResource,
		NewRoleAssignmentResource,
		NewRoleResource,
		NewServicePrivateEndpointsAttachmentResource,
		NewServiceResource,
		NewServiceScheduledScalingResource,
		NewServiceTransparentDataEncryptionKeyAssociationResource,
		NewServiceUpgradeWindowResource,
	}
}
