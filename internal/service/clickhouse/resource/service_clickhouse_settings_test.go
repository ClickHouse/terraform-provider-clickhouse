package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func TestClickhouseSettingsPayload_SendsIntegersAsNumbers(t *testing.T) {
	got := clickhouseSettingsPayload(map[string]string{
		"compatibility":        "26.2",
		"max_query_size":       "262144",
		"mark_cache_ram_ratio": "0.25",
	})

	want := map[string]any{
		"compatibility":        "26.2",
		"max_query_size":       int64(262144),
		"mark_cache_ram_ratio": "0.25",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("clickhouseSettingsPayload mismatch (-want +got):\n%s", diff)
	}
}

func TestManagedClickhouseSettings_IgnoresUnmanagedAndDropsMissing(t *testing.T) {
	managed := map[string]string{"compatibility": "26.2", "max_query_size": "262144"}
	current := map[string]string{"compatibility": "26.1", "keep_alive_timeout": "30"}

	got := managedClickhouseSettings(managed, current)

	want := map[string]string{"compatibility": "26.1"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("managedClickhouseSettings mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceClickhouseSettingsResource_Update_ChangesAndResetsInOnePatch(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("compatibility", "max_query_size", "keep_alive_timeout"), nil).
		UpdateServiceClickhouseSettingsMock.
		Times(1).
		Expect(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}, []string{"keep_alive_timeout"}).
		Return(&api.ServiceClickhouseSettingsUpdateResult{
			Settings: map[string]string{"max_query_size": "524288"},
			Warnings: []api.ServiceClickhouseSettingWarning{{Name: "keep_alive_timeout", Message: "restarts the service"}},
		}, nil)

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"compatibility":      "26.2",
		"max_query_size":     "262144",
		"keep_alive_timeout": "30",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"compatibility":  "26.2",
		"max_query_size": "524288",
	})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diags: %v", resp.Diagnostics)
	}
	if len(resp.Diagnostics) != 1 || resp.Diagnostics[0].Detail() != "restarts the service" {
		t.Errorf("diagnostics = %v; want one warning with detail \"restarts the service\"", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Update_ChangeOnlySkipsSchemaRead(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}, nil).
		Return(&api.ServiceClickhouseSettingsUpdateResult{Settings: map[string]string{"max_query_size": "524288"}}, nil)

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "262144"})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "524288"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diags: %v", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Create_SurfacesWarnings(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{"compatibility": "26.2"}, nil).
		Return(&api.ServiceClickhouseSettingsUpdateResult{
			Settings: map[string]string{"compatibility": "26.2"},
			Warnings: []api.ServiceClickhouseSettingWarning{{Name: "compatibility", Message: "test first"}},
		}, nil)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2"})},
	}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diags: %v", resp.Diagnostics)
	}
	if len(resp.Diagnostics) != 1 || resp.Diagnostics[0].Detail() != "test first" {
		t.Errorf("diagnostics = %v; want one warning with detail \"test first\"", resp.Diagnostics)
	}
}

func clickhouseSettingsResourceWithSchema(t *testing.T) (*ServiceClickhouseSettingsResource, schema.Schema) {
	t.Helper()
	r := NewServiceClickhouseSettingsResource().(*ServiceClickhouseSettingsResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", schemaResp.Diagnostics)
	}
	return r, schemaResp.Schema
}

func clickhouseSettingsSchema(names ...string) []api.ServiceClickhouseSettingSchemaEntry {
	entries := make([]api.ServiceClickhouseSettingSchemaEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, api.ServiceClickhouseSettingSchemaEntry{Name: name})
	}
	return entries
}

func runClickhouseSettingsUpdate(ctx context.Context, r *ServiceClickhouseSettingsResource, sch schema.Schema, state, plan tftypes.Value) *resource.UpdateResponse {
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: sch, Raw: plan},
		State: tfsdk.State{Schema: sch, Raw: state},
	}, resp)
	return resp
}

func runClickhouseSettingsDelete(ctx context.Context, r *ServiceClickhouseSettingsResource, sch schema.Schema, state tftypes.Value) *resource.DeleteResponse {
	resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch, Raw: state}}
	r.Delete(ctx, resource.DeleteRequest{State: tfsdk.State{Schema: sch, Raw: state}}, resp)
	return resp
}

func clickhouseSettingsRaw(ctx context.Context, sch schema.Schema, serviceID string, settings map[string]string) tftypes.Value {
	values := make(map[string]tftypes.Value, len(settings))
	for k, v := range settings {
		values[k] = tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(sch.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, serviceID),
		"service_id": tftypes.NewValue(tftypes.String, serviceID),
		"settings":   tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, values),
	})
}
