package clickstack

import (
	"context"
	"io"
	"net/http"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

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
}

// mkAlert builds a valid saved-search alert model; mods tweaks it per case.
func mkAlert(mods func(*alertResourceModel)) alertResourceModel {
	m := alertResourceModel{
		SavedSearchID:         types.StringValue("ss1"),
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
