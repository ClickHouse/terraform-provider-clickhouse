package clickhouse_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Covers the shape a read-modify-write consumer depends on: an apply, a plan
// that comes back empty, a rule set that changes, an import, and a destroy. A
// perpetual diff on any of those would mean the provider and the API disagree
// about what was written.
func TestAccNotificationSettingsResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	serviceID := os.Getenv("CLICKHOUSE_TEST_SERVICE_ID")
	userEmail := os.Getenv("CLICKHOUSE_TEST_USER_EMAIL")

	initialConfig := notificationSettingsAccConfig(userEmail, `["ui", "email"]`, "disabled", "service_backup_failed", serviceID)
	// Narrows the channels and swaps the notification type, so the apply has to
	// hand the email setting and the old type back to their defaults as well as
	// write the new rule.
	updatedConfig := notificationSettingsAccConfig(userEmail, `["ui"]`, "enabled", "service_metrics_too_many_parts", serviceID)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t, "CLICKHOUSE_TEST_USER_EMAIL") },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(
						"clickhouse_notification_settings.test", "id",
						"clickhouse_notification_settings.test", "user_id",
					),
					resource.TestCheckResourceAttr("clickhouse_notification_settings.test", "channels.#", "2"),
					resource.TestCheckResourceAttr("clickhouse_notification_settings.test", "rules.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("clickhouse_notification_settings.test", "rules.*", map[string]string{
						"effect":          "disabled",
						"notifications.#": "1",
						"notifications.0": "service_backup_failed",
						"resources.#":     "1",
						"resources.0":     serviceID,
					}),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_notification_settings.test", "channels.#", "1"),
					resource.TestCheckTypeSetElemNestedAttrs("clickhouse_notification_settings.test", "rules.*", map[string]string{
						"effect":          "enabled",
						"notifications.0": "service_metrics_too_many_parts",
					}),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName: "clickhouse_notification_settings.test",
				ImportState:  true,
				// An import brings the member under management; which rules the
				// configuration declares is not something the stored settings can
				// answer, so there is nothing to verify the imported state against.
				ImportStateVerify: false,
				ImportStateIdFunc: func(state *terraform.State) (string, error) {
					resourceState, ok := state.RootModule().Resources["clickhouse_notification_settings.test"]
					if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
						return "", fmt.Errorf("clickhouse_notification_settings.test has no state ID for import")
					}
					return resourceState.Primary.ID, nil
				},
			},
		},
	})
}

// The registry is what a rule names its notification types from, so an empty
// list would leave every rule unwritable.
func TestAccNotificationTypesDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "clickhouse_notification_types" "test" {}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.clickhouse_notification_types.test", "types.0.name"),
					resource.TestCheckResourceAttrSet("data.clickhouse_notification_types.test", "types.0.category"),
					resource.TestCheckResourceAttrSet("data.clickhouse_notification_types.test", "types.0.resource_type"),
				),
			},
		},
	})
}

func notificationSettingsAccConfig(userEmail, channels, effect, notification, resourceID string) string {
	return fmt.Sprintf(`
data "clickhouse_user" "test" {
  email = %q
}

resource "clickhouse_notification_settings" "test" {
  user_id  = data.clickhouse_user.test.id
  channels = %s

  rules = [
    {
      effect        = %q
      notifications = [%q]
      resources     = [%q]
    },
  ]
}
`, userEmail, channels, effect, notification, resourceID)
}
