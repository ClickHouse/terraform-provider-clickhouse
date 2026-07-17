package clickstack_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccDashboardPreCheck validates required env vars before running the
// dashboard acceptance test. CLICKSTACK_SOURCE_ID must point to an existing
// data source so the builder tile config is valid.
func testAccDashboardPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	if os.Getenv("CLICKSTACK_SOURCE_ID") == "" {
		t.Skip("CLICKSTACK_SOURCE_ID must be set to run dashboard acceptance tests (skipping)")
	}
}

// TestAccDashboardResource exercises the full CRUD + import lifecycle for the
// clickhouse_clickstack_dashboard resource against a real ClickStack API. It requires
// TF_ACC, CLICKSTACK_API_KEY, and CLICKSTACK_SOURCE_ID.
func TestAccDashboardResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDashboardPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create: assert id and normalized_json are populated.
			{
				Config: testAccDashboardResourceConfig("tf-acc-dashboard"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_dashboard.test", "id"),
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_dashboard.test", "normalized_json"),
				),
			},
			// Update: change the dashboard name; id must remain set.
			{
				Config: testAccDashboardResourceConfig("tf-acc-dashboard-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_dashboard.test", "id"),
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_dashboard.test", "normalized_json"),
				),
			},
			// Import: dashboard_json is config-owned and reconstructed from the
			// server response via canonicalization, so it is added to
			// ImportStateVerifyIgnore to avoid spurious mismatches between the
			// locally-supplied JSON and the server-returned canonical form.
			{
				ResourceName:            "clickhouse_clickstack_dashboard.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"team", "dashboard_json"},
			},
		},
	})
}

func testAccDashboardResourceConfig(name string) string {
	sourceID := os.Getenv("CLICKSTACK_SOURCE_ID")
	return fmt.Sprintf(`
resource "clickhouse_clickstack_dashboard" "test" {
  dashboard_json = jsonencode({
    name = %q
    tiles = [
      {
        name = "spans"
        x    = 0
        y    = 0
        w    = 6
        h    = 3
        config = {
          displayType = "line"
          sourceId    = %q
          select = [
            {
              aggFn = "count"
              alias = "count"
            }
          ]
        }
      }
    ]
  })
}
`, name, sourceID)
}
