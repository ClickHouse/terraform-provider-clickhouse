package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// ScheduledScalingEntryModel mirrors a single entry in the scaling schedule.
type ScheduledScalingEntryModel struct {
	Name               types.String `tfsdk:"name"`
	Weekdays           types.Set    `tfsdk:"weekdays"`
	StartHourUtc       types.Int64  `tfsdk:"start_hour_utc"`
	EndHourUtc         types.Int64  `tfsdk:"end_hour_utc"`
	MinReplicaMemoryGb types.Int64  `tfsdk:"min_replica_memory_gb"`
	MaxReplicaMemoryGb types.Int64  `tfsdk:"max_replica_memory_gb"`
	MinReplicas        types.Int64  `tfsdk:"min_replicas"`
	MaxReplicas        types.Int64  `tfsdk:"max_replicas"`
	IdleScaling        types.Bool   `tfsdk:"idle_scaling"`
	IdleTimeoutMinutes types.Int64  `tfsdk:"idle_timeout_minutes"`
}

func (m ScheduledScalingEntryModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name":                  types.StringType,
			"weekdays":              types.SetType{ElemType: types.Int64Type},
			"start_hour_utc":        types.Int64Type,
			"end_hour_utc":          types.Int64Type,
			"min_replica_memory_gb": types.Int64Type,
			"max_replica_memory_gb": types.Int64Type,
			"min_replicas":          types.Int64Type,
			"max_replicas":          types.Int64Type,
			"idle_scaling":          types.BoolType,
			"idle_timeout_minutes":  types.Int64Type,
		},
	}
}

func (m ScheduledScalingEntryModel) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"name":                  m.Name,
		"weekdays":              m.Weekdays,
		"start_hour_utc":        m.StartHourUtc,
		"end_hour_utc":          m.EndHourUtc,
		"min_replica_memory_gb": m.MinReplicaMemoryGb,
		"max_replica_memory_gb": m.MaxReplicaMemoryGb,
		"min_replicas":          m.MinReplicas,
		"max_replicas":          m.MaxReplicas,
		"idle_scaling":          m.IdleScaling,
		"idle_timeout_minutes":  m.IdleTimeoutMinutes,
	})
}

// ScheduledScalingBaseConfigModel mirrors the schedule's fallback configuration
// applied when no entry is currently active.
type ScheduledScalingBaseConfigModel struct {
	MinReplicaMemoryGb types.Int64 `tfsdk:"min_replica_memory_gb"`
	MaxReplicaMemoryGb types.Int64 `tfsdk:"max_replica_memory_gb"`
	MinReplicas        types.Int64 `tfsdk:"min_replicas"`
	MaxReplicas        types.Int64 `tfsdk:"max_replicas"`
	IdleScaling        types.Bool  `tfsdk:"idle_scaling"`
	IdleTimeoutMinutes types.Int64 `tfsdk:"idle_timeout_minutes"`
}

func (m ScheduledScalingBaseConfigModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"min_replica_memory_gb": types.Int64Type,
			"max_replica_memory_gb": types.Int64Type,
			"min_replicas":          types.Int64Type,
			"max_replicas":          types.Int64Type,
			"idle_scaling":          types.BoolType,
			"idle_timeout_minutes":  types.Int64Type,
		},
	}
}

func (m ScheduledScalingBaseConfigModel) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"min_replica_memory_gb": m.MinReplicaMemoryGb,
		"max_replica_memory_gb": m.MaxReplicaMemoryGb,
		"min_replicas":          m.MinReplicas,
		"max_replicas":          m.MaxReplicas,
		"idle_scaling":          m.IdleScaling,
		"idle_timeout_minutes":  m.IdleTimeoutMinutes,
	})
}

// ServiceScheduledScalingResourceModel is the Terraform state model for the
// clickhouse_service_scheduled_scaling resource.
type ServiceScheduledScalingResourceModel struct {
	ID         types.String `tfsdk:"id"`
	ServiceID  types.String `tfsdk:"service_id"`
	Entries    types.Set    `tfsdk:"entries"`
	BaseConfig types.Object `tfsdk:"base_config"`
}
