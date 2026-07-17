package clickstack_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccWebhookResource exercises create + update + import against a real
// ClickStack API. The webhook has no dependencies, so it needs only TF_ACC and
// CLICKSTACK_API_KEY. Write-only secret fields and their version triggers are
// not stored in state, so they are ignored on import verification.
func TestAccWebhookResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccWebhookResourceConfig("tf-acc-webhook", "https://example.com/hook"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_webhook.test", "id"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_webhook.test", "service", "generic"),
				),
			},
			{
				Config: testAccWebhookResourceConfig("tf-acc-webhook", "https://example.com/hook2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_webhook.test", "id"),
				),
			},
			{
				ResourceName:            "clickhouse_clickstack_webhook.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"team", "headers", "query_params", "headers_version", "query_params_version"},
			},
		},
	})
}

func testAccWebhookResourceConfig(name, url string) string {
	return fmt.Sprintf(`
resource "clickhouse_clickstack_webhook" "test" {
  name    = %q
  service = "generic"
  url     = %q
  headers = {
    Authorization = "Bearer test-token"
  }
  headers_version = "1"
}
`, name, url)
}
