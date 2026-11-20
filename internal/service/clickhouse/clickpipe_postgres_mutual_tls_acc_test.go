package clickhouse_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestAccClickPipePostgresMutualTLS creates a Postgres CDC ClickPipe that authenticates with a
// client certificate and then rotates the certificate in place.
//
// The source must be a Postgres that accepts the client certificates below (for example a
// `hostssl all all all cert` rule in pg_hba.conf) and exposes the table and publication named here:
//
//	CLICKHOUSE_TEST_SERVICE_ID                     destination ClickHouse service
//	CLICKHOUSE_TEST_POSTGRES_HOST / _PORT / _DATABASE / _USERNAME / _PASSWORD
//	CLICKHOUSE_TEST_POSTGRES_TABLE                 source table in the public schema
//	CLICKHOUSE_TEST_POSTGRES_PUBLICATION           publication that contains the table
//	CLICKHOUSE_TEST_POSTGRES_CA_CERT_PATH          CA that signed the server certificate
//	CLICKHOUSE_TEST_POSTGRES_CLIENT_CERT_PATH      client certificate and key
//	CLICKHOUSE_TEST_POSTGRES_CLIENT_KEY_PATH
//	CLICKHOUSE_TEST_POSTGRES_ROTATED_CLIENT_CERT_PATH  a second accepted client certificate and key
//	CLICKHOUSE_TEST_POSTGRES_ROTATED_CLIENT_KEY_PATH
func TestAccClickPipePostgresMutualTLS(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	variables := []string{
		"CLICKHOUSE_TEST_SERVICE_ID",
		"CLICKHOUSE_TEST_POSTGRES_HOST",
		"CLICKHOUSE_TEST_POSTGRES_PORT",
		"CLICKHOUSE_TEST_POSTGRES_DATABASE",
		"CLICKHOUSE_TEST_POSTGRES_USERNAME",
		"CLICKHOUSE_TEST_POSTGRES_PASSWORD",
		"CLICKHOUSE_TEST_POSTGRES_TABLE",
		"CLICKHOUSE_TEST_POSTGRES_PUBLICATION",
		"CLICKHOUSE_TEST_POSTGRES_CA_CERT_PATH",
		"CLICKHOUSE_TEST_POSTGRES_CLIENT_CERT_PATH",
		"CLICKHOUSE_TEST_POSTGRES_CLIENT_KEY_PATH",
		"CLICKHOUSE_TEST_POSTGRES_ROTATED_CLIENT_CERT_PATH",
		"CLICKHOUSE_TEST_POSTGRES_ROTATED_CLIENT_KEY_PATH",
	}
	readFile := func(variable string) string {
		t.Helper()
		contents, err := os.ReadFile(os.Getenv(variable))
		if err != nil {
			t.Fatalf("read %s: %v", variable, err)
		}
		return string(contents)
	}

	const resourceName = "clickhouse_clickpipe.test"
	name := fmt.Sprintf("tf-acc-postgres-mtls-%d", time.Now().UnixNano())

	var pipeID string
	checkStablePipeID := func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
			return fmt.Errorf("%s has no state ID", resourceName)
		}
		if pipeID == "" {
			pipeID = resourceState.Primary.ID
			return nil
		}
		if resourceState.Primary.ID != pipeID {
			return fmt.Errorf("ClickPipe ID changed from %q to %q: rotating the certificate must update in place", pipeID, resourceState.Primary.ID)
		}
		return nil
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccClickHousePreCheck(t, variables...) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: clickPipePostgresMutualTLSAccConfig(name,
					readFile("CLICKHOUSE_TEST_POSTGRES_CA_CERT_PATH"),
					readFile("CLICKHOUSE_TEST_POSTGRES_CLIENT_CERT_PATH"),
					readFile("CLICKHOUSE_TEST_POSTGRES_CLIENT_KEY_PATH")),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStablePipeID,
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttrSet(resourceName, "source.postgres.mutual_tls.certificate"),
					resource.TestCheckResourceAttrSet(resourceName, "source.postgres.mutual_tls.private_key"),
					resource.TestCheckResourceAttr(resourceName, "source.postgres.disable_tls", "false"),
				),
			},
			{
				Config: clickPipePostgresMutualTLSAccConfig(name,
					readFile("CLICKHOUSE_TEST_POSTGRES_CA_CERT_PATH"),
					readFile("CLICKHOUSE_TEST_POSTGRES_ROTATED_CLIENT_CERT_PATH"),
					readFile("CLICKHOUSE_TEST_POSTGRES_ROTATED_CLIENT_KEY_PATH")),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStablePipeID,
					resource.TestCheckResourceAttr(resourceName, "source.postgres.mutual_tls.certificate",
						readFile("CLICKHOUSE_TEST_POSTGRES_ROTATED_CLIENT_CERT_PATH")),
				),
			},
		},
	})
}

func clickPipePostgresMutualTLSAccConfig(name, caCertificate, clientCertificate, clientPrivateKey string) string {
	return fmt.Sprintf(`
resource "clickhouse_clickpipe" "test" {
  name       = %[1]q
  service_id = %[2]q

  source = {
    postgres = {
      host     = %[3]q
      port     = %[4]s
      database = %[5]q

      credentials = {
        username = %[6]q
        password = %[7]q
      }

      ca_certificate = %[8]q

      mutual_tls = {
        certificate = %[9]q
        private_key = %[10]q
      }

      settings = {
        replication_mode = "cdc"
        publication_name = %[12]q
      }

      table_mappings = [
        {
          source_schema_name = "public"
          source_table       = %[11]q
          target_table       = %[11]q
        }
      ]
    }
  }

  destination = {
    database = "default"
  }
}
`,
		name,
		os.Getenv("CLICKHOUSE_TEST_SERVICE_ID"),
		os.Getenv("CLICKHOUSE_TEST_POSTGRES_HOST"),
		os.Getenv("CLICKHOUSE_TEST_POSTGRES_PORT"),
		os.Getenv("CLICKHOUSE_TEST_POSTGRES_DATABASE"),
		os.Getenv("CLICKHOUSE_TEST_POSTGRES_USERNAME"),
		os.Getenv("CLICKHOUSE_TEST_POSTGRES_PASSWORD"),
		caCertificate,
		clientCertificate,
		clientPrivateKey,
		os.Getenv("CLICKHOUSE_TEST_POSTGRES_TABLE"),
		os.Getenv("CLICKHOUSE_TEST_POSTGRES_PUBLICATION"),
	)
}
