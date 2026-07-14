// Package service defines the contract every service group (clickhouse,
// postgres, clickstack, ...) implements to contribute resources and data
// sources to the provider. See docs/rfcs/0001 and decisions/0002.
package service

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// ServicePackage is implemented once per service group. A group
// self-describes its metadata and the resources and data sources it
// contributes to the provider.
type ServicePackage interface {
	Meta() Metadata
	Resources() []func() resource.Resource
	DataSources() []func() datasource.DataSource
}

type Stability string

const (
	StabilityStable Stability = "stable"
	StabilityAlpha  Stability = "alpha"
)

// Metadata is the machine-readable definition of a group.
type Metadata struct {
	Name      string    // stable identifier, e.g. "clickstack"
	HumanName string    // docs/diagnostics label, e.g. "ClickStack"
	Owner     string    // CODEOWNERS team, e.g. "@ClickHouse/clickstack"
	Stability Stability // default maturity for the group
}
