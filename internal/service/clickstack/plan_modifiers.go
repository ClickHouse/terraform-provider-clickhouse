package clickstack

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// canonicalizeJSON returns a stable form of a JSON document for semantic
// comparison: it decodes and re-encodes so object keys are emitted in sorted
// order and insignificant whitespace is removed. It strips no keys — the whole
// document is compared literally.
func canonicalizeJSON(s string) (string, error) {
	return canonicalizeJSONWith(s, nil)
}

// canonicalizeJSONWith is the shared canonicalizer: it decodes and re-encodes a
// JSON document (json.Marshal deep-sorts map keys), optionally applying strip to
// the decoded top-level value first — used to drop server-volatile keys before
// comparison. A nil strip compares the document literally.
func canonicalizeJSONWith(s string, strip func(any)) (string, error) { //nolint:forbidigo // generic JSON canonicalization needs dynamic typing
	// Dynamic typing is required: this canonicalizes arbitrary user-supplied
	// JSON whose schema is not fixed at this layer.
	var v any //nolint:forbidigo // generic JSON canonicalization needs dynamic typing
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return "", err
	}
	if strip != nil {
		strip(v)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// suppressEqualJSONDiff sets the planned value to the current state value when
// state and config canonicalize (via canon) to the same JSON. Shared by the
// generic and dashboard JSON plan modifiers. If either value is null/unknown or
// canonicalization fails, the default plan is left unchanged.
func suppressEqualJSONDiff(req planmodifier.StringRequest, resp *planmodifier.StringResponse, canon func(string) (string, error)) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	cs, err := canon(req.StateValue.ValueString())
	if err != nil {
		return
	}
	cc, err := canon(req.ConfigValue.ValueString())
	if err != nil {
		return
	}
	if cs == cc {
		resp.PlanValue = req.StateValue
	}
}

// rfc3339Equal reports whether two RFC3339 timestamp strings denote the same
// instant. Returns false when either side is not parseable.
func rfc3339Equal(a, b string) bool {
	ta, err := time.Parse(time.RFC3339, a)
	if err != nil {
		return false
	}
	tb, err := time.Parse(time.RFC3339, b)
	if err != nil {
		return false
	}
	return ta.Equal(tb)
}

// rfc3339EqualPlanModifier suppresses a spurious diff on an RFC3339 timestamp
// string attribute when the config and stored state denote the same instant
// (e.g. the server canonicalizes "2026-01-01T00:00:00Z" to
// "2026-01-01T00:00:00.000Z"). If either value is null/unknown or not parseable,
// the default plan is left unchanged.
type rfc3339EqualPlanModifier struct{}

func (m rfc3339EqualPlanModifier) Description(_ context.Context) string {
	return "Suppresses diffs when the config and stored state are the same RFC3339 instant."
}

func (m rfc3339EqualPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m rfc3339EqualPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if rfc3339Equal(req.StateValue.ValueString(), req.ConfigValue.ValueString()) {
		resp.PlanValue = req.StateValue
	}
}

// jsonEqualPlanModifier suppresses a spurious diff on a JSON-string attribute
// when the config and stored state are semantically equal (same JSON, possibly
// reformatted or with reordered object keys).
type jsonEqualPlanModifier struct{}

func (m jsonEqualPlanModifier) Description(_ context.Context) string {
	return "Suppresses diffs when the config and stored state are semantically-equal JSON."
}

func (m jsonEqualPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

// PlanModifyString suppresses the diff when state and config are semantically
// equal JSON (whole-document comparison, no key stripping).
func (m jsonEqualPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	suppressEqualJSONDiff(req, resp, canonicalizeJSON)
}
