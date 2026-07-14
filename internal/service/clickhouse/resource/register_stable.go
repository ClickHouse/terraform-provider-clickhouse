//go:build !alpha

package resource

import (
	upstreamresource "github.com/hashicorp/terraform-plugin-framework/resource"

	pgresource "github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource"
)

func GetResourceFactories() []func() upstreamresource.Resource {
	return []func() upstreamresource.Resource{
		NewClickPipeCdcInfrastructureResource,
		NewClickPipeResource,
		NewClickPipeReversePrivateEndpointCustomPrivateDNSResource,
		NewClickPipeReversePrivateEndpointResource,
		NewOrganizationSettingsResource,
		pgresource.NewPostgresServiceResource,
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
