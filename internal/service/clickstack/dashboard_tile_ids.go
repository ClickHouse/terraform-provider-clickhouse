package clickstack

import (
	"encoding/json"
	"fmt"
)

// mergeTileIDs injects server-assigned tile IDs from a prior dashboard body
// (priorNormalized) into the authored dashboard body for tiles that omit an id.
//
// It exists to stop the dashboard resource from silently deleting UI-created
// tile alerts: the server's dashboard PUT deletes any tile alert whose tile id
// is absent from the payload, and it mints a fresh id for every tile that
// arrives without one. Hand-authored dashboards omit tile ids, so without this
// merge the ids churn on every apply and the alerts are collateral.
//
// Matching is by tile name first (which survives reordering and tile removal),
// falling back to array index only when a name is absent or non-unique in the
// prior body. An authored tile whose name is present but not found in the prior
// body is treated as new (left without an id, so the server assigns one). If
// either body has no usable tiles array, the authored body is returned
// unchanged.
func mergeTileIDs(authored, priorNormalized json.RawMessage) (json.RawMessage, error) {
	// Dynamic typing is required: the dashboard body is an arbitrary
	// user-supplied document whose schema is not fixed at this layer.
	var authoredDoc map[string]any //nolint:forbidigo // generic JSON handling needs dynamic typing
	if err := json.Unmarshal(authored, &authoredDoc); err != nil {
		return nil, fmt.Errorf("merge tile ids: parse authored: %w", err)
	}

	authoredTiles, ok := jsonArray(authoredDoc["tiles"])
	if !ok {
		return authored, nil
	}

	var priorDoc map[string]any //nolint:forbidigo // generic JSON handling needs dynamic typing
	if err := json.Unmarshal(priorNormalized, &priorDoc); err != nil {
		// A missing/invalid prior body is not fatal — there is simply nothing to
		// merge from. Leave the authored body as-is.
		return authored, nil //nolint:nilerr // absent prior state means nothing to merge
	}
	priorTiles, ok := jsonArray(priorDoc["tiles"])
	if !ok {
		return authored, nil
	}

	// Index prior tile ids by name, tracking how many tiles share each name so
	// non-unique names fall back to positional matching.
	idByName := map[string]string{}
	nameCount := map[string]int{}
	for _, pt := range priorTiles {
		t, ok := pt.(map[string]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
		if !ok {
			continue
		}
		name := tileString(t, "name")
		id := tileString(t, "id")
		if name == "" || id == "" {
			continue
		}
		nameCount[name]++
		idByName[name] = id
	}

	// consumed tracks prior ids already assigned in this merge, including
	// author-pinned ids, so no id is ever placed on two tiles. A duplicate id in
	// the payload would let the server bind two tiles to one id and delete the
	// shadowed tile's alert — the exact corruption this fix exists to prevent.
	// A tile whose only candidate id is already consumed is left id-less (a new
	// tile), and the server assigns it a fresh one.
	consumed := map[string]bool{}
	for _, at := range authoredTiles {
		if t, ok := at.(map[string]any); ok { //nolint:forbidigo // generic JSON handling needs dynamic typing
			if id := tileString(t, "id"); id != "" {
				consumed[id] = true
			}
		}
	}

	changed := false
	for i, at := range authoredTiles {
		t, ok := at.(map[string]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
		if !ok {
			continue
		}
		if tileString(t, "id") != "" {
			continue // already pinned by the author
		}

		// Pick the candidate id: a unique name match, else the same-index prior id
		// for absent/ambiguous names. A name present but absent from the prior body
		// is a new or renamed tile — no candidate, left id-less.
		var candidate string
		name := tileString(t, "name")
		switch {
		case name != "" && nameCount[name] == 1:
			candidate = idByName[name]
		case name == "" || nameCount[name] > 1:
			candidate = priorTileIDAt(priorTiles, i)
		}

		if candidate != "" && !consumed[candidate] {
			t["id"] = candidate
			consumed[candidate] = true
			changed = true
		}
	}

	if !changed {
		return authored, nil
	}

	out, err := json.Marshal(authoredDoc)
	if err != nil {
		return nil, fmt.Errorf("merge tile ids: marshal: %w", err)
	}
	return out, nil
}

// jsonArray returns v as a []any when it is a non-empty JSON array.
func jsonArray(v any) ([]any, bool) { //nolint:forbidigo // generic JSON handling needs dynamic typing
	arr, ok := v.([]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
	if !ok || len(arr) == 0 {
		return nil, false
	}
	return arr, true
}

// tileString returns the string value of key k in tile t, or "" when absent or
// not a string.
func tileString(t map[string]any, k string) string { //nolint:forbidigo // generic JSON handling needs dynamic typing
	s, _ := t[k].(string)
	return s
}

// priorTileIDAt returns the id of the prior tile at index i, or "" when the
// index is out of range or the tile has no string id.
func priorTileIDAt(priorTiles []any, i int) string { //nolint:forbidigo // generic JSON handling needs dynamic typing
	if i >= len(priorTiles) {
		return ""
	}
	t, ok := priorTiles[i].(map[string]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
	if !ok {
		return ""
	}
	return tileString(t, "id")
}
