package resource

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

func TestApplyScheduleToState_PopulatesEntriesAndBaseConfig(t *testing.T) {
	schedule := &api.AutoScalingSchedule{
		Entries: []api.AutoScalingScheduleEntry{
			{
				Name:         "business",
				Weekdays:     []int{1, 2, 3, 4, 5},
				StartHourUtc: 8,
				EndHourUtc:   18,
				MinReplicas:  intPtr(3),
				MaxReplicas:  intPtr(3),
				IdleScaling:  boolPtr(false),
			},
		},
		BaseConfig: &api.AutoScalingScheduleBaseConfig{
			MinReplicaMemoryGb: intPtr(8),
			MaxReplicaMemoryGb: intPtr(32),
			IdleScaling:        boolPtr(true),
			IdleTimeoutMinutes: intPtr(5),
		},
	}

	state := &models.ServiceScheduledScalingResourceModel{
		ServiceID: types.StringValue("svc-1"),
	}

	diags := applyScheduleToState(schedule, state)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}

	if state.Entries.IsNull() {
		t.Fatalf("Entries should not be null")
	}
	if state.BaseConfig.IsNull() {
		t.Errorf("BaseConfig should not be null when API returns a base config")
	}

	var entries []models.ScheduledScalingEntryModel
	diags = state.Entries.ElementsAs(context.Background(), &entries, false)
	if diags.HasError() {
		t.Fatalf("ElementsAs: %v", diags)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d; want 1", len(entries))
	}
	e := entries[0]
	if e.MinReplicas.ValueInt64() != 3 || e.MaxReplicas.ValueInt64() != 3 {
		t.Errorf("replicas = (%d, %d); want (3, 3)", e.MinReplicas.ValueInt64(), e.MaxReplicas.ValueInt64())
	}
	if e.IdleScaling.IsNull() || e.IdleScaling.ValueBool() {
		t.Errorf("idle_scaling should be present and false")
	}
	if !e.MinReplicaMemoryGb.IsNull() {
		t.Errorf("min_replica_memory_gb should be null when API omits it")
	}

	// Weekday content assertion (set ordering is non-deterministic — sort).
	var weekdays []int64
	diags = e.Weekdays.ElementsAs(context.Background(), &weekdays, false)
	if diags.HasError() {
		t.Fatalf("Weekdays.ElementsAs: %v", diags)
	}
	got := make([]int, len(weekdays))
	for i, v := range weekdays {
		got[i] = int(v)
	}
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("weekdays = %v; want [1 2 3 4 5]", got)
	}
}

func TestApplyScheduleToState_NullBaseConfigWhenAbsent(t *testing.T) {
	state := &models.ServiceScheduledScalingResourceModel{
		ServiceID: types.StringValue("svc-1"),
	}

	diags := applyScheduleToState(&api.AutoScalingSchedule{
		Entries:    []api.AutoScalingScheduleEntry{},
		BaseConfig: nil,
	}, state)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if !state.BaseConfig.IsNull() {
		t.Errorf("BaseConfig should be null when API omits it")
	}
}

func TestApplyScheduleToState_CapturesAllEntries(t *testing.T) {
	schedule := &api.AutoScalingSchedule{
		Entries: []api.AutoScalingScheduleEntry{
			{Name: "first", Weekdays: []int{1}, StartHourUtc: 0, EndHourUtc: 8},
			{Name: "second", Weekdays: []int{1}, StartHourUtc: 8, EndHourUtc: 16},
			{Name: "third", Weekdays: []int{1}, StartHourUtc: 16, EndHourUtc: 24},
		},
	}

	state := &models.ServiceScheduledScalingResourceModel{ServiceID: types.StringValue("svc-1")}
	diags := applyScheduleToState(schedule, state)
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}

	var entries []models.ScheduledScalingEntryModel
	if d := state.Entries.ElementsAs(context.Background(), &entries, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	got := map[string]bool{}
	for _, e := range entries {
		got[e.Name.ValueString()] = true
	}
	want := map[string]bool{"first": true, "second": true, "third": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("entry names = %v; want %v", got, want)
	}
}

// buildEntryList constructs a types.List of ScheduledScalingEntryModel for
// driving planEntriesToAPI in tests.
func buildEntryList(t *testing.T, entries ...models.ScheduledScalingEntryModel) types.List {
	t.Helper()
	values := make([]attr.Value, len(entries))
	for i, e := range entries {
		values[i] = e.ObjectValue()
	}
	list, diags := types.ListValue(models.ScheduledScalingEntryModel{}.ObjectType(), values)
	if diags.HasError() {
		t.Fatalf("ListValue: %v", diags)
	}
	return list
}

func TestPlanEntriesToAPI_EmptyAndNullInputs(t *testing.T) {
	ctx := context.Background()

	got, _, diags := planEntriesToAPI(ctx, types.ListNull(models.ScheduledScalingEntryModel{}.ObjectType()))
	if diags.HasError() {
		t.Fatalf("null input diags: %v", diags)
	}
	if len(got) != 0 {
		t.Errorf("null input: len = %d; want 0", len(got))
	}

	got, _, diags = planEntriesToAPI(ctx, buildEntryList(t))
	if diags.HasError() {
		t.Fatalf("empty input diags: %v", diags)
	}
	if len(got) != 0 {
		t.Errorf("empty input: len = %d; want 0", len(got))
	}
}

func TestPlanEntriesToAPI_ConvertsAllFields(t *testing.T) {
	// Build the set in deliberately non-sorted order to verify planEntriesToAPI
	// sorts before sending.
	weekdaySet, diags := types.SetValue(types.Int64Type, []attr.Value{types.Int64Value(3), types.Int64Value(1)})
	if diags.HasError() {
		t.Fatalf("SetValue: %v", diags)
	}

	entry := models.ScheduledScalingEntryModel{
		Name:               types.StringValue("primary"),
		Weekdays:           weekdaySet,
		StartHourUtc:       types.Int64Value(9),
		EndHourUtc:         types.Int64Value(17),
		MinReplicaMemoryGb: types.Int64Value(8),
		MaxReplicaMemoryGb: types.Int64Value(32),
		MinReplicas:        types.Int64Value(2),
		MaxReplicas:        types.Int64Value(2),
		IdleScaling:        types.BoolValue(true),
		IdleTimeoutMinutes: types.Int64Value(15),
	}

	got, _, diags := planEntriesToAPI(context.Background(), buildEntryList(t, entry))
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d; want 1", len(got))
	}
	g := got[0]
	if g.Name != "primary" || g.StartHourUtc != 9 || g.EndHourUtc != 17 {
		t.Errorf("scalar fields mismatch: %+v", g)
	}
	if !reflect.DeepEqual(g.Weekdays, []int{1, 3}) {
		t.Errorf("weekdays = %v; want [1 3] (sorted)", g.Weekdays)
	}
	if g.MinReplicaMemoryGb == nil || *g.MinReplicaMemoryGb != 8 {
		t.Errorf("MinReplicaMemoryGb = %v; want 8", g.MinReplicaMemoryGb)
	}
	if g.IdleScaling == nil || !*g.IdleScaling {
		t.Errorf("IdleScaling = %v; want true", g.IdleScaling)
	}
	if g.IdleTimeoutMinutes == nil || *g.IdleTimeoutMinutes != 15 {
		t.Errorf("IdleTimeoutMinutes = %v; want 15", g.IdleTimeoutMinutes)
	}
}

func TestValidateScheduledScalingEntries(t *testing.T) {
	tests := []struct {
		name         string
		entry        models.ScheduledScalingEntryModel
		wantErrCount int
	}{
		{
			name: "valid entry",
			entry: models.ScheduledScalingEntryModel{
				Name:         types.StringValue("ok"),
				Weekdays:     weekdaySetOf(t, 1),
				StartHourUtc: types.Int64Value(8),
				EndHourUtc:   types.Int64Value(18),
				MinReplicas:  types.Int64Value(2),
				MaxReplicas:  types.Int64Value(2),
			},
			wantErrCount: 0,
		},
		{
			name: "start equals end",
			entry: models.ScheduledScalingEntryModel{
				Name:         types.StringValue("bad-window"),
				Weekdays:     weekdaySetOf(t, 1),
				StartHourUtc: types.Int64Value(10),
				EndHourUtc:   types.Int64Value(10),
			},
			wantErrCount: 1,
		},
		// Pair-mismatch ("set together") cases are caught at schema level by
		// int64validator.AlsoRequires and so never reach this helper.
		{
			name: "memory min > max",
			entry: models.ScheduledScalingEntryModel{
				Name:               types.StringValue("inverted-memory"),
				Weekdays:           weekdaySetOf(t, 1),
				StartHourUtc:       types.Int64Value(0),
				EndHourUtc:         types.Int64Value(24),
				MinReplicaMemoryGb: types.Int64Value(64),
				MaxReplicaMemoryGb: types.Int64Value(8),
			},
			wantErrCount: 1,
		},
		{
			name: "min != max",
			entry: models.ScheduledScalingEntryModel{
				Name:         types.StringValue("uneven"),
				Weekdays:     weekdaySetOf(t, 1),
				StartHourUtc: types.Int64Value(0),
				EndHourUtc:   types.Int64Value(24),
				MinReplicas:  types.Int64Value(2),
				MaxReplicas:  types.Int64Value(5),
			},
			wantErrCount: 1,
		},
		// idle_scaling and idle_timeout_minutes are independently optional on
		// the server — all four combinations below must validate cleanly.
		{
			name: "idle: both set, idle_scaling=true",
			entry: models.ScheduledScalingEntryModel{
				Name:               types.StringValue("both-true"),
				Weekdays:           weekdaySetOf(t, 1),
				StartHourUtc:       types.Int64Value(0),
				EndHourUtc:         types.Int64Value(24),
				IdleScaling:        types.BoolValue(true),
				IdleTimeoutMinutes: types.Int64Value(10),
			},
			wantErrCount: 0,
		},
		{
			// UI persists this combination — see PR #536 bug report.
			name: "idle: both set, idle_scaling=false",
			entry: models.ScheduledScalingEntryModel{
				Name:               types.StringValue("ui-persisted"),
				Weekdays:           weekdaySetOf(t, 1),
				StartHourUtc:       types.Int64Value(0),
				EndHourUtc:         types.Int64Value(24),
				IdleScaling:        types.BoolValue(false),
				IdleTimeoutMinutes: types.Int64Value(15),
			},
			wantErrCount: 0,
		},
		{
			name: "idle: only idle_timeout set",
			entry: models.ScheduledScalingEntryModel{
				Name:               types.StringValue("lone-timeout"),
				Weekdays:           weekdaySetOf(t, 1),
				StartHourUtc:       types.Int64Value(0),
				EndHourUtc:         types.Int64Value(24),
				IdleTimeoutMinutes: types.Int64Value(10),
			},
			wantErrCount: 0,
		},
		{
			name: "idle: only idle_scaling set",
			entry: models.ScheduledScalingEntryModel{
				Name:         types.StringValue("lone-scaling"),
				Weekdays:     weekdaySetOf(t, 1),
				StartHourUtc: types.Int64Value(0),
				EndHourUtc:   types.Int64Value(24),
				IdleScaling:  types.BoolValue(true),
			},
			wantErrCount: 0,
		},
		{
			name: "idle: both unset",
			entry: models.ScheduledScalingEntryModel{
				Name:         types.StringValue("no-idle"),
				Weekdays:     weekdaySetOf(t, 1),
				StartHourUtc: types.Int64Value(0),
				EndHourUtc:   types.Int64Value(24),
			},
			wantErrCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := validateScheduledScalingEntries([]models.ScheduledScalingEntryModel{tt.entry})
			if diags.ErrorsCount() != tt.wantErrCount {
				t.Errorf("ErrorsCount = %d; want %d; diags = %v", diags.ErrorsCount(), tt.wantErrCount, diags)
			}
		})
	}
}

func TestPlanEntriesToAPI_OmitsNullOptionalFields(t *testing.T) {
	weekdaySet, diags := types.SetValue(types.Int64Type, []attr.Value{types.Int64Value(0)})
	if diags.HasError() {
		t.Fatalf("SetValue: %v", diags)
	}

	entry := models.ScheduledScalingEntryModel{
		Name:               types.StringValue("minimal"),
		Weekdays:           weekdaySet,
		StartHourUtc:       types.Int64Value(0),
		EndHourUtc:         types.Int64Value(24),
		MinReplicaMemoryGb: types.Int64Null(),
		MaxReplicaMemoryGb: types.Int64Null(),
		MinReplicas:        types.Int64Null(),
		MaxReplicas:        types.Int64Null(),
		IdleScaling:        types.BoolNull(),
		IdleTimeoutMinutes: types.Int64Null(),
	}

	got, _, diags := planEntriesToAPI(context.Background(), buildEntryList(t, entry))
	if diags.HasError() {
		t.Fatalf("diags: %v", diags)
	}
	g := got[0]
	if g.MinReplicaMemoryGb != nil || g.MaxReplicaMemoryGb != nil {
		t.Errorf("memory pointers should be nil, got %v / %v", g.MinReplicaMemoryGb, g.MaxReplicaMemoryGb)
	}
	if g.MinReplicas != nil || g.MaxReplicas != nil {
		t.Errorf("replica pointers should be nil, got %v / %v", g.MinReplicas, g.MaxReplicas)
	}
	if g.IdleScaling != nil || g.IdleTimeoutMinutes != nil {
		t.Errorf("idle pointers should be nil, got %v / %v", g.IdleScaling, g.IdleTimeoutMinutes)
	}
}

// TestServiceScheduledScalingResource_ImportState verifies that the custom
// ImportState handler writes both `id` and `service_id` from the user-supplied
// import ID. Drives the actual resource method through a constructed
// tfsdk.State (no acceptance harness needed).
func TestServiceScheduledScalingResource_ImportState(t *testing.T) {
	ctx := context.Background()
	r := NewServiceScheduledScalingResource().(*ServiceScheduledScalingResource)

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", schemaResp.Diagnostics)
	}
	sch := schemaResp.Schema

	req := resource.ImportStateRequest{ID: "abc-123"}
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: sch,
			Raw:    tftypes.NewValue(sch.Type().TerraformType(ctx), nil),
		},
	}

	r.ImportState(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diags: %v", resp.Diagnostics)
	}

	var id, serviceID types.String
	if d := resp.State.GetAttribute(ctx, path.Root("id"), &id); d.HasError() {
		t.Fatalf("read id: %v", d)
	}
	if d := resp.State.GetAttribute(ctx, path.Root("service_id"), &serviceID); d.HasError() {
		t.Fatalf("read service_id: %v", d)
	}
	if id.ValueString() != "abc-123" {
		t.Errorf("id = %q; want abc-123", id.ValueString())
	}
	if serviceID.ValueString() != "abc-123" {
		t.Errorf("service_id = %q; want abc-123", serviceID.ValueString())
	}
}

// TestRoundTrip_NoServerNormalization simulates: user writes config, provider
// POSTs entries, server echoes them back verbatim, Read maps back to state.
// State should equal the original plan — proves the provider doesn't introduce
// gratuitous drift when the server is a faithful echo. Catches model-typing
// regressions (e.g. nil-pointer-to-Null mapping).
func TestRoundTrip_NoServerNormalization(t *testing.T) {
	ctx := context.Background()

	weekdaySet, _ := types.SetValue(types.Int64Type, []attr.Value{types.Int64Value(1), types.Int64Value(2), types.Int64Value(3)})
	planEntry := models.ScheduledScalingEntryModel{
		Name:               types.StringValue("planA"),
		Weekdays:           weekdaySet,
		StartHourUtc:       types.Int64Value(9),
		EndHourUtc:         types.Int64Value(17),
		MinReplicaMemoryGb: types.Int64Value(8),
		MaxReplicaMemoryGb: types.Int64Value(32),
		MinReplicas:        types.Int64Value(2),
		MaxReplicas:        types.Int64Value(2),
		IdleScaling:        types.BoolValue(true),
		IdleTimeoutMinutes: types.Int64Value(15),
	}
	planList := buildEntryList(t, planEntry)

	apiEntries, _, d := planEntriesToAPI(ctx, planList)
	if d.HasError() {
		t.Fatalf("planEntriesToAPI: %v", d)
	}

	// Server echoes the request verbatim, no defaults filled.
	serverResponse := &api.AutoScalingSchedule{Entries: apiEntries}

	state := &models.ServiceScheduledScalingResourceModel{}
	if d := applyScheduleToState(serverResponse, state); d.HasError() {
		t.Fatalf("applyScheduleToState: %v", d)
	}

	var roundTripped []models.ScheduledScalingEntryModel
	if d := state.Entries.ElementsAs(ctx, &roundTripped, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	if len(roundTripped) != 1 {
		t.Fatalf("len = %d; want 1", len(roundTripped))
	}
	r := roundTripped[0]

	if r.Name.ValueString() != "planA" {
		t.Errorf("Name = %q; want planA", r.Name.ValueString())
	}
	if r.MinReplicas.ValueInt64() != 2 || r.MaxReplicas.ValueInt64() != 2 {
		t.Errorf("replicas = (%d, %d); want (2, 2)", r.MinReplicas.ValueInt64(), r.MaxReplicas.ValueInt64())
	}
	if !r.IdleScaling.ValueBool() {
		t.Errorf("IdleScaling = false; want true")
	}
	if r.IdleTimeoutMinutes.ValueInt64() != 15 {
		t.Errorf("IdleTimeoutMinutes = %d; want 15", r.IdleTimeoutMinutes.ValueInt64())
	}

	var wd []int64
	if d := r.Weekdays.ElementsAs(ctx, &wd, false); d.HasError() {
		t.Fatalf("Weekdays.ElementsAs: %v", d)
	}
	got := make([]int, len(wd))
	for i, v := range wd {
		got[i] = int(v)
	}
	sort.Ints(got)
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Errorf("weekdays = %v; want [1 2 3]", got)
	}
}

// reconcileSingleEntry drives applyScheduleToStateWithPlan for one plan entry
// against one server-returned entry and returns the resulting state entry.
func reconcileSingleEntry(t *testing.T, planEntry models.ScheduledScalingEntryModel, serverEntry api.AutoScalingScheduleEntry) models.ScheduledScalingEntryModel {
	t.Helper()
	ctx := context.Background()

	state := &models.ServiceScheduledScalingResourceModel{}
	schedule := &api.AutoScalingSchedule{Entries: []api.AutoScalingScheduleEntry{serverEntry}}
	if d := applyScheduleToStateWithPlan(schedule, []models.ScheduledScalingEntryModel{planEntry}, state); d.HasError() {
		t.Fatalf("applyScheduleToStateWithPlan: %v", d)
	}

	var entries []models.ScheduledScalingEntryModel
	if d := state.Entries.ElementsAs(ctx, &entries, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d; want 1", len(entries))
	}
	return entries[0]
}

func weekdaySetOf(t *testing.T, days ...int64) types.Set {
	t.Helper()
	elems := make([]attr.Value, len(days))
	for i, d := range days {
		elems[i] = types.Int64Value(d)
	}
	s, diags := types.SetValue(types.Int64Type, elems)
	if diags.HasError() {
		t.Fatalf("SetValue: %v", diags)
	}
	return s
}

// TestApplyScheduleToStateWithPlan_PreservesReplicaBand reproduces the actual
// issue #611 normalization (UC-1252): the server collapsed a vertical entry's
// equal min/max replica band to numReplicas (a field the provider does not
// model) and omitted minReplicas/maxReplicas from the response, while echoing
// the idle fields. Without reconciliation the planned 10/10 becomes null/null
// and Terraform aborts with "inconsistent result after apply". Fixed
// server-side in control-plane#35956; kept here so the provider stays correct
// against any server version.
func TestApplyScheduleToStateWithPlan_PreservesReplicaBand(t *testing.T) {
	plan := models.ScheduledScalingEntryModel{
		Name:               types.StringValue("Business Hours"),
		Weekdays:           weekdaySetOf(t, 1, 2, 3, 4, 5),
		StartHourUtc:       types.Int64Value(5),
		EndHourUtc:         types.Int64Value(19),
		MinReplicaMemoryGb: types.Int64Value(236),
		MaxReplicaMemoryGb: types.Int64Value(236),
		MinReplicas:        types.Int64Value(10),
		MaxReplicas:        types.Int64Value(10),
		IdleScaling:        types.BoolValue(false),
		// Left unset by the user: unknown at plan time.
		IdleTimeoutMinutes: types.Int64Unknown(),
	}
	// Pre-#35956 server response: memory and idle fields echoed, replica band
	// omitted (returned as numReplicas, which the provider does not decode).
	server := api.AutoScalingScheduleEntry{
		Name:               "Business Hours",
		Weekdays:           []int{1, 2, 3, 4, 5},
		StartHourUtc:       5,
		EndHourUtc:         19,
		MinReplicaMemoryGb: intPtr(236),
		MaxReplicaMemoryGb: intPtr(236),
		IdleScaling:        boolPtr(false),
	}

	got := reconcileSingleEntry(t, plan, server)

	if got.MinReplicas.ValueInt64() != 10 || got.MaxReplicas.ValueInt64() != 10 {
		t.Errorf("replicas = (%d, %d); want (10, 10) (planned values must survive)", got.MinReplicas.ValueInt64(), got.MaxReplicas.ValueInt64())
	}
	if got.IdleScaling.IsNull() || got.IdleScaling.IsUnknown() {
		t.Fatalf("idle_scaling should be a known value, got %v", got.IdleScaling)
	}
	if got.IdleScaling.ValueBool() {
		t.Errorf("idle_scaling = true; want false")
	}
	if !got.IdleTimeoutMinutes.IsNull() {
		t.Errorf("idle_timeout_minutes = %v; want null (unset in plan, not echoed by server)", got.IdleTimeoutMinutes)
	}
}

// TestApplyScheduleToStateWithPlan_PreservesExplicitIdleTimeout guards the
// general property behind the fix: any value the user set explicitly survives
// a server response that does not echo it. The fixture drops the idle fields —
// a normalization no current server version performs — precisely to prove the
// reconcile holds for fields beyond the replica band.
func TestApplyScheduleToStateWithPlan_PreservesExplicitIdleTimeout(t *testing.T) {
	plan := models.ScheduledScalingEntryModel{
		Name:               types.StringValue("Business Hours"),
		Weekdays:           weekdaySetOf(t, 1),
		StartHourUtc:       types.Int64Value(5),
		EndHourUtc:         types.Int64Value(19),
		MinReplicas:        types.Int64Value(10),
		MaxReplicas:        types.Int64Value(10),
		MinReplicaMemoryGb: types.Int64Null(),
		MaxReplicaMemoryGb: types.Int64Null(),
		IdleScaling:        types.BoolValue(false),
		IdleTimeoutMinutes: types.Int64Value(5),
	}
	server := api.AutoScalingScheduleEntry{
		Name:         "Business Hours",
		Weekdays:     []int{1},
		StartHourUtc: 5,
		EndHourUtc:   19,
		MinReplicas:  intPtr(10),
		MaxReplicas:  intPtr(10),
	}

	got := reconcileSingleEntry(t, plan, server)

	if got.IdleScaling.ValueBool() {
		t.Errorf("idle_scaling = true; want false")
	}
	if got.IdleTimeoutMinutes.IsNull() || got.IdleTimeoutMinutes.ValueInt64() != 5 {
		t.Errorf("idle_timeout_minutes = %v; want 5 (planned value must survive)", got.IdleTimeoutMinutes)
	}
}

// TestApplyScheduleToStateWithPlan_FillsUnknownFromServer verifies the other
// direction: a field the user left unset (unknown at plan) is resolved from the
// server's echoed value.
func TestApplyScheduleToStateWithPlan_FillsUnknownFromServer(t *testing.T) {
	plan := models.ScheduledScalingEntryModel{
		Name:               types.StringValue("Overnight"),
		Weekdays:           weekdaySetOf(t, 0, 6),
		StartHourUtc:       types.Int64Value(22),
		EndHourUtc:         types.Int64Value(6),
		MinReplicaMemoryGb: types.Int64Unknown(),
		MaxReplicaMemoryGb: types.Int64Unknown(),
		MinReplicas:        types.Int64Unknown(),
		MaxReplicas:        types.Int64Unknown(),
		IdleScaling:        types.BoolValue(true),
		IdleTimeoutMinutes: types.Int64Value(5),
	}
	server := api.AutoScalingScheduleEntry{
		Name:               "Overnight",
		Weekdays:           []int{0, 6},
		StartHourUtc:       22,
		EndHourUtc:         6,
		MinReplicaMemoryGb: intPtr(16),
		MaxReplicaMemoryGb: intPtr(16),
		MinReplicas:        intPtr(1),
		MaxReplicas:        intPtr(1),
		IdleScaling:        boolPtr(true),
		IdleTimeoutMinutes: intPtr(5),
	}

	got := reconcileSingleEntry(t, plan, server)

	if got.MinReplicas.ValueInt64() != 1 || got.MaxReplicas.ValueInt64() != 1 {
		t.Errorf("replicas = (%d, %d); want (1, 1) from server", got.MinReplicas.ValueInt64(), got.MaxReplicas.ValueInt64())
	}
	if got.MinReplicaMemoryGb.ValueInt64() != 16 || got.MaxReplicaMemoryGb.ValueInt64() != 16 {
		t.Errorf("memory = (%d, %d); want (16, 16) from server", got.MinReplicaMemoryGb.ValueInt64(), got.MaxReplicaMemoryGb.ValueInt64())
	}
	if !got.IdleScaling.ValueBool() {
		t.Errorf("idle_scaling = false; want true")
	}
}

// TestServiceScheduledScalingResource_UpgradeStateV0ToV1 verifies that state
// written by an older provider (entries as a set) upgrades cleanly to the v1
// layout (entries as a list) without losing entry content.
func TestServiceScheduledScalingResource_UpgradeStateV0ToV1(t *testing.T) {
	ctx := context.Background()
	r := NewServiceScheduledScalingResource().(*ServiceScheduledScalingResource)

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", schemaResp.Diagnostics)
	}
	currentSchema := schemaResp.Schema

	upgrader, ok := r.UpgradeState(ctx)[0]
	if !ok {
		t.Fatalf("no upgrader registered for schema version 0")
	}
	if upgrader.PriorSchema == nil {
		t.Fatalf("upgrader is missing a PriorSchema")
	}
	priorSchema := *upgrader.PriorSchema

	entry := models.ScheduledScalingEntryModel{
		Name:               types.StringValue("business"),
		Weekdays:           weekdaySetOf(t, 1, 2),
		StartHourUtc:       types.Int64Value(8),
		EndHourUtc:         types.Int64Value(18),
		MinReplicaMemoryGb: types.Int64Null(),
		MaxReplicaMemoryGb: types.Int64Null(),
		MinReplicas:        types.Int64Value(3),
		MaxReplicas:        types.Int64Value(3),
		IdleScaling:        types.BoolValue(true),
		IdleTimeoutMinutes: types.Int64Value(5),
	}
	entrySet, d := types.SetValue(models.ScheduledScalingEntryModel{}.ObjectType(), []attr.Value{entry.ObjectValue()})
	if d.HasError() {
		t.Fatalf("SetValue: %v", d)
	}

	priorState := tfsdk.State{
		Schema: priorSchema,
		Raw:    tftypes.NewValue(priorSchema.Type().TerraformType(ctx), nil),
	}
	if d := priorState.Set(ctx, scheduledScalingModelV0{
		ID:         types.StringValue("svc-1"),
		ServiceID:  types.StringValue("svc-1"),
		Entries:    entrySet,
		BaseConfig: types.ObjectNull(models.ScheduledScalingBaseConfigModel{}.ObjectType().AttrTypes),
	}); d.HasError() {
		t.Fatalf("prior state Set: %v", d)
	}

	resp := &resource.UpgradeStateResponse{
		State: tfsdk.State{
			Schema: currentSchema,
			Raw:    tftypes.NewValue(currentSchema.Type().TerraformType(ctx), nil),
		},
	}
	upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{State: &priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrade diags: %v", resp.Diagnostics)
	}

	var upgraded models.ServiceScheduledScalingResourceModel
	if d := resp.State.Get(ctx, &upgraded); d.HasError() {
		t.Fatalf("Get upgraded (entries must decode as a list): %v", d)
	}
	if upgraded.ServiceID.ValueString() != "svc-1" {
		t.Errorf("service_id = %q; want svc-1", upgraded.ServiceID.ValueString())
	}
	var entries []models.ScheduledScalingEntryModel
	if d := upgraded.Entries.ElementsAs(ctx, &entries, false); d.HasError() {
		t.Fatalf("ElementsAs: %v", d)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d; want 1", len(entries))
	}
	if entries[0].Name.ValueString() != "business" {
		t.Errorf("name = %q; want business", entries[0].Name.ValueString())
	}
	if entries[0].MinReplicas.ValueInt64() != 3 || !entries[0].IdleScaling.ValueBool() {
		t.Errorf("entry content not preserved: %+v", entries[0])
	}
}

// TestApiEntryToModel_DefaultsIdleScalingFalse guards the Read path: a server
// entry that omits idle_scaling maps to its effective default, false, so a
// refresh can never report perpetual drift on it. Current server versions echo
// idle_scaling; this pins the defensive default.
func TestApiEntryToModel_DefaultsIdleScalingFalse(t *testing.T) {
	got, diags := apiEntryToModel(api.AutoScalingScheduleEntry{
		Name:         "no-idle",
		Weekdays:     []int{1},
		StartHourUtc: 0,
		EndHourUtc:   24,
		MinReplicas:  intPtr(2),
		MaxReplicas:  intPtr(2),
	})
	if diags.HasError() {
		t.Fatalf("apiEntryToModel: %v", diags)
	}
	if got.IdleScaling.IsNull() || got.IdleScaling.IsUnknown() {
		t.Fatalf("idle_scaling should default to a known value, got %v", got.IdleScaling)
	}
	if got.IdleScaling.ValueBool() {
		t.Errorf("idle_scaling = true; want false")
	}
}
