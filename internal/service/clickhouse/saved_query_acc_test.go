package clickhouse_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccSavedQueryResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	serviceID := os.Getenv("CLICKHOUSE_TEST_SERVICE_ID")
	database := os.Getenv("CLICKHOUSE_TEST_DATABASE")
	if database == "" {
		database = "default"
	}

	name := fmt.Sprintf("tf-acc-saved-query-%d", time.Now().UnixNano())
	initialSQL := "SELECT {value:String} AS value, {revision:UInt8} AS revision"
	updatedSQL := "SELECT upper({value:String}) AS value, {revision:UInt8} AS revision"
	initialConfig := savedQueryAccConfig(serviceID, database, name, initialSQL, "created", "1")
	updatedConfig := savedQueryAccConfig(serviceID, database, name+"-updated", updatedSQL, "updated", "2")

	var queryID string
	checkStableQueryID := func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources["clickhouse_saved_query.test"]
		if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
			return fmt.Errorf("clickhouse_saved_query.test has no state ID")
		}
		if queryID == "" {
			queryID = resourceState.Primary.ID
			return nil
		}
		if resourceState.Primary.ID != queryID {
			return fmt.Errorf("saved query ID changed from %q to %q during update", queryID, resourceState.Primary.ID)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStableQueryID,
					resource.TestCheckResourceAttr("clickhouse_saved_query.test", "name", name),
					resource.TestCheckResourceAttr("clickhouse_saved_query.test", "sql", initialSQL),
					resource.TestCheckResourceAttr("clickhouse_saved_query.test", "database", database),
					resource.TestCheckResourceAttr("clickhouse_saved_query.test", "parameters.value", "created"),
					resource.TestCheckResourceAttr("clickhouse_saved_query.test", "parameters.revision", "1"),
					resource.TestCheckResourceAttrSet("clickhouse_saved_query.test", "id"),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStableQueryID,
					resource.TestCheckResourceAttr("clickhouse_saved_query.test", "name", name+"-updated"),
					resource.TestCheckResourceAttr("clickhouse_saved_query.test", "sql", updatedSQL),
					resource.TestCheckResourceAttr("clickhouse_saved_query.test", "parameters.value", "updated"),
					resource.TestCheckResourceAttr("clickhouse_saved_query.test", "parameters.revision", "2"),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName:      "clickhouse_saved_query.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					resourceState, ok := state.RootModule().Resources["clickhouse_saved_query.test"]
					if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
						return "", fmt.Errorf("clickhouse_saved_query.test has no state ID for import")
					}
					return serviceID + ":" + resourceState.Primary.ID, nil
				},
			},
		},
	})
}

func savedQueryAccConfig(serviceID, database, name, sql, value, revision string) string {
	return fmt.Sprintf(`
resource "clickhouse_saved_query" "test" {
  service_id = %q

  name     = %q
  sql      = %q
  database = %q

  parameters = {
    value    = %q
    revision = %q
  }
}
`, serviceID, name, sql, database, value, revision)
}
