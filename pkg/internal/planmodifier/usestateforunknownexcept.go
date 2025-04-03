package planmodifier

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// UseStateForUnknownExcept is a factory function for a plan modifier to be applied to Computed NestedObjects.
// The goal of this Plan Modifier is to keep some of the NestedObject fields as immutable (like it happens when using
// built-in UseStateForUnknown Plan Modifier) while leaving other fields modifiable.
// This modifiable fields are passed to this constructor.
func UseStateForUnknownExcept(fields map[string]map[string]attr.Type) planmodifier.Object {
	return useStateForUnknownExceptPlanModifier{
		fields: fields,
	}
}

// useStateForUnknownExceptPlanModifier implements the plan modifier.
type useStateForUnknownExceptPlanModifier struct {
	fields map[string]map[string]attr.Type
}

func (m useStateForUnknownExceptPlanModifier) Description(_ context.Context) string {
	return "The plan modifier for status attribute. It will apply useStateForUnknownModifier to all the nested attributes except the cluster_status attribute."
}

// MarkdownDescription returns a markdown description of the plan modifier.
func (m useStateForUnknownExceptPlanModifier) MarkdownDescription(_ context.Context) string {
	return "The plan modifier for status attribute. It will apply useStateForUnknownModifier to all the nested attributes except the cluster_status attribute."
}

// PlanModifyObject implements the plan modification logic.
func (m useStateForUnknownExceptPlanModifier) PlanModifyObject(ctx context.Context, req planmodifier.ObjectRequest, resp *planmodifier.ObjectResponse) {
	// Do nothing if there is no state value.
	if req.StateValue.IsNull() {
		return
	}

	// Do nothing if there is a known planned value.
	if !req.PlanValue.IsUnknown() {
		return
	}

	// Do nothing if there is an unknown configuration value, otherwise interpolation gets messed up.
	if req.ConfigValue.IsUnknown() {
		return
	}

	// Mark desired attributes as unknown.
	attributes := req.StateValue.Attributes()
	for name, attrTypes := range m.fields {
		attributes[name] = types.ObjectUnknown(attrTypes)
	}

	newStateValue, diag := basetypes.NewObjectValue(req.StateValue.AttributeTypes(ctx), attributes)

	resp.Diagnostics.Append(diag...)
	resp.PlanValue = newStateValue
}
