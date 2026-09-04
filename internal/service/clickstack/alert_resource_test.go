package clickstack

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

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
		SavedSearchID:         types.StringValue("ss1"),
		GroupBy:               types.StringNull(),
		Channel:               chanObj(webhookChannel("wh1")),
		Channels:              nullChanList(),
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
