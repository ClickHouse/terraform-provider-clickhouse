package resource

import (
	"context"
	"reflect"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func buildCustomPrivateDNSMappingList(t *testing.T, names ...string) types.List {
	t.Helper()

	values := make([]attr.Value, len(names))
	for i, name := range names {
		values[i] = models.CustomPrivateDNSMappingModel{
			PrivateDNSName: types.StringValue(name),
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
