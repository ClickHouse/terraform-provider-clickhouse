package clickhouse_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// GitHub Actions OIDC exposes a real, public, RS256-only JWKS, so the control
// plane's JWKS fetch and validation succeed against it.
const (
	githubActionsIssuer  = "https://token.actions.githubusercontent.com"
	githubActionsJWKSURL = "https://token.actions.githubusercontent.com/.well-known/jwks"
)

func TestAccServiceJWTProviderResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	// JWT providers are an Enterprise-tier functionality, so opt in only when
	// the target organization has it.
	if os.Getenv("CLICKHOUSE_TEST_JWT_PROVIDERS") != "yes" {
		t.Skip("set CLICKHOUSE_TEST_JWT_PROVIDERS=yes when the target organization can use JWT providers")
	}

	serviceID := os.Getenv("CLICKHOUSE_TEST_SERVICE_ID")
	name := fmt.Sprintf("tf-acc-%d", time.Now().UnixNano())
	const address = "clickhouse_service_jwt_provider.test"

	initialConfig := serviceJWTProviderAccConfig(serviceID, name, "tf-acc-audience", `roles_claim = "clickhouse:roles"`)
	updatedConfig := serviceJWTProviderAccConfig(serviceID, name, "tf-acc-audience-updated", "")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "service_id", serviceID),
					resource.TestCheckResourceAttr(address, "name", name),
					resource.TestCheckResourceAttr(address, "issuer", githubActionsIssuer),
					resource.TestCheckResourceAttr(address, "audience", "tf-acc-audience"),
					resource.TestCheckResourceAttr(address, "jwks_url", githubActionsJWKSURL),
					resource.TestCheckResourceAttr(address, "roles_claim", "clickhouse:roles"),
				),
			},
			{
				Config:   initialConfig,
				PlanOnly: true,
			},
			{
				// Removing roles_claim and changing the audience updates in place.
				Config: updatedConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "name", name),
					resource.TestCheckResourceAttr(address, "audience", "tf-acc-audience-updated"),
					resource.TestCheckNoResourceAttr(address, "roles_claim"),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName:                         address,
				ImportState:                          true,
				ImportStateId:                        serviceID + ":" + name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
		},
	})
}

func serviceJWTProviderAccConfig(serviceID, name, audience, extra string) string {
	return fmt.Sprintf(`
resource "clickhouse_service_jwt_provider" "test" {
  service_id = %q
  name       = %q

  issuer   = %q
  audience = %q
  jwks_url = %q
  %s
}
`, serviceID, name, githubActionsIssuer, audience, githubActionsJWKSURL, extra)
}
