package clickstack_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccConnectionResource exercises the full CRUD + import lifecycle against
// a real ClickStack API. It requires TF_ACC, CLICKSTACK_API_KEY, and
// CLICKSTACK_ENDPOINT (e.g. http://localhost:8000 for a local HyperDX).
func TestAccConnectionResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with all attributes.
			{
				Config: testAccConnectionResourceConfig("tf-acc-test", "http://localhost:8123", "default", "", "http://prometheus:9090"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "name", "tf-acc-test"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "host", "http://localhost:8123"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "username", "default"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "prometheus_endpoint", "http://prometheus:9090"),
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_connection.test", "id"),
				),
			},
			// Update in place: rename and clear prometheus_endpoint.
			{
				Config: testAccConnectionResourceConfig("tf-acc-test-renamed", "http://localhost:8123", "default", "", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_connection.test", "name", "tf-acc-test-renamed"),
					resource.TestCheckNoResourceAttr("clickhouse_clickstack_connection.test", "prometheus_endpoint"),
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

func testAccConnectionResourceConfig(name, host, username, password, prometheusEndpoint string) string {
	cfg := fmt.Sprintf(`
resource "clickhouse_clickstack_connection" "test" {
  name     = %q
  host     = %q
  username = %q
`, name, host, username)
	if password != "" {
		cfg += fmt.Sprintf("  password = %q\n", password)
	}
	if prometheusEndpoint != "" {
		cfg += fmt.Sprintf("  prometheus_endpoint = %q\n", prometheusEndpoint)
	}
	return cfg + "}\n"
}
