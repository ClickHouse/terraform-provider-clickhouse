package clickstack_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccDefaultTags checks that provider default tags reach every taggable
// resource without showing in its own tags, and that changing them alone updates it.
func TestAccDefaultTags(t *testing.T) {
	const ss = "clickhouse_clickstack_saved_search.test"
	const dash = "clickhouse_clickstack_dashboard.test"
	const alert = "clickhouse_clickstack_alert.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSourceChainPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDefaultTagsConfig("tf-acc-default"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ss, "tags.#", "1"),
					resource.TestCheckTypeSetElemAttr(ss, "tags_all.*", "tf-acc-default"),
					resource.TestCheckResourceAttr(ss, "tags_all.#", "2"),
					resource.TestCheckResourceAttr(dash, "tags_all.#", "2"),
					resource.TestCheckResourceAttr(alert, "tags.#", "1"),
					resource.TestCheckResourceAttr(alert, "tags_all.#", "2"),
				),
			},
			{
				Config: testAccDefaultTagsConfig("tf-acc-default", "tf-acc-default-2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ss, "tags.#", "1"),
					resource.TestCheckResourceAttr(ss, "tags_all.#", "3"),
					resource.TestCheckResourceAttr(dash, "tags_all.#", "3"),
					resource.TestCheckResourceAttr(alert, "tags_all.#", "3"),
				),
			},
			{
				ResourceName:            ss,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"team"},
			},
		},
	})
}

func testAccDefaultTagsConfig(defaults ...string) string {
	return fmt.Sprintf(`
provider "clickhouse" {
  clickstack_default_tags = ["%s"]
}

resource "clickhouse_clickstack_webhook" "test" {
  name    = "tf-acc-default-tags-webhook"
  service = "generic"
  url     = "https://example.com/hook"
}

resource "clickhouse_clickstack_saved_search" "test" {
  name      = "tf-acc-default-tags"
  source_id = %q
  where     = "SeverityText:error"
  tags      = ["tf-acc-ss"]
}

resource "clickhouse_clickstack_dashboard" "test" {
  dashboard_json = jsonencode({
    name  = "tf-acc-default-tags"
    tiles = []
    tags  = ["tf-acc-dash"]
  })
}

resource "clickhouse_clickstack_alert" "test" {
  saved_search_id = clickhouse_clickstack_saved_search.test.id
  channels = [{
    type       = "webhook"
    webhook_id = clickhouse_clickstack_webhook.test.id
  }]
  threshold      = 100
  threshold_type = "above"
  interval       = "5m"
  tags           = ["tf-acc-alert"]
}
`, strings.Join(defaults, `", "`), os.Getenv("CLICKSTACK_SOURCE_ID"))
}
