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
