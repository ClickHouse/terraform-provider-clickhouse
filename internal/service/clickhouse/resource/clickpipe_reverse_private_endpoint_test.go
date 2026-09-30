package resource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func buildCustomPrivateDNSMappingList(t *testing.T, names ...string) types.List {
	t.Helper()

	values := make([]attr.Value, len(names))
	for i, name := range names {
		values[i] = models.CustomPrivateDNSMappingModel{
			PrivateDNSName:  types.StringValue(name),
			InternalDNSName: types.StringNull(),
		}.ObjectValue()
	}

	mappingList, diags := types.ListValue(models.CustomPrivateDNSMappingModel{}.ObjectType(), values)
	if diags.HasError() {
		t.Fatalf("ListValue: %v", diags)
	}

	return mappingList
}

func TestCustomPrivateDNSMappingsInternalTargetsRoundTrip(t *testing.T) {
	want := []api.CustomPrivateDNSMapping{
		{PrivateDNSName: "node-00-pri.example.com", InternalDNSName: "CHILD.internal.example.com."},
		{PrivateDNSName: "default.example.com"},
	}
	list, diags := customPrivateDNSMappingsToModel(want)
	if diags.HasError() {
		t.Fatal(diags)
	}
	var mappings []models.CustomPrivateDNSMappingModel
	if d := list.ElementsAs(context.Background(), &mappings, false); d.HasError() {
		t.Fatal(d)
	}
	if !mappings[1].InternalDNSName.IsNull() {
		t.Fatal("omitted target must remain null, not an empty string")
	}
	got, diags := customPrivateDNSMappingsFromPlan(context.Background(), list)
	if diags.HasError() || !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, %v; want %#v", got, diags, want)
	}
	unknown := models.CustomPrivateDNSMappingModel{
		PrivateDNSName:  types.StringValue("node.example.com"),
		InternalDNSName: types.StringUnknown(),
	}.ObjectValue()
	_, diags = customPrivateDNSMappingsFromPlan(context.Background(), types.ListValueMust(models.CustomPrivateDNSMappingModel{}.ObjectType(), []attr.Value{unknown}))
	if !diags.HasError() {
		t.Fatal("unknown internal target must not silently fall back to default")
	}
}

func TestValidateCustomPrivateDNSInternalNames(t *testing.T) {
	endpoint := &api.ReversePrivateEndpoint{
		ID:       "rpe-1",
		DNSNames: []string{"child-00.internal.example.com."},
		PrivateDNSMappings: []api.PrivateDNSMapping{
			{PrivateDNSName: "node-01.example.com", InternalDNSName: "CHILD-01.internal.example.com"},
		},
	}
	for _, name := range []string{"", "CHILD-00.INTERNAL.EXAMPLE.COM", "child-00.internal.example.com.", "child-01.internal.example.com."} {
		t.Run(name, func(t *testing.T) {
			diags := validateCustomPrivateDNSInternalNames([]api.CustomPrivateDNSMapping{{PrivateDNSName: "node-pri.example.com", InternalDNSName: name}}, endpoint)
			if diags.HasError() {
				t.Fatal(diags)
			}
		})
	}
	diags := validateCustomPrivateDNSInternalNames([]api.CustomPrivateDNSMapping{{PrivateDNSName: "node-pri.example.com", InternalDNSName: "missing.example.com"}}, endpoint)
	if !diags.HasError() {
		t.Fatal("expected invalid target diagnostic")
	}
	for _, text := range []string{"mapping[0].internal_dns_name", "missing.example.com", "node-pri.example.com", "child-00.internal.example.com.", "CHILD-01.internal.example.com"} {
		if !strings.Contains(diags[0].Detail(), text) {
			t.Errorf("diagnostic %q does not include %q", diags[0].Detail(), text)
		}
	}
	diags = validateCustomPrivateDNSInternalNames([]api.CustomPrivateDNSMapping{{InternalDNSName: "missing.example.com"}}, &api.ReversePrivateEndpoint{ID: "rpe-empty"})
	if !diags.HasError() || !strings.Contains(diags[0].Detail(), "Available internal DNS names: []") {
		t.Fatalf("empty names diagnostic: %v", diags)
	}
}

func TestReversePrivateEndpointReady(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     string
		names      []string
		requireDNS bool
		wantReady  bool
		wantError  bool
	}{
		{name: "ready with names", status: api.ReversePrivateEndpointStatusReady, names: []string{"child.internal"}, requireDNS: true, wantReady: true},
		{name: "ready without names", status: api.ReversePrivateEndpointStatusReady, requireDNS: true},
		{name: "DNS mapping wait only needs Ready", status: api.ReversePrivateEndpointStatusReady, wantReady: true},
		{name: "provisioning", status: api.ReversePrivateEndpointStatusProvisioning, requireDNS: true},
		{name: "pending acceptance", status: api.ReversePrivateEndpointStatusPendingAcceptance, requireDNS: true},
		{name: "failed", status: api.ReversePrivateEndpointStatusFailed, wantError: true},
		{name: "rejected", status: api.ReversePrivateEndpointStatusRejected, requireDNS: true, wantError: true},
		{name: "expired", status: api.ReversePrivateEndpointStatusExpired, requireDNS: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ready, err := reversePrivateEndpointReady(&api.ReversePrivateEndpoint{ID: "rpe-1", Status: tc.status, DNSNames: tc.names}, tc.requireDNS)
			if ready != tc.wantReady || (err != nil) != tc.wantError {
				t.Fatalf("ready = %v, err = %v; want %v, error %v", ready, err, tc.wantReady, tc.wantError)
			}
			if err != nil && (!strings.Contains(err.Error(), "rpe-1") || !strings.Contains(err.Error(), tc.status)) {
				t.Fatalf("terminal error lacks endpoint or status: %v", err)
			}
		})
	}
}

func TestApplyReversePrivateEndpointPrivateDNSMappings(t *testing.T) {
	ctx := context.Background()
	endpoint := &api.ReversePrivateEndpoint{
		PrivateDNSMappings: []api.PrivateDNSMapping{{PrivateDNSName: "node.example.com", InternalDNSName: "child.internal.example.com"}},
	}
	state := models.ClickPipeReversePrivateEndpointResourceModel{WaitForReady: types.BoolValue(true)}
	if d := applyReversePrivateEndpointToModel(ctx, "svc-1", endpoint, &state); d.HasError() {
		t.Fatal(d)
	}
	var mappings []models.CustomPrivateDNSMappingModel
	if d := state.PrivateDNSMappings.ElementsAs(ctx, &mappings, false); d.HasError() {
		t.Fatal(d)
	}
	if len(mappings) != 1 || mappings[0].PrivateDNSName.ValueString() != "node.example.com" || mappings[0].InternalDNSName.ValueString() != "child.internal.example.com" {
		t.Fatalf("private_dns_mappings = %#v", mappings)
	}
	if !state.WaitForReady.ValueBool() {
		t.Fatal("read must preserve wait_for_ready")
	}
	if d := applyReversePrivateEndpointToModel(ctx, "svc-1", &api.ReversePrivateEndpoint{}, &state); d.HasError() {
		t.Fatal(d)
	}
	if state.PrivateDNSMappings.IsNull() || len(state.PrivateDNSMappings.Elements()) != 0 {
		t.Fatalf("missing private_dns_mappings should be an empty list: %v", state.PrivateDNSMappings)
	}
}

func TestReversePrivateEndpointUpdateWaitForReady(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeReversePrivateEndpointResource{}
	var schemaResponse resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
	sch := schemaResponse.Schema
	waitAttribute := sch.Attributes["wait_for_ready"].(schema.BoolAttribute)
	if !waitAttribute.Optional || !waitAttribute.Computed || waitAttribute.Default == nil || len(waitAttribute.PlanModifiers) != 0 {
		t.Fatal("wait_for_ready must be Optional + Computed with a default and no replacement modifier")
	}
	var dnsSchemaResponse resource.SchemaResponse
	(&ClickPipeReversePrivateEndpointCustomPrivateDNSResource{}).Schema(ctx, resource.SchemaRequest{}, &dnsSchemaResponse)
	internalDNSAttribute := dnsSchemaResponse.Schema.Attributes["mapping"].(schema.ListNestedAttribute).NestedObject.Attributes["internal_dns_name"].(schema.StringAttribute)
	if !internalDNSAttribute.Optional || internalDNSAttribute.Computed {
		t.Fatal("internal_dns_name must be Optional only")
	}
	var stateModel models.ClickPipeReversePrivateEndpointResourceModel
	if d := applyReversePrivateEndpointToModel(ctx, "svc-1", &api.ReversePrivateEndpoint{
		ID: "rpe-1", EndpointID: "vpce-1", Status: api.ReversePrivateEndpointStatusReady, DNSNames: []string{"child.internal"},
		CreateReversePrivateEndpoint: api.CreateReversePrivateEndpoint{Description: "mongo", Type: api.ReversePrivateEndpointTypeVPCResource},
	}, &stateModel); d.HasError() {
		t.Fatal(d)
	}
	state := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if d := state.Set(ctx, &stateModel); d.HasError() {
		t.Fatal(d)
	}
	for _, tc := range []struct {
		name              string
		changeDescription bool
	}{{name: "wait only"}, {name: "immutable change", changeDescription: true}} {
		t.Run(tc.name, func(t *testing.T) {
			planModel := stateModel
			planModel.WaitForReady = types.BoolValue(true)
			// Computed outputs can be unknown in an update plan.
			planModel.DNSNames = types.ListUnknown(types.StringType)
			planModel.Status = types.StringUnknown()
			if tc.changeDescription {
				planModel.Description = types.StringValue("changed")
			}
			plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
			if d := plan.Set(ctx, &planModel); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.UpdateResponse{State: state}
			r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
			if tc.changeDescription {
				if !resp.Diagnostics.HasError() {
					t.Fatal("immutable configuration must not be updated")
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var got models.ClickPipeReversePrivateEndpointResourceModel
			if d := resp.State.Get(ctx, &got); d.HasError() {
				t.Fatal(d)
			}
			want := stateModel
			want.WaitForReady = types.BoolValue(true)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("update must only change wait_for_ready: got %#v; want %#v", got, want)
			}
		})
	}
	importResp := resource.ImportStateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "svc-1:rpe-1"}, &importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatal(importResp.Diagnostics)
	}
	var wait types.Bool
	if d := importResp.State.GetAttribute(ctx, path.Root("wait_for_ready"), &wait); d.HasError() {
		t.Fatal(d)
	}
	if wait.IsNull() || wait.IsUnknown() || wait.ValueBool() {
		t.Fatalf("import wait_for_ready = %v; want false", wait)
	}
}

func TestReversePrivateEndpointCreateWaitForReady(t *testing.T) {
	for _, wait := range []bool{false, true} {
		name := "default allows pending acceptance"
		if wait {
			name = "wait captures populated DNS names"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			configurationID, shareARN := "rcfg-group", "arn:aws:ram:us-east-1:123456789012:resource-share/share-1"
			gets := 0
			endpoint := api.ReversePrivateEndpoint{
				ID: "rpe-1", EndpointID: "vpce-1", Status: api.ReversePrivateEndpointStatusProvisioning,
				CreateReversePrivateEndpoint: api.CreateReversePrivateEndpoint{
					Description: "mongo", Type: api.ReversePrivateEndpointTypeVPCResource,
					VPCResourceConfigurationID: &configurationID, VPCResourceShareArn: &shareARN,
				},
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				response := endpoint
				switch req.Method {
				case http.MethodPost:
				case http.MethodGet:
					gets++
					response.Status = api.ReversePrivateEndpointStatusPendingAcceptance
					if wait {
						response.Status = api.ReversePrivateEndpointStatusReady
						// Ready alone is not enough: the first GET has no DNS names.
						if gets > 1 {
							response.DNSNames = []string{"child-01.internal", "child-00.internal"}
						}
					}
				default:
					t.Errorf("unexpected request method: %s", req.Method)
				}
				_ = json.NewEncoder(w).Encode(api.ResponseWithResult[api.ReversePrivateEndpoint]{Result: response})
			}))
			defer server.Close()
			client, err := api.NewClient(api.ClientConfig{ApiURL: server.URL, OrganizationID: "org-1", TokenKey: "key", TokenSecret: "secret"})
			if err != nil {
				t.Fatal(err)
			}
			r := &ClickPipeReversePrivateEndpointResource{client: client}
			var schemaResponse resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
			sch := schemaResponse.Schema
			var planModel models.ClickPipeReversePrivateEndpointResourceModel
			if d := applyReversePrivateEndpointToModel(ctx, "svc-1", &endpoint, &planModel); d.HasError() {
				t.Fatal(d)
			}
			planModel.WaitForReady = types.BoolValue(wait)
			plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
			if d := plan.Set(ctx, &planModel); d.HasError() {
				t.Fatal(d)
			}
			resp := resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var got models.ClickPipeReversePrivateEndpointResourceModel
			if d := resp.State.Get(ctx, &got); d.HasError() {
				t.Fatal(d)
			}
			if wait {
				if gets != 2 || got.Status.ValueString() != api.ReversePrivateEndpointStatusReady || len(got.DNSNames.Elements()) != 2 {
					t.Fatalf("creation returned before DNS names populated: GETs=%d, state=%#v", gets, got)
				}
			} else if gets != 1 || got.Status.ValueString() != api.ReversePrivateEndpointStatusPendingAcceptance {
				t.Fatalf("default must return PendingAcceptance: GETs=%d, status=%v", gets, got.Status)
			}
		})
	}
}

func TestUpdateCustomPrivateDNSMappingsReadinessAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		target    string
		status    string
		wantGet   int
		wantPatch int
		wantError bool
	}{
		{name: "default skips wait", status: api.ReversePrivateEndpointStatusProvisioning, wantPatch: 1},
		{name: "valid target", target: "child.internal.example.com", status: api.ReversePrivateEndpointStatusReady, wantGet: 1, wantPatch: 1},
		{name: "invalid target", target: "missing.internal.example.com", status: api.ReversePrivateEndpointStatusReady, wantGet: 1, wantError: true},
		{name: "failed", target: "child.internal.example.com", status: api.ReversePrivateEndpointStatusFailed, wantGet: 1, wantError: true},
		{name: "rejected", target: "child.internal.example.com", status: api.ReversePrivateEndpointStatusRejected, wantGet: 1, wantError: true},
		{name: "expired", target: "child.internal.example.com", status: api.ReversePrivateEndpointStatusExpired, wantGet: 1, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gets, patches := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				endpoint := api.ReversePrivateEndpoint{ID: "rpe-1", Status: tc.status, DNSNames: []string{"child.internal.example.com"}}
				switch req.Method {
				case http.MethodGet:
					gets++
				case http.MethodPatch:
					patches++
					var payload api.UpdateReversePrivateEndpoint
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					if payload.CustomPrivateDNSMappings == nil || (*payload.CustomPrivateDNSMappings)[0].InternalDNSName != tc.target {
						t.Errorf("unexpected PATCH: %#v", payload)
					}
					endpoint.CustomPrivateDNSMappings = *payload.CustomPrivateDNSMappings
				default:
					t.Errorf("unexpected method %s", req.Method)
				}
				_ = json.NewEncoder(w).Encode(api.ResponseWithResult[api.ReversePrivateEndpoint]{Result: endpoint})
			}))
			defer server.Close()
			client, err := api.NewClient(api.ClientConfig{ApiURL: server.URL, OrganizationID: "org-1", TokenKey: "key", TokenSecret: "secret"})
			if err != nil {
				t.Fatal(err)
			}
			list, diags := customPrivateDNSMappingsToModel([]api.CustomPrivateDNSMapping{{PrivateDNSName: "node-pri.example.com", InternalDNSName: tc.target}})
			if diags.HasError() {
				t.Fatal(diags)
			}
			data := models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel{ServiceID: types.StringValue("svc-1"), ReversePrivateEndpointID: types.StringValue("rpe-1"), Mapping: list}
			r := &ClickPipeReversePrivateEndpointCustomPrivateDNSResource{client: client}
			_, ok := r.updateCustomPrivateDNSMappings(context.Background(), &data, &diags)
			if ok == tc.wantError || diags.HasError() != tc.wantError || gets != tc.wantGet || patches != tc.wantPatch {
				t.Fatalf("ok=%v, diags=%v, GETs=%d, PATCHes=%d; want error=%v, GETs=%d, PATCHes=%d", ok, diags, gets, patches, tc.wantError, tc.wantGet, tc.wantPatch)
			}
		})
	}
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
