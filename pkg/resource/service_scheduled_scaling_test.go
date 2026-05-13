package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

func TestApplyScheduleToState_PopulatesEntriesAndBaseConfig(t *testing.T) {
	schedule := &api.AutoScalingSchedule{
		Entries: []api.AutoScalingScheduleEntry{
			{
				ID:           "entry-1",
				Name:         "business",
				Weekdays:     []int{1, 2, 3, 4, 5},
				StartHourUtc: 8,
				EndHourUtc:   18,
				MinReplicas:  intPtr(3),
				MaxReplicas:  intPtr(3),
				IdleScaling:  boolPtr(false),
				IsActiveNow:  true, // server-only field, not in TF state but must not crash
			},
		},
		BaseConfig: &api.AutoScalingScheduleBaseConfig{
			MinReplicaMemoryGb: intPtr(8),
			MaxReplicaMemoryGb: intPtr(32),
			IdleScaling:        boolPtr(true),
			IdleTimeoutMinutes: intPtr(5),
		},
		ActiveEntryID: "entry-1",
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
	if e.ID.ValueString() != "entry-1" {
		t.Errorf("entry.ID = %q; want entry-1", e.ID.ValueString())
	}
	if e.MinReplicas.ValueInt64() != 3 || e.MaxReplicas.ValueInt64() != 3 {
		t.Errorf("replicas = (%d, %d); want (3, 3)", e.MinReplicas.ValueInt64(), e.MaxReplicas.ValueInt64())
	}
	if e.IdleScaling.IsNull() || e.IdleScaling.ValueBool() {
		t.Errorf("idle_scaling should be present and false")
	}
	if !e.MinReplicaMemoryGb.IsNull() {
		t.Errorf("min_replica_memory_gb should be null when API omits it")
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
	weekdaySet, diags := types.SetValue(types.Int64Type, []attr.Value{types.Int64Value(1), types.Int64Value(3)})
	if diags.HasError() {
		t.Fatalf("SetValue: %v", diags)
	}

	entry := models.ScheduledScalingEntryModel{
		ID:                 types.StringNull(),
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
	if len(g.Weekdays) != 2 {
		t.Errorf("weekdays len = %d; want 2", len(g.Weekdays))
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
		name         string
		entry        models.ScheduledScalingEntryModel
		wantErrCount int
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
		ID:                 types.StringNull(),
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
