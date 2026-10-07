package clickhouse_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

// The BYOC acceptance tests run the real Terraform CLI against a local mock
// of the ClickHouse Cloud API: provisioning a real BYOC data plane takes up
// to an hour per create in a dedicated cloud account, which no shared test
// fixture can provide. The mock keeps the replacement-payload and
// import-adoption semantics of the plan modifiers testable end to end.
// testAccClickHousePreCheck is deliberately not used: these tests are
// hermetic and need no ClickHouse Cloud credentials.

const byocAccOrgID = "aaaaaaaa-0000-0000-0000-000000000000"

type byocMockInfra struct {
	details api.ByocInfrastructureDetails
	tags    map[string]string
}

type byocMockAPI struct {
	t *testing.T

	mu             sync.Mutex
	nextID         int
	infras         map[string]*byocMockInfra
	createRequests []api.ByocInfrastructureCreateRequest
	validateCount  int

	server *httptest.Server
}

func newByocMockAPI(t *testing.T) *byocMockAPI {
	t.Helper()
	m := &byocMockAPI{
		t:      t,
		infras: map[string]*byocMockInfra{},
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

func (m *byocMockAPI) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := "/organizations/" + byocAccOrgID + "/byocInfrastructure"
	rest, ok := strings.CutPrefix(r.URL.Path, prefix)
	if !ok {
		m.t.Errorf("unexpected request path %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected path", http.StatusNotFound)
		return
	}
	rest = strings.TrimPrefix(rest, "/")

	switch {
	case rest == "" && r.Method == http.MethodPost:
		m.handleCreate(w, r)
	case rest == "validate" && r.Method == http.MethodPost:
		m.validateCount++
		writeByocResult(w, api.ByocInfrastructureValidation{
			CloudProvider: "aws",
			Supported:     true,
			AllPassed:     true,
			AnyPassed:     true,
		})
	default:
		id, sub, _ := strings.Cut(rest, "/")
		infra, found := m.infras[id]
		if !found {
			http.Error(w, fmt.Sprintf(`{"error":"BYOC infrastructure %s not found"}`, id), http.StatusNotFound)
			return
		}
		switch {
		case sub == "" && r.Method == http.MethodGet:
			writeByocResult(w, infra.details)
		case sub == "tags" && r.Method == http.MethodGet:
			writeByocResult(w, struct {
				Tags map[string]string `json:"tags"`
			}{Tags: infra.tags})
		case sub == "" && r.Method == http.MethodPatch:
			m.handleUpdate(w, r, infra)
		case sub == "" && r.Method == http.MethodDelete:
			infra.details.State = api.ByocStateTerminated
			writeByocResult(w, byocSummary(infra))
		default:
			m.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}
}

func (m *byocMockAPI) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req api.ByocInfrastructureCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.t.Errorf("invalid create BYOC infrastructure payload: %s", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	m.createRequests = append(m.createRequests, req)

	m.nextID++
	id := fmt.Sprintf("byoc-mock-%d", m.nextID)
	displayName := "byoc-generated-" + id
	if req.DisplayName != nil {
		displayName = *req.DisplayName
	}

	enabled := false
	isByoVpc := req.VpcId != nil
	details := api.ByocInfrastructureDetails{
		Id:                           id,
		State:                        api.ByocStateReady,
		AccountId:                    req.AccountId,
		RegionId:                     req.RegionId,
		CloudProvider:                "aws",
		DisplayName:                  displayName,
		EnablePrivateLink:            &enabled,
		EnablePrivateLoadBalancer:    &enabled,
		EnablePublicLoadBalancer:     &enabled,
		IsByoVpc:                     &isByoVpc,
		ByoVpcId:                     req.VpcId,
		ByoVpcPrivateSubnetIds:       req.PrivateSubnetIds,
		ByoVpcPodCidrRangeNames:      req.GcpPodCidrRangeNames,
		ByoVpcSharedVpcHostProjectId: req.GcpSharedVpcHostProjectId,
	}
	if !isByoVpc {
		cidr := "10.0.0.0/16"
		if req.VpcCidrRange != nil {
			cidr = *req.VpcCidrRange
		}
		details.VpcCidrRange = &cidr
	}

	tags := map[string]string{}
	for k, v := range req.Tags {
		tags[k] = v
	}
	infra := &byocMockInfra{details: details, tags: tags}
	m.infras[id] = infra
	writeByocResult(w, byocSummary(infra))
}

func (m *byocMockAPI) handleUpdate(w http.ResponseWriter, r *http.Request, infra *byocMockInfra) {
	var req api.ByocInfrastructureUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		m.t.Errorf("invalid update BYOC infrastructure payload: %s", err)
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	if req.DisplayName != nil {
		infra.details.DisplayName = *req.DisplayName
	}
	if req.EnablePrivateLink != nil {
		infra.details.EnablePrivateLink = req.EnablePrivateLink
	}
	if req.EnablePrivateLoadBalancer != nil {
		infra.details.EnablePrivateLoadBalancer = req.EnablePrivateLoadBalancer
	}
	if req.EnablePublicLoadBalancer != nil {
		infra.details.EnablePublicLoadBalancer = req.EnablePublicLoadBalancer
	}
	if req.GcpPscSubnetId != nil {
		infra.details.GcpPscSubnetId = req.GcpPscSubnetId
	}
	if req.Tags != nil {
		tags := map[string]string{}
		for k, v := range *req.Tags {
			tags[k] = v
		}
		infra.tags = tags
	}
	writeByocResult(w, byocSummary(infra))
}

func byocSummary(infra *byocMockInfra) api.ByocInfrastructure {
	return api.ByocInfrastructure{
		Id:            infra.details.Id,
		State:         infra.details.State,
		AccountId:     infra.details.AccountId,
		RegionId:      infra.details.RegionId,
		CloudProvider: infra.details.CloudProvider,
		DisplayName:   infra.details.DisplayName,
	}
}

func writeByocResult(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.ResponseWithResult[any]{Result: result})
}

// seed registers an infrastructure that exists before any Terraform run, as
// if it had been created from the console, so tests can import it.
func (m *byocMockAPI) seed(details api.ByocInfrastructureDetails) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.infras[details.Id] = &byocMockInfra{details: details, tags: map[string]string{}}
}

func (m *byocMockAPI) createCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.createRequests)
}

func (m *byocMockAPI) lastCreateRequest() api.ByocInfrastructureCreateRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createRequests[len(m.createRequests)-1]
}

func (m *byocMockAPI) checkCreateCount(want int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := m.createCount(); got != want {
			return fmt.Errorf("expected %d create calls to the BYOC API, got %d", want, got)
		}
		return nil
	}
}

func (m *byocMockAPI) checkDestroy(*terraform.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, infra := range m.infras {
		if infra.details.State != api.ByocStateTerminated {
			return fmt.Errorf("BYOC infrastructure %s was not terminated on destroy (state %q)", id, infra.details.State)
		}
	}
	return nil
}

func byocAccProviderConfig(apiURL string) string {
	return fmt.Sprintf(`
provider "clickhouse" {
  api_url         = %q
  organization_id = %q
  token_key       = "mock-key"
  token_secret    = "mock-secret"
}
`, apiURL, byocAccOrgID)
}

func byocResourceID(state *terraform.State) (string, error) {
	resourceState, ok := state.RootModule().Resources["clickhouse_byoc_infrastructure.test"]
	if !ok || resourceState.Primary == nil || resourceState.Primary.ID == "" {
		return "", fmt.Errorf("clickhouse_byoc_infrastructure.test has no state ID")
	}
	return resourceState.Primary.ID, nil
}

// byocIDTracker records the resource ID across steps so tests can assert
// in-place updates (same ID) and replacements (different ID).
type byocIDTracker struct {
	id string
}

func (tr *byocIDTracker) checkStable(state *terraform.State) error {
	id, err := byocResourceID(state)
	if err != nil {
		return err
	}
	if tr.id == "" {
		tr.id = id
		return nil
	}
	if id != tr.id {
		return fmt.Errorf("BYOC infrastructure was replaced: ID changed from %q to %q", tr.id, id)
	}
	return nil
}

func (tr *byocIDTracker) checkReplaced(state *terraform.State) error {
	id, err := byocResourceID(state)
	if err != nil {
		return err
	}
	if tr.id == "" {
		return fmt.Errorf("no prior BYOC infrastructure ID recorded to compare against")
	}
	if id == tr.id {
		return fmt.Errorf("BYOC infrastructure %q was expected to be replaced but kept its ID", id)
	}
	tr.id = id
	return nil
}

func TestAccByocInfrastructureResourceManagedVpc(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	mock := newByocMockAPI(t)
	tracker := &byocIDTracker{}

	config := func(displayName, tagValue string) string {
		return byocAccProviderConfig(mock.server.URL) + fmt.Sprintf(`
resource "clickhouse_byoc_infrastructure" "test" {
  region_id      = "us-east-1"
  account_id     = "999999999999"
  display_name   = %q
  vpc_cidr_range = "172.19.0.0/16"

  tags = {
    env = %q
  }
}
`, displayName, tagValue)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: config("tf-acc-byoc-managed", "acc"),
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkStable,
					mock.checkCreateCount(1),
					resource.TestCheckResourceAttrSet("clickhouse_byoc_infrastructure.test", "id"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "state", "infra-ready"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "cloud_provider", "aws"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "display_name", "tf-acc-byoc-managed"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "vpc_cidr_range", "172.19.0.0/16"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "is_byo_vpc", "false"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "tags.env", "acc"),
					func(*terraform.State) error {
						mock.mu.Lock()
						defer mock.mu.Unlock()
						if mock.validateCount == 0 {
							return fmt.Errorf("expected the preflight validation endpoint to be called before create")
						}
						return nil
					},
				),
			},
			{
				Config:   config("tf-acc-byoc-managed", "acc"),
				PlanOnly: true,
			},
			{
				Config: config("tf-acc-byoc-managed-renamed", "acc-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkStable,
					mock.checkCreateCount(1),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "display_name", "tf-acc-byoc-managed-renamed"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "tags.env", "acc-updated"),
				),
			},
			{
				Config:   config("tf-acc-byoc-managed-renamed", "acc-updated"),
				PlanOnly: true,
			},
			{
				ResourceName:      "clickhouse_byoc_infrastructure.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccByocInfrastructureResourceTopologyReplacement covers the custom plan
// modifiers across topology switches: a managed -> BYO-VPC replacement must
// not leak the recorded managed CIDR into the create payload, a BYO-VPC ->
// managed replacement must not leak the stale BYO-VPC wiring, and a
// display-name-only update of a BYO-VPC infrastructure must stay in place.
func TestAccByocInfrastructureResourceTopologyReplacement(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	mock := newByocMockAPI(t)
	tracker := &byocIDTracker{}

	managedConfig := func(cidr string) string {
		return byocAccProviderConfig(mock.server.URL) + fmt.Sprintf(`
resource "clickhouse_byoc_infrastructure" "test" {
  region_id      = "us-east-1"
  account_id     = "999999999999"
  display_name   = "tf-acc-byoc-topology"
  vpc_cidr_range = %q

  skip_preflight_validation = true
}
`, cidr)
	}
	byoConfig := func(displayName string) string {
		return byocAccProviderConfig(mock.server.URL) + fmt.Sprintf(`
resource "clickhouse_byoc_infrastructure" "test" {
  region_id    = "us-east-1"
  account_id   = "999999999999"
  display_name = %q

  vpc_id             = "vpc-0123456789abcdef0"
  private_subnet_ids = ["subnet-aaa111", "subnet-bbb222"]

  skip_preflight_validation = true
}
`, displayName)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps: []resource.TestStep{
			{
				Config: managedConfig("172.20.0.0/16"),
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkStable,
					mock.checkCreateCount(1),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "is_byo_vpc", "false"),
				),
			},
			{
				Config: byoConfig("tf-acc-byoc-topology"),
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkReplaced,
					mock.checkCreateCount(2),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "is_byo_vpc", "true"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "vpc_id", "vpc-0123456789abcdef0"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "private_subnet_ids.#", "2"),
					resource.TestCheckNoResourceAttr("clickhouse_byoc_infrastructure.test", "vpc_cidr_range"),
					func(*terraform.State) error {
						req := mock.lastCreateRequest()
						if req.VpcId == nil || *req.VpcId != "vpc-0123456789abcdef0" {
							return fmt.Errorf("BYO-VPC create payload is missing the configured vpcId: %+v", req)
						}
						if req.VpcCidrRange != nil {
							return fmt.Errorf("managed-VPC CIDR %q leaked into the BYO-VPC create payload", *req.VpcCidrRange)
						}
						return nil
					},
				),
			},
			{
				Config:   byoConfig("tf-acc-byoc-topology"),
				PlanOnly: true,
			},
			{
				Config: byoConfig("tf-acc-byoc-topology-renamed"),
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkStable,
					mock.checkCreateCount(2),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "display_name", "tf-acc-byoc-topology-renamed"),
				),
			},
			{
				Config: managedConfig("172.21.0.0/16"),
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkReplaced,
					mock.checkCreateCount(3),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "is_byo_vpc", "false"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "vpc_cidr_range", "172.21.0.0/16"),
					func(*terraform.State) error {
						req := mock.lastCreateRequest()
						if req.VpcCidrRange == nil || *req.VpcCidrRange != "172.21.0.0/16" {
							return fmt.Errorf("managed-VPC create payload is missing the configured vpcCidrRange: %+v", req)
						}
						if req.VpcId != nil {
							return fmt.Errorf("stale BYO-VPC ID %q leaked into the managed-VPC create payload", *req.VpcId)
						}
						if len(req.PrivateSubnetIds) != 0 {
							return fmt.Errorf("stale BYO-VPC subnets %v leaked into the managed-VPC create payload", req.PrivateSubnetIds)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccByocInfrastructureResourceImportAdopt covers adopting write-only
// creation parameters over an imported null state: a console-created
// infrastructure is imported, then configuring the write-only parameters must
// update in place, while changing an adopted value afterwards must replace
// the infrastructure.
func TestAccByocInfrastructureResourceImportAdopt(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests skipped unless env 'TF_ACC' is set")
	}

	mock := newByocMockAPI(t)
	tracker := &byocIDTracker{}

	const importedID = "byoc-imported-1"
	enabled := false
	notByoVpc := false
	cidr := "172.22.0.0/16"
	mock.seed(api.ByocInfrastructureDetails{
		Id:                        importedID,
		State:                     api.ByocStateReady,
		AccountId:                 "999999999999",
		RegionId:                  "us-east-1",
		CloudProvider:             "aws",
		DisplayName:               "tf-acc-byoc-adopt",
		EnablePrivateLink:         &enabled,
		EnablePrivateLoadBalancer: &enabled,
		EnablePublicLoadBalancer:  &enabled,
		IsByoVpc:                  &notByoVpc,
		VpcCidrRange:              &cidr,
	})

	config := func(writeOnlyAttributes string) string {
		return byocAccProviderConfig(mock.server.URL) + fmt.Sprintf(`
resource "clickhouse_byoc_infrastructure" "test" {
  region_id      = "us-east-1"
  account_id     = "999999999999"
  display_name   = "tf-acc-byoc-adopt"
  vpc_cidr_range = "172.22.0.0/16"
%s
  skip_preflight_validation = true
}
`, writeOnlyAttributes)
	}
	adoptedAttributes := `
  external_id                = "ext-adopted"
  availability_zone_suffixes = ["a", "b"]
`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             mock.checkDestroy,
		Steps: []resource.TestStep{
			{
				Config:             config(adoptedAttributes),
				ResourceName:       "clickhouse_byoc_infrastructure.test",
				ImportState:        true,
				ImportStateId:      importedID,
				ImportStatePersist: true,
			},
			{
				Config: config(adoptedAttributes),
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkStable,
					mock.checkCreateCount(0),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "id", importedID),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "external_id", "ext-adopted"),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "availability_zone_suffixes.#", "2"),
				),
			},
			{
				Config:   config(adoptedAttributes),
				PlanOnly: true,
			},
			{
				Config: config(strings.Replace(adoptedAttributes, "ext-adopted", "ext-rotated", 1)),
				Check: resource.ComposeAggregateTestCheckFunc(
					tracker.checkReplaced,
					mock.checkCreateCount(1),
					resource.TestCheckResourceAttr("clickhouse_byoc_infrastructure.test", "external_id", "ext-rotated"),
					func(*terraform.State) error {
						req := mock.lastCreateRequest()
						if req.ExternalId == nil || *req.ExternalId != "ext-rotated" {
							return fmt.Errorf("replacement create payload is missing the rotated externalId: %+v", req)
						}
						return nil
					},
				),
			},
		},
	})
}
