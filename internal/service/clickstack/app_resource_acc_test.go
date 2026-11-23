package clickstack_test

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccAppsPreCheck skips unless Apps is known to be enabled for the target
// team: it is off by default (ENABLE_APPS self-hosted, an instance flag on
// Cloud) and every /apps call answers 403 while it is.
func testAccAppsPreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	if os.Getenv("CLICKSTACK_APPS_ENABLED") == "" {
		t.Skip("CLICKSTACK_APPS_ENABLED must be set to run the app acceptance test (skipping)")
	}
}

// TestAccAppResource exercises plan-time compile errors, create, a rename that
// keeps the version, a code change that saves a new one, and import. Requires
// TF_ACC, CLICKSTACK_API_KEY and CLICKSTACK_APPS_ENABLED.
func TestAccAppResource(t *testing.T) {
	const name = "clickhouse_clickstack_app.test"
	var firstVersion string
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccAppsPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccAppResourceConfig("tf-acc-app", "export default function App( {"),
				ExpectError: regexp.MustCompile(`Invalid app configuration`),
			},
			{
				Config: testAccAppResourceConfig("tf-acc-app", "export default function App() { return <div>One</div>; }"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(name, "id"),
					resource.TestCheckResourceAttr(name, "is_shared", "true"),
					resource.TestCheckResourceAttr(name, "purpose", "Acceptance test app."),
					resource.TestCheckResourceAttrWith(name, "version_id", func(v string) error {
						firstVersion = v
						return nil
					}),
				),
			},
			{
				Config: testAccAppResourceConfig("tf-acc-app-renamed", "export default function App() { return <div>One</div>; }"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "name", "tf-acc-app-renamed"),
					resource.TestCheckResourceAttrWith(name, "version_id", func(v string) error {
						if v != firstVersion {
							return fmt.Errorf("rename changed version_id from %s to %s", firstVersion, v)
						}
						return nil
					}),
				),
			},
			{
				Config: testAccAppResourceConfig("tf-acc-app-renamed", "export default function App() { return <div>Two</div>; }"),
				Check: resource.TestCheckResourceAttrWith(name, "version_id", func(v string) error {
					if v == firstVersion {
						return fmt.Errorf("code change kept version_id %s", v)
					}
					return nil
				}),
			},
			{
				ResourceName:            name,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"team"},
			},
		},
	})
}

func testAccAppResourceConfig(name, code string) string {
	return fmt.Sprintf(`
resource "clickhouse_clickstack_app" "test" {
  name    = %q
  purpose = "Acceptance test app."
  code    = %q
}
`, name, code)
}
