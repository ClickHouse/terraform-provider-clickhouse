package resource

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// requiresReplaceIfSourceTypeChanges is a custom plan modifier that requires replacement
// only when the source type changes (null → non-null or non-null → null), but allows
// updates to fields within the same source type.
type requiresReplaceIfSourceTypeChanges struct{}

func (r requiresReplaceIfSourceTypeChanges) Description(ctx context.Context) string {
	return "Requires replacement if the source type changes (e.g., switching from Kafka to Postgres)."
}

func (r requiresReplaceIfSourceTypeChanges) MarkdownDescription(ctx context.Context) string {
	return "Requires replacement if the source type changes (e.g., switching from Kafka to Postgres)."
}

func (r requiresReplaceIfSourceTypeChanges) PlanModifyObject(ctx context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	// If we're creating or destroying the entire resource, don't need to check
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	// Check if this source type attribute is transitioning between null and non-null
	stateIsNull := req.StateValue.IsNull()
	planIsNull := req.PlanValue.IsNull()

	// If transitioning from null to non-null or vice versa, this means the source type
	// is changing (e.g., kafka → postgres), so require replacement
	if stateIsNull != planIsNull {
		resp.RequiresReplace = true
	}

	// If both are non-null (values changing within same source type), no replacement needed
	// If both are null (staying null), no replacement needed
}

// planStateAttribute decides the planned `state` and must run as ModifyPlan's
// final step: the framework marks `state` Unknown whenever the proposed plan
// differs from prior state, even when ModifyPlan's repairs resolve the
// difference. Keep the prior value when nothing else changed so unchanged
// pipes plan as no-ops; otherwise leave it Unknown because an update may
// settle in a transient state (e.g., Snapshot).
func (c *ClickPipeResource) planStateAttribute(ctx context.Context, request resource.ModifyPlanRequest, response *resource.ModifyPlanResponse) {
	if request.State.Raw.IsNull() || request.Plan.Raw.IsNull() || response.Diagnostics.HasError() {
		return
	}

	var priorState types.String
	response.Diagnostics.Append(request.State.GetAttribute(ctx, path.Root("state"), &priorState)...)
	response.Diagnostics.Append(response.Plan.SetAttribute(ctx, path.Root("state"), priorState)...)
	if !response.Plan.Raw.Equal(request.State.Raw) {
		response.Diagnostics.Append(response.Plan.SetAttribute(ctx, path.Root("state"), types.StringUnknown())...)
	}
}
