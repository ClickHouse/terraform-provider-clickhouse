package main

import (
	"context"
	"flag"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/provider"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource"
)

// Provider documentation generation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name clickhouse

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()
	providerserver.Serve(context.Background(), provider.NewBuilder(resource.GetResourceFactories()), providerserver.ServeOpts{ //nolint:errcheck
		Address: "registry.terraform.io/ClickHouse/clickhouse",
		Debug:   debug,
	})
}
