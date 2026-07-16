package clickstack_test

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/provider"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/registry"
)

// testAccProtoV6ProviderFactories builds the full consolidated provider (via the
// registry, so behavior matches production) for acceptance tests. It lives in the
// external clickstack_test package to avoid the registry -> clickstack import
// cycle that an in-package harness would create.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"clickhouse": providerserver.NewProtocol6WithError(provider.NewBuilder(registry.ServicePackages())()),
}

// testAccPreCheck asserts the ClickStack credentials required by the acceptance
// suite are present. Cloud credentials are not required: these tests exercise
// only clickhouse_clickstack_* resources.
func testAccPreCheck(t *testing.T) {
	if os.Getenv("CLICKSTACK_API_KEY") == "" {
		t.Fatal("CLICKSTACK_API_KEY must be set for acceptance tests")
	}
}
