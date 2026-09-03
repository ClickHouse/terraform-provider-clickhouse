package clickstack

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// mergeTileIDs injects server-assigned tile IDs from a prior dashboard body
// (priorNormalized) into the authored dashboard body for tiles that omit an id.
//
// It exists to stop the dashboard resource from deleting tile alerts: the
// server's dashboard PUT deletes any tile alert whose tile id is absent from
// the payload, and it mints a fresh id for every tile that arrives without one
// or with one the server does not recognise. Tile ids therefore cannot be
// authored at all, so without this merge the ids churn on every apply and the
// alerts are collateral.
func mergeTileIDs(authored, priorNormalized json.RawMessage) (json.RawMessage, error) {
	return mergeArrayIDsByName(authored, priorNormalized, "tiles")
}

// mergeFilterIDs does the same for dashboard filters, then mints ids for any
// that remain id-less. It works around a hard API asymmetry on ClickHouse
// Cloud: POST /dashboards rejects a filter that carries an id, while PUT
// /dashboards/{id} *requires* one on every filter ("filters.N.id: Required on
// an update"). Authored bodies never carry filter ids (they would break
// create), so on update each filter's id must come from somewhere:
//   - an existing filter reuses the server-assigned id from the prior body;
//   - a newly added filter has none to carry, so a placeholder is minted.
//
// The server does not preserve the submitted id — it assigns its own in the
// response — so the carried/minted value only has to satisfy validation
// (present, valid ObjectId shape, unique within the payload).
func mergeFilterIDs(authored, priorNormalized json.RawMessage) (json.RawMessage, error) {
	merged, err := mergeArrayIDsByName(authored, priorNormalized, "filters")
	if err != nil {
		return nil, err
	}
	return mintMissingFilterIDs(merged)
}

// mintMissingFilterIDs assigns a freshly generated ObjectId to every filter
// that still lacks one, so a PUT that adds or renames a filter satisfies the
// Cloud API's "id required on update" rule. Filters that already carry an id
// (carried forward from the prior body) are left untouched. See mergeFilterIDs
// for why the value is throwaway.
func mintMissingFilterIDs(body json.RawMessage) (json.RawMessage, error) {
	var doc map[string]any //nolint:forbidigo // generic JSON handling needs dynamic typing
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("mint filter ids: parse: %w", err)
	}
	filters, ok := jsonArray(doc["filters"])
	if !ok {
		return body, nil
	}

	seen := map[string]bool{}
	for _, fe := range filters {
		if f, ok := fe.(map[string]any); ok { //nolint:forbidigo // generic JSON handling needs dynamic typing
			if id := elemString(f, "id"); id != "" {
				seen[id] = true
			}
		}
	}

	changed := false
	for _, fe := range filters {
		f, ok := fe.(map[string]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
		if !ok || elemString(f, "id") != "" {
			continue
		}
		id, err := newObjectID(seen)
		if err != nil {
			return nil, err
		}
		f["id"] = id
		seen[id] = true
		changed = true
	}

	if !changed {
		return body, nil
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("mint filter ids: marshal: %w", err)
	}
	return out, nil
}

// stripServerIDs drops the ids that a fetched dashboard body carries but the
// write path rejects: the dashboard's own "id" and the "id" on every filter.
// It is for the import path — a dashboard_json synthesized from a fetched body
// keeps neither, or every plan fails:
//
//	top-level id: the Cloud gateway 400s the request outright
//	              ("request body property can't be validated: id")
//	filter ids:   /dashboards/validate reports
//	              "filters.N: Unrecognized key(s) in object: 'id'"
//
// Tile ids stay: the write path accepts them and they keep UI-created tile
// alerts bound. Filter ids are re-injected on update from normalized_json via
// mergeFilterIDs, which the Cloud API requires there.
func stripServerIDs(body json.RawMessage) (json.RawMessage, error) {
	var doc map[string]any //nolint:forbidigo // generic JSON handling needs dynamic typing
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("strip server ids: parse: %w", err)
	}

	changed := false
	if _, has := doc["id"]; has {
		delete(doc, "id")
		changed = true
	}
	if filters, ok := jsonArray(doc["filters"]); ok {
		for _, fe := range filters {
			f, ok := fe.(map[string]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
			if !ok {
				continue
			}
			if _, has := f["id"]; has {
				delete(f, "id")
				changed = true
			}
		}
	}

	if !changed {
		return body, nil
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("strip server ids: marshal: %w", err)
	}
	return out, nil
}

// newObjectID returns a random 24-hex-character id (Mongo ObjectId shape, which
// is what the ClickStack API issues) that is not already in seen.
func newObjectID(seen map[string]bool) (string, error) {
	for range 4 {
		var b [12]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", fmt.Errorf("mint filter id: %w", err)
		}
		id := hex.EncodeToString(b[:])
		if !seen[id] {
			return id, nil
		}
	}
	return "", fmt.Errorf("mint filter id: could not generate a unique id")
}

// mergeArrayIDsByName injects server-assigned ids from a prior dashboard body
// into the authored body for elements of the array at key that omit an id.
//
// Matching is by element name first (which survives reordering and removal),
// falling back to array index only when a name is absent or non-unique in the
// prior body. An authored element whose name is present but not found in the
// prior body is treated as new (left without an id for the caller to handle).
//
// An id already on an authored element is honoured only when the prior body has
// that id too. The server discards an id it does not recognise and mints a new
// one, so an unrecognised authored id is dropped here and the element treated
// as id-less.
//
// If either body has no usable array under key, the authored body is returned
// unchanged.
func mergeArrayIDsByName(authored, priorNormalized json.RawMessage, key string) (json.RawMessage, error) {
	// Dynamic typing is required: the dashboard body is an arbitrary
	// user-supplied document whose schema is not fixed at this layer.
	var authoredDoc map[string]any //nolint:forbidigo // generic JSON handling needs dynamic typing
	if err := json.Unmarshal(authored, &authoredDoc); err != nil {
		return nil, fmt.Errorf("merge %s: parse authored body: %w", key, err)
	}

	authoredElems, ok := jsonArray(authoredDoc[key])
	if !ok {
		return authored, nil
	}

	var priorDoc map[string]any //nolint:forbidigo // generic JSON handling needs dynamic typing
	if err := json.Unmarshal(priorNormalized, &priorDoc); err != nil {
		// A missing/invalid prior body is not fatal — there is simply nothing to
		// merge from. Leave the authored body as-is.
		return authored, nil //nolint:nilerr // absent prior state means nothing to merge
	}
	priorElems, ok := jsonArray(priorDoc[key])
	if !ok {
		return authored, nil
	}

	// Index prior ids by name, tracking how many elements share each name so
	// non-unique names fall back to positional matching.
	idByName := map[string]string{}
	nameCount := map[string]int{}
	priorIDs := map[string]bool{}
	for _, pe := range priorElems {
		e, ok := pe.(map[string]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
		if !ok {
			continue
		}
		name := elemString(e, "name")
		id := elemString(e, "id")
		if id != "" {
			priorIDs[id] = true
		}
		if name == "" || id == "" {
			continue
		}
		nameCount[name]++
		idByName[name] = id
	}

	changed := false

	// An authored id the prior body does not have counts as absent: the server
	// discards an id it does not recognise and mints a fresh one, so keeping it
	// would only block the name match below from carrying the real id forward.
	for _, ae := range authoredElems {
		e, ok := ae.(map[string]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
		if !ok {
			continue
		}
		if id := elemString(e, "id"); id != "" && !priorIDs[id] {
			delete(e, "id")
			changed = true
		}
	}

	// consumed tracks prior ids already assigned in this merge, including the ids
	// the authored body kept above, so no id is ever placed on two elements. A
	// duplicate id in the payload would let the server bind two elements to one
	// id and delete the shadowed one's alert — the exact corruption this fix
	// exists to prevent. An element whose only candidate id is already consumed
	// is left id-less (a new element), and the server assigns it a fresh one.
	consumed := map[string]bool{}
	for _, ae := range authoredElems {
		if e, ok := ae.(map[string]any); ok { //nolint:forbidigo // generic JSON handling needs dynamic typing
			if id := elemString(e, "id"); id != "" {
				consumed[id] = true
			}
		}
	}

	for i, ae := range authoredElems {
		e, ok := ae.(map[string]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
		if !ok {
			continue
		}
		if elemString(e, "id") != "" {
			continue // an id the server knows; keep it
		}

		// Pick the candidate id: a unique name match, else the same-index prior id
		// for absent/ambiguous names. A name present but absent from the prior body
		// is a new or renamed element — no candidate, left id-less.
		var candidate string
		name := elemString(e, "name")
		switch {
		case name != "" && nameCount[name] == 1:
			candidate = idByName[name]
		case name == "" || nameCount[name] > 1:
			candidate = priorIDAt(priorElems, i)
		}

		if candidate != "" && !consumed[candidate] {
			e["id"] = candidate
			consumed[candidate] = true
			changed = true
		}
	}

	if !changed {
		return authored, nil
	}

	out, err := json.Marshal(authoredDoc)
	if err != nil {
		return nil, fmt.Errorf("merge %s: marshal: %w", key, err)
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

// elemString returns the string value of key k in element e, or "" when absent
// or not a string.
func elemString(e map[string]any, k string) string { //nolint:forbidigo // generic JSON handling needs dynamic typing
	s, _ := e[k].(string)
	return s
}

// priorIDAt returns the id of the prior element at index i, or "" when the
// index is out of range or the element has no string id.
func priorIDAt(priorElems []any, i int) string { //nolint:forbidigo // generic JSON handling needs dynamic typing
	if i >= len(priorElems) {
		return ""
	}
	e, ok := priorElems[i].(map[string]any) //nolint:forbidigo // generic JSON handling needs dynamic typing
	if !ok {
		return ""
	}
	return elemString(e, "id")
}
