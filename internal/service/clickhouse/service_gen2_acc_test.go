package clickhouse_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

// TestAccServiceGen2 needs a development organization enabled for Gen 2 hardware and scheduled scaling;
// it runs only when CLICKHOUSE_TEST_GEN2_REGION names an AWS region where Gen 2 is available.
func TestAccServiceGen2(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	region := os.Getenv("CLICKHOUSE_TEST_GEN2_REGION")
	if region == "" {
		t.Skip("Gen 2 acceptance test skipped unless env 'CLICKHOUSE_TEST_GEN2_REGION' is set")
	}

	name := fmt.Sprintf("tf-acc-gen2-%d", time.Now().UnixNano())
	// The password is random per run and never kept: the test only manages the service, it never connects to it.
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("generating a password failed: %v", err)
	}
	hash := sha256.Sum256(secret)
	base := serviceGen2AccArgs{name: name, region: region, profile: api.ProfileGen2, passwordHash: base64.StdEncoding.EncodeToString(hash[:])}
	with := func(mut func(*serviceGen2AccArgs)) string {
		args := base
		mut(&args)
		return serviceGen2AccConfig(args)
	}
	rangedConfig := with(func(a *serviceGen2AccArgs) { a.replicas, a.minMemory, a.maxMemory = 3, 16, 32 })
	initialConfig := with(func(a *serviceGen2AccArgs) { a.replicas, a.minMemory, a.maxMemory = 3, 16, 16 })
	resizedConfig := with(func(a *serviceGen2AccArgs) { a.replicas, a.minMemory, a.maxMemory = 4, 32, 32 })
	scheduledConfig := with(func(a *serviceGen2AccArgs) {
		a.replicas, a.minMemory, a.maxMemory, a.withSchedule = 4, 32, 32, true
	})
	switchConfig := with(func(a *serviceGen2AccArgs) {
		a.profile, a.replicas, a.minMemory, a.maxMemory, a.withSchedule = "v1-default", 4, 32, 32, true
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      rangedConfig,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`use a fixed size`),
			},
			{
				Config: initialConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_service.gen2", "profile", api.ProfileGen2),
					resource.TestCheckResourceAttr("clickhouse_service.gen2", "num_replicas", "3"),
					resource.TestCheckResourceAttr("clickhouse_service.gen2", "min_replica_memory_gb", "16"),
					resource.TestCheckResourceAttr("clickhouse_service.gen2", "max_replica_memory_gb", "16"),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				Config: resizedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_service.gen2", "num_replicas", "4"),
					resource.TestCheckResourceAttr("clickhouse_service.gen2", "min_replica_memory_gb", "32"),
					resource.TestCheckResourceAttr("clickhouse_service.gen2", "max_replica_memory_gb", "32"),
				),
			},
			{
				Config:   resizedConfig,
				PlanOnly: true,
			},
			{
				Config: scheduledConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("clickhouse_service_scheduled_scaling.gen2", "entries.#", "1"),
				),
			},
			{
				Config:   scheduledConfig,
				PlanOnly: true,
			},
			{
				Config:      switchConfig,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Gen 1 and Gen 2`),
			},
		},
	})
}

type serviceGen2AccArgs struct {
	name         string
	region       string
	profile      string
	replicas     int
	minMemory    int
	maxMemory    int
	withSchedule bool
	passwordHash string
}

func serviceGen2AccConfig(args serviceGen2AccArgs) string {
	config := fmt.Sprintf(`
resource "clickhouse_service" "gen2" {
  name           = %q
  cloud_provider = "aws"
  region         = %q
  profile        = %q
  idle_scaling   = false

  ip_access = [
    {
      source      = "192.0.2.1/32"
      description = "Documentation address, never reachable"
    }
  ]

  num_replicas          = %d
  min_replica_memory_gb = %d
  max_replica_memory_gb = %d

  password_hash = %q
}
`, args.name, args.region, args.profile, args.replicas, args.minMemory, args.maxMemory, args.passwordHash)
	if !args.withSchedule {
		return config
	}
	return config + `
resource "clickhouse_service_scheduled_scaling" "gen2" {
  service_id = clickhouse_service.gen2.id

  entries = [
    {
      name                  = "Business hours"
      weekdays              = [1, 2, 3, 4, 5]
      start_hour_utc        = 8
      end_hour_utc          = 18
      min_replicas          = 4
      max_replicas          = 4
      min_replica_memory_gb = 32
      max_replica_memory_gb = 32
      idle_scaling          = false
    },
  ]
}
`
}
