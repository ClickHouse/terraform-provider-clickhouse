package clickstack_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccAlertResource exercises create + update + import for a saved-search
// alert notifying two webhooks, standing up its webhooks and saved search in the
// same config so the dependency ordering (and the no-409-on-destroy behaviour)
// is exercised.
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
					resource.TestCheckResourceAttr("clickhouse_clickstack_alert.test", "channels.#", "2"),
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_alert.test", "channels.0.webhook_id", "clickhouse_clickstack_webhook.test", "id"),
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_alert.test", "channels.1.webhook_id", "clickhouse_clickstack_webhook.second", "id"),
				),
			},
			{
				Config: testAccAlertResourceConfig(250),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_alert.test", "threshold", "250"),
				),
			},
			{
				// Switching representations is the migration path users take, and
				// the API reduces the alert to the single channel. `channels` must
				// go null rather than keeping a stale list from the response.
				Config: testAccAlertResourceSingleChannelConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_alert.test", "channel.webhook_id", "clickhouse_clickstack_webhook.test", "id"),
					resource.TestCheckNoResourceAttr("clickhouse_clickstack_alert.test", "channels.#"),
				),
			},
			{
				// ...and back, so neither direction leaves the other field set.
				Config: testAccAlertResourceConfig(250),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_alert.test", "channels.#", "2"),
					resource.TestCheckNoResourceAttr("clickhouse_clickstack_alert.test", "channel.type"),
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

resource "clickhouse_clickstack_webhook" "second" {
  name    = "tf-acc-alert-webhook-2"
  service = "generic"
  url     = "https://example.com/hook-2"
}

resource "clickhouse_clickstack_saved_search" "test" {
  name      = "tf-acc-alert-ss"
  source_id = %q
  where     = "SeverityText:error"
}

resource "clickhouse_clickstack_alert" "test" {
  saved_search_id = clickhouse_clickstack_saved_search.test.id

  channels = [
    {
      type       = "webhook"
      webhook_id = clickhouse_clickstack_webhook.test.id
    },
    {
      type       = "webhook"
      webhook_id = clickhouse_clickstack_webhook.second.id
    },
  ]

  threshold      = %d
  threshold_type = "above"
  interval       = "5m"
}
`, os.Getenv("CLICKSTACK_SOURCE_ID"), threshold)
}

// testAccAlertResourceSingleChannelConfig is testAccAlertResourceConfig with the
// alert switched to the deprecated single `channel`, keeping the same webhooks
// and saved search so the step is an in-place update rather than a replace.
func testAccAlertResourceSingleChannelConfig() string {
	full := testAccAlertResourceConfig(250)
	return strings.Replace(full, `  channels = [
    {
      type       = "webhook"
      webhook_id = clickhouse_clickstack_webhook.test.id
    },
    {
      type       = "webhook"
      webhook_id = clickhouse_clickstack_webhook.second.id
    },
  ]`, `  channel = {
    type       = "webhook"
    webhook_id = clickhouse_clickstack_webhook.test.id
  }`, 1)
}

// TestAccAlertResourceDeprecatedChannel keeps the pre-multi-channel `channel`
// form covered: it must still apply, and must not pick up a `channels` value
// from the response (which always carries both fields). Import is exercised
// without ImportStateVerify: an imported alert always populates `channels`, so
// it deliberately does not match a `channel`-based prior state.
func TestAccAlertResourceDeprecatedChannel(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSourceChainPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAlertResourceDeprecatedChannelConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_alert.legacy", "channel.webhook_id", "clickhouse_clickstack_webhook.legacy", "id"),
					resource.TestCheckNoResourceAttr("clickhouse_clickstack_alert.legacy", "channels.#"),
				),
			},
			{
				// Pins the behaviour the schema description promises: import
				// populates `channels`, not `channel`. ImportStateVerify is off
				// because that mismatch against the prior state is the point.
				ResourceName:      "clickhouse_clickstack_alert.legacy",
				ImportState:       true,
				ImportStateVerify: false,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 imported instance, got %d", len(states))
					}
					if got := states[0].Attributes["channels.#"]; got != "1" {
						return fmt.Errorf("expected import to populate channels.# = 1, got %q", got)
					}
					if got := states[0].Attributes["channel.type"]; got != "" {
						return fmt.Errorf("expected import to leave channel unset, got type %q", got)
					}
					return nil
				},
			},
		},
	})
}

func testAccAlertResourceDeprecatedChannelConfig() string {
	return fmt.Sprintf(`
resource "clickhouse_clickstack_webhook" "legacy" {
  name    = "tf-acc-alert-legacy-webhook"
  service = "generic"
  url     = "https://example.com/legacy-hook"
}

resource "clickhouse_clickstack_saved_search" "legacy" {
  name      = "tf-acc-alert-legacy-ss"
  source_id = %q
  where     = "SeverityText:error"
}

resource "clickhouse_clickstack_alert" "legacy" {
  saved_search_id = clickhouse_clickstack_saved_search.legacy.id

  channel = {
    type       = "webhook"
    webhook_id = clickhouse_clickstack_webhook.legacy.id
  }

  threshold      = 100
  threshold_type = "above"
  interval       = "5m"
}
`, os.Getenv("CLICKSTACK_SOURCE_ID"))
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

  channels = [
    {
      type       = "webhook"
      webhook_id = clickhouse_clickstack_webhook.tile.id
    },
  ]

  threshold      = %d
  threshold_type = "above"
  interval       = "5m"
}
`, os.Getenv("CLICKSTACK_SOURCE_ID"), threshold)
}
