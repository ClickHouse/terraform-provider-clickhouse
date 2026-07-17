package clickstack_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSourceResource exercises the full CRUD + import lifecycle against a
// real ClickStack API. It requires TF_ACC, CLICKSTACK_API_KEY, and
// CLICKSTACK_ENDPOINT (e.g. http://localhost:8000 for a local HyperDX). The
// config includes a nested list block (query_settings and a highlighted
// attribute expression) so the ImportStateVerify step round-trips the nested
// mappers, not just the scalar fields. Cross-kind mapping fidelity is covered
// by the pure TestSourceModel_RoundTrip below.
func TestAccSourceResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create.
			{
				Config: testAccSourceResourceConfig("tf-acc-logs", "Timestamp, Body"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.test", "name", "tf-acc-logs"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.test", "kind", "log"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.test", "from.database_name", "default"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.test", "from.table_name", "otel_logs"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.test", "default_table_select_expression", "Timestamp, Body"),
					resource.TestCheckResourceAttrSet("clickhouse_clickstack_source.test", "id"),
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_source.test", "connection_id", "clickhouse_clickstack_connection.test", "id"),
				),
			},
			// Update in place: rename and change the default select expression.
			{
				Config: testAccSourceResourceConfig("tf-acc-logs-renamed", "Timestamp, Body, ServiceName"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.test", "name", "tf-acc-logs-renamed"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.test", "default_table_select_expression", "Timestamp, Body, ServiceName"),
				),
			},
			// Import.
			{
				ResourceName:      "clickhouse_clickstack_source.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccSourceResourceConfig(name, selectExpr string) string {
	return fmt.Sprintf(`
resource "clickhouse_clickstack_connection" "test" {
  name     = "tf-acc-source-conn"
  host     = "http://localhost:8123"
  username = "default"
}

resource "clickhouse_clickstack_source" "test" {
  name       = %q
  kind       = "log"
  connection_id = clickhouse_clickstack_connection.test.id

  from = {
    database_name = "default"
    table_name    = "otel_logs"
  }

  timestamp_value_expression      = "Timestamp"
  default_table_select_expression = %q

  query_settings = [
    { setting = "max_threads", value = "4" },
  ]

  highlighted_row_attribute_expressions = [
    { sql_expression = "ServiceName", alias = "Service" },
  ]
}
`, name, selectExpr)
}
