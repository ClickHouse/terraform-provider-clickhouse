package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func TestClickhouseSettingsPayload_UsesSchemaTypes(t *testing.T) {
	settingsSchema := []api.ServiceClickhouseSettingSchemaEntry{
		{Name: "compatibility", Type: "string"},
		{Name: "max_query_size", Type: "integer"},
	}

	got := clickhouseSettingsPayload(map[string]string{
		"compatibility":  "26",
		"max_query_size": "262144",
		"unknown":        "abc",
	}, settingsSchema)

	want := map[string]any{
		"compatibility":  "26",
		"max_query_size": int64(262144),
		"unknown":        "abc",
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

func TestServiceClickhouseSettingsResource_Update_PatchesChangedAndResetsRemoved(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return([]api.ServiceClickhouseSettingSchemaEntry{{Name: "max_query_size", Type: "integer"}}, nil).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}).
		Return(&api.ServiceClickhouseSettingsUpdateResult{Settings: map[string]string{"max_query_size": "524288"}}, nil).
		DeleteServiceClickhouseSettingMock.
		Expect(ctx, "svc-1", "keep_alive_timeout").
		Return(nil)

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"compatibility":      "26.2",
		"max_query_size":     "262144",
		"keep_alive_timeout": "30",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"compatibility":  "26.2",
		"max_query_size": "524288",
	})

	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: state}}
	r.Update(ctx, resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: sch, Raw: plan},
		State: tfsdk.State{Schema: sch, Raw: state},
	}, resp)

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
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return([]api.ServiceClickhouseSettingSchemaEntry{{Name: "compatibility", Type: "string"}}, nil).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{"compatibility": "26.2"}).
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

func TestDesiredClickhouseSettings_MapsMergesEnabled(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		value types.Bool
		want  map[string]string
	}{
		{name: "unset", value: types.BoolNull(), want: map[string]string{"compatibility": "26.2"}},
		{name: "enabled", value: types.BoolValue(true), want: map[string]string{"compatibility": "26.2", settingDisableMergesAndMutations: "0"}},
		{name: "disabled", value: types.BoolValue(false), want: map[string]string{"compatibility": "26.2", settingDisableMergesAndMutations: "1"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings, d := types.MapValueFrom(ctx, types.StringType, map[string]string{"compatibility": "26.2"})
			if d.HasError() {
				t.Fatalf("MapValueFrom: %v", d)
			}
			m := &models.ServiceClickhouseSettingsResourceModel{Settings: settings, MergesEnabled: tc.value}

			got, diags := desiredClickhouseSettings(ctx, m)
			if diags.HasError() {
				t.Fatalf("desiredClickhouseSettings: %v", diags)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("desiredClickhouseSettings mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestServiceClickhouseSettingsResource_ImportState_SplitsTypedSettings(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		ListServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1").
		Return(map[string]string{"compatibility": "26.2", settingDisableMergesAndMutations: "1"}, nil)

	resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "svc-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diags: %v", resp.Diagnostics)
	}

	var state models.ServiceClickhouseSettingsResourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatalf("State.Get: %v", d)
	}
	if state.MergesEnabled.IsNull() || state.MergesEnabled.ValueBool() {
		t.Errorf("merges_enabled = %v; want false", state.MergesEnabled)
	}
	settings := map[string]string{}
	state.Settings.ElementsAs(ctx, &settings, false)
	if diff := cmp.Diff(map[string]string{"compatibility": "26.2"}, settings); diff != "" {
		t.Errorf("settings mismatch (-want +got):\n%s", diff)
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

func clickhouseSettingsRaw(ctx context.Context, sch schema.Schema, serviceID string, settings map[string]string) tftypes.Value {
	values := make(map[string]tftypes.Value, len(settings))
	for k, v := range settings {
		values[k] = tftypes.NewValue(tftypes.String, v)
	}
	return tftypes.NewValue(sch.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, serviceID),
		"service_id":     tftypes.NewValue(tftypes.String, serviceID),
		"settings":       tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, values),
		"merges_enabled": tftypes.NewValue(tftypes.Bool, nil),
	})
}
