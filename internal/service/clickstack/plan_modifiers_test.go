package clickstack

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRFC3339Equal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, a, b string
		want       bool
	}{
		{"identical", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", true},
		{"same instant with milliseconds", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00.000Z", true},
		{"same instant different offset", "2026-01-01T00:00:00Z", "2026-01-01T01:00:00+01:00", true},
		{"different instant", "2026-01-01T00:00:00Z", "2026-01-01T00:00:01Z", false},
		{"left unparseable", "not-a-time", "2026-01-01T00:00:00Z", false},
		{"right unparseable", "2026-01-01T00:00:00Z", "not-a-time", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := rfc3339Equal(tc.a, tc.b); got != tc.want {
				t.Errorf("rfc3339Equal(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestRFC3339EqualPlanModifier(t *testing.T) {
	t.Parallel()
	str := types.StringValue
	cases := []struct {
		name         string
		state, cfg   types.String
		wantSuppress bool // true => plan set to state (no diff)
	}{
		{"same instant with milliseconds", str("2026-01-01T00:00:00.000Z"), str("2026-01-01T00:00:00Z"), true},
		{"identical", str("2026-01-01T00:00:00Z"), str("2026-01-01T00:00:00Z"), true},
		{"different instant", str("2026-01-01T00:00:00Z"), str("2026-01-01T00:00:01Z"), false},
		{"null state", types.StringNull(), str("2026-01-01T00:00:00Z"), false},
		{"unknown config", str("2026-01-01T00:00:00Z"), types.StringUnknown(), false},
		{"null config", str("2026-01-01T00:00:00Z"), types.StringNull(), false},
		{"unparseable leaves plan unchanged", str("garbage"), str("2026-01-01T00:00:00Z"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Default plan value is the config value; suppression rewrites it to state.
			resp := planmodifier.StringResponse{PlanValue: tc.cfg}
			rfc3339EqualPlanModifier{}.PlanModifyString(context.Background(),
				planmodifier.StringRequest{StateValue: tc.state, ConfigValue: tc.cfg}, &resp)
			suppressed := resp.PlanValue.Equal(tc.state)
			if suppressed != tc.wantSuppress {
				t.Errorf("suppress=%v, want %v (plan=%v)", suppressed, tc.wantSuppress, resp.PlanValue)
			}
		})
	}
}

func TestJSONEqualPlanModifier(t *testing.T) {
	t.Parallel()
	str := types.StringValue
	cases := []struct {
		name         string
		state, cfg   types.String
		wantSuppress bool // true => plan set to state (no diff)
	}{
		{"equal after key reorder", str(`{"a":1,"b":2}`), str(`{"b":2,"a":1}`), true},
		{"equal after reformat", str(`[{"type":"sql","expr":"x"}]`), str("[ {\n  \"expr\": \"x\", \"type\": \"sql\" } ]"), true},
		{"genuine change", str(`[{"type":"sql"}]`), str(`[{"type":"lucene"}]`), false},
		{"null state", types.StringNull(), str(`[]`), false},
		{"unknown config", str(`[]`), types.StringUnknown(), false},
		{"null config", str(`[]`), types.StringNull(), false},
		{"invalid JSON leaves plan unchanged", str(`[bad`), str(`[]`), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := planmodifier.StringResponse{PlanValue: tc.cfg}
			jsonEqualPlanModifier{}.PlanModifyString(context.Background(),
				planmodifier.StringRequest{StateValue: tc.state, ConfigValue: tc.cfg}, &resp)
			suppressed := resp.PlanValue.Equal(tc.state)
			if suppressed != tc.wantSuppress {
				t.Errorf("suppress=%v, want %v (plan=%v)", suppressed, tc.wantSuppress, resp.PlanValue)
			}
		})
	}
}
