package resource

import (
	"context"

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

// volatileComputedString preserves the prior state for refresh-only plans, but marks
// the planned value as Unknown whenever the resource is actually being updated. This
// is intended for server-driven fields like `state` that may transition through
// transient values (e.g., Snapshot during a table-mapping update on a CDC pipe, or
// Paused during a stopped=true toggle) which would otherwise trip Terraform's
// post-apply consistency check.
type volatileComputedString struct{}

func (v volatileComputedString) Description(ctx context.Context) string {
	return "Preserves the prior state during refresh; marks the attribute Unknown during updates so transient server-side values do not fail the consistency check."
}

func (v volatileComputedString) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v volatileComputedString) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// On create, leave the planned value as Unknown.
	if req.State.Raw.IsNull() {
		return
	}

	// On destroy, the framework supplies a null plan; nothing to do.
	if req.Plan.Raw.IsNull() {
		return
	}

	// Refresh-only: the entire planned resource matches the prior state. Use the
	// state value so plan output does not churn on every refresh.
	if req.State.Raw.Equal(req.Plan.Raw) {
		resp.PlanValue = req.StateValue
		return
	}

	// A real update is in flight. The server may transition this attribute to a
	// transient value (e.g., Snapshot, Paused) before settling, so mark it Unknown.
	resp.PlanValue = types.StringUnknown()
}
