~> **Note:** This resource is in beta.

Use the *clickhouse_notification_settings* resource to manage which notifications an organization member receives, and on which channels.

Settings are always addressed by an explicit member: the provider authenticates with an organization-bound API key, so there is no "current user" to fall back on. Use the `clickhouse_user` data source to look a member up by email, and the `clickhouse_notification_types` data source to discover the notification types you can name.

A member may have several of these resources, one per rule set. Two of them are safe as long as they declare different notifications, resources and channels; two that declare the same combination fight over it on every apply.

Within one resource, two rules may not name the same combination either: the apply fails and names the combination they disagree about. Two rules that ask for the *same* effect on an overlapping resource are fine, so a `for` expression and an explicit rule can cover some of the same ground.

## Example Usage

```hcl
data "clickhouse_user" "alice" {
  email = "alice@example.com"
}

resource "clickhouse_notification_settings" "alice_services" {
  user_id  = data.clickhouse_user.alice.id
  channels = ["ui", "email"]

  rules = [
    # Explicitly off for this one service, even though the platform default is on.
    {
      effect        = "disabled"
      notifications = ["service_metrics_too_many_parts"]
      resources     = [clickhouse_service.prod.id]
    },
    # Hands the setting back to the platform default, whoever set it — including
    # someone who changed it in the console.
    {
      effect        = "inherit"
      notifications = ["service_backup_failed"]
      resources     = [clickhouse_service.prod.id]
    },
  ]
}
```

In-app only for one set of notifications, by naming only the `ui` channel:

```hcl
resource "clickhouse_notification_settings" "alice_in_app" {
  user_id  = data.clickhouse_user.alice.id
  channels = ["ui"]

  rules = [
    {
      effect        = "enabled"
      notifications = ["service_auto_scaling_configuration_changed"]
      resources     = [clickhouse_service.prod.id]
    },
  ]
}
```

## Applying a rule to many resources

There are no wildcards: a rule applies to the resource IDs it names and to nothing created afterwards. Where the resources are declared in the same configuration, a `for` expression covers them, and the next apply picks up anything added since:

```hcl
rules = [
  {
    effect        = "enabled"
    notifications = ["clickpipe_failed"]
    resources     = [for p in clickhouse_clickpipe.ingest : p.id]
  },
]
```

A rule names resources of the kind its notification types are about — `resource_type` on the `clickhouse_notification_types` data source says which. Notification types about the organization itself, such as billing, take the organization ID.

An unknown resource and a resource the API key cannot see are rejected the same way on purpose, so this resource cannot be used to find out whether something exists.

## Effects

| Effect | Meaning |
| --- | --- |
| `enabled` | The member receives the notification on the named channels. |
| `disabled` | The member does not receive it, even where the platform default is on. |
| `inherit` | The stored setting is removed, so the notification follows the platform default for the member's role again, whoever set it. |

A notification type no rule names keeps whatever setting it had. An empty set of settings does not mean notifications are off — it means every notification follows its platform default.

## Rule reconciliation

The provider only tracks the rules you declare in configuration.

As a result, a setting changed outside Terraform (for example, via the console) for something the configuration never declared is not reported as drift and will not be reverted on the next apply. To manage a setting with Terraform, declare a rule that names it.

A rule you declared that the stored settings no longer agree with **is** detected as drift: the refresh records the effect those settings report, so the plan shows that one rule changing back to what the configuration asks for.

Removing a rule from configuration, or destroying the resource, writes `inherit` for the settings it covered rather than `disabled`, so a notification returns to its platform default instead of being forced off.

## Request size

One apply sends one request, which changes at most 1000 settings and carries at most 100 rules — including the `inherit` rules the provider adds for settings the configuration stopped declaring. The request is built from the settings the rules address rather than from the rules as written, so a notification type named across many services, or many types named for one service, cost one rule each rather than one per setting.

A change larger than the caps fails rather than being split, because a settings write either applies completely or changes nothing. Split the rules across several `clickhouse_notification_settings` resources instead.
