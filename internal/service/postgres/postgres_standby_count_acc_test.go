package postgres_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Exercise Terraform's plan/apply consistency checks for ha_type and
// standby_count against the local API. Uses no Cloud credentials.
func TestAccPostgresStandbyCountLifecycle(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	mock := newPostgresIPMockAPI(t)
	const name = "clickhouse_postgres_service.test"
	check := func(patches int, haType string, standbyCount string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			func(*terraform.State) error {
				mock.mu.Lock()
				defer mock.mu.Unlock()
				if mock.patches != patches {
					return fmt.Errorf("expected %d PATCHes; got %d", patches, mock.patches)
				}
				return nil
			},
			resource.TestCheckResourceAttr(name, "ha_type", haType),
			resource.TestCheckResourceAttr(name, "standby_count", standbyCount),
			resource.TestCheckResourceAttr("data.clickhouse_postgres_service.test", "tags.environment", "test"),
			resource.TestCheckResourceAttr("data.clickhouse_postgres_service.test", "standby_count", standbyCount),
		)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: postgresIPProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: mock.config("ha_type = \"sync\"\nstandby_count = 1", "test"),
				Check: resource.ComposeAggregateTestCheckFunc(
					check(0, "sync", "1"),
					func(*terraform.State) error {
						mock.mu.Lock()
						defer mock.mu.Unlock()
						if mock.create.HaType != "sync" || mock.create.StandbyCount == nil || *mock.create.StandbyCount != 1 {
							return fmt.Errorf("create must send haType=sync and standbyCount=1; got %+v", mock.create)
						}
						return nil
					},
				),
			},
			// Omitting standby_count keeps the current count while ha_type is unchanged.
			{Config: mock.config(`ha_type = "sync"`, "test"), Check: check(0, "sync", "1")},
			// A count-only change is PATCHed.
			{Config: mock.config("ha_type = \"sync\"\nstandby_count = 2", "test"), Check: check(1, "sync", "2")},
			// A new ha_type without standby_count takes that type's default.
			{Config: mock.config(`ha_type = "async"`, "test"), Check: check(2, "async", "1")},
		},
	})
}

func TestAccPostgresStandbyCountValidation(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	mock := newPostgresIPMockAPI(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: postgresIPProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      mock.config("ha_type = \"async\"\nstandby_count = 2", "test"),
				ExpectError: regexp.MustCompile(`Unsupported standby_count for ha_type`),
			},
			{
				Config:      mock.config("standby_count = 1", "test"),
				ExpectError: regexp.MustCompile(`ha_type`),
			},
		},
	})
}
