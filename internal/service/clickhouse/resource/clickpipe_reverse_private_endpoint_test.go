package resource

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func buildCustomPrivateDNSMappingList(t *testing.T, names ...string) types.List {
	t.Helper()

	values := make([]attr.Value, len(names))
	for i, name := range names {
		values[i] = models.CustomPrivateDNSMappingModel{
			PrivateDNSName: types.StringValue(name),
			TargetID:       types.StringNull(),
		}.ObjectValue()
	}

	mappingList, diags := types.ListValue(models.CustomPrivateDNSMappingModel{}.ObjectType(), values)
	if diags.HasError() {
		t.Fatalf("ListValue: %v", diags)
	}

	return mappingList
}

func TestCustomPrivateDNSMappingsFromPlan(t *testing.T) {
	ctx := context.Background()

	got, diags := customPrivateDNSMappingsFromPlan(ctx, types.ListNull(models.CustomPrivateDNSMappingModel{}.ObjectType()))
	if diags.HasError() {
		t.Fatalf("null input diags: %v", diags)
	}
	if len(got) != 0 {
		t.Fatalf("null input = %#v; want empty list", got)
	}

	got, diags = customPrivateDNSMappingsFromPlan(ctx, buildCustomPrivateDNSMappingList(t, "one.example.com", "two.example.com"))
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	want := []api.CustomPrivateDNSMapping{
		{PrivateDNSName: "one.example.com"},
		{PrivateDNSName: "two.example.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mappings = %#v; want %#v", got, want)
	}
}

func TestApplyReversePrivateEndpointToModel_GCPPSCAndCustomDNSMappings(t *testing.T) {
	ctx := context.Background()
	gcpServiceAttachment := "projects/my-project/regions/us-central1/serviceAttachments/my-service"
	endpoint := &api.ReversePrivateEndpoint{
		CreateReversePrivateEndpoint: api.CreateReversePrivateEndpoint{
			Description:          "gcp psc endpoint",
			Type:                 api.ReversePrivateEndpointTypeGCPPSCServiceAttachment,
			GCPServiceAttachment: &gcpServiceAttachment,
		},
		ID:              "rpe-1",
		EndpointID:      "psc-endpoint",
		DNSNames:        []string{"internal.example.com"},
		PrivateDNSNames: []string{"private.example.com"},
		Status:          api.ReversePrivateEndpointStatusReady,
	}

	state := models.ClickPipeReversePrivateEndpointResourceModel{}
	diags := applyReversePrivateEndpointToModel(ctx, "svc-1", endpoint, &state)
	if diags.HasError() {
		t.Fatalf("applyReversePrivateEndpointToModel: %v", diags)
	}

	if state.ID.ValueString() != "rpe-1" || state.ServiceID.ValueString() != "svc-1" {
		t.Fatalf("ids = (%q, %q); want (rpe-1, svc-1)", state.ID.ValueString(), state.ServiceID.ValueString())
	}
	if state.Type.ValueString() != api.ReversePrivateEndpointTypeGCPPSCServiceAttachment {
		t.Fatalf("type = %q; want %s", state.Type.ValueString(), api.ReversePrivateEndpointTypeGCPPSCServiceAttachment)
	}
	if state.GCPServiceAttachment.ValueString() != gcpServiceAttachment {
		t.Fatalf("gcp_service_attachment = %q; want %s", state.GCPServiceAttachment.ValueString(), gcpServiceAttachment)
	}
}

func TestApplyReversePrivateEndpointCustomPrivateDNSToModel(t *testing.T) {
	endpoint := &api.ReversePrivateEndpoint{
		CreateReversePrivateEndpoint: api.CreateReversePrivateEndpoint{
			CustomPrivateDNSMappings: []api.CustomPrivateDNSMapping{
				{PrivateDNSName: "my-service.example.com"},
			},
		},
	}

	state := models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel{
		ServiceID:                types.StringValue("svc-1"),
		ReversePrivateEndpointID: types.StringValue("rpe-1"),
	}
	diags := applyReversePrivateEndpointCustomPrivateDNSToModel(endpoint, &state)
	if diags.HasError() {
		t.Fatalf("applyReversePrivateEndpointCustomPrivateDNSToModel: %v", diags)
	}

	if state.ID.ValueString() != "svc-1:rpe-1" {
		t.Fatalf("id = %q; want svc-1:rpe-1", state.ID.ValueString())
	}

	var mappings []models.CustomPrivateDNSMappingModel
	if d := state.Mapping.ElementsAs(context.Background(), &mappings, false); d.HasError() {
		t.Fatalf("Mapping.ElementsAs: %v", d)
	}
	if len(mappings) != 1 || mappings[0].PrivateDNSName.ValueString() != "my-service.example.com" {
		t.Fatalf("mapping = %#v; want my-service.example.com", mappings)
	}
}

func TestCustomPrivateDNSMappingsTargetIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	want := []api.CustomPrivateDNSMapping{
		{PrivateDNSName: "node-00-pri.example.com", TargetID: "rcfg-097648d8068504966"},
		{PrivateDNSName: "default.example.com"},
	}

	list, diags := customPrivateDNSMappingsToModel(want)
	if diags.HasError() {
		t.Fatalf("customPrivateDNSMappingsToModel: %v", diags)
	}

	var mappings []models.CustomPrivateDNSMappingModel
	if d := list.ElementsAs(ctx, &mappings, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	if mappings[0].TargetID.ValueString() != "rcfg-097648d8068504966" {
		t.Fatalf("target_id = %v; want rcfg-097648d8068504966", mappings[0].TargetID)
	}
	if !mappings[1].TargetID.IsNull() {
		t.Fatalf("target_id = %v; want null so unset config does not drift", mappings[1].TargetID)
	}

	got, diags := customPrivateDNSMappingsFromPlan(ctx, list)
	if diags.HasError() {
		t.Fatalf("customPrivateDNSMappingsFromPlan: %v", diags)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v; want %#v", got, want)
	}
}

func TestCustomPrivateDNSMappingsFromPlan_UnknownTargetID(t *testing.T) {
	unknown := models.CustomPrivateDNSMappingModel{
		PrivateDNSName: types.StringValue("node-00-pri.example.com"),
		TargetID:       types.StringUnknown(),
	}.ObjectValue()
	list := types.ListValueMust(models.CustomPrivateDNSMappingModel{}.ObjectType(), []attr.Value{unknown})

	_, diags := customPrivateDNSMappingsFromPlan(context.Background(), list)
	if !diags.HasError() {
		t.Fatal("unknown target_id must error instead of falling back to the default target")
	}
}

func TestApplyReversePrivateEndpointToModel_DNSTargetsAndPrivateDNSMappings(t *testing.T) {
	ctx := context.Background()
	endpoint := &api.ReversePrivateEndpoint{
		ID:     "rpe-1",
		Status: api.ReversePrivateEndpointStatusReady,
		DNSTargets: []api.DNSTarget{
			{ID: "rcfg-00", Kind: "RESOURCE_CONFIGURATION", InternalDNSName: "vpce-1.rcfg-00.example.on.aws"},
			{ID: "rcfg-01", Kind: "RESOURCE_CONFIGURATION", InternalDNSName: "vpce-1.rcfg-01.example.on.aws"},
		},
		PrivateDNSMappings: []api.PrivateDNSMapping{
			{PrivateDNSName: "node-00.example.com", InternalDNSName: "vpce-1.rcfg-00.example.on.aws"},
		},
	}

	state := models.ClickPipeReversePrivateEndpointResourceModel{}
	if d := applyReversePrivateEndpointToModel(ctx, "svc-1", endpoint, &state); d.HasError() {
		t.Fatalf("applyReversePrivateEndpointToModel: %v", d)
	}

	var targets []models.DNSTargetModel
	if d := state.DNSTargets.ElementsAs(ctx, &targets, false); d.HasError() {
		t.Fatalf("DNSTargets.ElementsAs: %v", d)
	}
	if len(targets) != 2 ||
		targets[1].ID.ValueString() != "rcfg-01" ||
		targets[1].Kind.ValueString() != "RESOURCE_CONFIGURATION" ||
		targets[1].InternalDNSName.ValueString() != "vpce-1.rcfg-01.example.on.aws" {
		t.Fatalf("dns_targets = %#v", targets)
	}

	var private []models.PrivateDNSMappingModel
	if d := state.PrivateDNSMappings.ElementsAs(ctx, &private, false); d.HasError() {
		t.Fatalf("PrivateDNSMappings.ElementsAs: %v", d)
	}
	if len(private) != 1 ||
		private[0].PrivateDNSName.ValueString() != "node-00.example.com" ||
		private[0].InternalDNSName.ValueString() != "vpce-1.rcfg-00.example.on.aws" {
		t.Fatalf("private_dns_mappings = %#v", private)
	}

	// Non-VPC_RESOURCE endpoints (and older API responses) report no targets.
	if d := applyReversePrivateEndpointToModel(ctx, "svc-1", &api.ReversePrivateEndpoint{ID: "rpe-2"}, &state); d.HasError() {
		t.Fatalf("applyReversePrivateEndpointToModel: %v", d)
	}
	if state.DNSTargets.IsNull() || len(state.DNSTargets.Elements()) != 0 {
		t.Fatalf("dns_targets = %v; want empty list", state.DNSTargets)
	}
	if state.PrivateDNSMappings.IsNull() || len(state.PrivateDNSMappings.Elements()) != 0 {
		t.Fatalf("private_dns_mappings = %v; want empty list", state.PrivateDNSMappings)
	}
}

func TestReversePrivateEndpointSchemas_DNSTargetContract(t *testing.T) {
	ctx := context.Background()

	var rpeSchema resource.SchemaResponse
	(&ClickPipeReversePrivateEndpointResource{}).Schema(ctx, resource.SchemaRequest{}, &rpeSchema)
	if _, exists := rpeSchema.Schema.Attributes["wait_for_ready"]; exists {
		t.Fatal("wait_for_ready must not be exposed")
	}
	for _, name := range []string{"dns_targets", "private_dns_mappings"} {
		attribute, ok := rpeSchema.Schema.Attributes[name].(schema.ListNestedAttribute)
		if !ok || !attribute.Computed || attribute.Optional || attribute.Required {
			t.Fatalf("%s must be a computed-only nested list", name)
		}
	}

	var dnsSchema resource.SchemaResponse
	(&ClickPipeReversePrivateEndpointCustomPrivateDNSResource{}).Schema(ctx, resource.SchemaRequest{}, &dnsSchema)
	mappingAttributes := dnsSchema.Schema.Attributes["mapping"].(schema.ListNestedAttribute).NestedObject.Attributes
	if _, exists := mappingAttributes["internal_dns_name"]; exists {
		t.Fatal("internal_dns_name must not be exposed")
	}
	targetID, ok := mappingAttributes["target_id"].(schema.StringAttribute)
	if !ok || !targetID.Optional || targetID.Computed {
		t.Fatal("target_id must be Optional only")
	}
}

func TestUpdateCustomPrivateDNSMappings_SendsTargetIDWithoutReadyWait(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusCode int
		wantError  bool
	}{
		{name: "accepted", statusCode: http.StatusOK},
		{name: "API validation error surfaces", statusCode: http.StatusBadRequest, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var methods []string
			var payload api.UpdateReversePrivateEndpoint
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				methods = append(methods, req.Method)
				if req.Method != http.MethodPatch {
					t.Errorf("unexpected %s request: no readiness wait expected", req.Method)
				}
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Errorf("decode request body: %v", err)
				}
				if tc.statusCode != http.StatusOK {
					w.WriteHeader(tc.statusCode)
					_, _ = io.WriteString(w, `{"status":400,"error":"targetId is supported only for VPC_RESOURCE"}`)
					return
				}
				endpoint := api.ReversePrivateEndpoint{ID: "rpe-1", Status: api.ReversePrivateEndpointStatusProvisioning}
				endpoint.CustomPrivateDNSMappings = *payload.CustomPrivateDNSMappings
				_ = json.NewEncoder(w).Encode(api.ResponseWithResult[api.ReversePrivateEndpoint]{Result: endpoint})
			}))
			defer server.Close()

			client, err := api.NewClient(api.ClientConfig{ApiURL: server.URL, OrganizationID: "org-1", TokenKey: "key", TokenSecret: "secret"})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			list, diags := customPrivateDNSMappingsToModel([]api.CustomPrivateDNSMapping{
				{PrivateDNSName: "node-00-pri.example.com", TargetID: "rcfg-097648d8068504966"},
			})
			if diags.HasError() {
				t.Fatalf("customPrivateDNSMappingsToModel: %v", diags)
			}
			data := models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel{
				ServiceID:                types.StringValue("svc-1"),
				ReversePrivateEndpointID: types.StringValue("rpe-1"),
				Mapping:                  list,
			}

			r := &ClickPipeReversePrivateEndpointCustomPrivateDNSResource{client: client}
			endpoint, ok := r.updateCustomPrivateDNSMappings(context.Background(), &data, &diags)

			if !reflect.DeepEqual(methods, []string{http.MethodPatch}) {
				t.Fatalf("requests = %v; want a single PATCH", methods)
			}
			if payload.CustomPrivateDNSMappings == nil || (*payload.CustomPrivateDNSMappings)[0].TargetID != "rcfg-097648d8068504966" {
				t.Fatalf("PATCH payload = %#v; want targetId", payload)
			}
			if tc.wantError {
				if ok || !diags.HasError() || !strings.Contains(diags[0].Detail(), "targetId is supported only for VPC_RESOURCE") {
					t.Fatalf("ok = %v, diags = %v; want API error surfaced", ok, diags)
				}
				return
			}
			if !ok || diags.HasError() || endpoint.CustomPrivateDNSMappings[0].TargetID != "rcfg-097648d8068504966" {
				t.Fatalf("ok = %v, diags = %v, endpoint = %#v", ok, diags, endpoint)
			}
		})
	}
}
