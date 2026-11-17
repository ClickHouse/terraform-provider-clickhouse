package clickstack_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccConnectionResource exercises the full CRUD + import lifecycle against
// a real ClickStack API. It requires TF_ACC, CLICKSTACK_API_KEY, and
// CLICKSTACK_ENDPOINT (e.g. http://localhost:8000 for a local ClickStack).
func TestAccConnectionResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a ClickHouse connection; the flag defaults to false.
			{
				Config: testAccConnectionResourceConfig("tf-acc-test", "http://localhost:8123", "default", "", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "name", "tf-acc-test"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "host", "http://localhost:8123"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "username", "default"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "is_prometheus_endpoint", "false"),
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_connection.test", "id"),
				),
			},
			// Update in place to a Prometheus endpoint.
			{
				Config: testAccConnectionResourceConfig("tf-acc-test-prom", "http://prometheus:9090", "", "", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "name", "tf-acc-test-prom"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "host", "http://prometheus:9090"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "is_prometheus_endpoint", "true"),
				),
			},
			// Back to ClickHouse by omitting the flag: the API keeps a stored
			// true unless false is sent, so this covers the explicit clear.
			{
				Config: testAccConnectionResourceConfig("tf-acc-test-renamed", "http://localhost:8123", "default", "", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "name", "tf-acc-test-renamed"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "is_prometheus_endpoint", "false"),
				),
			},
			// Import. The password is write-only so it cannot be verified.
			{
				ResourceName:            "clickhouse_clickstack_connection.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

func testAccConnectionResourceConfig(name, host, username, password string, isPrometheusEndpoint bool) string {
	cfg := fmt.Sprintf(`
resource "clickhouse_clickstack_connection" "test" {
  name     = %q
  host     = %q
  username = %q
`, name, host, username)
	if password != "" {
		cfg += fmt.Sprintf("  password = %q\n", password)
	}
	if isPrometheusEndpoint {
		cfg += "  is_prometheus_endpoint = true\n"
	}
	return cfg + "}\n"
}
