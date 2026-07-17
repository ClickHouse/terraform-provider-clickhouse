package clickstack

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestCanonicalizeDashboardJSON_Error(t *testing.T) {
	t.Parallel()
	_, err := canonicalizeDashboardJSON("{not json")
	if err == nil {
		t.Error("expected non-nil error for invalid JSON input, got nil")
	}
}

func TestCanonicalizeDashboardJSON(t *testing.T) {
	t.Parallel()
	// key order differs; server-added top-level id/timestamps present in one form
	a, err := canonicalizeDashboardJSON(`{"name":"D","tiles":[{"name":"t","config":{}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicalizeDashboardJSON(`{"tiles":[{"config":{},"name":"t"}],"name":"D","id":"d1","updatedAt":"x"}`)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("expected canonical forms equal after stripping volatile keys:\n a=%s\n b=%s", a, b)
	}

	t.Run("createdAt stripped", func(t *testing.T) {
		t.Parallel()
		// Both createdAt and updatedAt must be stripped from the top-level object
		// so timestamps never cause diffs.
		withTimestamps, err := canonicalizeDashboardJSON(`{"name":"D","tiles":[{"name":"t","config":{}}],"createdAt":"y","updatedAt":"x"}`)
		if err != nil {
			t.Fatal(err)
		}
		without, err := canonicalizeDashboardJSON(`{"name":"D","tiles":[{"name":"t","config":{}}]}`)
		if err != nil {
			t.Fatal(err)
		}
		if withTimestamps != without {
			t.Errorf("createdAt/updatedAt not stripped:\n with=%s\n without=%s", withTimestamps, without)
		}
	})

	t.Run("nested tile/filter/container id edits not suppressed", func(t *testing.T) {
		t.Parallel()
		// In the v2 format ids inside tiles/filters/containers are authored and
		// meaningful (tile ids preserve alert bindings; container ids are
		// referenced by tiles), so editing or removing one must change the
		// canonical form — never be dropped as a no-op.
		cases := []struct {
			name          string
			before, after string
		}{
			{
				"tile id removed",
				`{"name":"D","tiles":[{"id":"t1","name":"t","config":{}}]}`,
				`{"name":"D","tiles":[{"name":"t","config":{}}]}`,
			},
			{
				"filter id changed",
				`{"name":"D","filters":[{"id":"f1","key":"env"}]}`,
				`{"name":"D","filters":[{"id":"f2","key":"env"}]}`,
			},
			{
				"container id changed",
				`{"name":"D","containers":[{"id":"c1","kind":"row"}]}`,
				`{"name":"D","containers":[{"id":"c2","kind":"row"}]}`,
			},
		}
		for _, tc := range cases {
			before, err := canonicalizeDashboardJSON(tc.before)
			if err != nil {
				t.Fatal(err)
			}
			after, err := canonicalizeDashboardJSON(tc.after)
			if err != nil {
				t.Fatal(err)
			}
			if before == after {
				t.Errorf("%s: nested id edit was suppressed: both canonicalize to %s", tc.name, before)
			}
		}
	})

	t.Run("authored id in config not stripped", func(t *testing.T) {
		t.Parallel()
		// "id" inside a tile's config is an authored field, not a server-assigned
		// node id. Changing only its value must produce a different canonical form
		// so the plan modifier does not drop the edit as a no-op.
		before, err := canonicalizeDashboardJSON(`{"name":"D","tiles":[{"name":"t","config":{"id":"a"}}]}`)
		if err != nil {
			t.Fatal(err)
		}
		after, err := canonicalizeDashboardJSON(`{"name":"D","tiles":[{"name":"t","config":{"id":"b"}}]}`)
		if err != nil {
			t.Fatal(err)
		}
		if before == after {
			t.Errorf("authored config.id change was suppressed: both canonicalize to %s", before)
		}
	})

	t.Run("genuine change not suppressed", func(t *testing.T) {
		t.Parallel()
		// Tiles whose config genuinely differs must produce different canonical forms
		// so the plan modifier does not hide real edits.
		line, err := canonicalizeDashboardJSON(`{"name":"D","tiles":[{"name":"t","config":{"displayType":"line"}}]}`)
		if err != nil {
			t.Fatal(err)
		}
		table, err := canonicalizeDashboardJSON(`{"name":"D","tiles":[{"name":"t","config":{"displayType":"table"}}]}`)
		if err != nil {
			t.Fatal(err)
		}
		if line == table {
			t.Errorf("genuine tile config change was suppressed: both canonicalize to %s", line)
		}
	})
}

func TestDashboardJSONPlanModifier(t *testing.T) {
	t.Parallel()
	str := types.StringValue
	cases := []struct {
		name         string
		state, cfg   types.String
		wantSuppress bool // true => plan set to state (no diff)
	}{
		{"equal after stripping volatile keys", str(`{"id":"d1","name":"D","updatedAt":"x"}`), str(`{"name":"D"}`), true},
		{"genuine change", str(`{"name":"D"}`), str(`{"name":"D2"}`), false},
		{"null state", types.StringNull(), str(`{"name":"D"}`), false},
		{"unknown config", str(`{"name":"D"}`), types.StringUnknown(), false},
		{"parse error leaves plan unchanged", str(`{bad`), str(`{"name":"D"}`), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Default plan value is the config value; suppression rewrites it to state.
			resp := planmodifier.StringResponse{PlanValue: tc.cfg}
			dashboardJSONPlanModifier{}.PlanModifyString(context.Background(),
				planmodifier.StringRequest{StateValue: tc.state, ConfigValue: tc.cfg}, &resp)
			suppressed := resp.PlanValue.Equal(tc.state)
			if suppressed != tc.wantSuppress {
				t.Errorf("suppress=%v, want %v (plan=%v)", suppressed, tc.wantSuppress, resp.PlanValue)
			}
		})
	}
}
