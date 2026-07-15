// Package service defines the contract every service group (clickhouse,
// postgres, clickstack, ...) implements to contribute resources and data
// sources to the provider. See docs/rfcs/0001 and decisions/0002.
package service

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
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

// ProviderData is what the provider's Configure hands to every resource and
// data source. Each service group reads the client(s) it needs; a nil client
// means the user did not configure that group.
//
// The unwrap invariant (assert *ProviderData, check the client is non-nil,
// then take it) is currently repeated in every resource/data-source Configure
// method. Extracting a shared helper here (e.g. CloudClientFromProviderData)
// is a deliberate follow-up: Phase 1 keeps the explicit per-Configure form to
// stay behavior-preserving, and Phase 3 re-touches all Configure methods, so
// the de-duplication is best done then. See docs/rfcs/0001.
type ProviderData struct {
	API api.Client // ClickHouse Cloud OpenAPI (Basic auth)
}
