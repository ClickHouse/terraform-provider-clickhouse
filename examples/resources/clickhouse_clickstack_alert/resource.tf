# A threshold alert on a saved search, notifying two webhooks.
#
# The alert references its saved search and webhooks by id, so Terraform creates
# them first and destroys the alert before them (a webhook cannot be deleted
# while an alert still references it).
resource "clickhouse_clickstack_alert" "too_many_errors" {
  saved_search_id = clickhouse_clickstack_saved_search.errors.id

  # Up to 10 channels, notified in order. Duplicates are rejected.
  channels = [
    {
      type       = "webhook"
      webhook_id = clickhouse_clickstack_webhook.slack.id
    },
    {
      type       = "webhook"
      webhook_id = clickhouse_clickstack_webhook.pagerduty.id
    },
  ]

  # Fire when the saved search returns more than 100 rows in a 5-minute window.
  threshold      = 100
  threshold_type = "above"
  interval       = "5m"

  name    = "Too many production errors"
  message = "Error volume exceeded threshold"
}

# A range alert grouped per service, requiring two consecutive breaching windows.
resource "clickhouse_clickstack_alert" "latency_band" {
  saved_search_id = clickhouse_clickstack_saved_search.errors.id
  group_by        = "ServiceName"

  channels = [{
    type       = "webhook"
    webhook_id = clickhouse_clickstack_webhook.generic.id
  }]

  # between/not_between require threshold_max (>= threshold).
  threshold      = 200
  threshold_max  = 800
  threshold_type = "not_between"
  interval       = "15m"

  num_consecutive_windows = 2
}

# The pre-multi-channel `channel` form still applies, but it is deprecated and
# can only ever notify one target. Switch it to a single-entry `channels` list.
resource "clickhouse_clickstack_alert" "legacy_single_channel" {
  saved_search_id = clickhouse_clickstack_saved_search.errors.id

  channel = {
    type       = "webhook"
    webhook_id = clickhouse_clickstack_webhook.generic.id
  }

  threshold      = 50
  threshold_type = "above"
  interval       = "1h"
}
