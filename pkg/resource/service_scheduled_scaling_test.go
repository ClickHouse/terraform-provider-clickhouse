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

func TestApplyScheduleToState_PreservesMultiEntryOrder(t *testing.T) {
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
	names := []string{entries[0].Name.ValueString(), entries[1].Name.ValueString(), entries[2].Name.ValueString()}
	if !reflect.DeepEqual(names, []string{"first", "second", "third"}) {
		t.Errorf("entry order = %v; want first/second/third", names)
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

	got, diags := planEntriesToAPI(ctx, types.ListNull(models.ScheduledScalingEntryModel{}.ObjectType()))
	if diags.HasError() {
		t.Fatalf("null input diags: %v", diags)
	}
	if len(got) != 0 {
		t.Errorf("null input: len = %d; want 0", len(got))
	}

	got, diags = planEntriesToAPI(ctx, buildEntryList(t))
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

	got, diags := planEntriesToAPI(context.Background(), buildEntryList(t, entry))
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
	mustSet := func(vals ...int64) types.Set {
		elems := make([]attr.Value, len(vals))
		for i, v := range vals {
			elems[i] = types.Int64Value(v)
		}
		s, diags := types.SetValue(types.Int64Type, elems)
		if diags.HasError() {
			t.Fatalf("SetValue: %v", diags)
		}
		return s
	}

	tests := []struct {
		name          string
		entry         models.ScheduledScalingEntryModel
		wantErrCount  int
		wantWarnCount int
	}{
		{
			name: "valid entry",
			entry: models.ScheduledScalingEntryModel{
				Name:         types.StringValue("ok"),
				Weekdays:     mustSet(1),
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
				Weekdays:     mustSet(1),
				StartHourUtc: types.Int64Value(10),
				EndHourUtc:   types.Int64Value(10),
			},
			wantErrCount: 1,
		},
		{
			name: "memory pair mismatch",
			entry: models.ScheduledScalingEntryModel{
				Name:               types.StringValue("partial-memory"),
				Weekdays:           mustSet(1),
				StartHourUtc:       types.Int64Value(0),
				EndHourUtc:         types.Int64Value(24),
				MinReplicaMemoryGb: types.Int64Value(8),
				MaxReplicaMemoryGb: types.Int64Null(),
			},
			wantErrCount: 1,
		},
		{
			name: "memory min > max",
			entry: models.ScheduledScalingEntryModel{
				Name:               types.StringValue("inverted-memory"),
				Weekdays:           mustSet(1),
				StartHourUtc:       types.Int64Value(0),
				EndHourUtc:         types.Int64Value(24),
				MinReplicaMemoryGb: types.Int64Value(64),
				MaxReplicaMemoryGb: types.Int64Value(8),
			},
			wantErrCount: 1,
		},
		{
			name: "replica pair mismatch",
			entry: models.ScheduledScalingEntryModel{
				Name:         types.StringValue("partial-replicas"),
				Weekdays:     mustSet(1),
				StartHourUtc: types.Int64Value(0),
				EndHourUtc:   types.Int64Value(24),
				MinReplicas:  types.Int64Value(3),
				MaxReplicas:  types.Int64Null(),
			},
			wantErrCount: 1,
		},
		{
			name: "min != max",
			entry: models.ScheduledScalingEntryModel{
				Name:         types.StringValue("uneven"),
				Weekdays:     mustSet(1),
				StartHourUtc: types.Int64Value(0),
				EndHourUtc:   types.Int64Value(24),
				MinReplicas:  types.Int64Value(2),
				MaxReplicas:  types.Int64Value(5),
			},
			wantErrCount: 1,
		},
		{
			// Server accepts idle_timeout_minutes without idle_scaling (UI
			// creates schedules with this combination). Validator warns,
			// doesn't error — otherwise imports break.
			name: "idle_timeout without idle_scaling",
			entry: models.ScheduledScalingEntryModel{
				Name:               types.StringValue("orphan-timeout"),
				Weekdays:           mustSet(1),
				StartHourUtc:       types.Int64Value(0),
				EndHourUtc:         types.Int64Value(24),
				IdleTimeoutMinutes: types.Int64Value(10),
			},
			wantErrCount:  0,
			wantWarnCount: 1,
		},
		{
			name: "idle_timeout with idle_scaling=false",
			entry: models.ScheduledScalingEntryModel{
				Name:               types.StringValue("explicit-false"),
				Weekdays:           mustSet(1),
				StartHourUtc:       types.Int64Value(0),
				EndHourUtc:         types.Int64Value(24),
				IdleScaling:        types.BoolValue(false),
				IdleTimeoutMinutes: types.Int64Value(10),
			},
			wantErrCount:  0,
			wantWarnCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := validateScheduledScalingEntries([]models.ScheduledScalingEntryModel{tt.entry})
			if diags.ErrorsCount() != tt.wantErrCount {
				t.Errorf("ErrorsCount = %d; want %d; diags = %v", diags.ErrorsCount(), tt.wantErrCount, diags)
			}
			if diags.WarningsCount() != tt.wantWarnCount {
				t.Errorf("WarningsCount = %d; want %d; diags = %v", diags.WarningsCount(), tt.wantWarnCount, diags)
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

	got, diags := planEntriesToAPI(context.Background(), buildEntryList(t, entry))
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

	apiEntries, d := planEntriesToAPI(ctx, planList)
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
