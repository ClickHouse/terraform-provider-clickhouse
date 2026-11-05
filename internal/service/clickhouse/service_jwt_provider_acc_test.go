package clickhouse_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

// GitHub Actions OIDC exposes a real, public, RS256-only JWKS, so the control
// plane's JWKS fetch and validation succeed against it.
const (
	githubActionsIssuer  = "https://token.actions.githubusercontent.com"
	githubActionsJWKSURL = "https://token.actions.githubusercontent.com/.well-known/jwks"

	jwtProviderAddress = "clickhouse_service_jwt_provider.test"
)

func TestAccServiceJWTProviderResource(t *testing.T) {
	client := jwtProviderAccPreCheck(t)

	serviceID := os.Getenv("CLICKHOUSE_TEST_SERVICE_ID")
	name := jwtProviderAccName("tf-acc")

	initialConfig := serviceJWTProviderAccConfig(serviceID, name, "tf-acc-audience", `roles_claim = "clickhouse:roles"`)
	updatedConfig := serviceJWTProviderAccConfig(serviceID, name, "tf-acc-audience-updated", "")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJWTProvidersDestroyed(client),
		Steps: []resource.TestStep{
			{
				Config: initialConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(jwtProviderAddress, "service_id", serviceID),
					resource.TestCheckResourceAttr(jwtProviderAddress, "name", name),
					resource.TestCheckResourceAttr(jwtProviderAddress, "issuer", githubActionsIssuer),
					resource.TestCheckResourceAttr(jwtProviderAddress, "audience", "tf-acc-audience"),
					resource.TestCheckResourceAttr(jwtProviderAddress, "jwks_url", githubActionsJWKSURL),
					resource.TestCheckResourceAttr(jwtProviderAddress, "roles_claim", "clickhouse:roles"),
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
					resource.TestCheckResourceAttr(jwtProviderAddress, "name", name),
					resource.TestCheckResourceAttr(jwtProviderAddress, "audience", "tf-acc-audience-updated"),
					resource.TestCheckNoResourceAttr(jwtProviderAddress, "roles_claim"),
					checkJWTProviderInAPI(client, serviceID, api.JWTProvider{
						Name: name, Issuer: githubActionsIssuer, Audience: "tf-acc-audience-updated", JWKSURL: githubActionsJWKSURL,
					}),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
			{
				ResourceName:                         jwtProviderAddress,
				ImportState:                          true,
				ImportStateId:                        serviceID + ":" + name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
		},
	})
}

// TestAccServiceJWTProviderResourceImportExisting covers a customer bringing
// a provider they created outside Terraform (e.g. in the console) under
// management with `terraform import`: the imported state matches the API, a
// config describing it plans no changes, and Terraform can then update and
// delete it.
func TestAccServiceJWTProviderResourceImportExisting(t *testing.T) {
	client := jwtProviderAccPreCheck(t)

	serviceID := os.Getenv("CLICKHOUSE_TEST_SERVICE_ID")
	name := jwtProviderAccName("tf-acc-import")
	rolesClaim := "clickhouse:roles"
	existing := api.JWTProvider{
		Name: name, Issuer: githubActionsIssuer, Audience: "tf-acc-existing", JWKSURL: githubActionsJWKSURL, RolesClaim: &rolesClaim,
	}
	createJWTProviderOutsideTerraform(t, client, serviceID, existing)

	importedConfig := serviceJWTProviderAccConfig(serviceID, name, "tf-acc-existing", `roles_claim = "clickhouse:roles"`)
	updatedConfig := serviceJWTProviderAccConfig(serviceID, name, "tf-acc-existing-updated", "")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJWTProvidersDestroyed(client),
		Steps: []resource.TestStep{
			{
				Config:             importedConfig,
				ResourceName:       jwtProviderAddress,
				ImportState:        true,
				ImportStateId:      serviceID + ":" + name,
				ImportStatePersist: true,
				ImportStateCheck: checkImportedJWTProvider(map[string]string{
					"service_id":  serviceID,
					"name":        name,
					"issuer":      githubActionsIssuer,
					"audience":    "tf-acc-existing",
					"jwks_url":    githubActionsJWKSURL,
					"roles_claim": rolesClaim,
				}),
			},
			{
				// A config that describes the imported provider plans no changes.
				Config:   importedConfig,
				PlanOnly: true,
			},
			{
				// Terraform now owns it: changes apply in place, without replacement.
				Config: updatedConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(jwtProviderAddress, plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(jwtProviderAddress, "audience", "tf-acc-existing-updated"),
					resource.TestCheckNoResourceAttr(jwtProviderAddress, "roles_claim"),
					checkJWTProviderInAPI(client, serviceID, api.JWTProvider{
						Name: name, Issuer: githubActionsIssuer, Audience: "tf-acc-existing-updated", JWKSURL: githubActionsJWKSURL,
					}),
				),
			},
			{
				Config:   updatedConfig,
				PlanOnly: true,
			},
		},
	})
}

// TestAccServiceJWTProviderResourceImportBlock covers config-driven import
// (Terraform 1.5+ `import` blocks) of a provider created outside Terraform
// without a roles claim: the plan imports it with no changes, and once the
// import block is removed the plan stays empty.
func TestAccServiceJWTProviderResourceImportBlock(t *testing.T) {
	client := jwtProviderAccPreCheck(t)

	serviceID := os.Getenv("CLICKHOUSE_TEST_SERVICE_ID")
	name := jwtProviderAccName("tf-acc-import-block")
	existing := api.JWTProvider{
		Name: name, Issuer: githubActionsIssuer, Audience: "tf-acc-existing", JWKSURL: githubActionsJWKSURL,
	}
	createJWTProviderOutsideTerraform(t, client, serviceID, existing)

	config := serviceJWTProviderAccConfig(serviceID, name, "tf-acc-existing", "")
	importBlock := fmt.Sprintf(`
import {
  to = %s
  id = "%s:%s"
}
`, jwtProviderAddress, serviceID, name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJWTProvidersDestroyed(client),
		Steps: []resource.TestStep{
			{
				Config: importBlock + config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(jwtProviderAddress, plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(jwtProviderAddress, "service_id", serviceID),
					resource.TestCheckResourceAttr(jwtProviderAddress, "name", name),
					resource.TestCheckResourceAttr(jwtProviderAddress, "issuer", githubActionsIssuer),
					resource.TestCheckResourceAttr(jwtProviderAddress, "audience", "tf-acc-existing"),
					resource.TestCheckResourceAttr(jwtProviderAddress, "jwks_url", githubActionsJWKSURL),
					resource.TestCheckNoResourceAttr(jwtProviderAddress, "roles_claim"),
				),
			},
			{
				Config:   config,
				PlanOnly: true,
			},
		},
	})
}

// TestAccServiceJWTProviderResourceExistingName covers creating a resource
// whose name is already taken on the service: with a different configuration
// the apply fails, leaves the existing provider alone and points at
// `terraform import`; with the same configuration the resource adopts it.
func TestAccServiceJWTProviderResourceExistingName(t *testing.T) {
	client := jwtProviderAccPreCheck(t)

	serviceID := os.Getenv("CLICKHOUSE_TEST_SERVICE_ID")
	name := jwtProviderAccName("tf-acc-existing")
	rolesClaim := "clickhouse:roles"
	existing := api.JWTProvider{
		Name: name, Issuer: githubActionsIssuer, Audience: "tf-acc-existing", JWKSURL: githubActionsJWKSURL, RolesClaim: &rolesClaim,
	}
	createJWTProviderOutsideTerraform(t, client, serviceID, existing)

	sameConfig := serviceJWTProviderAccConfig(serviceID, name, "tf-acc-existing", `roles_claim = "clickhouse:roles"`)
	// Terraform wraps long diagnostics, so allow any whitespace between words.
	importHint := regexp.MustCompile(`(?s)different\s+configuration.*terraform\s+import\s+clickhouse_service_jwt_provider\.<name>\s+` +
		regexp.QuoteMeta(serviceID+":"+name))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkJWTProvidersDestroyed(client),
		Steps: []resource.TestStep{
			{
				Config:      serviceJWTProviderAccConfig(serviceID, name, "tf-acc-different", `roles_claim = "clickhouse:roles"`),
				ExpectError: importHint,
			},
			{
				// The failed create left the existing provider as it was, and
				// an identical configuration adopts it.
				Config: sameConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(jwtProviderAddress, "audience", "tf-acc-existing"),
					resource.TestCheckResourceAttr(jwtProviderAddress, "roles_claim", rolesClaim),
					checkJWTProviderInAPI(client, serviceID, existing),
				),
			},
			{
				Config:   sameConfig,
				PlanOnly: true,
			},
		},
	})
}

// jwtProviderAccPreCheck skips unless the JWT provider acceptance tests are
// enabled and returns an API client for checks made outside Terraform.
func jwtProviderAccPreCheck(t *testing.T) api.Client {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	// JWT providers are an Enterprise-tier functionality, so opt in only when
	// the target organization has it.
	if os.Getenv("CLICKHOUSE_TEST_JWT_PROVIDERS") != "yes" {
		t.Skip("set CLICKHOUSE_TEST_JWT_PROVIDERS=yes when the target organization can use JWT providers")
	}
	testAccClickHousePreCheck(t)

	client, err := api.NewClient(api.ClientConfig{
		ApiURL:         os.Getenv("CLICKHOUSE_API_URL"),
		OrganizationID: os.Getenv("CLICKHOUSE_ORG_ID"),
		TokenKey:       os.Getenv("CLICKHOUSE_CLOUD_API_KEY"),
		TokenSecret:    os.Getenv("CLICKHOUSE_CLOUD_API_SECRET"),
	})
	if err != nil {
		t.Fatalf("create API client: %v", err)
	}
	return client
}

func jwtProviderAccName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// createJWTProviderOutsideTerraform creates a provider directly through the
// API, as a customer would have before adopting Terraform. It is deleted when
// the test ends if Terraform did not already delete it.
func createJWTProviderOutsideTerraform(t *testing.T, client api.Client, serviceID string, provider api.JWTProvider) {
	t.Helper()
	if _, err := client.CreateJWTProvider(context.Background(), serviceID, provider); err != nil {
		t.Fatalf("create JWT provider %q outside Terraform: %v", provider.Name, err)
	}
	t.Cleanup(func() {
		err := client.DeleteJWTProvider(context.Background(), serviceID, provider.Name)
		if err != nil && !api.IsNotFound(err) {
			t.Errorf("clean up JWT provider %q: %v", provider.Name, err)
		}
	})
}

func checkImportedJWTProvider(want map[string]string) resource.ImportStateCheckFunc {
	return func(states []*terraform.InstanceState) error {
		if len(states) != 1 {
			return fmt.Errorf("imported %d instances; want 1", len(states))
		}
		for key, value := range want {
			if got := states[0].Attributes[key]; got != value {
				return fmt.Errorf("imported %s = %q; want %q", key, got, value)
			}
		}
		return nil
	}
}

// checkJWTProviderInAPI reads the provider back from the API, independently
// of the Terraform state.
func checkJWTProviderInAPI(client api.Client, serviceID string, want api.JWTProvider) resource.TestCheckFunc {
	return func(*terraform.State) error {
		got, err := client.GetJWTProvider(context.Background(), serviceID, want.Name)
		if err != nil {
			return fmt.Errorf("get JWT provider %q: %w", want.Name, err)
		}
		gotRoles, wantRoles := "<none>", "<none>"
		if got.RolesClaim != nil {
			gotRoles = *got.RolesClaim
		}
		if want.RolesClaim != nil {
			wantRoles = *want.RolesClaim
		}
		if got.Name != want.Name || got.Issuer != want.Issuer || got.Audience != want.Audience ||
			got.JWKSURL != want.JWKSURL || gotRoles != wantRoles {
			return fmt.Errorf("API has JWT provider %+v (roles claim %s); want %+v (roles claim %s)", *got, gotRoles, want, wantRoles)
		}
		return nil
	}
}

func checkJWTProvidersDestroyed(client api.Client) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		for _, rs := range state.RootModule().Resources {
			if rs.Type != "clickhouse_service_jwt_provider" {
				continue
			}
			serviceID, name := rs.Primary.Attributes["service_id"], rs.Primary.Attributes["name"]
			_, err := client.GetJWTProvider(context.Background(), serviceID, name)
			if err == nil {
				return fmt.Errorf("JWT provider %q still exists on service %s", name, serviceID)
			}
			if !api.IsNotFound(err) {
				return fmt.Errorf("check JWT provider %q was deleted: %w", name, err)
			}
		}
		return nil
	}
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
