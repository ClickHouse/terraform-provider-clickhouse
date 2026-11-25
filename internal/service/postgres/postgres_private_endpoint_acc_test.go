package postgres_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

const postgresPrivateEndpointTestID = "1b4e28ba-2fa1-4d3b-a3f5-ef19b5a7633b"

// A local API for the Postgres private endpoint resources. It mirrors the
// server: the private endpoint is `creating` until the first read, deletion
// is reported as `deleting` once, and endpoints come back sorted by ID.
type postgresPrivateEndpointMockAPI struct {
	t        *testing.T
	mu       sync.Mutex
	server   *httptest.Server
	endpoint *api.PostgresPrivateEndpoint
	deleting bool
	patches  int
}

func newPostgresPrivateEndpointMockAPI(t *testing.T) *postgresPrivateEndpointMockAPI {
	t.Helper()
	m := &postgresPrivateEndpointMockAPI{t: t}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

func (m *postgresPrivateEndpointMockAPI) write(w http.ResponseWriter, status int, body any) {
	w.WriteHeader(status)
	require.NoError(m.t, json.NewEncoder(w).Encode(body))
}

func (m *postgresPrivateEndpointMockAPI) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	notFound := map[string]string{"error": "NOT_FOUND: Not Found"}
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/postgres/"+postgresPrivateEndpointTestID) {
		m.write(w, http.StatusOK, map[string]any{"result": api.Postgres{Id: postgresPrivateEndpointTestID, Provider: "aws", Region: "us-east-1", State: "running"}})
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/postgres/"+postgresPrivateEndpointTestID+"/privateEndpoint") {
		m.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodPost:
		if m.endpoint != nil {
			m.write(w, http.StatusBadRequest, map[string]string{"error": "BAD_REQUEST: already exists"})
			return
		}
		var body map[string]any
		require.NoError(m.t, json.NewDecoder(r.Body).Decode(&body))
		require.Empty(m.t, body, "a create sends no fields")
		m.deleting = false
		m.endpoint = &api.PostgresPrivateEndpoint{State: "creating", AllowedVpcEndpoints: []api.PostgresAllowedVpcEndpoint{}}
	case http.MethodGet:
		switch {
		case m.endpoint == nil:
			m.write(w, http.StatusNotFound, notFound)
			return
		case m.deleting && m.endpoint.State == "deleting":
			m.endpoint = nil
			m.write(w, http.StatusNotFound, notFound)
			return
		case m.deleting:
			m.endpoint.State = "deleting"
		case m.endpoint.State == "creating":
			m.endpoint.State = api.PostgresPrivateEndpointStateAvailable
			m.endpoint.EndpointServiceName = "com.amazonaws.vpce.us-east-1.vpce-svc-0123456789abcdef0"
			m.endpoint.PrivateHostname = "pg-test.private.example.test"
		}
	case http.MethodPatch:
		if m.endpoint == nil {
			m.write(w, http.StatusNotFound, notFound)
			return
		}
		var body api.PostgresPrivateEndpointUpdate
		require.NoError(m.t, json.NewDecoder(r.Body).Decode(&body))
		require.NotNil(m.t, body.AllowedVpcEndpoints, "an update must set the allowed VPC endpoints")
		endpoints := *body.AllowedVpcEndpoints
		slices.SortFunc(endpoints, func(a, b api.PostgresAllowedVpcEndpoint) int {
			return strings.Compare(a.VpcEndpointId, b.VpcEndpointId)
		})
		m.endpoint.AllowedVpcEndpoints = endpoints
		m.patches++
	case http.MethodDelete:
		if m.endpoint == nil {
			m.write(w, http.StatusNotFound, notFound)
			return
		}
		m.deleting = true
		m.write(w, http.StatusOK, map[string]any{})
		return
	}
	m.write(w, http.StatusOK, map[string]any{"result": m.endpoint})
}

func (m *postgresPrivateEndpointMockAPI) config(endpoints string) string {
	return fmt.Sprintf(`
provider "clickhouse" {
  api_url         = %q
  organization_id = "aaaaaaaa-0000-0000-0000-000000000000"
  token_key       = "mock-key"
  token_secret    = "mock-secret"
}
resource "clickhouse_postgres_private_endpoint" "test" {
  postgres_id = %q
}
resource "clickhouse_postgres_private_endpoint_attachment" "test" {
  postgres_id           = clickhouse_postgres_private_endpoint.test.postgres_id
  allowed_vpc_endpoints = %s
}
`, m.server.URL, postgresPrivateEndpointTestID, endpoints)
}

func (m *postgresPrivateEndpointMockAPI) check(patches int, ids ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.endpoint == nil {
			return fmt.Errorf("private endpoint was deleted")
		}
		got := make([]string, 0, len(m.endpoint.AllowedVpcEndpoints))
		for _, e := range m.endpoint.AllowedVpcEndpoints {
			got = append(got, e.VpcEndpointId+"="+e.Description)
		}
		if m.patches != patches || !slices.Equal(got, ids) {
			return fmt.Errorf("expected %d PATCHes and endpoints %v; got %d PATCHes and %v", patches, ids, m.patches, got)
		}
		return nil
	}
}

func (m *postgresPrivateEndpointMockAPI) checkDestroy(*terraform.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.endpoint != nil {
		return fmt.Errorf("private endpoint was not deleted: %+v", m.endpoint)
	}
	return nil
}

func TestAccPostgresPrivateEndpointLifecycle(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	mock := newPostgresPrivateEndpointMockAPI(t)
	const batch = `"vpce-0123456789abcdef0" = {}`
	const renamed = `{ "vpce-0123456789abcdef1" = { description = "renamed" } }`
	const endpoint = "clickhouse_postgres_private_endpoint.test"
	const attachment = "clickhouse_postgres_private_endpoint_attachment.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: postgresIPProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: mock.config(`{ "vpce-0123456789abcdef1" = { description = "app" }, ` + batch + ` }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					mock.check(1, "vpce-0123456789abcdef0=", "vpce-0123456789abcdef1=app"),
					resource.TestCheckResourceAttr(endpoint, "state", "available"),
					resource.TestCheckResourceAttr(endpoint, "endpoint_service_name", "com.amazonaws.vpce.us-east-1.vpce-svc-0123456789abcdef0"),
					resource.TestCheckResourceAttr(endpoint, "private_hostname", "pg-test.private.example.test"),
					resource.TestCheckResourceAttr(attachment, "allowed_vpc_endpoints.%", "2"),
					resource.TestCheckResourceAttr(attachment, "allowed_vpc_endpoints.vpce-0123456789abcdef0.description", ""),
				),
			},
			{
				// A new description updates that endpoint in place.
				Config: mock.config(`{ "vpce-0123456789abcdef1" = { description = "renamed" }, ` + batch + ` }`),
				Check:  mock.check(2, "vpce-0123456789abcdef0=", "vpce-0123456789abcdef1=renamed"),
			},
			{Config: mock.config(renamed), Check: mock.check(3, "vpce-0123456789abcdef1=renamed")},
			{
				// A VPC endpoint added outside Terraform is removed again.
				PreConfig: func() {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					mock.endpoint.AllowedVpcEndpoints = append(mock.endpoint.AllowedVpcEndpoints,
						api.PostgresAllowedVpcEndpoint{VpcEndpointId: "vpce-0123456789abcdef2", Description: "console"})
				},
				Config: mock.config(renamed),
				Check:  mock.check(4, "vpce-0123456789abcdef1=renamed"),
			},
			{
				ResourceName: endpoint, ImportState: true, ImportStateId: postgresPrivateEndpointTestID,
				ImportStateVerify: true, ImportStateVerifyIdentifierAttribute: "postgres_id",
			},
			{
				ResourceName: attachment, ImportState: true, ImportStateId: postgresPrivateEndpointTestID,
				ImportStateVerify: true, ImportStateVerifyIdentifierAttribute: "postgres_id",
			},
			// Without the UUID check these IDs would read the Postgres service itself.
			{
				ResourceName: endpoint, ImportState: true, ImportStateId: postgresPrivateEndpointTestID + "?x=",
				ExpectError: regexp.MustCompile(`Expected\s+the\s+UUID\s+of\s+a\s+Postgres\s+service`),
			},
			{
				ResourceName: attachment, ImportState: true, ImportStateId: postgresPrivateEndpointTestID + "#",
				ExpectError: regexp.MustCompile(`Expected\s+the\s+UUID\s+of\s+a\s+Postgres\s+service`),
			},
			{Config: mock.config("{}"), Check: mock.check(5)},
		},
	})
}

// Real configs pass aws_vpc_endpoint.x.id, which is unknown until apply.
func TestAccPostgresPrivateEndpointUnknownID(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	mock := newPostgresPrivateEndpointMockAPI(t)
	config := mock.config(`{ (terraform_data.vpce.output) = {}, (terraform_data.vpce_described.output) = { description = "app" } }`) + `
resource "terraform_data" "vpce" {
  input = "vpce-0123456789abcdef3"
}
resource "terraform_data" "vpce_described" {
  input = "vpce-0123456789abcdef4"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: postgresIPProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps: []resource.TestStep{
			{Config: config, Check: mock.check(1, "vpce-0123456789abcdef3=", "vpce-0123456789abcdef4=app")},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestAccPostgresPrivateEndpointValidation(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	mock := newPostgresPrivateEndpointMockAPI(t)
	for _, tc := range []struct {
		config string
		err    string
	}{
		{strings.Replace(mock.config("{}"), postgresPrivateEndpointTestID, "pg-test", 1), `must\s+be\s+a\s+UUID`},
		{mock.config(`{ "vpce-0123456789ABCDEF0" = {} }`), `must\s+be\s+an\s+AWS\s+VPC\s+endpoint\s+ID`},
		{mock.config(`{ "vpce-0123456789abcdef0" = null }`), `Null\s+Map\s+Value`},
		{mock.config(`{ "vpce-0123456789abcdef0" = { description = " app" } }`), `does\s+not\s+start\s+or\s+end\s+with\s+whitespace`},
		{mock.config(`{ "vpce-0123456789abcdef0" = { description = "two\nlines" } }`), `must\s+be\s+one\s+line\s+of\s+printable\s+characters`},
		{mock.config(fmt.Sprintf(`{ "vpce-0123456789abcdef0" = { description = %q } }`, strings.Repeat("x", 256))), `character\s+count\s+must\s+be\s+at\s+most\s+255`},
	} {
		t.Run(tc.err, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: postgresIPProviderFactories,
				Steps: []resource.TestStep{{
					Config: tc.config, PlanOnly: true,
					ExpectError: regexp.MustCompile(tc.err),
				}},
			})
		})
	}
	// The limit counts characters, not bytes, like the API.
	t.Run("multibyte description", func(t *testing.T) {
		resource.Test(t, resource.TestCase{
			ProtoV6ProviderFactories: postgresIPProviderFactories,
			Steps: []resource.TestStep{{
				Config:   mock.config(fmt.Sprintf(`{ "vpce-0123456789abcdef0" = { description = %q } }`, strings.Repeat("é", 255))),
				PlanOnly: true, ExpectNonEmptyPlan: true,
			}},
		})
	})
}
