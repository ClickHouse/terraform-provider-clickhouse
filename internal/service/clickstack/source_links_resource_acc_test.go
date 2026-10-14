package clickstack_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccSourceLinksResource links a log and a trace source to each other in a
// single apply, which clickhouse_clickstack_source alone cannot express. The
// rename step updates the trace source, a full replace on the API, and the
// post-apply empty-plan check then fails if that update cleared the link the
// links resource owns. Requires the same environment as TestAccSourceResource.
func TestAccSourceLinksResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSourceLinksResourceConfig("tf-acc-links-traces"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_source.logs", "trace_source_id", "clickhouse_clickstack_source.traces", "id"),
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_source_links.traces", "log_source_id", "clickhouse_clickstack_source.logs", "id"),
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_source_links.traces", "id", "clickhouse_clickstack_source.traces", "id"),
					resource.TestCheckNoResourceAttr("clickhouse_clickstack_source.traces", "log_source_id"),
				),
			},
			{
				Config: testAccSourceLinksResourceConfig("tf-acc-links-traces-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_clickstack_source.traces", "name", "tf-acc-links-traces-renamed"),
					resource.TestCheckResourceAttrPair("clickhouse_clickstack_source_links.traces", "log_source_id", "clickhouse_clickstack_source.logs", "id"),
				),
			},
			{
				ResourceName:      "clickhouse_clickstack_source_links.traces",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccSourceLinksResourceConfig(tracesName string) string {
	return fmt.Sprintf(`
resource "clickhouse_clickstack_connection" "test" {
  name     = "tf-acc-source-links-conn"
  host     = "http://localhost:8123"
  username = "default"
}

resource "clickhouse_clickstack_source" "logs" {
  name          = "tf-acc-links-logs"
  kind          = "log"
  connection_id = clickhouse_clickstack_connection.test.id

  from = {
    database_name = "default"
    table_name    = "otel_logs"
  }

  timestamp_value_expression      = "Timestamp"
  default_table_select_expression = "Timestamp, Body"

  trace_source_id = clickhouse_clickstack_source.traces.id
}

resource "clickhouse_clickstack_source" "traces" {
  name          = %q
  kind          = "trace"
  connection_id = clickhouse_clickstack_connection.test.id

  from = {
    database_name = "default"
    table_name    = "otel_traces"
  }

  timestamp_value_expression      = "Timestamp"
  default_table_select_expression = "Timestamp, SpanName"

  duration_expression       = "Duration"
  duration_precision        = 9
  trace_id_expression       = "TraceId"
  span_id_expression        = "SpanId"
  parent_span_id_expression = "ParentSpanId"
  span_name_expression      = "SpanName"
  span_kind_expression      = "SpanKind"
}

resource "clickhouse_clickstack_source_links" "traces" {
  source_id     = clickhouse_clickstack_source.traces.id
  log_source_id = clickhouse_clickstack_source.logs.id
}
`, tracesName)
}
