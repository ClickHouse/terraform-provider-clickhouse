package clickstack

import (
	"context"
	"io"
	"net/http"
	"testing"

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
	for _, attr := range []string{"id", "team", "saved_search_id", "channel", "threshold", "threshold_type", "interval"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
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

// mkAlert builds a valid saved-search alert model; mods tweaks it per case.
func mkAlert(mods func(*alertResourceModel)) alertResourceModel {
	m := alertResourceModel{
		Source:                types.StringValue(alertSourceSavedSearch),
		SavedSearchID:         types.StringValue("ss1"),
		DashboardID:           types.StringNull(),
		TileID:                types.StringNull(),
		GroupBy:               types.StringNull(),
		Channel:               &alertChannelModel{Type: types.StringValue("webhook"), WebhookID: types.StringValue("wh1")},
		Threshold:             types.Float64Value(100),
		ThresholdType:         types.StringValue(thresholdTypeAbove),
		ThresholdMax:          types.Float64Null(),
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

// TestAlertResource_TargetRequiresReplace: an unknown planned tile_id or
// dashboard_id must not replace the alert. tile_ids goes unknown whenever the
// dashboard body is unknown at plan, and replacing on that would destroy every
// tile alert on the dashboard.
func TestAlertResource_TargetRequiresReplace(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	exists := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})

	cases := []struct {
		name  string
		state types.String
		plan  types.String
		want  bool
	}{
		{"different known id replaces", types.StringValue("t1"), types.StringValue("t2"), true},
		{"same id does not replace", types.StringValue("t1"), types.StringValue("t1"), false},
		{"unknown planned id does not replace", types.StringValue("t1"), types.StringUnknown(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, attrName := range []string{tileIDAttr, dashboardIDAttr} {
				req := planmodifier.StringRequest{
					Path:       path.Root(attrName),
					StateValue: tc.state,
					PlanValue:  tc.plan,
					State:      tfsdk.State{Raw: exists},
					Plan:       tfsdk.Plan{Raw: exists},
				}
				resp := &planmodifier.StringResponse{PlanValue: tc.plan}
				targetRequiresReplace(attrName).PlanModifyString(ctx, req, resp)
				if resp.RequiresReplace != tc.want {
					t.Errorf("%s: RequiresReplace=%v, want %v", attrName, resp.RequiresReplace, tc.want)
				}
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
			func(m *alertResourceModel) { m.Channel.WebhookID = types.StringNull() },
			true,
		},
		{
			"invalid channel type",
			func(m *alertResourceModel) { m.Channel.Type = types.StringValue("email") },
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
			diags := m.validate()
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
		al := m.toClient()
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
		al := m.toClient()
		if al.ThresholdMax == nil || *al.ThresholdMax != 200 {
			t.Errorf("expected threshold_max 200, got %v", al.ThresholdMax)
		}
	})

	t.Run("optional pointers omitted when null", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(nil)
		al := m.toClient()
		if al.GroupBy != nil || al.Name != nil || al.ScheduleStartAt != nil || al.NumConsecutiveWindows != nil {
			t.Errorf("expected nil optional pointers, got groupBy=%v name=%v startAt=%v ncw=%v",
				al.GroupBy, al.Name, al.ScheduleStartAt, al.NumConsecutiveWindows)
		}
	})

	t.Run("tile alert sends dashboard and tile ids, no saved search", func(t *testing.T) {
		t.Parallel()
		m := mkAlert(asTile)
		al := m.toClient()
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
		al := m.toClient()
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
		m.applyAlert(&client.Alert{
			ID: "al1", SavedSearchID: "ss1",
			Channel:  client.AlertChannel{Type: "webhook", WebhookID: "wh1"},
			Interval: "1h", Threshold: 100, ThresholdType: thresholdTypeBetween, ThresholdMax: &max,
			GroupBy: &gb, Name: &name, NumConsecutiveWindows: &ncw, ScheduleOffsetMinutes: &off,
		})
		if m.ThresholdMax.ValueFloat64() != 200 {
			t.Errorf("threshold_max = %v, want 200", m.ThresholdMax.ValueFloat64())
		}
		if m.GroupBy.ValueString() != "svc" || m.Channel.WebhookID.ValueString() != "wh1" {
			t.Errorf("group_by/webhook_id not mapped: %+v", m)
		}
		if m.NumConsecutiveWindows.ValueInt64() != 3 || m.ScheduleOffsetMinutes.ValueInt64() != 7 {
			t.Errorf("numeric pointers not mapped: %+v", m)
		}
	})

	t.Run("non-range preserves configured threshold_max", func(t *testing.T) {
		t.Parallel()
		m := alertResourceModel{ThresholdMax: types.Float64Value(999)}
		m.applyAlert(&client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}})
		if m.ThresholdMax.ValueFloat64() != 999 {
			t.Errorf("non-range threshold_max should be left as configured, got %v", m.ThresholdMax.ValueFloat64())
		}
	})

	t.Run("zero offset from server maps to null", func(t *testing.T) {
		t.Parallel()
		zero := 0
		var m alertResourceModel
		m.applyAlert(&client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleOffsetMinutes: &zero})
		if !m.ScheduleOffsetMinutes.IsNull() {
			t.Errorf("server offset 0 must map to null, got %v", m.ScheduleOffsetMinutes.ValueInt64())
		}
	})

	t.Run("nil optionals map to null", func(t *testing.T) {
		t.Parallel()
		var m alertResourceModel
		m.applyAlert(&client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}})
		if !m.GroupBy.IsNull() || !m.Name.IsNull() || !m.ScheduleStartAt.IsNull() || !m.NumConsecutiveWindows.IsNull() {
			t.Errorf("nil server optionals must map to null: %+v", m)
		}
	})

	t.Run("tile response maps ids and nulls saved_search_id", func(t *testing.T) {
		t.Parallel()
		var m alertResourceModel
		m.applyAlert(&client.Alert{
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
		m.applyAlert(&client.Alert{
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
		m.applyAlert(&client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}})
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
		al := m.toClient()
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
		al := m.toClient()
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
	explicit.applyAlert(&client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleOffsetMinutes: &zero})
	if explicit.ScheduleOffsetMinutes.IsNull() || explicit.ScheduleOffsetMinutes.ValueInt64() != 0 {
		t.Errorf("explicit offset 0 must be preserved, got %v", explicit.ScheduleOffsetMinutes)
	}

	// A server-forced 0 with no configured offset stays null (no spurious diff).
	forced := alertResourceModel{ScheduleOffsetMinutes: types.Int64Null()}
	forced.applyAlert(&client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleOffsetMinutes: &zero})
	if !forced.ScheduleOffsetMinutes.IsNull() {
		t.Errorf("server-forced 0 with null config must stay null, got %v", forced.ScheduleOffsetMinutes.ValueInt64())
	}

	// Explicit config 0 must round-trip even if the server OMITS the offset field
	// (returns nil) — so correctness does not depend on the server echoing 0.
	omitted := alertResourceModel{ScheduleOffsetMinutes: types.Int64Value(0)}
	omitted.applyAlert(&client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleOffsetMinutes: nil})
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

	t.Run("update removes resource on 404", func(t *testing.T) {
		t.Parallel()
		r := &alertResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))}
		plan := tfsdk.Plan{Schema: sch}
		if d := plan.Set(ctx, mkAlert(func(m *alertResourceModel) { m.ID = types.StringValue("al1") })); d.HasError() {
			t.Fatalf("plan.Set: %s", d)
		}
		resp := &fwresource.UpdateResponse{State: tfsdk.State{Schema: sch}}
		r.Update(ctx, fwresource.UpdateRequest{Plan: plan}, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("Update: %s", resp.Diagnostics)
		}
		if !resp.State.Raw.IsNull() {
			t.Error("expected resource removed from state when update hits 404")
		}
	})

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
	m.applyAlert(&client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleStartAt: &server})
	if m.ScheduleStartAt.ValueString() != authored {
		t.Errorf("expected authored timestamp kept on canonicalization, got %q", m.ScheduleStartAt.ValueString())
	}

	// A genuinely different instant is adopted from the server.
	other := "2027-06-15T12:00:00Z"
	m2 := alertResourceModel{ScheduleStartAt: types.StringValue(authored)}
	m2.applyAlert(&client.Alert{ThresholdType: thresholdTypeAbove, Channel: client.AlertChannel{Type: "webhook"}, ScheduleStartAt: &other})
	if m2.ScheduleStartAt.ValueString() != other {
		t.Errorf("expected differing server instant adopted, got %q", m2.ScheduleStartAt.ValueString())
	}
}
