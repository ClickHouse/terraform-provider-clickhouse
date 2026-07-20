// Package registry is the central list of service groups. It lives in a
// subpackage (not internal/service itself) so that groups can import
// internal/service for the ServicePackage types without an import cycle.
package registry

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres"
)

// ProviderTypeName is the provider's Terraform type name, the prefix on every
// resource and data source type (e.g. clickhouse_service).
const ProviderTypeName = "clickhouse"

func ServicePackages() []service.ServicePackage {
	return []service.ServicePackage{
		clickhouse.ServicePackage(),
		postgres.ServicePackage(),
		clickstack.ServicePackage(),
	}
}

// Kind distinguishes a resource from a data source.
type Kind int

const (
	KindResource Kind = iota
	KindDataSource
)

// Component is one resource or data source together with the group that owns
// it. It is resolved by instantiating the factory and reading its Metadata.
type Component struct {
	Group    service.Metadata
	Kind     Kind
	TypeName string // Terraform type name, e.g. "clickhouse_service"
}

// Components walks every registered service package and returns its resources
// and data sources paired with their owning group's metadata. It is the shared
// source of truth for tooling that needs the type-name -> group mapping (docs
// subcategory stamping, the registry uniqueness/count test).
func Components() []Component {
	var out []Component
	for _, sp := range ServicePackages() {
		meta := sp.Meta()
		for _, f := range sp.Resources() {
			var mr resource.MetadataResponse
			f().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: ProviderTypeName}, &mr)
			out = append(out, Component{Group: meta, Kind: KindResource, TypeName: mr.TypeName})
		}
		for _, f := range sp.DataSources() {
			var mr datasource.MetadataResponse
			f().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: ProviderTypeName}, &mr)
			out = append(out, Component{Group: meta, Kind: KindDataSource, TypeName: mr.TypeName})
		}
	}
	return out
}
