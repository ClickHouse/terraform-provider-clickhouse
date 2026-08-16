package clickstack

import (
	"encoding/json"
	"fmt"
)

// The two aggregations the select-entry rules key off.
const (
	aggFnCount    = "count"
	aggFnQuantile = "quantile"
)

// dropInvalidSelectFields removes fields from a dashboard's select entries that
// the API exports but its own write schema rejects:
//
//	level:           only valid with the quantile aggregation
//	valueExpression: rejected with the count aggregation
//
// Both are leftovers from switching a tile's aggregation in the UI — the
// dashboard renders fine with them, and the API keeps returning them — but a
// body containing them fails /dashboards/validate ("tiles.8.config.select.0:
// Level can only be used with quantile aggregation function"), so an imported
// dashboard could not be planned without hand edits.
//
// It runs on the import path only: an authored body is the operator's to fix,
// and rewriting it here would hide a real mistake.
func dropInvalidSelectFields(body json.RawMessage) (json.RawMessage, error) {
	// Dynamic typing is required: the dashboard body is an arbitrary document
	// whose schema is not fixed at this layer.
	var doc any //nolint:forbidigo // generic JSON handling needs dynamic typing
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("drop invalid select fields: parse: %w", err)
	}

	// Walk the whole document rather than the tiles[].config.select path: an
	// aggFn identifies a select entry wherever it sits, so this holds if the
	// dashboard format nests one somewhere new.
	if !walkJSON(doc, fixSelectEntry) {
		return body, nil
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("drop invalid select fields: marshal: %w", err)
	}
	return out, nil
}

// fixSelectEntry drops the incompatible fields from a single select entry,
// reporting whether it changed anything. Objects without an aggFn are left
// alone.
func fixSelectEntry(obj map[string]any) bool { //nolint:forbidigo // generic JSON handling needs dynamic typing
	aggFn, ok := obj["aggFn"].(string)
	if !ok {
		return false
	}

	changed := false
	if _, has := obj["level"]; has && aggFn != aggFnQuantile {
		delete(obj, "level")
		changed = true
	}
	if _, has := obj["valueExpression"]; has && aggFn == aggFnCount {
		delete(obj, "valueExpression")
		changed = true
	}
	return changed
}

// walkJSON applies visit to every JSON object in v, depth first, and reports
// whether any visit changed something.
func walkJSON(v any, visit func(map[string]any) bool) bool { //nolint:forbidigo // generic JSON handling needs dynamic typing
	changed := false
	switch t := v.(type) {
	case map[string]any: //nolint:forbidigo // generic JSON handling needs dynamic typing
		if visit(t) {
			changed = true
		}
		for _, child := range t {
			if walkJSON(child, visit) {
				changed = true
			}
		}
	case []any: //nolint:forbidigo // generic JSON handling needs dynamic typing
		for _, child := range t {
			if walkJSON(child, visit) {
				changed = true
			}
		}
	}
	return changed
}
