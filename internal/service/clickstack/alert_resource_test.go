package clickstack

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

// alertChannelAttrTypes is hand-maintained alongside alertChannelAttributes();
// this pins them together so adding a schema attribute without the matching
// attr.Type fails here rather than at runtime inside ObjectValueFrom.
func TestAlertChannelAttrTypesMatchSchema(t *testing.T) {
	t.Parallel()
	schemaAttrs := alertChannelAttributes()
	if len(schemaAttrs) != len(alertChannelAttrTypes) {
		t.Fatalf("schema has %d channel attributes, alertChannelAttrTypes has %d",
			len(schemaAttrs), len(alertChannelAttrTypes))
	}
	for name, a := range schemaAttrs {
		typ, ok := alertChannelAttrTypes[name]
		if !ok {
			t.Errorf("schema attribute %q missing from alertChannelAttrTypes", name)
			continue
		}
		if got := a.GetType(); got != typ {
			t.Errorf("attribute %q: schema type %s, alertChannelAttrTypes %s", name, got, typ)
		}
	}
}

func TestAnomalyConfigAttrTypesMatchSchema(t *testing.T) {
	t.Parallel()
	schemaAttrs := anomalyConfigAttributes()
	if len(schemaAttrs) != len(anomalyConfigAttrTypes) {
		t.Fatalf("schema has %d anomaly_config attributes, anomalyConfigAttrTypes has %d",
			len(schemaAttrs), len(anomalyConfigAttrTypes))
	}
	for name, a := range schemaAttrs {
		typ, ok := anomalyConfigAttrTypes[name]
		if !ok {
			t.Errorf("schema attribute %q missing from anomalyConfigAttrTypes", name)
			continue
		}
		if got := a.GetType(); got != typ {
			t.Errorf("attribute %q: schema type %s, anomalyConfigAttrTypes %s", name, got, typ)
		}
	}
}

func TestAlertResource_DetectionMode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// The server resolves an omitted detectionMode to threshold instead of
	// keeping the stored one, so a write that leaves it out demotes an anomaly
	// alert. Every write has to carry it, including from legacy state.
	t.Run("toClient always sends a mode", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			mod  func(*alertResourceModel)
			want string
		}{
			{"explicit threshold", nil, detectionModeThreshold},
			{"anomaly", asAnomaly(fullAnomalyConfig()), detectionModeAnomaly},
			{"null mode from older state", func(m *alertResourceModel) { m.DetectionMode = types.StringNull() }, detectionModeThreshold},
		} {
			m := mkAlert(tc.mod)
			al, d := m.toClient(ctx)
			if d.HasError() {
				t.Fatalf("%s: toClient: %s", tc.name, d)
			}
			if al.DetectionMode != tc.want {
				t.Errorf("%s: detection mode = %q, want %q", tc.name, al.DetectionMode, tc.want)
			}
		}
	})

	t.Run("toClient sends the whole config in anomaly mode", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(asAnomaly(fullAnomalyConfig()))
		al, d := m.toClient(ctx)
		if d.HasError() {
			t.Fatalf("toClient: %s", d)
		}
		want := client.AnomalyConfig{
			BucketSizeSeconds: ptr(120), ZScoreThreshold: ptr(4.0), Condition: ptr(anomalyConditionAbove),
			MinAbsoluteDelta: ptr(5.0), NonNegative: ptr(false), MaxSeries: ptr(50), BaselineLookbackMinutes: ptr(720),
		}
		if diff := cmp.Diff(&want, al.AnomalyConfig); diff != "" {
			t.Errorf("anomaly config mismatch (-want +got):\n%s", diff)
		}
	})

	// A field the config leaves out is omitted rather than filled in here, so
	// the server's defaults stay in one place.
	t.Run("toClient sends only the fields the config set", func(t *testing.T) {
		t.Parallel()
		partial := anomalyConfigModel{
			BucketSizeSeconds:       types.Int64Null(),
			ZScoreThreshold:         types.Float64Value(6),
			Condition:               types.StringNull(),
			MinAbsoluteDelta:        types.Float64Null(),
			NonNegative:             types.BoolNull(),
			MaxSeries:               types.Int64Null(),
			BaselineLookbackMinutes: types.Int64Null(),
		}
		m := mkAlert(asAnomaly(partial))
		al, d := m.toClient(ctx)
		if d.HasError() {
			t.Fatalf("toClient: %s", d)
		}
		want := client.AnomalyConfig{ZScoreThreshold: ptr(6.0)}
		if diff := cmp.Diff(&want, al.AnomalyConfig); diff != "" {
			t.Errorf("anomaly config mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("toClient omits the config in threshold mode", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(nil)
		al, d := m.toClient(ctx)
		if d.HasError() {
			t.Fatalf("toClient: %s", d)
		}
		if al.AnomalyConfig != nil {
			t.Errorf("anomaly config = %+v, want nil", al.AnomalyConfig)
		}
	})

	t.Run("applyAlert maps the mode and config back", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(asAnomaly(fullAnomalyConfig()))
		cfg := client.AnomalyConfig{
			BucketSizeSeconds: ptr(300), ZScoreThreshold: ptr(2.5), Condition: ptr(anomalyConditionBelow),
			MinAbsoluteDelta: ptr(1.0), NonNegative: ptr(true), MaxSeries: ptr(10), BaselineLookbackMinutes: ptr(60),
		}
		if d := m.applyAlert(ctx, &client.Alert{
			ID: "al1", Source: alertSourceSavedSearch, SavedSearchID: "ss1",
			Interval: "5m", Threshold: 100, ThresholdType: thresholdTypeAbove,
			DetectionMode: detectionModeAnomaly, AnomalyConfig: &cfg,
		}); d.HasError() {
			t.Fatalf("applyAlert: %s", d)
		}
		if m.DetectionMode.ValueString() != detectionModeAnomaly {
			t.Errorf("detection mode = %q, want anomaly", m.DetectionMode.ValueString())
		}
		got, ok, d := asAnomalyConfig(ctx, m.AnomalyConfig)
		if d.HasError() || !ok || got == nil {
			t.Fatalf("decode anomaly config: ok=%v cfg=%v diags=%s", ok, got, d)
		}
		if got.BucketSizeSeconds.ValueInt64() != 300 || got.ZScoreThreshold.ValueFloat64() != 2.5 ||
			got.Condition.ValueString() != "below" || got.MaxSeries.ValueInt64() != 10 {
			t.Errorf("anomaly config = %+v", got)
		}
	})

	t.Run("applyAlert nulls the config in threshold mode", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(asAnomaly(fullAnomalyConfig()))
		if d := m.applyAlert(ctx, &client.Alert{
			ID: "al1", Source: alertSourceSavedSearch, SavedSearchID: "ss1",
			Interval: "5m", Threshold: 100, ThresholdType: thresholdTypeAbove,
			DetectionMode: detectionModeThreshold,
		}); d.HasError() {
			t.Fatalf("applyAlert: %s", d)
		}
		if !m.AnomalyConfig.IsNull() {
			t.Errorf("anomaly config = %s, want null", m.AnomalyConfig)
		}
	})

	// A server predating detection modes returns none at all. Overwriting the
	// planned value then would report an anomaly alert as threshold and fail
	// the apply with an inconsistent result.
	t.Run("applyAlert keeps the planned mode when the server returns none", func(t *testing.T) {
		t.Parallel()
		for _, tc := range []struct {
			name string
			mod  func(*alertResourceModel)
			want string
		}{
			{"planned threshold", nil, detectionModeThreshold},
			{"planned anomaly", asAnomaly(fullAnomalyConfig()), detectionModeAnomaly},
			{"unknown, as on a create", func(m *alertResourceModel) { m.DetectionMode = types.StringUnknown() }, detectionModeThreshold},
		} {
			m := mkAlert(tc.mod)
			if d := m.applyAlert(ctx, &client.Alert{
				ID: "al1", Source: alertSourceSavedSearch, SavedSearchID: "ss1",
				Interval: "5m", Threshold: 100, ThresholdType: thresholdTypeAbove,
			}); d.HasError() {
				t.Fatalf("%s: applyAlert: %s", tc.name, d)
			}
			if m.DetectionMode.ValueString() != tc.want {
				t.Errorf("%s: detection mode = %q, want %q", tc.name, m.DetectionMode.ValueString(), tc.want)
			}
		}
	})
}

// An Optional+Computed attribute whose config is null keeps its prior value in
// the proposed plan, so without ModifyPlan a switch back to threshold would
// plan the old block and then fail against the null applyAlert writes.
func TestAlertResource_ModifyPlan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sch := alertSchema(t)
	r := &alertResource{}

	modify := func(t *testing.T, state, plan alertResourceModel) alertResourceModel {
		t.Helper()
		st := tfsdk.State{Schema: sch}
		if d := st.Set(ctx, state); d.HasError() {
			t.Fatalf("state.Set: %s", d)
		}
		pl := tfsdk.Plan{Schema: sch}
		if d := pl.Set(ctx, plan); d.HasError() {
			t.Fatalf("plan.Set: %s", d)
		}
		resp := &fwresource.ModifyPlanResponse{Plan: pl}
		r.ModifyPlan(ctx, fwresource.ModifyPlanRequest{State: st, Plan: pl}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("ModifyPlan: %s", resp.Diagnostics)
		}
		var got alertResourceModel
		resp.Plan.Get(ctx, &got)
		return got
	}

	t.Run("drops a stale config when the plan is threshold", func(t *testing.T) {
		t.Parallel()
		// The plan Terraform proposes when the block is deleted from config:
		// threshold mode, but the prior object carried over.
		plan := mkAlert(asAnomaly(fullAnomalyConfig()))
		plan.DetectionMode = types.StringValue(detectionModeThreshold)
		got := modify(t, mkAlert(asAnomaly(fullAnomalyConfig())), plan)
		if !got.AnomalyConfig.IsNull() {
			t.Errorf("anomaly_config = %s, want null", got.AnomalyConfig)
		}
	})

	t.Run("leaves the config alone in anomaly mode", func(t *testing.T) {
		t.Parallel()
		got := modify(t, mkAlert(asAnomaly(fullAnomalyConfig())), mkAlert(asAnomaly(fullAnomalyConfig())))
		if got.AnomalyConfig.IsNull() {
			t.Error("anomaly_config was dropped in anomaly mode")
		}
	})

	t.Run("no-op on create", func(t *testing.T) {
		t.Parallel()
		plan := mkAlert(asAnomaly(fullAnomalyConfig()))
		plan.DetectionMode = types.StringValue(detectionModeThreshold)
		pl := tfsdk.Plan{Schema: sch}
		if d := pl.Set(ctx, plan); d.HasError() {
			t.Fatalf("plan.Set: %s", d)
		}
		resp := &fwresource.ModifyPlanResponse{Plan: pl}
		// Null state is a create; there is no prior value to be stale.
		r.ModifyPlan(ctx, fwresource.ModifyPlanRequest{State: tfsdk.State{Schema: sch}, Plan: pl}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("ModifyPlan: %s", resp.Diagnostics)
		}
		var got alertResourceModel
		resp.Plan.Get(ctx, &got)
		if got.AnomalyConfig.IsNull() {
			t.Error("anomaly_config was dropped on create")
		}
	})
}

func TestAlertResource_Metadata(t *testing.T) {
	t.Parallel()
	r := NewAlertResource()
	resp := &fwresource.MetadataResponse{}
	r.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "clickhouse"}, resp)
	if resp.TypeName != "clickhouse_clickstack_alert" {
		t.Errorf("expected clickhouse_clickstack_alert, got %q", resp.TypeName)
	}
}

func TestAlertResource_Schema(t *testing.T) {
	t.Parallel()
	r := NewAlertResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %s", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "team", "saved_search_id", "channel", "channels", "threshold", "threshold_type", "interval"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	// channel is superseded by channels but still accepted, so it must be
	// Optional (not Required) and carry a deprecation notice.
	ch, ok := resp.Schema.Attributes["channel"].(rschema.SingleNestedAttribute)
	if !ok || ch.IsRequired() || ch.GetDeprecationMessage() == "" {
		t.Error("channel must be an Optional, deprecated single-nested attribute")
	}

	// group_by is kept-on-omit and cannot be cleared via the API, so it is
	// Optional+Computed (sticky).
	if a, ok := resp.Schema.Attributes["group_by"]; !ok || !a.IsComputed() {
		t.Error("group_by must be Optional+Computed (server keeps it on omit)")
	}
	// The mutually-exclusive schedule fields must NOT be sticky/Computed, or a
	// mode switch would resend a stale value; the client clears them explicitly.
	for _, attr := range []string{"schedule_offset_minutes", "schedule_start_at"} {
		if a, ok := resp.Schema.Attributes[attr]; !ok || a.IsComputed() {
			t.Errorf("%q must be Optional (not Computed) so mode switches clear it", attr)
		}
	}
	for _, attr := range []string{"source", "dashboard_id", "tile_id"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	// Tile alerts have no saved search, so saved_search_id can no longer be Required.
	if a := resp.Schema.Attributes["saved_search_id"]; a != nil && a.IsRequired() {
		t.Error("saved_search_id must be Optional now that tile alerts exist")
	}
	// source is Optional+Computed so the saved_search default applies at plan time
	// and existing configs that never set it see no diff.
	if a := resp.Schema.Attributes["source"]; a != nil && (!a.IsOptional() || !a.IsComputed()) {
		t.Error("source must be Optional+Computed so the default applies")
	}
	// The default has to be saved_search: it is what an existing config that never
	// mentioned source resolves to, so anything else would plan a change on upgrade.
	if sa, ok := resp.Schema.Attributes["source"].(rschema.StringAttribute); !ok || sa.Default == nil {
		t.Error("source must have a default")
	} else {
		dresp := &defaults.StringResponse{}
		sa.Default.DefaultString(context.Background(), defaults.StringRequest{}, dresp)
		if dresp.PlanValue.ValueString() != alertSourceSavedSearch {
			t.Errorf("source default = %q, want %q", dresp.PlanValue.ValueString(), alertSourceSavedSearch)
		}
	}
}

// webhookChannel builds a valid webhook channel block.
func webhookChannel(webhookID string) alertChannelModel {
	return alertChannelModel{Type: types.StringValue("webhook"), WebhookID: types.StringValue(webhookID)}
}

// The channel attributes are framework types so they can hold a wholly-unknown
// value, so tests build and read them through these helpers. They panic rather
// than take a *testing.T: the only way the conversions below can fail is a
// static mismatch between alertChannelModel and alertChannelAttrTypes, which is
// a bug in the test itself, not something a case can legitimately produce.

func chanObj(c alertChannelModel) types.Object {
	o, d := types.ObjectValueFrom(context.Background(), alertChannelAttrTypes, c)
	if d.HasError() {
		panic(fmt.Sprintf("build channel object: %s", d))
	}
	return o
}

func chanList(cs ...alertChannelModel) types.List {
	if cs == nil {
		cs = []alertChannelModel{}
	}
	l, d := types.ListValueFrom(context.Background(), alertChannelObjectType, cs)
	if d.HasError() {
		panic(fmt.Sprintf("build channels list: %s", d))
	}
	return l
}

func nullChan() types.Object   { return types.ObjectNull(alertChannelAttrTypes) }
func nullChanList() types.List { return types.ListNull(alertChannelObjectType) }

// fullAnomalyConfig is a config with every field set to something other than
// its server default, so a test that loses one can tell which.
func fullAnomalyConfig() anomalyConfigModel {
	return anomalyConfigModel{
		BucketSizeSeconds:       types.Int64Value(120),
		ZScoreThreshold:         types.Float64Value(4),
		Condition:               types.StringValue("above"),
		MinAbsoluteDelta:        types.Float64Value(5),
		NonNegative:             types.BoolValue(false),
		MaxSeries:               types.Int64Value(50),
		BaselineLookbackMinutes: types.Int64Value(720),
	}
}

func anomalyObj(c anomalyConfigModel) types.Object {
	o, d := types.ObjectValueFrom(context.Background(), anomalyConfigAttrTypes, c)
	if d.HasError() {
		panic(fmt.Sprintf("build anomaly config object: %s", d))
	}
	return o
}

// asAnomaly turns a model into an anomaly alert carrying cfg.
func asAnomaly(cfg anomalyConfigModel) func(*alertResourceModel) {
	return func(m *alertResourceModel) {
		m.DetectionMode = types.StringValue(detectionModeAnomaly)
		m.AnomalyConfig = anomalyObj(cfg)
	}
}

// readChan decodes the deprecated single channel for assertions.
func readChan(t *testing.T, o types.Object) alertChannelModel {
	t.Helper()
	c, ok, d := asChannel(context.Background(), o)
	if d.HasError() || !ok || c == nil {
		t.Fatalf("decode channel (ok=%v): %s", ok, d)
	}
	return *c
}

// readChanList decodes the channels list for assertions.
func readChanList(t *testing.T, l types.List) []alertChannelModel {
	t.Helper()
	cs, ok, d := asChannels(context.Background(), l)
	if d.HasError() || !ok {
		t.Fatalf("decode channels (ok=%v): %s", ok, d)
	}
	return cs
}

// mkAlert builds a valid saved-search alert model; mods tweaks it per case.
func mkAlert(mods func(*alertResourceModel)) alertResourceModel {
	m := alertResourceModel{
		Source:                types.StringValue(alertSourceSavedSearch),
		SavedSearchID:         types.StringValue("ss1"),
		DashboardID:           types.StringNull(),
		TileID:                types.StringNull(),
		GroupBy:               types.StringNull(),
		Channel:               chanObj(webhookChannel("wh1")),
		Channels:              nullChanList(),
		Threshold:             types.Float64Value(100),
		ThresholdType:         types.StringValue(thresholdTypeAbove),
		ThresholdMax:          types.Float64Null(),
		DetectionMode:         types.StringValue(detectionModeThreshold),
		AnomalyConfig:         types.ObjectNull(anomalyConfigAttrTypes),
		Interval:              types.StringValue("5m"),
		NumConsecutiveWindows: types.Int64Null(),
		ScheduleOffsetMinutes: types.Int64Null(),
		ScheduleStartAt:       types.StringNull(),
		Name:                  types.StringNull(),
		Message:               types.StringNull(),
		Note:                  types.StringNull(),
	}
	if mods != nil {
		mods(&m)
	}
	return m
}

// asTile turns the mkAlert saved-search model into a valid tile alert.
func asTile(m *alertResourceModel) {
	m.Source = types.StringValue(alertSourceTile)
	m.SavedSearchID = types.StringNull()
	m.DashboardID = types.StringValue("d1")
	m.TileID = types.StringValue("t1")
}

func TestAlertResource_SourceRequiresReplace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// A non-null tftypes value stands in for "the resource exists" in both Plan
	// and State, so the modifier does not short-circuit as create or destroy.
	exists := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})

	cases := []struct {
		name  string
		state types.String
		plan  types.String
		want  bool
	}{
		{"legacy null state to default is an upgrade, not a change", types.StringNull(), types.StringValue(alertSourceSavedSearch), false},
		{"unchanged source", types.StringValue(alertSourceSavedSearch), types.StringValue(alertSourceSavedSearch), false},
		{"saved_search to tile replaces", types.StringValue(alertSourceSavedSearch), types.StringValue(alertSourceTile), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := planmodifier.StringRequest{
				Path:       path.Root("source"),
				StateValue: tc.state,
				PlanValue:  tc.plan,
				State:      tfsdk.State{Raw: exists},
				Plan:       tfsdk.Plan{Raw: exists},
			}
			resp := &planmodifier.StringResponse{PlanValue: tc.plan}
			sourceRequiresReplace().PlanModifyString(ctx, req, resp)
			if resp.RequiresReplace != tc.want {
				t.Errorf("RequiresReplace=%v, want %v", resp.RequiresReplace, tc.want)
			}
		})
	}
}

// TestAlertResource_TargetRequiresReplace drives the target ids through the
// real schema, because the two differ on the unknown case. An unknown tile_id
// must not replace: tile_ids goes unknown whenever the dashboard body is
// unknown at plan, and replacing on that would destroy every tile alert on the
// dashboard. An unknown dashboard_id must replace: dashboard id only goes
// unknown when the dashboard is being replaced, and the server deletes its tile
// alerts with it, so an in-place update would 404 and fail the apply.
func TestAlertResource_TargetRequiresReplace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	exists := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	schema := alertSchema(t)

	cases := []struct {
		name     string
		attrName string
		state    types.String
		plan     types.String
		want     bool
	}{
		{"different known tile_id replaces", tileIDAttr, types.StringValue("t1"), types.StringValue("t2"), true},
		{"same tile_id does not replace", tileIDAttr, types.StringValue("t1"), types.StringValue("t1"), false},
		{"unknown tile_id does not replace", tileIDAttr, types.StringValue("t1"), types.StringUnknown(), false},
		{"different known dashboard_id replaces", dashboardIDAttr, types.StringValue("d1"), types.StringValue("d2"), true},
		{"same dashboard_id does not replace", dashboardIDAttr, types.StringValue("d1"), types.StringValue("d1"), false},
		{"unknown dashboard_id replaces", dashboardIDAttr, types.StringValue("d1"), types.StringUnknown(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			attr, ok := schema.Attributes[tc.attrName].(rschema.StringAttribute)
			if !ok {
				t.Fatalf("%s is not a StringAttribute", tc.attrName)
			}
			if len(attr.PlanModifiers) != 1 {
				t.Fatalf("%s has %d plan modifiers, want 1", tc.attrName, len(attr.PlanModifiers))
			}
			req := planmodifier.StringRequest{
				Path:       path.Root(tc.attrName),
				StateValue: tc.state,
				PlanValue:  tc.plan,
				State:      tfsdk.State{Raw: exists},
				Plan:       tfsdk.Plan{Raw: exists},
			}
			resp := &planmodifier.StringResponse{PlanValue: tc.plan}
			attr.PlanModifiers[0].PlanModifyString(ctx, req, resp)
			if resp.RequiresReplace != tc.want {
				t.Errorf("RequiresReplace=%v, want %v", resp.RequiresReplace, tc.want)
			}
		})
	}
}

func TestAlertResource_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mod     func(*alertResourceModel)
		wantErr bool
	}{
		{"valid saved-search alert", nil, false},
		{"invalid threshold_type", func(m *alertResourceModel) { m.ThresholdType = types.StringValue("bogus") }, true},
		{"invalid interval", func(m *alertResourceModel) { m.Interval = types.StringValue("2m") }, true},
		{
			"between without threshold_max",
			func(m *alertResourceModel) { m.ThresholdType = types.StringValue(thresholdTypeBetween) },
			true,
		},
		{
			"between with threshold_max < threshold",
			func(m *alertResourceModel) {
				m.ThresholdType = types.StringValue(thresholdTypeBetween)
				m.ThresholdMax = types.Float64Value(50)
			},
			true,
		},
		{
			"between with valid threshold_max",
			func(m *alertResourceModel) {
				m.ThresholdType = types.StringValue(thresholdTypeBetween)
				m.ThresholdMax = types.Float64Value(200)
			},
			false,
		},
		{
			"above with threshold_max set is accepted",
			func(m *alertResourceModel) { m.ThresholdMax = types.Float64Value(200) },
			false,
		},
		{"anomaly alert with a full config", asAnomaly(fullAnomalyConfig()), false},
		{
			"anomaly mode with no config",
			func(m *alertResourceModel) { m.DetectionMode = types.StringValue(detectionModeAnomaly) },
			false,
		},
		{
			"invalid detection_mode",
			func(m *alertResourceModel) { m.DetectionMode = types.StringValue("bogus") },
			true,
		},
		{
			"anomaly_config in threshold mode",
			func(m *alertResourceModel) { m.AnomalyConfig = anomalyObj(fullAnomalyConfig()) },
			true,
		},
		{
			"anomaly_config with a null detection_mode",
			func(m *alertResourceModel) {
				m.DetectionMode = types.StringNull()
				m.AnomalyConfig = anomalyObj(fullAnomalyConfig())
			},
			true,
		},
		{
			"bucket_size_seconds below the minimum",
			asAnomaly(func() anomalyConfigModel {
				c := fullAnomalyConfig()
				c.BucketSizeSeconds = types.Int64Value(30)
				return c
			}()),
			true,
		},
		{
			"max_series above the maximum",
			asAnomaly(func() anomalyConfigModel {
				c := fullAnomalyConfig()
				c.MaxSeries = types.Int64Value(1001)
				return c
			}()),
			true,
		},
		{
			"zero z_score_threshold",
			asAnomaly(func() anomalyConfigModel {
				c := fullAnomalyConfig()
				c.ZScoreThreshold = types.Float64Value(0)
				return c
			}()),
			true,
		},
		{
			"negative min_absolute_delta",
			asAnomaly(func() anomalyConfigModel {
				c := fullAnomalyConfig()
				c.MinAbsoluteDelta = types.Float64Value(-1)
				return c
			}()),
			true,
		},
		{
			"invalid condition",
			asAnomaly(func() anomalyConfigModel {
				c := fullAnomalyConfig()
				c.Condition = types.StringValue("sideways")
				return c
			}()),
			true,
		},
		{
			"start_at and non-zero offset conflict",
			func(m *alertResourceModel) {
				m.ScheduleStartAt = types.StringValue("2026-01-01T00:00:00Z")
				m.ScheduleOffsetMinutes = types.Int64Value(5)
			},
			true,
		},
		{
			"start_at with zero offset is fine",
			func(m *alertResourceModel) {
				m.ScheduleStartAt = types.StringValue("2026-01-01T00:00:00Z")
				m.ScheduleOffsetMinutes = types.Int64Value(0)
			},
			false,
		},
		{
			"offset >= interval",
			func(m *alertResourceModel) { m.ScheduleOffsetMinutes = types.Int64Value(10) }, // interval 5m
			true,
		},
		{
			"offset < interval",
			func(m *alertResourceModel) {
				m.Interval = types.StringValue("1h")
				m.ScheduleOffsetMinutes = types.Int64Value(10)
			},
			false,
		},
		{
			"offset out of range",
			func(m *alertResourceModel) {
				m.Interval = types.StringValue("1d")
				m.ScheduleOffsetMinutes = types.Int64Value(2000)
			},
			true,
		},
		{
			"num_consecutive_windows below 1",
			func(m *alertResourceModel) { m.NumConsecutiveWindows = types.Int64Value(0) },
			true,
		},
		{
			"channel webhook without webhook_id",
			func(m *alertResourceModel) {
				m.Channel = chanObj(alertChannelModel{Type: types.StringValue("webhook"), WebhookID: types.StringNull()})
			},
			true,
		},
		{
			"invalid channel type",
			func(m *alertResourceModel) {
				m.Channel = chanObj(alertChannelModel{Type: types.StringValue("email"), WebhookID: types.StringValue("wh1")})
			},
			true,
		},
		{
			"neither channel nor channels",
			func(m *alertResourceModel) { m.Channel = nullChan() },
			true,
		},
		{
			"both channel and channels",
			func(m *alertResourceModel) { m.Channels = chanList(webhookChannel("wh2")) },
			true,
		},
		{
			"channels only, several targets",
			func(m *alertResourceModel) {
				m.Channel = nullChan()
				m.Channels = chanList(webhookChannel("wh1"), webhookChannel("wh2"))
			},
			false,
		},
		{
			"empty channels list",
			func(m *alertResourceModel) {
				m.Channel = nullChan()
				m.Channels = chanList()
			},
			true,
		},
		{
			"channels over the limit",
			func(m *alertResourceModel) {
				m.Channel = nullChan()
				over := make([]alertChannelModel, 0, client.MaxAlertChannels+1)
				for i := range client.MaxAlertChannels + 1 {
					over = append(over, webhookChannel(fmt.Sprintf("wh%d", i)))
				}
				m.Channels = chanList(over...)
			},
			true,
		},
		{
			// Regression: two channels referencing webhooks created in the same
			// apply both have an unknown webhook_id. They must not read as
			// duplicates of each other.
			"channels with unknown webhook_ids are not duplicates",
			func(m *alertResourceModel) {
				m.Channel = nullChan()
				m.Channels = chanList(
					alertChannelModel{Type: types.StringValue("webhook"), WebhookID: types.StringUnknown()},
					alertChannelModel{Type: types.StringValue("webhook"), WebhookID: types.StringUnknown()},
				)
			},
			false,
		},
		{
			"duplicate channels",
			func(m *alertResourceModel) {
				m.Channel = nullChan()
				m.Channels = chanList(webhookChannel("wh1"), webhookChannel("wh1"))
			},
			true,
		},
		{
			"channels entry without webhook_id",
			func(m *alertResourceModel) {
				m.Channel = nullChan()
				m.Channels = chanList(alertChannelModel{Type: types.StringValue("webhook"), WebhookID: types.StringNull()})
			},
			true,
		},
		{
			// Regression: a channels value taken from a module output or data
			// source is unknown as a whole until Terraform resolves it. It must
			// not fail Config.Get with a "always an error in the provider"
			// diagnostic, and no channel rule can be evaluated against it.
			"wholly unknown channels list",
			func(m *alertResourceModel) {
				m.Channel = nullChan()
				m.Channels = types.ListUnknown(alertChannelObjectType)
			},
			false,
		},
		{
			"wholly unknown deprecated channel",
			func(m *alertResourceModel) {
				m.Channel = types.ObjectUnknown(alertChannelAttrTypes)
				m.Channels = nullChanList()
			},
			false,
		},
		{
			// An attribute set to an unresolved reference is still set, so the
			// exactly-one-of rule must fire even though neither value can be read.
			"both set, channels wholly unknown",
			func(m *alertResourceModel) {
				m.Channels = types.ListUnknown(alertChannelObjectType)
			},
			true,
		},
		{
			"both set, channel wholly unknown",
			func(m *alertResourceModel) {
				m.Channel = types.ObjectUnknown(alertChannelAttrTypes)
				m.Channels = chanList(webhookChannel("wh1"))
			},
			true,
		},
		{
			"name too long",
			func(m *alertResourceModel) { m.Name = types.StringValue(string(make([]byte, 513))) },
			true,
		},
		{"null source is treated as saved_search", func(m *alertResourceModel) { m.Source = types.StringNull() }, false},
		{"unknown source skips the shape rules", func(m *alertResourceModel) { m.Source = types.StringUnknown() }, false},
		{"invalid source", func(m *alertResourceModel) { m.Source = types.StringValue("inline") }, true},
		{"saved_search without saved_search_id", func(m *alertResourceModel) { m.SavedSearchID = types.StringNull() }, true},
		{"saved_search with empty saved_search_id", func(m *alertResourceModel) { m.SavedSearchID = types.StringValue("") }, true},
		{"saved_search with dashboard_id", func(m *alertResourceModel) { m.DashboardID = types.StringValue("d1") }, true},
		{"saved_search with tile_id", func(m *alertResourceModel) { m.TileID = types.StringValue("t1") }, true},
		{"valid tile alert", asTile, false},
		{
			"tile with unknown dashboard_id is accepted at plan time",
			func(m *alertResourceModel) { asTile(m); m.DashboardID = types.StringUnknown() },
			false,
		},
		{"tile without dashboard_id", func(m *alertResourceModel) { asTile(m); m.DashboardID = types.StringNull() }, true},
		{"tile with empty dashboard_id", func(m *alertResourceModel) { asTile(m); m.DashboardID = types.StringValue("") }, true},
		{"tile without tile_id", func(m *alertResourceModel) { asTile(m); m.TileID = types.StringNull() }, true},
		{"tile with empty tile_id", func(m *alertResourceModel) { asTile(m); m.TileID = types.StringValue("") }, true},
		{"tile with saved_search_id", func(m *alertResourceModel) { asTile(m); m.SavedSearchID = types.StringValue("ss1") }, true},
		{"tile with group_by", func(m *alertResourceModel) { asTile(m); m.GroupBy = types.StringValue("svc") }, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := mkAlert(tc.mod)
			diags := m.validate(context.Background())
			if diags.HasError() != tc.wantErr {
				t.Fatalf("HasError()=%v, want %v: %s", diags.HasError(), tc.wantErr, diags)
			}
		})
	}
}

func TestAlertResource_ToClient(t *testing.T) {
	t.Parallel()

	t.Run("non-range omits threshold_max, forces channel", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(func(m *alertResourceModel) { m.ThresholdMax = types.Float64Value(999) })
		al, _ := m.toClient(context.Background())
		if al.ThresholdMax != nil {
			t.Errorf("expected threshold_max omitted for non-range type, got %v", *al.ThresholdMax)
		}
		if al.Channel.Type != "webhook" || al.Channel.WebhookID != "wh1" {
			t.Errorf("unexpected channel: %+v", al.Channel)
		}
		if al.SavedSearchID != "ss1" {
			t.Errorf("expected savedSearchId ss1, got %q", al.SavedSearchID)
		}
	})

	t.Run("range sends threshold_max", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(func(m *alertResourceModel) {
			m.ThresholdType = types.StringValue(thresholdTypeBetween)
			m.ThresholdMax = types.Float64Value(200)
		})
		al, _ := m.toClient(context.Background())
		if al.ThresholdMax == nil || *al.ThresholdMax != 200 {
			t.Errorf("expected threshold_max 200, got %v", al.ThresholdMax)
		}
	})

	t.Run("optional pointers omitted when null", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(nil)
		al, _ := m.toClient(context.Background())
		if al.GroupBy != nil || al.Name != nil || al.ScheduleStartAt != nil || al.NumConsecutiveWindows != nil {
			t.Errorf("expected nil optional pointers, got groupBy=%v name=%v startAt=%v ncw=%v",
				al.GroupBy, al.Name, al.ScheduleStartAt, al.NumConsecutiveWindows)
		}
	})

	t.Run("tile alert sends dashboard and tile ids, no saved search", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(asTile)
		al, _ := m.toClient(context.Background())
		if al.Source != client.AlertSourceTile || al.DashboardID != "d1" || al.TileID != "t1" {
			t.Errorf("tile fields not sent: %+v", al)
		}
		if al.SavedSearchID != "" {
			t.Errorf("expected empty savedSearchId for a tile alert, got %q", al.SavedSearchID)
		}
	})

	t.Run("null source sends saved_search", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(func(m *alertResourceModel) { m.Source = types.StringNull() })
		al, _ := m.toClient(context.Background())
		if al.Source != client.AlertSourceSavedSearch || al.SavedSearchID != "ss1" {
			t.Errorf("null source must mean saved_search: %+v", al)
		}
	})
}

func TestAlertResource_ApplyAlert(t *testing.T) {
	t.Parallel()

	t.Run("range reflects threshold_max and maps pointers", func(t *testing.T) {
		t.Parallel()
		max, ncw, off := 200.0, 3, 7
		gb, name := "svc", "n"
		var m alertResourceModel
		m.applyAlert(context.Background(), &client.Alert{
			ID: "al1", SavedSearchID: "ss1",
			Channel:  client.AlertChannel{Type: "webhook", WebhookID: "wh1"},
			Interval: "1h", Threshold: 100, ThresholdType: thresholdTypeBetween, ThresholdMax: &max,
			GroupBy: &gb, Name: &name, NumConsecutiveWindows: &ncw, ScheduleOffsetMinutes: &off,
		})
		if m.ThresholdMax.ValueFloat64() != 200 {
			t.Errorf("threshold_max = %v, want 200", m.ThresholdMax.ValueFloat64())
		}
		// The model had neither channel nor channels set (as on import), so the
		// response lands in channels.
		cs := readChanList(t, m.Channels)
		if m.GroupBy.ValueString() != "svc" || len(cs) != 1 || cs[0].WebhookID.ValueString() != "wh1" {
			t.Errorf("group_by/webhook_id not mapped: %+v", m)
		}
		if m.NumConsecutiveWindows.ValueInt64() != 3 || m.ScheduleOffsetMinutes.ValueInt64() != 7 {
			t.Errorf("numeric pointers not mapped: %+v", m)
		}
	})

	t.Run("non-range preserves configured threshold_max", func(t *testing.T) {
		t.Parallel()
		m := alertResourceModel{ThresholdMax: types.Float64Value(999)}
		m.applyAlert(context.Background(), &client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}})
		if m.ThresholdMax.ValueFloat64() != 999 {
			t.Errorf("non-range threshold_max should be left as configured, got %v", m.ThresholdMax.ValueFloat64())
		}
	})

	t.Run("zero offset from server maps to null", func(t *testing.T) {
		t.Parallel()
		zero := 0
		var m alertResourceModel
		m.applyAlert(context.Background(), &client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleOffsetMinutes: &zero})
		if !m.ScheduleOffsetMinutes.IsNull() {
			t.Errorf("server offset 0 must map to null, got %v", m.ScheduleOffsetMinutes.ValueInt64())
		}
	})

	t.Run("nil optionals map to null", func(t *testing.T) {
		t.Parallel()
		var m alertResourceModel
		m.applyAlert(context.Background(), &client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}})
		if !m.GroupBy.IsNull() || !m.Name.IsNull() || !m.ScheduleStartAt.IsNull() || !m.NumConsecutiveWindows.IsNull() {
			t.Errorf("nil server optionals must map to null: %+v", m)
		}
	})

	t.Run("tile response maps ids and nulls saved_search_id", func(t *testing.T) {
		t.Parallel()
		var m alertResourceModel
		m.applyAlert(context.Background(), &client.Alert{
			ID: "al2", Source: client.AlertSourceTile, DashboardID: "d1", TileID: "t1",
			ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"},
		})
		if m.Source.ValueString() != alertSourceTile || m.DashboardID.ValueString() != "d1" || m.TileID.ValueString() != "t1" {
			t.Errorf("tile fields not mapped: %+v", m)
		}
		if !m.SavedSearchID.IsNull() {
			t.Errorf("saved_search_id must be null for a tile alert, got %q", m.SavedSearchID.ValueString())
		}
	})

	t.Run("saved-search response nulls dashboard_id and tile_id", func(t *testing.T) {
		t.Parallel()
		var m alertResourceModel
		m.applyAlert(context.Background(), &client.Alert{
			ID: "al1", Source: client.AlertSourceSavedSearch, SavedSearchID: "ss1",
			ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"},
		})
		if m.Source.ValueString() != alertSourceSavedSearch || m.SavedSearchID.ValueString() != "ss1" {
			t.Errorf("saved-search fields not mapped: %+v", m)
		}
		if !m.DashboardID.IsNull() || !m.TileID.IsNull() {
			t.Errorf("dashboard_id/tile_id must be null for a saved-search alert: %+v", m)
		}
	})

	t.Run("empty source from server maps to saved_search", func(t *testing.T) {
		t.Parallel()
		var m alertResourceModel
		m.applyAlert(context.Background(), &client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}})
		if m.Source.ValueString() != alertSourceSavedSearch {
			t.Errorf("empty source must map to saved_search, got %q", m.Source.ValueString())
		}
	})
}

// TestAlertResource_ToClient_ScheduleModes guards the mutual-exclusivity fix:
// when schedule_start_at is set, toClient must NOT also emit schedule_offset_minutes
// (which the API rejects and which a sticky plan value would otherwise leak).
func TestAlertResource_ToClient_ScheduleModes(t *testing.T) {
	t.Parallel()

	t.Run("offset mode sends offset, no start_at", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(func(m *alertResourceModel) { m.ScheduleOffsetMinutes = types.Int64Value(5) })
		al, _ := m.toClient(context.Background())
		if al.ScheduleOffsetMinutes == nil || *al.ScheduleOffsetMinutes != 5 {
			t.Errorf("expected offset 5, got %v", al.ScheduleOffsetMinutes)
		}
		if al.ScheduleStartAt != nil {
			t.Errorf("expected start_at nil (sent as null), got %q", *al.ScheduleStartAt)
		}
	})

	t.Run("start_at mode omits offset even when a stale offset is present", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(func(m *alertResourceModel) {
			m.ScheduleStartAt = types.StringValue("2026-01-01T00:00:00Z")
			m.ScheduleOffsetMinutes = types.Int64Value(5) // simulates a leftover value
		})
		al, _ := m.toClient(context.Background())
		if al.ScheduleOffsetMinutes != nil {
			t.Errorf("offset must be omitted when schedule_start_at is set, got %v", *al.ScheduleOffsetMinutes)
		}
		if al.ScheduleStartAt == nil || *al.ScheduleStartAt != "2026-01-01T00:00:00Z" {
			t.Errorf("expected start_at sent, got %v", al.ScheduleStartAt)
		}
	})
}

func TestAlertResource_ApplyAlert_OffsetZero(t *testing.T) {
	t.Parallel()

	// Explicit config offset 0 must round-trip (not be nulled) — otherwise apply
	// reports "inconsistent result after apply".
	explicit := alertResourceModel{ScheduleOffsetMinutes: types.Int64Value(0)}
	zero := 0
	explicit.applyAlert(context.Background(), &client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleOffsetMinutes: &zero})
	if explicit.ScheduleOffsetMinutes.IsNull() || explicit.ScheduleOffsetMinutes.ValueInt64() != 0 {
		t.Errorf("explicit offset 0 must be preserved, got %v", explicit.ScheduleOffsetMinutes)
	}

	// A server-forced 0 with no configured offset stays null (no spurious diff).
	forced := alertResourceModel{ScheduleOffsetMinutes: types.Int64Null()}
	forced.applyAlert(context.Background(), &client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleOffsetMinutes: &zero})
	if !forced.ScheduleOffsetMinutes.IsNull() {
		t.Errorf("server-forced 0 with null config must stay null, got %v", forced.ScheduleOffsetMinutes.ValueInt64())
	}

	// Explicit config 0 must round-trip even if the server OMITS the offset field
	// (returns nil) — so correctness does not depend on the server echoing 0.
	omitted := alertResourceModel{ScheduleOffsetMinutes: types.Int64Value(0)}
	omitted.applyAlert(context.Background(), &client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleOffsetMinutes: nil})
	if omitted.ScheduleOffsetMinutes.IsNull() || omitted.ScheduleOffsetMinutes.ValueInt64() != 0 {
		t.Errorf("explicit config 0 must be preserved when server omits offset, got %v", omitted.ScheduleOffsetMinutes)
	}
}

func alertSchema(t *testing.T) rschema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	(&alertResource{}).Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %s", resp.Diagnostics)
	}
	return resp.Schema
}

func TestAlertResource_CRUD(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sch := alertSchema(t)

	t.Run("create maps server id into state", func(t *testing.T) {
		t.Parallel()
		r := &alertResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"data":{"id":"al1","source":"saved_search","savedSearchId":"ss1","interval":"5m","threshold":100,"thresholdType":"above","channel":{"type":"webhook","webhookId":"wh1"}}}`)
		}))}
		plan := tfsdk.Plan{Schema: sch}
		if d := plan.Set(ctx, mkAlert(nil)); d.HasError() {
			t.Fatalf("plan.Set: %s", d)
		}
		resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: sch}}
		r.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Create: %s", resp.Diagnostics)
		}
		var got alertResourceModel
		resp.State.Get(ctx, &got)
		if got.ID.ValueString() != "al1" {
			t.Errorf("id=%q, want al1", got.ID.ValueString())
		}
	})

	t.Run("create tile alert maps ids into state", func(t *testing.T) {
		t.Parallel()
		r := &alertResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"data":{"id":"al2","source":"tile","dashboardId":"d1","tileId":"t1","interval":"5m","threshold":100,"thresholdType":"above","channel":{"type":"webhook","webhookId":"wh1"}}}`)
		}))}
		plan := tfsdk.Plan{Schema: sch}
		if d := plan.Set(ctx, mkAlert(asTile)); d.HasError() {
			t.Fatalf("plan.Set: %s", d)
		}
		resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: sch}}
		r.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Create: %s", resp.Diagnostics)
		}
		var got alertResourceModel
		resp.State.Get(ctx, &got)
		if got.ID.ValueString() != "al2" || got.Source.ValueString() != alertSourceTile ||
			got.DashboardID.ValueString() != "d1" || got.TileID.ValueString() != "t1" || !got.SavedSearchID.IsNull() {
			t.Errorf("tile alert state = %+v", got)
		}
	})

	t.Run("read rejects an alert source the provider does not model", func(t *testing.T) {
		t.Parallel()
		r := &alertResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"data":{"id":"al3","source":"inline","interval":"5m","threshold":100,"thresholdType":"above","channel":{"type":"webhook","webhookId":"wh1"}}}`)
		}))}
		state := tfsdk.State{Schema: sch}
		m := mkAlert(func(m *alertResourceModel) { m.ID = types.StringValue("al3") })
		if d := state.Set(ctx, m); d.HasError() {
			t.Fatalf("state.Set: %s", d)
		}
		resp := &fwresource.ReadResponse{State: state}
		r.Read(ctx, fwresource.ReadRequest{State: state}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error for source inline, got none")
		}
		// State is left as it was: the default would otherwise plan a replacement.
		var got alertResourceModel
		resp.State.Get(ctx, &got)
		if got.Source.ValueString() != alertSourceSavedSearch {
			t.Errorf("state source = %q, want the prior %q left untouched", got.Source.ValueString(), alertSourceSavedSearch)
		}
	})

	t.Run("read removes resource on cascade-delete 404", func(t *testing.T) {
		t.Parallel()
		r := &alertResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))}
		state := tfsdk.State{Schema: sch}
		m := mkAlert(func(m *alertResourceModel) { m.ID = types.StringValue("al1") })
		if d := state.Set(ctx, m); d.HasError() {
			t.Fatalf("state.Set: %s", d)
		}
		resp := &fwresource.ReadResponse{State: state}
		r.Read(ctx, fwresource.ReadRequest{State: state}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Read: %s", resp.Diagnostics)
		}
		if !resp.State.Raw.IsNull() {
			t.Error("expected resource removed from state on 404")
		}
	})

	t.Run("update maps server response into state", func(t *testing.T) {
		t.Parallel()
		r := &alertResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"data":{"id":"al1","source":"saved_search","savedSearchId":"ss1","interval":"5m","threshold":250,"thresholdType":"above","channel":{"type":"webhook","webhookId":"wh1"}}}`)
		}))}
		plan := tfsdk.Plan{Schema: sch}
		if d := plan.Set(ctx, mkAlert(func(m *alertResourceModel) {
			m.ID = types.StringValue("al1")
			m.Threshold = types.Float64Value(250)
		})); d.HasError() {
			t.Fatalf("plan.Set: %s", d)
		}
		resp := &fwresource.UpdateResponse{State: tfsdk.State{Schema: sch}}
		r.Update(ctx, fwresource.UpdateRequest{Plan: plan}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Update: %s", resp.Diagnostics)
		}
		var got alertResourceModel
		resp.State.Get(ctx, &got)
		if got.Threshold.ValueFloat64() != 250 {
			t.Errorf("threshold=%v, want 250", got.Threshold.ValueFloat64())
		}
	})

	// Update must not clear state on a 404: the framework fails the apply with
	// "Missing Resource State After Update" rather than accepting the removal.
	// Prior state has to survive so the next refresh converges through Read.
	// Both sources are covered: the tile flavor is what the server cascade-deletes,
	// the saved-search flavor is what an out-of-band delete hits.
	for _, tc := range []struct {
		name string
		mods func(*alertResourceModel)
	}{
		{"saved_search", nil},
		{"tile", asTile},
	} {
		t.Run("update errors on 404 and keeps state ("+tc.name+")", func(t *testing.T) {
			t.Parallel()
			r := &alertResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))}
			mk := func(threshold float64) alertResourceModel {
				return mkAlert(func(m *alertResourceModel) {
					if tc.mods != nil {
						tc.mods(m)
					}
					m.ID = types.StringValue("al1")
					m.Threshold = types.Float64Value(threshold)
				})
			}
			state := tfsdk.State{Schema: sch}
			if d := state.Set(ctx, mk(100)); d.HasError() {
				t.Fatalf("state.Set: %s", d)
			}
			// A different planned threshold makes a stray state write detectable.
			plan := tfsdk.Plan{Schema: sch}
			if d := plan.Set(ctx, mk(250)); d.HasError() {
				t.Fatalf("plan.Set: %s", d)
			}
			resp := &fwresource.UpdateResponse{State: state}
			r.Update(ctx, fwresource.UpdateRequest{Plan: plan}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected an error when update hits 404, got none")
			}
			if got := resp.Diagnostics.Errors()[0].Summary(); got != "Alert No Longer Exists" {
				t.Errorf("error summary = %q, want %q", got, "Alert No Longer Exists")
			}
			var got alertResourceModel
			resp.State.Get(ctx, &got)
			if got.Threshold.ValueFloat64() != 100 {
				t.Errorf("threshold=%v, want the prior 100 left untouched", got.Threshold.ValueFloat64())
			}
		})
	}

	t.Run("delete treats 404 as a no-op", func(t *testing.T) {
		t.Parallel()
		r := &alertResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))}
		state := tfsdk.State{Schema: sch}
		m := mkAlert(func(m *alertResourceModel) { m.ID = types.StringValue("al1") })
		if d := state.Set(ctx, m); d.HasError() {
			t.Fatalf("state.Set: %s", d)
		}
		resp := &fwresource.DeleteResponse{State: state}
		r.Delete(ctx, fwresource.DeleteRequest{State: state}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("expected 404 delete to be a no-op, got %s", resp.Diagnostics)
		}
	})
}

func TestAlertResource_ApplyAlert_ScheduleStartAtCanonicalization(t *testing.T) {
	t.Parallel()

	// Server canonicalizes the timestamp (adds milliseconds); the authored value
	// denotes the same instant, so it must be kept to avoid an inconsistent result.
	authored := "2026-01-01T00:00:00Z"
	server := "2026-01-01T00:00:00.000Z"
	m := alertResourceModel{ScheduleStartAt: types.StringValue(authored)}
	m.applyAlert(context.Background(), &client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleStartAt: &server})
	if m.ScheduleStartAt.ValueString() != authored {
		t.Errorf("expected authored timestamp kept on canonicalization, got %q", m.ScheduleStartAt.ValueString())
	}

	// A genuinely different instant is adopted from the server.
	other := "2027-06-15T12:00:00Z"
	m2 := alertResourceModel{ScheduleStartAt: types.StringValue(authored)}
	m2.applyAlert(context.Background(), &client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleStartAt: &other})
	if m2.ScheduleStartAt.ValueString() != other {
		t.Errorf("expected differing server instant adopted, got %q", m2.ScheduleStartAt.ValueString())
	}
}

// A config using one of channel/channels must never get the other one back, or
// it would show a permanent diff.
func TestAlertResource_ChannelsRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("channels config sends and reads back the whole list", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(func(m *alertResourceModel) {
			m.Channel = nullChan()
			m.Channels = chanList(webhookChannel("wh1"), webhookChannel("wh2"))
		})
		al, _ := m.toClient(context.Background())
		if len(al.Channels) != 2 || al.Channels[1].WebhookID != "wh2" {
			t.Fatalf("channels not sent: %+v", al.Channels)
		}

		m.applyAlert(context.Background(), &client.Alert{
			ThresholdType: thresholdTypeAbove,
			Channel:       client.AlertChannel{Type: "webhook", WebhookID: "wh1"},
			Channels: []client.AlertChannel{
				{Type: "webhook", WebhookID: "wh1"}, {Type: "webhook", WebhookID: "wh2"},
			},
		})
		if !m.Channel.IsNull() {
			t.Errorf("channel must stay null for a channels config, got %+v", m.Channel)
		}
		if cs := readChanList(t, m.Channels); len(cs) != 2 || cs[1].WebhookID.ValueString() != "wh2" {
			t.Errorf("channels not read back: %+v", m.Channels)
		}
	})

	t.Run("deprecated channel config keeps channels null", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(nil)
		al, _ := m.toClient(context.Background())
		if al.Channels != nil || al.Channel.WebhookID != "wh1" {
			t.Fatalf("expected channel only, got channel=%+v channels=%+v", al.Channel, al.Channels)
		}

		// The server echoes both fields even for a single-channel alert.
		m.applyAlert(context.Background(), &client.Alert{
			ThresholdType: thresholdTypeAbove,
			Channel:       client.AlertChannel{Type: "webhook", WebhookID: "wh1"},
			Channels:      []client.AlertChannel{{Type: "webhook", WebhookID: "wh1"}},
		})
		if !m.Channels.IsNull() {
			t.Errorf("channels must stay null for a channel config, got %+v", m.Channels)
		}
		if readChan(t, m.Channel).WebhookID.ValueString() != "wh1" {
			t.Errorf("channel not read back: %+v", m.Channel)
		}
	})

	// A deployment that predates multi-channel returns only `channel`.
	t.Run("response without channels falls back to channel", func(t *testing.T) {
		t.Parallel()
		var m alertResourceModel
		m.applyAlert(context.Background(), &client.Alert{
			ThresholdType: thresholdTypeAbove,
			Channel:       client.AlertChannel{Type: "webhook", WebhookID: "wh1"},
		})
		if cs := readChanList(t, m.Channels); len(cs) != 1 || cs[0].WebhookID.ValueString() != "wh1" {
			t.Errorf("expected channel mirrored into channels, got %+v", m.Channels)
		}
	})
}
