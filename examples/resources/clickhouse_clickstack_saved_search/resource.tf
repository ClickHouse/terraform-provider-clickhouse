# A saved search over a log source. Alerts target a saved search by id.
resource "clickhouse_clickstack_saved_search" "errors" {
  name           = "Production errors"
  source_id      = clickhouse_clickstack_source.logs.id
  select         = "Timestamp, ServiceName, Body"
  where          = "SeverityText:error"
  where_language = "lucene"
  order_by       = "Timestamp DESC"
  tags           = ["production", "errors"]

  # filters is an opaque JSON array of pinned sidebar filters, round-tripped
  # verbatim. Omit it for none; any shape you set is preserved on update.
  filters = jsonencode([
    { type = "sql", condition = "ServiceName IN ('checkout', 'payments')" }
  ])
}
