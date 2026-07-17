package clickstack

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// volatileDashboardKeys are server-owned fields that must not cause diffs. They
// are stripped from the top-level dashboard object only: the dashboard id is
// server-assigned (and tracked in the resource's separate id attribute), and
// timestamps never belong in an authored body. Nested ids are a different
// story — in the v2 dashboard format a tile's id is user-defined (matching ids
// on update preserve tile identity and alert bindings), container ids are
// required and referenced by tiles, and filter ids are part of the update
// schema — so nested objects keep every key and are compared literally.
var volatileDashboardKeys = map[string]bool{"id": true, "createdAt": true, "updatedAt": true}

// canonicalizeDashboardJSON returns a stable form of a dashboard body for diff
// comparison: server-owned keys (id/timestamps) are stripped from the
// top-level dashboard object, and all object keys are emitted in sorted order
// (json.Marshal sorts map[string]any keys). Stripping the top-level id keeps
// an imported dashboard (which carries it) from showing a perpetual plan diff,
// while nested objects — including tiles/filters/containers elements, whose
// ids are authored-meaningful — are left untouched so genuine edits are never
// dropped as no-ops.
func canonicalizeDashboardJSON(s string) (string, error) {
	// Shares the generic canonicalizer (see plan_modifiers.go), passing a strip
	// function that drops top-level server-volatile keys before comparison.
	out, err := canonicalizeJSONWith(s, stripVolatileKeys)
	if err != nil {
		return "", fmt.Errorf("canonicalize dashboard JSON: %w", err)
	}
	return out, nil
}

// stripVolatileKeys deletes server-owned keys from v when it is the top-level
// dashboard object. It deliberately does not recurse: an id anywhere below the
// root (a tile, container, or filter element, or a field inside a tile's
// config) is authored-meaningful, so it survives and its edits are not
// silently dropped.
func stripVolatileKeys(v any) { //nolint:forbidigo // generic JSON canonicalization needs dynamic typing
	if t, ok := v.(map[string]any); ok { //nolint:forbidigo // generic JSON canonicalization needs dynamic typing
		for k := range volatileDashboardKeys {
			delete(t, k)
		}
	}
}

// dashboardJSONPlanModifier suppresses spurious diffs on the dashboard_json
// attribute when the config and state are semantically equal (top-level
// server-assigned keys like id and timestamps aside; nested ids are
// authored-meaningful and compared literally).
type dashboardJSONPlanModifier struct{}

// Description returns a plain-text description of the modifier.
func (m dashboardJSONPlanModifier) Description(_ context.Context) string {
	return "Suppresses diffs on dashboard_json when the config and stored state are semantically equal after stripping top-level server-volatile keys."
}

// MarkdownDescription returns a markdown description of the modifier.
func (m dashboardJSONPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

// PlanModifyString suppresses the diff when state and config are semantically
// equal after stripping top-level server-volatile keys. Shares the plan-modify
// helper in plan_modifiers.go.
func (m dashboardJSONPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	suppressEqualJSONDiff(req, resp, canonicalizeDashboardJSON)
}
