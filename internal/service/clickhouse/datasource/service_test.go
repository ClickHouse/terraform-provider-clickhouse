package datasource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func ptrInt(i int) *int    { return &i }
func ptrBool(b bool) *bool { return &b }

func TestServiceToObjectValue_MapsScalarsAndNulls(t *testing.T) {
	ctx := context.Background()
	svc := api.Service{
		Id:                "svc-1",
		Name:              "prod",
		Provider:          "aws",
		Region:            "us-east-1",
		Tier:              "production",
		State:             "running",
		ClickHouseVersion: "24.5",
		CreatedAt:         "2026-07-12T00:00:00Z",
		ReleaseChannel:    "default",
		IsPrimary:         ptrBool(true),
		ReadOnly:          false,
		NumReplicas:       ptrInt(3),
		IdleScaling:       true,
		IAMRole:           "arn:aws:iam::123:role/x",
		Endpoints: []api.Endpoint{
			{Protocol: "nativesecure", Host: "h1", Port: 9440, Enabled: true},
		},
		Tags: []api.Tag{{Key: "Env", Value: "prod"}},
		// BYOCId, DataWarehouseId, ComplianceType, IdleTimeoutMinutes,
		// MinTotalMemoryGb, etc. left nil to exercise null handling.
	}

	obj, diags := serviceToObjectValue(ctx, svc)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	attrs := obj.Attributes()

	if got := attrs["id"].(types.String).ValueString(); got != "svc-1" {
		t.Errorf("id = %q; want svc-1", got)
	}
	if got := attrs["clickhouse_version"].(types.String).ValueString(); got != "24.5" {
		t.Errorf("clickhouse_version = %q; want 24.5", got)
	}
	if got := attrs["is_primary"].(types.Bool).ValueBool(); got != true {
		t.Errorf("is_primary = %v; want true", got)
	}
	if got := attrs["num_replicas"].(types.Int64).ValueInt64(); got != 3 {
		t.Errorf("num_replicas = %d; want 3", got)
	}
	// nil pointers -> null
	if !attrs["byoc_id"].(types.String).IsNull() {
		t.Errorf("byoc_id should be null when BYOCId is nil")
	}
	if !attrs["idle_timeout_minutes"].(types.Int64).IsNull() {
		t.Errorf("idle_timeout_minutes should be null when nil")
	}
	// nested endpoints
	eps := attrs["endpoints"].(types.List).Elements()
	if len(eps) != 1 {
		t.Fatalf("endpoints len = %d; want 1", len(eps))
	}
	ep := eps[0].(types.Object).Attributes()
	if got := ep["host"].(types.String).ValueString(); got != "h1" {
		t.Errorf("endpoint host = %q; want h1", got)
	}
	if got := ep["port"].(types.Int64).ValueInt64(); got != 9440 {
		t.Errorf("endpoint port = %d; want 9440", got)
	}
	// tags
	tags := attrs["tags"].(types.Map).Elements()
	if got := tags["Env"].(types.String).ValueString(); got != "prod" {
		t.Errorf("tag Env = %q; want prod", got)
	}
}

func TestServiceToObjectValue_MapsNestedAccessAndEncryption(t *testing.T) {
	ctx := context.Background()
	svc := api.Service{
		Id:                 "svc-2",
		Name:               "n",
		Provider:           "aws",
		Region:             "eu-west-1",
		IpAccessList:       []api.IpAccess{{Source: "1.2.3.4", Description: "office"}},
		PrivateEndpointIds: []string{"pe-1", "pe-2"},
		PrivateEndpointConfig: &api.ServicePrivateEndpointConfig{
			EndpointServiceId:  "esid",
			PrivateDnsHostname: "dns",
		},
		EncryptionKey: "kkk",
	}

	obj, diags := serviceToObjectValue(ctx, svc)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	attrs := obj.Attributes()

	ip := attrs["ip_access"].(types.List).Elements()
	if len(ip) != 1 {
		t.Fatalf("ip_access len = %d; want 1", len(ip))
	}
	ipa := ip[0].(types.Object).Attributes()
	if got := ipa["source"].(types.String).ValueString(); got != "1.2.3.4" {
		t.Errorf("ip source = %q; want 1.2.3.4", got)
	}
	pids := attrs["private_endpoint_ids"].(types.List).Elements()
	if len(pids) != 2 {
		t.Errorf("private_endpoint_ids len = %d; want 2", len(pids))
	}
	pec := attrs["private_endpoint_config"].(types.Object).Attributes()
	if got := pec["endpoint_service_id"].(types.String).ValueString(); got != "esid" {
		t.Errorf("endpoint_service_id = %q; want esid", got)
	}
	if got := attrs["encryption_key"].(types.String).ValueString(); got != "kkk" {
		t.Errorf("encryption_key = %q; want kkk", got)
	}
}

// TestServiceSchemaMatchesObjectType is the guard against "triple drift": the
// schema attribute types (serviceComputedAttributesWithID) must match the state
// object type (serviceObjectType) exactly, or the framework throws a Value
// Conversion Error at terraform-plan time that no other unit test would catch.
func TestServiceSchemaMatchesObjectType(t *testing.T) {
	objTypes := serviceObjectType().AttrTypes
	schemaAttrs := serviceComputedAttributesWithID()

	if len(objTypes) != len(schemaAttrs) {
		t.Errorf("attr count: objectType=%d schema=%d", len(objTypes), len(schemaAttrs))
	}
	for name, at := range objTypes {
		sa, ok := schemaAttrs[name]
		if !ok {
			t.Errorf("attr %q in objectType but not in schema", name)
			continue
		}
		if got := sa.GetType(); !got.Equal(at) {
			t.Errorf("attr %q type mismatch: schema=%v objectType=%v", name, got, at)
		}
	}
	for name := range schemaAttrs {
		if _, ok := objTypes[name]; !ok {
			t.Errorf("attr %q in schema but not in objectType", name)
		}
	}
}

// TestServiceObjectRoundTripsToModel asserts the shared object maps cleanly into
// the singular struct model via obj.As (the state-setting path the singular Read
// uses). Catches tfsdk-tag/type drift between the struct and the object type.
func TestServiceObjectRoundTripsToModel(t *testing.T) {
	ctx := context.Background()
	obj, diags := serviceToObjectValue(ctx, api.Service{Id: "svc-1", Name: "n", Provider: "aws", Region: "r", NumReplicas: ptrInt(2)})
	if diags.HasError() {
		t.Fatalf("mapping diags: %v", diags)
	}
	var m serviceDataSourceModel
	if d := obj.As(ctx, &m, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatalf("obj.As diags: %v", d)
	}
	if m.ID.ValueString() != "svc-1" {
		t.Errorf("ID = %q; want svc-1", m.ID.ValueString())
	}
	if m.NumReplicas.ValueInt64() != 2 {
		t.Errorf("NumReplicas = %d; want 2", m.NumReplicas.ValueInt64())
	}
}
