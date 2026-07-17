package clickstack

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func boolPtr(b bool) *bool { return &b }

func permModel(action, subject, integration string, inverted bool, fields []string, conditions string) rolePermissionModel {
	m := rolePermissionModel{
		Action:      types.StringValue(action),
		Subject:     types.StringValue(subject),
		Integration: types.StringValue(integration),
		Inverted:    types.BoolValue(inverted),
		Fields:      types.ListNull(types.StringType),
		Conditions:  types.StringNull(),
	}
	if len(fields) > 0 {
		elems := make([]attr.Value, len(fields))
		for i, f := range fields {
			elems[i] = types.StringValue(f)
		}
		m.Fields = types.ListValueMust(types.StringType, elems)
	}
	if conditions != "" {
		m.Conditions = types.StringValue(conditions)
	}
	return m
}

func TestRoleResource_Schema(t *testing.T) {
	t.Parallel()

	r := NewRoleResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "team", "name", "description", "permissions"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected resource schema to contain attribute %q", attr)
		}
	}
}

func TestCanonicalJSON(t *testing.T) {
	t.Parallel()

	got := canonicalJSON(`{ "kind":  "log" ,  "n": 1 }`)
	want := `{"kind":"log","n":1}`
	if got != want {
		t.Errorf("canonicalJSON = %q, want %q", got, want)
	}

	// Invalid JSON is returned unchanged.
	if got := canonicalJSON("not json"); got != "not json" {
		t.Errorf("canonicalJSON(invalid) = %q", got)
	}
}

func TestPermissionsEqual(t *testing.T) {
	t.Parallel()

	a := []rolePermissionModel{
		permModel("read", "Dashboard", "mongodb", false, nil, ""),
		permModel("read", "Source", "mongodb", false, nil, `{"kind":"log"}`),
	}
	// Reordered, with differently-formatted (but equivalent) conditions JSON.
	b := []rolePermissionModel{
		permModel("read", "Source", "mongodb", false, nil, `{ "kind": "log" }`),
		permModel("read", "Dashboard", "mongodb", false, nil, ""),
	}
	if !permissionsEqual(a, b) {
		t.Error("expected permission sets to be equal ignoring order and JSON formatting")
	}

	c := []rolePermissionModel{
		permModel("manage", "Dashboard", "mongodb", false, nil, ""),
	}
	if permissionsEqual(a, c) {
		t.Error("expected permission sets of different length to be unequal")
	}

	d := []rolePermissionModel{
		permModel("read", "Dashboard", "mongodb", false, nil, ""),
		permModel("read", "Source", "mongodb", true, nil, `{"kind":"log"}`), // inverted differs
	}
	if permissionsEqual(a, d) {
		t.Error("expected permission sets with differing inverted to be unequal")
	}
}

func TestFilterAutoConnectionRead(t *testing.T) {
	t.Parallel()

	connRead := permModel("read", "Connection", "mongodb", false, nil, "")
	dashboard := permModel("read", "Dashboard", "mongodb", false, nil, "")

	// Not configured: the auto-injected permission is removed.
	server := []rolePermissionModel{dashboard, connRead}
	configured := []rolePermissionModel{dashboard}
	got := filterAutoConnectionRead(server, configured)
	if len(got) != 1 || permKey(got[0]) != permKey(dashboard) {
		t.Errorf("expected connRead filtered out, got %d perms", len(got))
	}

	// Configured explicitly: it is preserved.
	configured = []rolePermissionModel{dashboard, connRead}
	got = filterAutoConnectionRead(server, configured)
	if len(got) != 2 {
		t.Errorf("expected connRead preserved, got %d perms", len(got))
	}
}

func TestBuildAndFlattenPermissionsRoundTrip(t *testing.T) {
	t.Parallel()

	models := []rolePermissionModel{
		permModel("manage", "Dashboard", "mongodb", false, nil, ""),
		permModel("read", "Source", "clickhouse", true, []string{"a", "b"}, `{"kind":"log"}`),
	}

	perms, diags := buildPermissions(context.Background(), models)
	if diags.HasError() {
		t.Fatalf("buildPermissions diagnostics: %s", diags)
	}
	if len(perms) != 2 {
		t.Fatalf("expected 2 client perms, got %d", len(perms))
	}
	if perms[1].Integration != "clickhouse" || perms[1].Inverted == nil || !*perms[1].Inverted {
		t.Errorf("unexpected client perm: %+v", perms[1])
	}
	if len(perms[1].Fields) != 2 || string(perms[1].Conditions) != `{"kind":"log"}` {
		t.Errorf("unexpected fields/conditions: %+v", perms[1])
	}

	flat, diags := flattenPermissions(perms)
	if diags.HasError() {
		t.Fatalf("flattenPermissions diagnostics: %s", diags)
	}
	if !permissionsEqual(models, flat) {
		t.Errorf("round trip mismatch:\n got  %+v\n want %+v", flat, models)
	}
}

func TestFlattenPermissions_Normalization(t *testing.T) {
	t.Parallel()

	// Empty fields and "null" conditions normalize to null, and a missing
	// integration defaults to mongodb.
	flat, diags := flattenPermissions([]client.Permission{
		{Action: "read", Subject: "Dashboard", Inverted: boolPtr(false), Fields: []string{}, Conditions: []byte("null")},
	})
	if diags.HasError() {
		t.Fatalf("diagnostics: %s", diags)
	}
	if !flat[0].Fields.IsNull() {
		t.Error("expected empty fields to normalize to null")
	}
	if !flat[0].Conditions.IsNull() {
		t.Error("expected null conditions to normalize to null")
	}
	if flat[0].Integration.ValueString() != integrationMongoDB {
		t.Errorf("expected default integration, got %q", flat[0].Integration.ValueString())
	}
}
