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

type requiresReplaceIfSchemaRegistryChanges struct{}

func (r requiresReplaceIfSchemaRegistryChanges) Description(_ context.Context) string {
	return "Requires replacement if the schema registry changes (it is immutable after creation). " +
		"Credential differences are ignored while the state holds no credentials (fresh import)."
}

func (r requiresReplaceIfSchemaRegistryChanges) MarkdownDescription(ctx context.Context) string {
	return r.Description(ctx)
}

func (r requiresReplaceIfSchemaRegistryChanges) PlanModifyObject(_ context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	// If we're creating or destroying the entire resource, don't need to check
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}

	state, plan := req.StateValue, req.PlanValue
	// Never plan a destructive replacement on an unknown value, but warn
	if plan.IsUnknown() {
		addUnknownSchemaRegistryWarning(req.Path, resp)
		return
	}
	if state.IsNull() && plan.IsNull() {
		return
	}
	// Adding a registry to a pipe without one (or removing it) is an immutable change.
	if state.IsNull() != plan.IsNull() {
		resp.RequiresReplace = true
		return
	}

	stateAttrs, planAttrs := state.Attributes(), plan.Attributes()
	for name, stateVal := range stateAttrs {
		if name == "credentials" {
			continue
		}
		planVal, ok := planAttrs[name]
		if !ok {
			resp.RequiresReplace = true
			return
		}
		// A value not known until apply (e.g. url from another resource's output) can't
		// prove a change; warn rather than force a destructive replacement on a guess.
		if planVal.IsUnknown() {
			addUnknownSchemaRegistryWarning(req.Path, resp)
			return
		}
		if !planVal.Equal(stateVal) {
			resp.RequiresReplace = true
			return
		}
	}

	// Credentials: compare only once state holds them.
	// `terraform import` cannot read credentials, so the first post-import plan sees config
	// credentials against null state credentials; that difference alone must not force a replacement.
	stateCreds, ok := stateAttrs["credentials"].(types.Object)
	if !ok || stateCreds.IsNull() {
		return
	}
	planCreds, ok := planAttrs["credentials"].(types.Object)
	if !ok {
		return
	}
	if planCreds.IsUnknown() {
		addUnknownSchemaRegistryWarning(req.Path, resp)
		return
	}
	if credentialsObjectChanged(planCreds, stateCreds) {
		resp.RequiresReplace = true
	}
}

func addUnknownSchemaRegistryWarning(p path.Path, resp *planmodifier.ObjectResponse) {
	resp.Diagnostics.AddAttributeWarning(
		p,
		"Schema registry change cannot be determined",
		"The planned schema registry value is not known until apply, so the provider cannot tell "+
			"whether it differs from the pipe's immutable schema registry. Unknown values are recorded "+
			"in state but are never sent to the API, so the live pipe keeps its current schema registry. "+
			"To change the schema registry, recreate the pipe.",
	)
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
