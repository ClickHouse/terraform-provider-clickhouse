package clickhouse_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccServiceClickhouseSettingsResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	serviceID := os.Getenv("CLICKHOUSE_TEST_SERVICE_ID")
	initialConfig := serviceClickhouseSettingsAccConfig(serviceID, `max_query_size = "524288"`)
	updatedConfig := serviceClickhouseSettingsAccConfig(serviceID, `http_max_field_value_size = "262144"`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_service_clickhouse_settings.test", "id", serviceID),
					resource.TestCheckResourceAttr("clickhouse_service_clickhouse_settings.test", "settings.%", "1"),
					resource.TestCheckResourceAttr("clickhouse_service_clickhouse_settings.test", "settings.max_query_size", "524288"),
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
					resource.TestCheckResourceAttr("clickhouse_service_clickhouse_settings.test", "settings.http_max_field_value_size", "262144"),
					resource.TestCheckNoResourceAttr("clickhouse_service_clickhouse_settings.test", "settings.max_query_size"),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
		},
	})
}

func serviceClickhouseSettingsAccConfig(serviceID, setting string) string {
	return fmt.Sprintf(`
resource "clickhouse_service_clickhouse_settings" "test" {
  service_id = %q

  settings = {
    %s
  }
}
`, serviceID, setting)
}
