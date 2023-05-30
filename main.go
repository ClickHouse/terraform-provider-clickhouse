package main

import (
	"context"
	"terraform-provider-clickhouse/clickhouse"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// Provider documentation generation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name clickhouse

func main() {
	providerserver.Serve(context.Background(), clickhouse.New, providerserver.ServeOpts{
		Address: "clickhouse.cloud/terraform/clickhouse",
	})
}
