package clickstack_test

import (
	"fmt"
	"os"
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

// TestAccConnectionResource_AgentRole sets, changes, imports and clears
// agent_clickhouse_role_name. It needs an EE ClickStack and is skipped unless
// CLICKSTACK_AGENT_ROLE names a ClickHouse role (a second role, granted or
// not, is derived by suffixing "_alt"; an ungranted role only adds a warning).
func TestAccConnectionResource_AgentRole(t *testing.T) {
	role := os.Getenv("CLICKSTACK_AGENT_ROLE")
	if role == "" {
		t.Skip("CLICKSTACK_AGENT_ROLE must be set to run agent role acceptance tests against an EE ClickStack (skipping)")
	}
	const res = "clickhouse_clickstack_connection.test"
	config := func(role string) string {
		cfg := testAccConnectionResourceConfig("tf-acc-test-agent-role", "http://localhost:8123", "default", "", false)
		if role == "" {
			return cfg
		}
		return cfg[:len(cfg)-2] + fmt.Sprintf("  agent_clickhouse_role_name = %q\n}\n", role)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(role),
				Check:  resource.TestCheckResourceAttr(res, "agent_clickhouse_role_name", role),
			},
			{
				Config: config(role + "_alt"),
				Check:  resource.TestCheckResourceAttr(res, "agent_clickhouse_role_name", role+"_alt"),
			},
			{
				ResourceName:            res,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			// Removing the attribute must clear the role server-side; the
			// post-apply refresh and empty-plan check prove it stayed cleared.
			{
				Config: config(""),
				Check:  resource.TestCheckNoResourceAttr(res, "agent_clickhouse_role_name"),
			},
		},
	})
}
