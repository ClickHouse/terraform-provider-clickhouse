package clickhouse_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccServiceClickHouseSettingsResource creates its own service so that it
// never changes, or resets, a setting on the shared test service.
func TestAccServiceClickHouseSettingsResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	serviceName := fmt.Sprintf("tf-acc-clickhouse-settings-%d", time.Now().UnixNano())
	initialConfig := serviceClickHouseSettingsAccConfig(serviceName, "262144")
	updatedConfig := serviceClickHouseSettingsAccConfig(serviceName, "524288")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("clickhouse_service_clickhouse_settings.test", "id", "clickhouse_service.test", "id"),
					resource.TestCheckResourceAttr("clickhouse_service_clickhouse_settings.test", "settings.%", "1"),
					resource.TestCheckResourceAttr("clickhouse_service_clickhouse_settings.test", "settings.max_query_size", "262144"),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_service_clickhouse_settings.test", "settings.%", "1"),
					resource.TestCheckResourceAttr("clickhouse_service_clickhouse_settings.test", "settings.max_query_size", "524288"),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName: "clickhouse_service_clickhouse_settings.test",
				ImportState:  true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected one imported instance, got %d", len(states))
					}
					if got := states[0].Attributes["settings.max_query_size"]; got != "524288" {
						return fmt.Errorf("imported settings.max_query_size = %q; want 524288", got)
					}
					return nil
				},
			},
		},
	})
}

func serviceClickHouseSettingsAccConfig(serviceName, maxQuerySize string) string {
	return fmt.Sprintf(`
resource "clickhouse_service" "test" {
  name                 = %q
  cloud_provider       = "aws"
  region               = "us-east-2"
  idle_scaling         = true
  idle_timeout_minutes = 5
  password_hash        = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"

  ip_access = [
    {
      source      = "192.168.2.63"
      description = "Test IP"
    }
  ]

  min_replica_memory_gb = 8
  max_replica_memory_gb = 120
}

resource "clickhouse_service_clickhouse_settings" "test" {
  service_id = clickhouse_service.test.id

  settings = {
    max_query_size = %q
  }
}
`, serviceName, maxQuerySize)
}
