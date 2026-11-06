package postgres_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/provider"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres"
)

// Exercise Terraform's plan/apply/refresh/import consistency checks against
// a local API, including the backend's CIDR/port normalization and rule IDs.
// These tests use no Cloud credentials or live services.
type postgresIPMockAPI struct {
	t          *testing.T
	mu         sync.Mutex
	server     *httptest.Server
	pg         *api.Postgres
	nextRuleID int
	patches    int
	create     api.PostgresCreate
}

func newPostgresIPMockAPI(t *testing.T) *postgresIPMockAPI {
	t.Helper()
	m := &postgresIPMockAPI{t: t}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

func (m *postgresIPMockAPI) normalize(rule api.PostgresIpAccess) api.PostgresIpAccess {
	if rule.Id == "" {
		m.nextRuleID++
		rule.Id = fmt.Sprintf("opaque-rule-%d", m.nextRuleID)
	}
	source := rule.Source
	if !strings.Contains(source, "/") {
		source += "/32"
	}
	prefix, err := netip.ParsePrefix(source)
	require.NoError(m.t, err)
	rule.Source = prefix.Masked().String()
	ports := strings.Split(rule.PortRange, "..")
	for i, port := range ports {
		value, err := strconv.Atoi(port)
		require.NoError(m.t, err)
		ports[i] = strconv.Itoa(value)
	}
	if len(ports) == 2 && ports[0] == ports[1] {
		ports = ports[:1]
	}
	rule.PortRange = strings.Join(ports, "..")
	// Ubicloud uses Ruby String#strip, which preserves Unicode whitespace.
	rule.Description = strings.Trim(rule.Description, "\x00\t\n\v\f\r ")
	return rule
}

func (m *postgresIPMockAPI) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	var result any
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/postgres"):
		require.NoError(m.t, json.NewDecoder(r.Body).Decode(&m.create))
		body := m.create
		rules := []api.PostgresIpAccess{
			{Source: "0.0.0.0/0", PortRange: "5432", Description: "Anywhere"},
			{Source: "0.0.0.0/0", PortRange: "6432", Description: "Anywhere"},
		}
		if body.IpAccessList != nil {
			rules = *body.IpAccessList
		}
		m.pg = &api.Postgres{
			Id: "pg-test", Name: body.Name, Provider: body.Provider, Region: body.Region,
			Size: body.Size, HaType: "none", PostgresVersion: "18", IsPrimary: true,
			State: "running", CreatedAt: "2026-10-01T00:00:00Z",
			Hostname: "pg.example.test", Username: "postgres", Tags: body.Tags,
			IpAccessList: make([]api.PostgresIpAccess, 0, len(rules)),
		}
		for _, rule := range rules {
			m.pg.IpAccessList = append(m.pg.IpAccessList, m.normalize(rule))
		}
		result = m.pg
	case m.pg == nil:
		w.WriteHeader(http.StatusNotFound)
		result = map[string]string{"error": "not found"}
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/config"):
		result = api.PostgresConfig{PgConfig: api.PgConfigMap{}, PgBouncerConfig: api.PgConfigMap{}}
	case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/password"):
		result = api.PostgresPassword{}
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/postgres/pg-test"):
		copy := *m.pg
		copy.IpAccessList = slices.Clone(m.pg.IpAccessList)
		slices.Reverse(copy.IpAccessList)
		result = copy
	case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/postgres/pg-test"):
		var body api.PostgresUpdate
		require.NoError(m.t, json.NewDecoder(r.Body).Decode(&body))
		if body.IpAccessList != nil {
			// Metadata and firewall changes must use separate requests, allowing
			// firewall updates on read replicas and preserving tags.
			require.Nil(m.t, body.Tags)
			require.Empty(m.t, body.Size)
			require.Empty(m.t, body.HaType)
			m.patches++
			for _, remove := range body.IpAccessList.Remove {
				index := slices.IndexFunc(m.pg.IpAccessList, func(rule api.PostgresIpAccess) bool { return rule.Id == remove.Id })
				require.NotEqual(m.t, -1, index, "unknown remove ID")
				m.pg.IpAccessList = slices.Delete(m.pg.IpAccessList, index, index+1)
			}
			for _, add := range body.IpAccessList.Add {
				if add.Id == "" {
					m.pg.IpAccessList = append(m.pg.IpAccessList, m.normalize(add))
				} else {
					index := slices.IndexFunc(m.pg.IpAccessList, func(rule api.PostgresIpAccess) bool { return rule.Id == add.Id })
					require.NotEqual(m.t, -1, index, "unknown edit ID")
					m.pg.IpAccessList[index] = m.normalize(add)
				}
			}
		} else if body.Tags != nil {
			m.pg.Tags = *body.Tags
		} else {
			m.t.Errorf("unexpected metadata PATCH: %+v", body)
		}
		result = m.pg
	case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/postgres/pg-test"):
		m.pg = nil
		result = map[string]any{}
	default:
		m.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	}
	require.NoError(m.t, json.NewEncoder(w).Encode(map[string]any{"result": result}))
}

func (m *postgresIPMockAPI) config(access, tag string) string {
	return fmt.Sprintf(`
provider "clickhouse" {
  api_url         = %q
  organization_id = "aaaaaaaa-0000-0000-0000-000000000000"
  token_key       = "mock-key"
  token_secret    = "mock-secret"
}
resource "clickhouse_postgres_service" "test" {
  name           = "tf-ip-access"
  cloud_provider = "aws"
  region         = "us-east-1"
  size           = "m6gd.large"
  password       = "Example123Secret"
  tags           = { environment = %q }
  %s
}
data "clickhouse_postgres_service" "test" {
  id = clickhouse_postgres_service.test.id
  depends_on = [clickhouse_postgres_service.test]
}
`, m.server.URL, tag, access)
}

func (m *postgresIPMockAPI) check(patches, rules int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.patches != patches || m.pg == nil || len(m.pg.IpAccessList) != rules {
			return fmt.Errorf("expected %d PATCHes and %d rules; got %d PATCHes and service %+v", patches, rules, m.patches, m.pg)
		}
		return nil
	}
}

func (m *postgresIPMockAPI) checkDestroy(*terraform.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pg != nil {
		return fmt.Errorf("service was not deleted")
	}
	return nil
}

var postgresIPProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"clickhouse": providerserver.NewProtocol6WithError(provider.NewBuilder([]service.ServicePackage{postgres.ServicePackage()})()),
}

func TestAccPostgresIPAccessLifecycle(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	mock := newPostgresIPMockAPI(t)
	const office = `{source="192.0.2.17/24", port_range="05432..05432", description=" office "}`
	const pooler = `{source="192.0.2.1", port_range="6432", description="pooler"}`
	initial := "ip_access = [" + office + "," + pooler + "]"
	reordered := "ip_access = [" + pooler + "," + office + "]"
	changed := `ip_access = [
		{source="192.0.2.0/24", port_range="5432", description=""},
		{source="198.51.100.0/24", port_range="5432..6432", description="new"}
	]`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: postgresIPProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: mock.config(initial, "test"),
				Check: resource.ComposeAggregateTestCheckFunc(
					mock.check(0, 2),
					resource.TestCheckResourceAttr("clickhouse_postgres_service.test", "ip_access.0.source", "192.0.2.17/24"),
					resource.TestCheckResourceAttr("clickhouse_postgres_service.test", "ip_access.0.port_range", "05432..05432"),
					resource.TestCheckResourceAttr("clickhouse_postgres_service.test", "ip_access.0.description", " office "),
					resource.TestCheckResourceAttr("data.clickhouse_postgres_service.test", "ip_access.#", "2"),
					resource.TestCheckResourceAttr("data.clickhouse_postgres_service.test", "ip_access.0.source", "192.0.2.1/32"),
					resource.TestCheckResourceAttr("data.clickhouse_postgres_service.test", "ip_access.0.port_range", "6432"),
					resource.TestCheckResourceAttr("data.clickhouse_postgres_service.test", "ip_access.0.description", "pooler"),
				),
			},
			{Config: mock.config(reordered, "test"), Check: mock.check(0, 2)},
			{
				Config: mock.config(changed, "updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					mock.check(1, 2),
					resource.TestCheckResourceAttr("clickhouse_postgres_service.test", "tags.environment", "updated"),
					func(*terraform.State) error {
						mock.mu.Lock()
						defer mock.mu.Unlock()
						if mock.pg.IpAccessList[0].Id != "opaque-rule-1" || mock.pg.IpAccessList[0].Description != "" {
							return fmt.Errorf("description edit must retain the original rule ID")
						}
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					mock.pg.IpAccessList[0].Description = "external edit"
					mock.pg.IpAccessList = append(mock.pg.IpAccessList, mock.normalize(api.PostgresIpAccess{
						Source: "203.0.113.1", PortRange: "5432", Description: "out of band",
					}))
				},
				Config: mock.config(changed, "updated"),
				Check:  mock.check(2, 2),
			},
			{
				ResourceName: "clickhouse_postgres_service.test", ImportState: true,
				ImportStateVerify: true, ImportStateVerifyIgnore: []string{"password", "ip_access"},
				// Import has no prior list order or spelling to preserve. Check
				// every rule in the order returned by the API instead.
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					require.Len(t, states, 1)
					attrs := states[0].Attributes
					require.Equal(t, "2", attrs["ip_access.#"])
					require.Equal(t, "198.51.100.0/24", attrs["ip_access.0.source"])
					require.Equal(t, "5432..6432", attrs["ip_access.0.port_range"])
					require.Equal(t, "new", attrs["ip_access.0.description"])
					require.Equal(t, "192.0.2.0/24", attrs["ip_access.1.source"])
					require.Equal(t, "5432", attrs["ip_access.1.port_range"])
					require.Equal(t, "", attrs["ip_access.1.description"])
					return nil
				},
			},
			{Config: mock.config("ip_access = []", "updated"), Check: mock.check(3, 0)},
			// Removing the attribute preserves the existing rules during a metadata update.
			{Config: mock.config("", "omitted"), Check: mock.check(3, 0)},
			{Config: mock.config(changed, "omitted"), Check: mock.check(4, 2)},
			{Config: mock.config("", "omitted"), Check: mock.check(4, 2)},
		},
	})
}

func TestAccPostgresIPAccessCreateDefaults(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	for _, tc := range []struct {
		name, access string
		count        int
		omitted      bool
	}{
		{"omitted", "", 2, true},
		{"empty", "ip_access = []", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := newPostgresIPMockAPI(t)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: postgresIPProviderFactories,
				CheckDestroy:             mock.checkDestroy,
				Steps: []resource.TestStep{{
					Config: mock.config(tc.access, "test"),
					Check: resource.ComposeAggregateTestCheckFunc(
						mock.check(0, tc.count),
						resource.TestCheckResourceAttr("data.clickhouse_postgres_service.test", "ip_access.#", strconv.Itoa(tc.count)),
						func(*terraform.State) error {
							mock.mu.Lock()
							defer mock.mu.Unlock()
							if (mock.create.IpAccessList == nil) != tc.omitted {
								return fmt.Errorf("create did not preserve omitted versus empty ip_access")
							}
							return nil
						},
					),
				}},
			})
		})
	}
}

func TestAccPostgresIPAccessExistingAllPorts(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	mock := newPostgresIPMockAPI(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: postgresIPProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps: []resource.TestStep{
			{Config: mock.config("", "test"), Check: mock.check(0, 2)},
			{
				PreConfig: func() {
					mock.mu.Lock()
					defer mock.mu.Unlock()
					mock.pg.IpAccessList = []api.PostgresIpAccess{mock.normalize(api.PostgresIpAccess{
						Source: "0.0.0.0/0", PortRange: "0..65535", Description: "All TCP",
					})}
				},
				Config: mock.config("", "updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					mock.check(0, 1),
					resource.TestCheckResourceAttr("clickhouse_postgres_service.test", "ip_access.0.port_range", "0..65535"),
					resource.TestCheckResourceAttr("data.clickhouse_postgres_service.test", "ip_access.0.port_range", "0..65535"),
				),
			},
			{
				ResourceName: "clickhouse_postgres_service.test", ImportState: true,
				ImportStateVerify: true, ImportStateVerifyIgnore: []string{"password"},
			},
			{Config: mock.config("ip_access = []", "updated"), Check: mock.check(1, 0)},
		},
	})
}

func TestAccPostgresIPAccessDescriptionWhitespace(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	mock := newPostgresIPMockAPI(t)
	var steps []resource.TestStep
	for i, tc := range []struct {
		configured, stored string
	}{
		{" \u00a0office\u00a0 ", "\u00a0office\u00a0"},
		{"\t\u2003edited\u2003\n", "\u2003edited\u2003"},
		{" \t\r\n\v\foffice \t\r\n\v\f", "office"},
	} {
		description, err := json.Marshal(tc.configured)
		require.NoError(t, err)
		// HCL requires a Unicode escape for form feed.
		hclDescription := strings.ReplaceAll(string(description), `\f`, `\u000c`)
		access := fmt.Sprintf(`ip_access = [{source="192.0.2.1/32", port_range="5432", description=%s}]`, hclDescription)
		steps = append(steps, resource.TestStep{
			Config: mock.config(access, "test"),
			Check: resource.ComposeAggregateTestCheckFunc(
				mock.check(i, 1),
				resource.TestCheckResourceAttr("clickhouse_postgres_service.test", "ip_access.0.description", tc.configured),
				resource.TestCheckResourceAttr("data.clickhouse_postgres_service.test", "ip_access.0.description", tc.stored),
			),
		})
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: postgresIPProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps:                    steps,
	})
}

func TestAccPostgresIPAccessValidation(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}
	mock := newPostgresIPMockAPI(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: postgresIPProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:   mock.config(`ip_access = [{source="::/0", port_range="5432", description=""}]`, "test"),
				PlanOnly: true, ExpectError: regexp.MustCompile("Invalid IPv4 source"),
			},
			{
				Config:   mock.config(`ip_access = [{source="192.0.2.1", port_range="6432..5432", description=""}]`, "test"),
				PlanOnly: true, ExpectError: regexp.MustCompile("Invalid port range"),
			},
			{
				Config: mock.config(`ip_access = [
					{source="192.0.2.1", port_range="05432", description="first"},
					{source="192.0.2.1/32", port_range="5432..5432", description="duplicate"}
				]`, "test"),
				PlanOnly: true, ExpectError: regexp.MustCompile("Duplicate IP access entry"),
			},
		},
	})
}
