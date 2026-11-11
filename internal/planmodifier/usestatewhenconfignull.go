package planmodifier

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// UseStateWhenConfigNull plans an Optional+Computed bool as its prior state value when the attribute is
// absent from config, and as null on create, so removing the attribute never produces a diff or an
// "(known after apply)" value.
func UseStateWhenConfigNull() planmodifier.Bool {
	return useStateWhenConfigNullModifier{}
}

type useStateWhenConfigNullModifier struct{}

func (m useStateWhenConfigNullModifier) Description(_ context.Context) string {
	return "When the attribute is not set in configuration, the planned value is the prior state value (null on create)."
}

func (m useStateWhenConfigNullModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m useStateWhenConfigNullModifier) PlanModifyBool(_ context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	if !req.ConfigValue.IsNull() {
		return
	}

	// Resource is being destroyed.
	if req.Plan.Raw.IsNull() {
		return
	}

	if req.State.Raw.IsNull() {
		resp.PlanValue = types.BoolNull()
		return
	}

	resp.PlanValue = req.StateValue
}
