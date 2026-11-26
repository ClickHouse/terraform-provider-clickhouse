package models

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

// ByocRestrictedFactsModel is the registration of a restricted-access BYOC
// infrastructure, shared by the resource and the data source.
type ByocRestrictedFactsModel struct {
	ClusterName              types.String `tfsdk:"cluster_name"`
	PlatformNodeGroup        types.String `tfsdk:"platform_node_group"`
	AvailabilityZones        types.List   `tfsdk:"availability_zones"`
	NamespacePrefix          types.String `tfsdk:"namespace_prefix"`
	ExecutorRoleArn          types.String `tfsdk:"executor_role_arn"`
	NodeRoleArns             types.List   `tfsdk:"node_role_arns"`
	ImageMode                types.String `tfsdk:"image_mode"`
	ImageRegistry            types.String `tfsdk:"image_registry"`
	DataBucket               types.String `tfsdk:"data_bucket"`
	BackupBucket             types.String `tfsdk:"backup_bucket"`
	StorageRoleArn           types.String `tfsdk:"storage_role_arn"`
	ScraperEndpoint          types.String `tfsdk:"scraper_endpoint"`
	EnableMonitoring         types.Bool   `tfsdk:"enable_monitoring"`
	EnableSnapshotController types.Bool   `tfsdk:"enable_snapshot_controller"`
	EnableAutoscaler         types.Bool   `tfsdk:"enable_autoscaler"`
}

func ByocRestrictedFactsAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"cluster_name":               types.StringType,
		"platform_node_group":        types.StringType,
		"availability_zones":         types.ListType{ElemType: types.StringType},
		"namespace_prefix":           types.StringType,
		"executor_role_arn":          types.StringType,
		"node_role_arns":             types.ListType{ElemType: types.StringType},
		"image_mode":                 types.StringType,
		"image_registry":             types.StringType,
		"data_bucket":                types.StringType,
		"backup_bucket":              types.StringType,
		"storage_role_arn":           types.StringType,
		"scraper_endpoint":           types.StringType,
		"enable_monitoring":          types.BoolType,
		"enable_snapshot_controller": types.BoolType,
		"enable_autoscaler":          types.BoolType,
	}
}

// ByocRestrictedFactsObjectFromAPI maps the read-back facts; nil yields null.
func ByocRestrictedFactsObjectFromAPI(ctx context.Context, facts *api.ByocRestrictedFacts) (types.Object, diag.Diagnostics) {
	if facts == nil {
		return types.ObjectNull(ByocRestrictedFactsAttrTypes()), nil
	}
	var diags diag.Diagnostics
	zones, d := stringListOrNull(ctx, facts.AvailabilityZones)
	diags.Append(d...)
	nodeRoles, d := stringListOrNull(ctx, facts.NodeRoleArns)
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(ByocRestrictedFactsAttrTypes()), diags
	}
	model := ByocRestrictedFactsModel{
		ClusterName:              types.StringValue(facts.ClusterName),
		PlatformNodeGroup:        types.StringPointerValue(facts.PlatformNodeGroup),
		AvailabilityZones:        zones,
		NamespacePrefix:          types.StringPointerValue(facts.NamespacePrefix),
		ExecutorRoleArn:          types.StringPointerValue(facts.ExecutorRoleArn),
		NodeRoleArns:             nodeRoles,
		ImageMode:                types.StringValue(facts.ImageMode),
		ImageRegistry:            types.StringPointerValue(facts.ImageRegistry),
		DataBucket:               types.StringValue(facts.DataBucket),
		BackupBucket:             types.StringValue(facts.BackupBucket),
		StorageRoleArn:           types.StringValue(facts.StorageRoleArn),
		ScraperEndpoint:          types.StringPointerValue(facts.ScraperEndpoint),
		EnableMonitoring:         types.BoolPointerValue(facts.EnableMonitoring),
		EnableSnapshotController: types.BoolPointerValue(facts.EnableSnapshotController),
		EnableAutoscaler:         types.BoolPointerValue(facts.EnableAutoscaler),
	}
	obj, d := types.ObjectValueFrom(ctx, ByocRestrictedFactsAttrTypes(), model)
	diags.Append(d...)
	return obj, diags
}

// ByocRestrictedFactsFromObject builds the API facts from a known object.
// Null and unknown optional attributes are omitted so the API applies its
// defaults.
func ByocRestrictedFactsFromObject(ctx context.Context, obj types.Object) (*api.ByocRestrictedFacts, diag.Diagnostics) {
	var diags diag.Diagnostics
	if obj.IsNull() || obj.IsUnknown() {
		return nil, diags
	}
	var model ByocRestrictedFactsModel
	diags.Append(obj.As(ctx, &model, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return nil, diags
	}
	facts := &api.ByocRestrictedFacts{
		ClusterName:              model.ClusterName.ValueString(),
		PlatformNodeGroup:        knownStringPointer(model.PlatformNodeGroup),
		NamespacePrefix:          knownStringPointer(model.NamespacePrefix),
		ExecutorRoleArn:          knownStringPointer(model.ExecutorRoleArn),
		ImageMode:                model.ImageMode.ValueString(),
		ImageRegistry:            knownStringPointer(model.ImageRegistry),
		DataBucket:               model.DataBucket.ValueString(),
		BackupBucket:             model.BackupBucket.ValueString(),
		StorageRoleArn:           model.StorageRoleArn.ValueString(),
		ScraperEndpoint:          knownStringPointer(model.ScraperEndpoint),
		EnableMonitoring:         knownBoolPointer(model.EnableMonitoring),
		EnableSnapshotController: knownBoolPointer(model.EnableSnapshotController),
		EnableAutoscaler:         knownBoolPointer(model.EnableAutoscaler),
	}
	if !model.AvailabilityZones.IsNull() && !model.AvailabilityZones.IsUnknown() {
		diags.Append(model.AvailabilityZones.ElementsAs(ctx, &facts.AvailabilityZones, false)...)
	}
	if !model.NodeRoleArns.IsNull() && !model.NodeRoleArns.IsUnknown() {
		diags.Append(model.NodeRoleArns.ElementsAs(ctx, &facts.NodeRoleArns, false)...)
	}
	return facts, diags
}

// ByocResolveUnknownRestrictedFacts nulls attributes left unknown in a
// planned facts object, so the plan can be persisted when the read-back that
// would resolve them failed.
func ByocResolveUnknownRestrictedFacts(ctx context.Context, obj types.Object) (types.Object, diag.Diagnostics) {
	if obj.IsNull() {
		return obj, nil
	}
	if obj.IsUnknown() {
		return types.ObjectNull(ByocRestrictedFactsAttrTypes()), nil
	}
	attrTypes := ByocRestrictedFactsAttrTypes()
	values := map[string]attr.Value{}
	for name, value := range obj.Attributes() {
		if value.IsUnknown() {
			value = nullValueOf(attrTypes[name])
		}
		values[name] = value
	}
	return types.ObjectValue(attrTypes, values)
}

type ByocConnectorModel struct {
	LastHeartbeatAt types.String `tfsdk:"last_heartbeat_at"`
	PlatformState   types.String `tfsdk:"platform_state"`
	PlatformDetail  types.String `tfsdk:"platform_detail"`
	Components      types.List   `tfsdk:"components"`
}

type ByocConnectorComponentModel struct {
	Name            types.String `tfsdk:"name"`
	State           types.String `tfsdk:"state"`
	Version         types.String `tfsdk:"version"`
	LastHeartbeatAt types.String `tfsdk:"last_heartbeat_at"`
}

func ByocConnectorComponentAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"name":              types.StringType,
		"state":             types.StringType,
		"version":           types.StringType,
		"last_heartbeat_at": types.StringType,
	}
}

func ByocConnectorAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"last_heartbeat_at": types.StringType,
		"platform_state":    types.StringType,
		"platform_detail":   types.StringType,
		"components":        types.ListType{ElemType: types.ObjectType{AttrTypes: ByocConnectorComponentAttrTypes()}},
	}
}

// ByocConnectorObjectFromAPI maps the connector liveness; nil (standard
// infrastructures) yields null.
func ByocConnectorObjectFromAPI(ctx context.Context, connector *api.ByocConnector) (types.Object, diag.Diagnostics) {
	if connector == nil {
		return types.ObjectNull(ByocConnectorAttrTypes()), nil
	}
	components := make([]ByocConnectorComponentModel, 0, len(connector.Components))
	for _, c := range connector.Components {
		components = append(components, ByocConnectorComponentModel{
			Name:            types.StringValue(c.Name),
			State:           types.StringValue(c.State),
			Version:         types.StringPointerValue(c.Version),
			LastHeartbeatAt: types.StringPointerValue(c.LastHeartbeatAt),
		})
	}
	var diags diag.Diagnostics
	componentList, d := types.ListValueFrom(ctx, types.ObjectType{AttrTypes: ByocConnectorComponentAttrTypes()}, components)
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(ByocConnectorAttrTypes()), diags
	}
	obj, d := types.ObjectValueFrom(ctx, ByocConnectorAttrTypes(), ByocConnectorModel{
		LastHeartbeatAt: types.StringPointerValue(connector.LastHeartbeatAt),
		PlatformState:   types.StringPointerValue(connector.PlatformState),
		PlatformDetail:  types.StringPointerValue(connector.PlatformDetail),
		Components:      componentList,
	})
	diags.Append(d...)
	return obj, diags
}

func stringListOrNull(ctx context.Context, values []string) (types.List, diag.Diagnostics) {
	if values == nil {
		return types.ListNull(types.StringType), nil
	}
	return types.ListValueFrom(ctx, types.StringType, values)
}

func knownStringPointer(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return v.ValueStringPointer()
}

func knownBoolPointer(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return v.ValueBoolPointer()
}

func nullValueOf(t attr.Type) attr.Value {
	switch t := t.(type) {
	case types.ListType:
		return types.ListNull(t.ElemType)
	case basetypes.BoolType:
		return types.BoolNull()
	default:
		return types.StringNull()
	}
}
