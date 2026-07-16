package clickstack_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccSourceChainPreCheck requires an existing source id so saved-search and
// alert acceptance tests need not build the connection/source chain themselves.
func testAccSourceChainPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	if os.Getenv("CLICKSTACK_SOURCE_ID") == "" {
		t.Skip("CLICKSTACK_SOURCE_ID must be set to run this acceptance test (skipping)")
	}
}

// TestAccSavedSearchResource exercises create + update + import against a real
// ClickStack API. Requires TF_ACC, CLICKSTACK_API_KEY, and CLICKSTACK_SOURCE_ID.
func TestAccSavedSearchResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccSourceChainPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSavedSearchResourceConfig("tf-acc-ss", "SeverityText:error"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_saved_search.test", "id"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_saved_search.test", "where", "SeverityText:error"),
				),
			},
			{
				Config: testAccSavedSearchResourceConfig("tf-acc-ss", "SeverityText:warn"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_saved_search.test", "where", "SeverityText:warn"),
				),
			},
			{
				ResourceName:            "clickhouse_clickstack_saved_search.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"team"},
			},
		},
	})
}

func testAccSavedSearchResourceConfig(name, where string) string {
	return fmt.Sprintf(`
resource "clickhouse_clickstack_saved_search" "test" {
  name      = %q
  source_id = %q
  where     = %q
  tags      = ["tf-acc"]
}
`, name, os.Getenv("CLICKSTACK_SOURCE_ID"), where)
}
