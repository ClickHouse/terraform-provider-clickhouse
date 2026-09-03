package clickstack_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccAlertResource exercises create + update + import for a saved-search
// alert, standing up its webhook and saved search in the same config so the
// dependency ordering (and the no-409-on-destroy behaviour) is exercised.
// Requires TF_ACC, CLICKSTACK_API_KEY, and CLICKSTACK_SOURCE_ID.
func TestAccAlertResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSourceChainPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertResourceConfig(100),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_alert.test", "id"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_alert.test", "threshold", "100"),
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_alert.test", "channel.webhook_id", "clickhouse_clickstack_webhook.test", "id"),
				),
			},
			{
				Config: testAccAlertResourceConfig(250),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_alert.test", "threshold", "250"),
				),
			},
			{
				ResourceName:            "clickhouse_clickstack_alert.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"team"},
			},
		},
	})
}

func testAccAlertResourceConfig(threshold int) string {
	return fmt.Sprintf(`
resource "clickhouse_clickstack_webhook" "test" {
  name    = "tf-acc-alert-webhook"
  service = "generic"
  url     = "https://example.com/hook"
}

resource "clickhouse_clickstack_saved_search" "test" {
  name      = "tf-acc-alert-ss"
  source_id = %q
  where     = "SeverityText:error"
}

resource "clickhouse_clickstack_alert" "test" {
  saved_search_id = clickhouse_clickstack_saved_search.test.id

  channel = {
    type       = "webhook"
    webhook_id = clickhouse_clickstack_webhook.test.id
  }

  threshold      = %d
  threshold_type = "above"
  interval       = "5m"
}
`, os.Getenv("CLICKSTACK_SOURCE_ID"), threshold)
}

// TestAccAlertResource_Tile exercises create + update + import for a tile alert.
// The alert takes the tile id from the dashboard's tile_ids map; the webhook and
// dashboard are stood up in the same config so the dependency ordering on
// create and destroy is exercised. A destroy still succeeds if the server already
// cascade-deleted the alert with its dashboard, because the provider treats the
// 404 as a no-op. Requires TF_ACC, CLICKSTACK_API_KEY, and CLICKSTACK_SOURCE_ID.
func TestAccAlertResource_Tile(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSourceChainPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTileAlertResourceConfig(100),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_alert.tile", "id"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_alert.tile", "source", "tile"),
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_alert.tile", "tile_id", "clickhouse_clickstack_dashboard.tile", "tile_ids.spans"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_alert.tile", "threshold", "100"),
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_alert.tile", "dashboard_id", "clickhouse_clickstack_dashboard.tile", "id"),
					resource.TestCheckNoResourceAttr("clickhouse_clickstack_alert.tile", "saved_search_id"),
				),
			},
			{
				Config: testAccTileAlertResourceConfig(250),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_alert.tile", "threshold", "250"),
				),
			},
			{
				ResourceName:            "clickhouse_clickstack_alert.tile",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"team"},
			},
		},
	})
}

func testAccTileAlertResourceConfig(threshold int) string {
	return fmt.Sprintf(`
resource "clickhouse_clickstack_webhook" "tile" {
  name    = "tf-acc-tile-alert-webhook"
  service = "generic"
  url     = "https://example.com/hook"
}

resource "clickhouse_clickstack_dashboard" "tile" {
  dashboard_json = jsonencode({
    name = "tf-acc-tile-alert"
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

resource "clickhouse_clickstack_alert" "tile" {
  source       = "tile"
  dashboard_id = clickhouse_clickstack_dashboard.tile.id
  tile_id      = clickhouse_clickstack_dashboard.tile.tile_ids["spans"]

  channel = {
    type       = "webhook"
    webhook_id = clickhouse_clickstack_webhook.tile.id
  }

  threshold      = %d
  threshold_type = "above"
  interval       = "5m"
}
`, os.Getenv("CLICKSTACK_SOURCE_ID"), threshold)
}
