package clickstack_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSourceResource exercises the full CRUD + import lifecycle against a
// real ClickStack API. It requires TF_ACC, CLICKSTACK_API_KEY, and
// CLICKSTACK_ENDPOINT (e.g. http://localhost:8000 for a local ClickStack). The
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
					resource.TestCheckResourceAttrPair("data.clickhouse_clickstack_source.test", "id", "clickhouse_clickstack_source.test", "id"),
					resource.TestCheckResourceAttr("data.clickhouse_clickstack_source.test", "table_name", "otel_logs"),
				),
			},
			// Update in place: rename and change the default select expression.
			{
				Config: testAccSourceResourceConfig("tf-acc-logs-renamed", "Timestamp, Body, ServiceName"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.test", "name", "tf-acc-logs-renamed"),
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.test", "default_table_select_expression", "Timestamp, Body, ServiceName"),
					resource.TestCheckResourceAttrPair("data.clickhouse_clickstack_source.test", "id", "clickhouse_clickstack_source.test", "id"),
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

// TestAccSourceResource_Promql covers min_auto_granularity on a promql source:
// set, removed (the PUT is a full replace, so omitting it clears it), set again,
// then imported so the value round-trips through Read. It needs
// ENABLE_PROMQL=true on the API. ClickStack doesn't check that the TimeSeries table
// exists when saving.
func TestAccSourceResource_Promql(t *testing.T) {
	const res = "clickhouse_clickstack_source.test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSourcePromqlConfig("1 minute"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "kind", "promql"),
					resource.TestCheckResourceAttr(res, "from.table_name", "metrics_ts"),
					resource.TestCheckResourceAttr(res, "min_auto_granularity", "1 minute"),
				),
			},
			{
				Config: testAccSourcePromqlConfig(""),
				Check:  resource.TestCheckNoResourceAttr(res, "min_auto_granularity"),
			},
			{
				Config: testAccSourcePromqlConfig("5 minute"),
				Check:  resource.TestCheckResourceAttr(res, "min_auto_granularity", "5 minute"),
			},
			{
				ResourceName:      res,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// testAccSourcePromqlConfig leaves min_auto_granularity out when it is "".
func testAccSourcePromqlConfig(minAutoGranularity string) string {
	line := ""
	if minAutoGranularity != "" {
		line = fmt.Sprintf("min_auto_granularity = %q", minAutoGranularity)
	}
	return fmt.Sprintf(`
resource "clickhouse_clickstack_connection" "test" {
  name     = "tf-acc-promql-conn"
  host     = "http://localhost:8123"
  username = "default"
}

resource "clickhouse_clickstack_source" "test" {
  name          = "tf-acc-promql"
  kind          = "promql"
  connection_id = clickhouse_clickstack_connection.test.id

  from = {
    database_name = "default"
    table_name    = "metrics_ts"
  }

  timestamp_value_expression = "timestamp"
  %s
}
`, line)
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

data "clickhouse_clickstack_source" "test" {
  name = clickhouse_clickstack_source.test.name
  kind = clickhouse_clickstack_source.test.kind
}
`, name, selectExpr)
}
