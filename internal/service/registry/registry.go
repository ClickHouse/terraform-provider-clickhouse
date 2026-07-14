// Package registry is the central list of service groups. It lives in a
// subpackage (not internal/service itself) so that groups can import
// internal/service for the ServicePackage types without an import cycle.
package registry

import (
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres"
)

func ServicePackages() []service.ServicePackage {
	return []service.ServicePackage{
		clickhouse.ServicePackage(),
		postgres.ServicePackage(),
	}
}
