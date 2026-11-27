package resource

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

// Exact notes from control-plane's CustomerSettingsAllowlist.ts.
const (
	rolloutRestartNote = "Applying this setting triggers a ClickHouse server rollout restart."
	shortDelayNote     = "Changes to this setting take effect after a short delay while they propagate to all replicas."
)

const keepAliveWarning = rolloutRestartNote + " Lower values close idle connections sooner."

var testSettingsSchema = []api.ServiceClickhouseSettingSchemaEntry{
	{Name: "compatibility", Warning: rolloutRestartNote},
	{Name: "keep_alive_timeout", Warning: keepAliveWarning},
	{Name: "max_query_size", Warning: shortDelayNote},
	{Name: "max_threads"},
}

const (
	restartSummary      = "This change restarts the ClickHouse service"
	maybeRestartSummary = "This change may restart the ClickHouse service"
	uncheckedSummary    = "Could not check whether this change restarts the ClickHouse service"
)

func TestServiceClickhouseSettingsResource_ModifyPlan(t *testing.T) {
	ctx := context.Background()
	_, sch := clickhouseSettingsResourceWithSchema(t)
	str := func(v string) tftypes.Value { return tftypes.NewValue(tftypes.String, v) }
	unknown := tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	unknownMap := tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, tftypes.UnknownValue)
	settings := func(serviceID tftypes.Value, s map[string]tftypes.Value) tftypes.Value {
		return clickhouseSettingsValue(sch, serviceID, tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, s))
	}
	null := tftypes.NewValue(sch.Type().TerraformType(ctx), nil)

	tests := []struct {
		name string
		// schemaCalls lists the service IDs whose schema the plan must read, once each; nil means no call.
		schemaCalls []string
		schemaErr   error
		state       tftypes.Value
		plan        tftypes.Value
		// wantSummaries and wantDetails hold one entry per expected warning, in service ID order.
		wantSummaries []string
		// wantDetails holds the substrings each warning's detail must contain.
		wantDetails [][]string
		// wantAbsent lists substrings that must not appear in any diagnostic.
		wantAbsent []string
	}{
		{
			name:          "changing a restart setting warns with the API message",
			schemaCalls:   []string{"svc-1"},
			state:         settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.1")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.2")}),
			wantSummaries: []string{restartSummary},
			wantDetails:   [][]string{{"Applying this plan restarts service svc-1.", `"compatibility": ` + rolloutRestartNote}},
		},
		{
			name:          "removing a restart setting warns",
			schemaCalls:   []string{"svc-1"},
			state:         settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.1"), "max_threads": str("4")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"max_threads": str("4")}),
			wantSummaries: []string{restartSummary},
			wantDetails:   [][]string{{`"compatibility"`}},
		},
		{
			name:          "two restart settings on one service share one warning, sorted by name",
			schemaCalls:   []string{"svc-1"},
			state:         settings(str("svc-1"), map[string]tftypes.Value{"keep_alive_timeout": str("10"), "max_query_size": str("1")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.2"), "max_query_size": str("2")}),
			wantSummaries: []string{restartSummary},
			wantDetails: [][]string{{
				`"compatibility": ` + rolloutRestartNote + "\n" + `"keep_alive_timeout": ` + keepAliveWarning,
			}},
			wantAbsent: []string{"max_query_size"},
		},
		{
			name:          "a mix of restart and short-delay settings names only the restart ones",
			schemaCalls:   []string{"svc-1"},
			state:         settings(str("svc-1"), map[string]tftypes.Value{"keep_alive_timeout": str("10"), "max_query_size": str("1")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"keep_alive_timeout": str("30"), "max_query_size": str("2"), "max_threads": str("4")}),
			wantSummaries: []string{restartSummary},
			wantDetails:   [][]string{{`"keep_alive_timeout": ` + keepAliveWarning}},
			wantAbsent:    []string{"max_query_size", "max_threads"},
		},
		{
			name:        "only short-delay settings change: no warning",
			schemaCalls: []string{"svc-1"},
			state:       settings(str("svc-1"), map[string]tftypes.Value{"max_query_size": str("1")}),
			plan:        settings(str("svc-1"), map[string]tftypes.Value{"max_query_size": str("2")}),
		},
		{
			name:          "a touched setting the schema doesn't list is skipped",
			schemaCalls:   []string{"svc-1"},
			state:         settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.1"), "a_removed_setting": str("1")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.2")}),
			wantSummaries: []string{restartSummary},
			wantDetails:   [][]string{{`"compatibility"`}},
			wantAbsent:    []string{"a_removed_setting"},
		},
		{
			name:  "no setting changes: no schema call",
			state: settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.1"), "max_threads": str("4")}),
			plan:  settings(str("svc-1"), map[string]tftypes.Value{"max_threads": str("4"), "compatibility": str("26.1")}),
		},
		{
			name:          "a value unknown until apply counts as a possible change",
			schemaCalls:   []string{"svc-1"},
			state:         settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.1"), "max_threads": str("4")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"compatibility": unknown, "max_threads": str("4")}),
			wantSummaries: []string{maybeRestartSummary},
			wantDetails: [][]string{{
				"Applying this plan may restart service svc-1.",
				`"compatibility" (new value not known until apply; restarts only if it changes): ` + rolloutRestartNote,
			}},
			wantAbsent: []string{"max_threads"},
		},
		{
			name:          "a known restart change next to an unknown one keeps the definite wording",
			schemaCalls:   []string{"svc-1"},
			state:         settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.1"), "keep_alive_timeout": str("10")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.2"), "keep_alive_timeout": unknown}),
			wantSummaries: []string{restartSummary},
			wantDetails:   [][]string{{"Applying this plan restarts service svc-1.", `"keep_alive_timeout" (new value not known until apply`}},
		},
		{
			name:        "an unknown value on a setting that doesn't restart: no warning",
			schemaCalls: []string{"svc-1"},
			state:       settings(str("svc-1"), map[string]tftypes.Value{"max_query_size": str("1")}),
			plan:        settings(str("svc-1"), map[string]tftypes.Value{"max_query_size": unknown}),
		},
		{
			name:          "a removed setting and a changed one are listed together, sorted by name",
			schemaCalls:   []string{"svc-1"},
			state:         settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.1"), "keep_alive_timeout": str("10")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"keep_alive_timeout": str("30")}),
			wantSummaries: []string{restartSummary},
			wantDetails: [][]string{{
				`"compatibility": ` + rolloutRestartNote + "\n" + `"keep_alive_timeout": ` + keepAliveWarning,
			}},
		},
		{
			name:          "an update whose settings map is unknown until apply says it cannot check",
			state:         settings(str("svc-1"), map[string]tftypes.Value{"max_query_size": str("1")}),
			plan:          clickhouseSettingsValue(sch, str("svc-1"), unknownMap),
			wantSummaries: []string{uncheckedSummary},
			wantDetails:   [][]string{{"svc-1", "not known until apply"}},
		},
		{
			name:          "create on an existing service warns",
			schemaCalls:   []string{"svc-1"},
			state:         null,
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.2"), "max_threads": str("4")}),
			wantSummaries: []string{restartSummary},
			wantDetails:   [][]string{{`"compatibility"`}},
			wantAbsent:    []string{"max_threads"},
		},
		{
			name:          "create with a settings map unknown until apply says it cannot check",
			state:         null,
			plan:          clickhouseSettingsValue(sch, str("svc-1"), unknownMap),
			wantSummaries: []string{uncheckedSummary},
			wantDetails:   [][]string{{"svc-1", "not known until apply"}},
		},
		{
			name:          "create with a value unknown until apply may restart",
			schemaCalls:   []string{"svc-1"},
			state:         null,
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"compatibility": unknown}),
			wantSummaries: []string{maybeRestartSummary},
			wantDetails:   [][]string{{"may restart service svc-1", `"compatibility" (new value not known until apply`}},
		},
		{
			name:  "create on a service made in the same apply: no schema call",
			state: null,
			plan:  settings(unknown, map[string]tftypes.Value{"compatibility": str("26.2")}),
		},
		{
			name:          "destroy warns for the settings in state and notes the service may be deleted too",
			schemaCalls:   []string{"svc-1"},
			state:         settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.2")}),
			plan:          null,
			wantSummaries: []string{restartSummary},
			wantDetails:   [][]string{{"resets these settings on service svc-1, which restarts it.", "If this apply also deletes the service", `"compatibility"`}},
		},
		{
			// svc-new sorts before svc-old, which fixes the warning order.
			name:          "replace warns for both the old and the new service",
			schemaCalls:   []string{"svc-new", "svc-old"},
			state:         settings(str("svc-old"), map[string]tftypes.Value{"compatibility": str("26.2")}),
			plan:          settings(str("svc-new"), map[string]tftypes.Value{"keep_alive_timeout": str("30")}),
			wantSummaries: []string{restartSummary, restartSummary},
			wantDetails:   [][]string{{"restarts service svc-new", `"keep_alive_timeout"`}, {"resets these settings on service svc-old", `"compatibility"`}},
		},
		{
			name:          "replace onto a service made in the same apply warns only for the old service",
			schemaCalls:   []string{"svc-old"},
			state:         settings(str("svc-old"), map[string]tftypes.Value{"compatibility": str("26.2")}),
			plan:          settings(unknown, map[string]tftypes.Value{"compatibility": str("26.2")}),
			wantSummaries: []string{restartSummary},
			wantDetails:   [][]string{{"svc-old", `"compatibility"`}},
		},
		{
			name:          "replace with a settings map unknown until apply checks the old service and says it cannot check the new one",
			schemaCalls:   []string{"svc-old"},
			state:         settings(str("svc-old"), map[string]tftypes.Value{"compatibility": str("26.2")}),
			plan:          clickhouseSettingsValue(sch, str("svc-new"), unknownMap),
			wantSummaries: []string{uncheckedSummary, restartSummary},
			wantDetails:   [][]string{{"svc-new", "not known until apply"}, {"resets these settings on service svc-old", `"compatibility"`}},
		},
		{
			name:          "a schema lookup that runs past the deadline says so",
			schemaCalls:   []string{"svc-1"},
			schemaErr:     context.DeadlineExceeded,
			state:         settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.1")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.2")}),
			wantSummaries: []string{uncheckedSummary},
			wantDetails:   [][]string{{"did not load within 30s"}},
		},
		{
			name:          "a schema read error warns and does not fail the plan",
			schemaCalls:   []string{"svc-1"},
			schemaErr:     errors.New("status: 500"),
			state:         settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.1")}),
			plan:          settings(str("svc-1"), map[string]tftypes.Value{"compatibility": str("26.2")}),
			wantSummaries: []string{uncheckedSummary},
			wantDetails:   [][]string{{"status: 500"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.wantDetails) != len(tt.wantSummaries) {
				t.Fatalf("bad test row: %d summaries but %d detail sets", len(tt.wantSummaries), len(tt.wantDetails))
			}
			r, _ := clickhouseSettingsResourceWithSchema(t)
			mc := minimock.NewController(t)
			mock := api.NewClientMock(mc)
			for _, id := range tt.schemaCalls {
				mock.GetServiceClickhouseSettingsSchemaMock.When(minimock.AnyContext, id).Then(testSettingsSchema, tt.schemaErr)
			}
			if len(tt.schemaCalls) > 0 {
				mock.GetServiceClickhouseSettingsSchemaMock.Times(uint64(len(tt.schemaCalls)))
				mock.GetServiceClickhouseSettingsSchemaMock.Inspect(func(ctx context.Context, _ string) {
					deadline, ok := ctx.Deadline()
					if !ok {
						t.Error("schema lookup has no deadline")
						return
					}
					if left := time.Until(deadline); left > settingsSchemaTimeout || left < settingsSchemaTimeout-5*time.Second {
						t.Errorf("schema lookup deadline is %v away; want about %v", left, settingsSchemaTimeout)
					}
				})
			}
			r.client = mock

			resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: sch, Raw: tt.plan}}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{
				Plan:  tfsdk.Plan{Schema: sch, Raw: tt.plan},
				State: tfsdk.State{Schema: sch, Raw: tt.state},
			}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan errors: %v", resp.Diagnostics)
			}
			if len(resp.Diagnostics) != len(tt.wantSummaries) {
				t.Fatalf("got %d diagnostics, want %d: %v", len(resp.Diagnostics), len(tt.wantSummaries), resp.Diagnostics)
			}
			for i, d := range resp.Diagnostics {
				if d.Summary() != tt.wantSummaries[i] {
					t.Errorf("diagnostic %d summary = %q; want %q", i, d.Summary(), tt.wantSummaries[i])
				}
				for _, want := range tt.wantDetails[i] {
					if !strings.Contains(d.Detail(), want) {
						t.Errorf("diagnostic %d detail = %q; want it to contain %q", i, d.Detail(), want)
					}
				}
				for _, absent := range tt.wantAbsent {
					if strings.Contains(d.Detail(), absent) {
						t.Errorf("diagnostic %d detail = %q; want no mention of %q", i, d.Detail(), absent)
					}
				}
			}
		})
	}
}

// A plan that is being cancelled adds no "could not check" warning.
func TestServiceClickhouseSettingsResource_ModifyPlan_CancelledPlan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, sch := clickhouseSettingsResourceWithSchema(t)
	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).GetServiceClickhouseSettingsSchemaMock.Return(nil, context.Canceled)

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.1"})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2"})
	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: sch, Raw: plan}}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: sch, Raw: plan},
		State: tfsdk.State{Schema: sch, Raw: state},
	}, resp)

	if len(resp.Diagnostics) != 0 {
		t.Errorf("diagnostics = %v; want none", resp.Diagnostics)
	}
}

// Validate-only runs have no configured client; the plan must not panic or call the API.
func TestServiceClickhouseSettingsResource_ModifyPlan_NoClient(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2"})

	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: sch, Raw: plan}}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{
		Plan:  tfsdk.Plan{Schema: sch, Raw: plan},
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}, resp)

	if len(resp.Diagnostics) != 0 {
		t.Errorf("diagnostics = %v; want none", resp.Diagnostics)
	}
}
