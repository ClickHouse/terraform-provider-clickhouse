# A Slack webhook. Slack posts a fixed payload, so headers, query_params, and
# body are not allowed for the slack service.
resource "clickhouse_clickstack_webhook" "slack" {
  name    = "alerts-slack"
  service = "slack"
  url     = var.slack_incoming_webhook_url # sensitive: embeds a channel token
}

# A generic HTTP webhook with a secret auth header.
#
# headers and query_params are write-only: they are sent to the API but never
# stored in Terraform state. Because Terraform cannot see a write-only value,
# bump headers_version (any new string) to force the secret to be re-sent after
# you rotate it. Write-only attributes require Terraform >= 1.11.
resource "clickhouse_clickstack_webhook" "generic" {
  name        = "pagerduty"
  service     = "generic"
  url         = "https://events.pagerduty.com/v2/enqueue"
  description = "Routes alerts to PagerDuty"

  headers = {
    Authorization = var.pagerduty_token
  }
  headers_version = "1"

  body = jsonencode({ routing_key = "REPLACE", event_action = "trigger" })
}
