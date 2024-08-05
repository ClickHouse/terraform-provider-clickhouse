package main

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/provider"
)

// Provider documentation generation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name clickhouse

func main() {
	providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{ // nolint:errcheck
		Address: "clickhouse.cloud/terraform/clickhouse",
	})
}
