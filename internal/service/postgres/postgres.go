package postgres

import (
	upstreamdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	upstreamresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/datasource"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource"
)

func ServicePackage() service.ServicePackage { return servicePackage{} }

type servicePackage struct{}

func (servicePackage) Meta() service.Metadata {
	return service.Metadata{
		Name:      "postgres",
		HumanName: "Postgres",
		Owner:     "@ClickHouse/clickgres",
		Stability: service.StabilityStable, // resource-level alpha warnings are unchanged
	}
}

func (servicePackage) Resources() []func() upstreamresource.Resource {
	return []func() upstreamresource.Resource{resource.NewPostgresServiceResource}
}

func (servicePackage) DataSources() []func() upstreamdatasource.DataSource {
	return []func() upstreamdatasource.DataSource{
		datasource.NewPostgresServiceDataSource,
		datasource.NewPostgresServicesDataSource,
		datasource.NewPostgresServiceCaCertificatesDataSource,
	}
}
