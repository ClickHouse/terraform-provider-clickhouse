package planmodifier

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestUseStateWhenConfigNull(t *testing.T) {
	present := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	absent := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, nil)

	cases := []struct {
		name        string
		config      types.Bool
		state       types.Bool
		stateRaw    tftypes.Value
		planRaw     tftypes.Value
		planValue   types.Bool
		wantPlanned types.Bool
	}{
		{
			name:        "removed from config keeps the stored true",
			config:      types.BoolNull(),
			state:       types.BoolValue(true),
			stateRaw:    present,
			planRaw:     present,
			planValue:   types.BoolUnknown(),
			wantPlanned: types.BoolValue(true),
		},
		{
			name:        "removed from config keeps the stored false",
			config:      types.BoolNull(),
			state:       types.BoolValue(false),
			stateRaw:    present,
			planRaw:     present,
			planValue:   types.BoolUnknown(),
			wantPlanned: types.BoolValue(false),
		},
		{
			name:        "not set on create plans null instead of unknown",
			config:      types.BoolNull(),
			state:       types.BoolNull(),
			stateRaw:    absent,
			planRaw:     present,
			planValue:   types.BoolUnknown(),
			wantPlanned: types.BoolNull(),
		},
		{
			name:        "configured value is left alone",
			config:      types.BoolValue(false),
			state:       types.BoolValue(true),
			stateRaw:    present,
			planRaw:     present,
			planValue:   types.BoolValue(false),
			wantPlanned: types.BoolValue(false),
		},
		{
			name:        "existing resource with an unset attribute stays unset",
			config:      types.BoolNull(),
			state:       types.BoolNull(),
			stateRaw:    present,
			planRaw:     present,
			planValue:   types.BoolUnknown(),
			wantPlanned: types.BoolNull(),
		},
		{
			name:        "unknown configured value is left alone",
			config:      types.BoolUnknown(),
			state:       types.BoolValue(true),
			stateRaw:    present,
			planRaw:     present,
			planValue:   types.BoolUnknown(),
			wantPlanned: types.BoolUnknown(),
		},
		{
			name:        "destroy plan is left alone",
			config:      types.BoolNull(),
			state:       types.BoolValue(true),
			stateRaw:    present,
			planRaw:     absent,
			planValue:   types.BoolNull(),
			wantPlanned: types.BoolNull(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := planmodifier.BoolRequest{
				ConfigValue: tc.config,
				StateValue:  tc.state,
				PlanValue:   tc.planValue,
				State:       tfsdk.State{Raw: tc.stateRaw},
				Plan:        tfsdk.Plan{Raw: tc.planRaw},
			}
			resp := &planmodifier.BoolResponse{PlanValue: tc.planValue}

			UseStateWhenConfigNull().PlanModifyBool(context.Background(), req, resp)

			if !resp.PlanValue.Equal(tc.wantPlanned) {
				t.Errorf("planned value = %v; want %v", resp.PlanValue, tc.wantPlanned)
			}
		})
	}
}
