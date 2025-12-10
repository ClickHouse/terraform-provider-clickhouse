//go:build alpha

package resource

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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
