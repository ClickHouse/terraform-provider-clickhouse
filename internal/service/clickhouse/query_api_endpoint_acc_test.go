package clickhouse_test

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

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

func TestAccQueryAPIEndpointResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	serviceID := os.Getenv("CLICKHOUSE_TEST_SERVICE_ID")
	databaseRole := os.Getenv("CLICKHOUSE_TEST_DATABASE_ROLE")
	database := os.Getenv("CLICKHOUSE_TEST_DATABASE")
	if database == "" {
		database = "default"
	}

	name := fmt.Sprintf("tf-acc-query-api-%d", time.Now().UnixNano())
	initialSQL := "SELECT {value:String} AS value, {revision:UInt8} AS revision"
	updatedSQL := "SELECT upper({value:String}) AS value, {revision:UInt8} AS revision"
	initialConfig := queryAPIEndpointAccConfig(serviceID, databaseRole, database, name, initialSQL, "created", "1", "[]")
	updatedConfig := queryAPIEndpointAccConfig(serviceID, databaseRole, database, name+"-updated", updatedSQL, "updated", "2", "[\"https://admin.example.com\", \"https://example.com\"]")

	var endpointID string
	checkStableEndpointID := func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources["clickhouse_query_api_endpoint.test"]
		if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
			return fmt.Errorf("clickhouse_query_api_endpoint.test has no state ID")
		}
		if endpointID == "" {
			endpointID = resourceState.Primary.ID
			return nil
		}
		if resourceState.Primary.ID != endpointID {
			return fmt.Errorf("endpoint ID changed from %q to %q during update", endpointID, resourceState.Primary.ID)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStableEndpointID,
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "name", name),
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "sql", initialSQL),
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "database", database),
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "parameters.value", "created"),
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "parameters.revision", "1"),
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "allowed_origins.#", "0"),
					resource.TestCheckResourceAttrSet("clickhouse_query_api_endpoint.test", "id"),
					resource.TestCheckResourceAttrSet("clickhouse_query_api_endpoint.test", "url"),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStableEndpointID,
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "name", name+"-updated"),
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "sql", updatedSQL),
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "parameters.value", "updated"),
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "parameters.revision", "2"),
					resource.TestCheckResourceAttr("clickhouse_query_api_endpoint.test", "allowed_origins.#", "2"),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName:      "clickhouse_query_api_endpoint.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					resourceState, ok := state.RootModule().Resources["clickhouse_query_api_endpoint.test"]
					if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
						return "", fmt.Errorf("clickhouse_query_api_endpoint.test has no state ID for import")
					}
					return serviceID + ":" + resourceState.Primary.ID, nil
				},
			},
		},
	})
}

func queryAPIEndpointAccConfig(serviceID, databaseRole, database, name, sql, value, revision, allowedOrigins string) string {
	return fmt.Sprintf(`
data "clickhouse_api_key_id" "current" {}

resource "clickhouse_query_api_endpoint" "test" {
  service_id = %q

  name     = %q
  sql      = %q
  database = %q

  parameters = {
    value    = %q
    revision = %q
  }

  api_key_ids     = [data.clickhouse_api_key_id.current.id]
  roles           = [%q]
  allowed_origins = %s
}
`, serviceID, name, sql, database, value, revision, databaseRole, allowedOrigins)
}
