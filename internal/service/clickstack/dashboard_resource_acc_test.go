package clickstack_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
			// ImportStateCheck covers what that ignore gives up: the imported
			// body must be one the write path accepts.
			{
				ResourceName:            "clickhouse_clickstack_dashboard.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"team", "dashboard_json"},
				ImportStateCheck:        checkImportedDashboardJSON,
			},
		},
	})
}

// checkImportedDashboardJSON asserts the imported dashboard_json drops the ids
// the write path rejects (the dashboard's own id, and filter ids) while keeping
// the tile ids that hold UI-created tile alerts.
func checkImportedDashboardJSON(states []*terraform.InstanceState) error {
	if len(states) != 1 {
		return fmt.Errorf("expected 1 imported state, got %d", len(states))
	}
	var doc struct {
		ID      string `json:"id"`
		Filters []struct {
			ID string `json:"id"`
		} `json:"filters"`
		Tiles []struct {
			ID string `json:"id"`
		} `json:"tiles"`
	}
	raw := states[0].Attributes["dashboard_json"]
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return fmt.Errorf("imported dashboard_json is not a JSON object: %w", err)
	}
	if doc.ID != "" {
		return fmt.Errorf("imported dashboard_json kept the dashboard id %q; the API rejects it", doc.ID)
	}
	for i, f := range doc.Filters {
		if f.ID != "" {
			return fmt.Errorf("imported dashboard_json kept filters.%d id %q; the API rejects it", i, f.ID)
		}
	}
	for i, tile := range doc.Tiles {
		if tile.ID == "" {
			return fmt.Errorf("imported dashboard_json dropped tiles.%d id; tile alerts need it", i)
		}
	}
	return nil
}

// testAccDashboardResourceConfig builds a dashboard with one tile and one
// filter. The filter is what makes the import check meaningful: the server
// assigns it an id that the write path then refuses to accept back.
func testAccDashboardResourceConfig(name string) string {
	sourceID := os.Getenv("CLICKSTACK_SOURCE_ID")
	return fmt.Sprintf(`
resource "clickhouse_clickstack_dashboard" "test" {
  dashboard_json = jsonencode({
    name = %q
    filters = [
      {
        type          = "QUERY_EXPRESSION"
        name          = "Service"
        expression    = "ServiceName"
        sourceId      = %q
        whereLanguage = "sql"
      }
    ]
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
`, name, sourceID, sourceID)
}
