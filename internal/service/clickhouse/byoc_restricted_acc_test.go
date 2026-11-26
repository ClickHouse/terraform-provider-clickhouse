package clickhouse_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func byocRestrictedAccConfig(apiURL, displayName string) string {
	return byocAccProviderConfig(apiURL) + fmt.Sprintf(`
resource "clickhouse_byoc_infrastructure" "test" {
  region_id    = "us-east-1"
  account_id   = "999999999999"
  display_name = %q
  access_model = "restricted"

  restricted_facts = {
    cluster_name       = "ra-cluster"
    availability_zones = ["us-east-1a", "us-east-1b", "us-east-1c"]
    image_mode         = "direct"
    executor_role_arn  = "arn:aws:iam::999999999999:role/executor"
    node_role_arns     = ["arn:aws:iam::999999999999:role/node"]
    data_bucket        = "ra-data"
    backup_bucket      = "ra-backup"
    storage_role_arn   = "arn:aws:iam::999999999999:role/storage"
    enable_monitoring  = false
  }
}

resource "clickhouse_byoc_enrollment_token" "test" {
  byoc_id     = clickhouse_byoc_infrastructure.test.id
  ttl_seconds = 7200
}

resource "clickhouse_byoc_infrastructure_ready" "test" {
  byoc_id = clickhouse_byoc_infrastructure.test.id

  # Stands in for the connector installation that redeems the token.
  depends_on = [clickhouse_byoc_enrollment_token.test]
}

data "clickhouse_byoc_infrastructure" "test" {
  id         = clickhouse_byoc_infrastructure_ready.test.id
}
`, displayName)
}

// TestAccByocInfrastructureRestricted covers the restricted-access flow end
// to end: registration returns while awaiting the connector, the token is
// minted once (after the trust-admission conflict) and never on refresh, the
// readiness gate waits for the connector, and the API-refused endpoints
// (tags, customer-authorized access) are never called.
func TestAccByocInfrastructureRestricted(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	mock := newByocMockAPI(t)
	mock.tokenConflicts = 1
	tracker := &byocIDTracker{}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: byocRestrictedAccConfig(mock.server.URL, "tf-acc-byoc-ra"),
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkStable,
					mock.checkCreateCount(1),
					mock.checkTokensMinted(1),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "access_model", "restricted"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "state", api.ByocStateAwaitingConnector),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "restricted_facts.platform_node_group", "system"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "tags.%", "0"),
					resource.TestCheckResourceAttr("clickhouse_byoc_enrollment_token.test", "token", "enroll-byoc-mock-1-1"),
					resource.TestCheckResourceAttr("clickhouse_byoc_enrollment_token.test", "expires_at", "2026-01-01T04:00:00Z"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure_ready.test", "state", api.ByocStateReady),
					resource.TestCheckResourceAttr("data.clickhouse_byoc_infrastructure.test", "access_model", "restricted"),
					resource.TestCheckResourceAttr("data.clickhouse_byoc_infrastructure.test", "state", api.ByocStateReady),
					resource.TestCheckResourceAttr("data.clickhouse_byoc_infrastructure.test", "connector.components.0.name", "executor"),
					resource.TestCheckResourceAttr("data.clickhouse_byoc_infrastructure.test", "restricted_facts.cluster_name", "ra-cluster"),
					func(*terraform.State) error {
						call := mock.lastCreateCall()
						if err := call.requireAbsentKeys("tags", "externalId", "vpcCidrRange"); err != nil {
							return err
						}
						if call.request.RestrictedFacts == nil || call.request.RestrictedFacts.PlatformNodeGroup != nil {
							return fmt.Errorf("create must send the facts and leave platformNodeGroup to the API default: %+v", call.request.RestrictedFacts)
						}
						mock.mu.Lock()
						defer mock.mu.Unlock()
						if len(mock.validateBodies) != 1 || mock.validateBodies[0].RestrictedFacts == nil {
							return fmt.Errorf("the preflight validation must receive the restricted facts: %+v", mock.validateBodies)
						}
						return nil
					},
				),
			},
			{
				// The refresh picks up infra-ready; nothing is re-minted or replaced.
				Config:   byocRestrictedAccConfig(mock.server.URL, "tf-acc-byoc-ra"),
				PlanOnly: true,
			},
			{
				Config: byocRestrictedAccConfig(mock.server.URL, "tf-acc-byoc-ra-renamed"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("clickhouse_byoc_infrastructure.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectResourceAction("clickhouse_byoc_enrollment_token.test", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkStable,
					mock.checkTokensMinted(1),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "display_name", "tf-acc-byoc-ra-renamed"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "state", api.ByocStateReady),
				),
			},
			{
				ResourceName:      "clickhouse_byoc_infrastructure.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccByocInfrastructureRestrictedRejectsStandardOnlyAttributes covers the
// plan-time refusals: the API would reject these on create, so they must
// fail before any call.
func TestAccByocInfrastructureRestrictedRejectsStandardOnlyAttributes(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	mock := newByocMockAPI(t)
	config := func(extra string) string {
		return byocAccProviderConfig(mock.server.URL) + fmt.Sprintf(`
resource "clickhouse_byoc_infrastructure" "test" {
  region_id    = "us-east-1"
  account_id   = "999999999999"
  access_model = "restricted"
%s
  restricted_facts = {
    cluster_name     = "ra-cluster"
    image_mode       = "bundle"
    image_registry   = "registry.example.com/clickhouse"
    data_bucket      = "ra-data"
    backup_bucket    = "ra-backup"
    storage_role_arn = "arn:aws:iam::999999999999:role/storage"
    enable_monitoring = false
  }
}
`, extra)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      config(`  tags = { team = "data" }`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`tags is not supported with access_model = "restricted"`),
			},
			{
				Config:      config(`  enable_private_link = true`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`enable_private_link is not supported`),
			},
			{
				Config:      config(`  vpc_cidr_range = "10.0.0.0/16"`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`vpc_cidr_range is not supported`),
			},
		},
	})
	if got := mock.createCount(); got != 0 {
		t.Errorf("expected no create calls, got %d", got)
	}
}

// TestAccByocInfrastructureRestrictedStagingRead reads an existing
// restricted-access infrastructure from a development ClickHouse Cloud,
// through the data sources only: the provider must not call the endpoints
// the API refuses for restricted access. It never manages the infrastructure,
// so it cannot destroy it.
func TestAccByocInfrastructureRestrictedStagingRead(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	byocID := os.Getenv("CLICKHOUSE_TEST_BYOC_RA_INFRA_ID")
	if byocID == "" {
		t.Skip("set CLICKHOUSE_TEST_BYOC_RA_INFRA_ID to the ID of a restricted-access BYOC infrastructure in the development organization")
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHouseReadOnlyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "clickhouse_byoc_infrastructure" "ra" {
  id = %q
}

data "clickhouse_byoc_infrastructures" "all" {}
`, byocID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.clickhouse_byoc_infrastructure.ra", "access_model", "restricted"),
					resource.TestCheckResourceAttr("data.clickhouse_byoc_infrastructure.ra", "cloud_provider", "aws"),
					resource.TestCheckResourceAttrSet("data.clickhouse_byoc_infrastructure.ra", "state"),
					resource.TestCheckResourceAttrSet("data.clickhouse_byoc_infrastructure.ra", "restricted_facts.cluster_name"),
					resource.TestCheckResourceAttrSet("data.clickhouse_byoc_infrastructure.ra", "restricted_facts.image_mode"),
					resource.TestCheckResourceAttr("data.clickhouse_byoc_infrastructure.ra", "tags.%", "0"),
					resource.TestCheckNoResourceAttr("data.clickhouse_byoc_infrastructure.ra", "customer_authorized_access.enabled"),
					resource.TestCheckTypeSetElemNestedAttrs("data.clickhouse_byoc_infrastructures.all", "infrastructures.*", map[string]string{
						"id":           byocID,
						"access_model": "restricted",
					}),
				),
			},
		},
	})
}
