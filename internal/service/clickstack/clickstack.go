package clickstack

import (
	upstreamdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	upstreamresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
)

func ServicePackage() service.ServicePackage { return servicePackage{} }

type servicePackage struct{}

func (servicePackage) Meta() service.Metadata {
	return service.Metadata{
		Name:      "clickstack",
		HumanName: "ClickStack",
		Owner:     "@ClickHouse/clickstack",
		Stability: service.StabilityAlpha,
	}
}

func (servicePackage) Resources() []func() upstreamresource.Resource {
	return []func() upstreamresource.Resource{
		NewConnectionResource,
		NewDashboardResource,
		NewSourceResource,
		NewRoleResource,
		NewTeamResource,
		NewTeamMemberResource,
		NewWebhookResource,
		NewSavedSearchResource,
		NewAlertResource,
	}
}

func (servicePackage) DataSources() []func() upstreamdatasource.DataSource {
	return []func() upstreamdatasource.DataSource{
		NewDashboardDataSource,
		NewRoleDataSource,
	}
}
