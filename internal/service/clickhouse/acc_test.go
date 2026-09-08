package clickhouse_test

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/provider"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/registry"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"clickhouse": providerserver.NewProtocol6WithError(provider.NewBuilder(registry.ServicePackages())()),
}

func testAccClickHousePreCheck(t *testing.T) {
	t.Helper()

	if os.Getenv("CLICKHOUSE_ACC_ALLOW_MUTATION") != "yes" {
		t.Fatal("refusing ClickHouse Cloud acceptance mutations: set CLICKHOUSE_ACC_ALLOW_MUTATION=yes after selecting a disposable development target")
	}

	for _, name := range []string{
		"CLICKHOUSE_API_URL",
		"CLICKHOUSE_ORG_ID",
		"CLICKHOUSE_CLOUD_API_KEY",
		"CLICKHOUSE_CLOUD_API_SECRET",
		"CLICKHOUSE_TEST_SERVICE_ID",
		"CLICKHOUSE_TEST_DATABASE_ROLE",
	} {
		if strings.TrimSpace(os.Getenv(name)) == "" {
			t.Fatalf("%s must be set for ClickHouse Cloud acceptance tests", name)
		}
	}

	rawURL := strings.TrimRight(os.Getenv("CLICKHOUSE_API_URL"), "/")
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		t.Fatalf("CLICKHOUSE_API_URL must be a valid API URL, got %q", os.Getenv("CLICKHOUSE_API_URL"))
	}

	host := strings.ToLower(parsedURL.Hostname())
	devURL := parsedURL.Scheme == "https" && host == "api.control-plane.clickhouse-dev.com" && parsedURL.Path == "/v1"
	localURL := parsedURL.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1")
	if !devURL && !localURL {
		t.Fatalf("refusing acceptance target %q; only api.control-plane.clickhouse-dev.com/v1 or localhost URLs are allowed (api.clickhouse.cloud and production hosts are blocked)", os.Getenv("CLICKHOUSE_API_URL"))
	}
}
