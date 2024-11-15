package main

import (
	"context"
	"flag"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/provider"
)

// Provider documentation generation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name clickhouse

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()
	providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{ //nolint:errcheck
		Address: "clickhouse.cloud/terraform/clickhouse",
		Debug:   debug,
	})
}
