package clickhouse

import (
	upstreamdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	upstreamresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/datasource"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource"
)

func ServicePackage() service.ServicePackage { return servicePackage{} }

type servicePackage struct{}

func (servicePackage) Meta() service.Metadata {
	return service.Metadata{
		Name:      "clickhouse",
		HumanName: "ClickHouse Cloud",
		Owner:     "@ClickHouse/cloud-platform-engineering",
		Stability: service.StabilityStable,
	}
}

func (servicePackage) Resources() []func() upstreamresource.Resource {
	return []func() upstreamresource.Resource{
		resource.NewApiKeyResource,
		resource.NewServiceResource,
		resource.NewClickPipeResource,
		resource.NewClickPipeCdcInfrastructureResource,
		resource.NewClickPipeReversePrivateEndpointResource,
		resource.NewClickPipeReversePrivateEndpointCustomPrivateDNSResource,
		resource.NewOrganizationSettingsResource,
		resource.NewPrivateEndpointRegistrationResource,
		resource.NewRoleResource,
		resource.NewRoleAssignmentResource,
		resource.NewServicePrivateEndpointsAttachmentResource,
		resource.NewServiceScheduledScalingResource,
		resource.NewServiceTransparentDataEncryptionKeyAssociationResource,
		resource.NewServiceUpgradeWindowResource,
	}
}

func (servicePackage) DataSources() []func() upstreamdatasource.DataSource {
	return []func() upstreamdatasource.DataSource{
		datasource.NewPrivateEndpointConfigDataSource,
		datasource.NewApiKeyIDDataSource,
		datasource.NewRolesDataSource,
		datasource.NewRoleDataSource,
		datasource.NewUserDataSource,
		datasource.NewServiceDataSource,
		datasource.NewServicesDataSource,
	}
}
