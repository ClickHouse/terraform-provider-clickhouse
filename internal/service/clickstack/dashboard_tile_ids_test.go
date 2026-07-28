package clickstack

import (
	"encoding/hex"
	"encoding/json"
	"testing"
)

// tileID extracts the id of the tile named name from a dashboard body, or "" if
// absent.
func tileID(t *testing.T, body json.RawMessage, name string) string {
	t.Helper()
	var doc struct {
		Tiles []map[string]any `json:"tiles"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, tile := range doc.Tiles {
		if n, _ := tile["name"].(string); n == name {
			id, _ := tile["id"].(string)
			return id
		}
	}
	return ""
}

// filterID extracts the id of the filter named name from a dashboard body.
func filterID(t *testing.T, body json.RawMessage, name string) string {
	t.Helper()
	var doc struct {
		Filters []map[string]any `json:"filters"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, f := range doc.Filters {
		if n, _ := f["name"].(string); n == name {
			id, _ := f["id"].(string)
			return id
		}
	}
	return ""
}

// TestMergeFilterIDs_NameMatch guards the Cloud update path: authored bodies
// omit filter ids (create rejects them) but update requires them, so the
// server-assigned id must be carried forward by name.
func TestMergeFilterIDs_NameMatch(t *testing.T) {
	t.Parallel()
	authored := json.RawMessage(`{"name":"D","filters":[{"name":"Machine","sourceId":"s1"}]}`)
	prior := json.RawMessage(`{"name":"D","filters":[{"id":"f-1","name":"Machine","sourceId":"s1"}]}`)

	merged, err := mergeFilterIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeFilterIDs: %v", err)
	}
	if got := filterID(t, merged, "Machine"); got != "f-1" {
		t.Errorf("filter Machine id = %q, want f-1", got)
	}
}

// TestMergeFilterIDs_NoFiltersUnchanged guards that a dashboard without filters
// (the common case) is passed through untouched, so create bodies stay id-free.
func TestMergeFilterIDs_NoFiltersUnchanged(t *testing.T) {
	t.Parallel()
	authored := json.RawMessage(`{"name":"D","tiles":[{"name":"A"}]}`)
	merged, err := mergeFilterIDs(authored, json.RawMessage(`{"name":"D","filters":[{"id":"f-1","name":"Machine"}]}`))
	if err != nil {
		t.Fatalf("mergeFilterIDs: %v", err)
	}
	if string(merged) != string(authored) {
		t.Errorf("expected authored body unchanged, got %s", merged)
	}
}

// isObjectID reports whether s is a 24-hex-character id (the shape the API
// requires on every filter of an update).
func isObjectID(s string) bool {
	if len(s) != 24 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// TestMergeFilterIDs_NewFilterGetsMintedID guards the add/rename-on-update case
// the Cloud API rejects without an id on every filter: a filter with no prior
// match must be minted a valid id rather than left id-less.
func TestMergeFilterIDs_NewFilterGetsMintedID(t *testing.T) {
	t.Parallel()
	// "Machine" exists in prior (carried forward); "Env" is new (must be minted).
	authored := json.RawMessage(`{"name":"D","filters":[{"name":"Machine","sourceId":"s1"},{"name":"Env","sourceId":"s1"}]}`)
	prior := json.RawMessage(`{"name":"D","filters":[{"id":"f-machine","name":"Machine","sourceId":"s1"}]}`)

	merged, err := mergeFilterIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeFilterIDs: %v", err)
	}
	if got := filterID(t, merged, "Machine"); got != "f-machine" {
		t.Errorf("Machine id = %q, want carried-forward f-machine", got)
	}
	env := filterID(t, merged, "Env")
	if !isObjectID(env) {
		t.Errorf("Env id = %q, want a minted 24-hex ObjectId", env)
	}
	if env == filterID(t, merged, "Machine") {
		t.Error("minted Env id collided with Machine id")
	}
}

// TestMergeFilterIDs_AllNewWhenNoPrior guards a first update after a create with
// no filters: with no prior filters array, every authored filter is minted an id.
func TestMergeFilterIDs_AllNewWhenNoPrior(t *testing.T) {
	t.Parallel()
	authored := json.RawMessage(`{"name":"D","filters":[{"name":"Machine","sourceId":"s1"}]}`)
	merged, err := mergeFilterIDs(authored, json.RawMessage(`{"name":"D"}`))
	if err != nil {
		t.Fatalf("mergeFilterIDs: %v", err)
	}
	if !isObjectID(filterID(t, merged, "Machine")) {
		t.Errorf("Machine id = %q, want a minted 24-hex ObjectId", filterID(t, merged, "Machine"))
	}
}

func TestMergeTileIDs_NameMatch(t *testing.T) {
	t.Parallel()
	authored := json.RawMessage(`{"name":"D","tiles":[{"name":"A"},{"name":"B"}]}`)
	prior := json.RawMessage(`{"name":"D","tiles":[{"id":"id-a","name":"A"},{"id":"id-b","name":"B"}]}`)

	merged, err := mergeTileIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	if got := tileID(t, merged, "A"); got != "id-a" {
		t.Errorf("tile A id = %q, want id-a", got)
	}
	if got := tileID(t, merged, "B"); got != "id-b" {
		t.Errorf("tile B id = %q, want id-b", got)
	}
}

func TestMergeTileIDs_Reordered(t *testing.T) {
	t.Parallel()
	// Author reorders the tiles; each must keep its own id via name match.
	authored := json.RawMessage(`{"tiles":[{"name":"B"},{"name":"A"}]}`)
	prior := json.RawMessage(`{"tiles":[{"id":"id-a","name":"A"},{"id":"id-b","name":"B"}]}`)

	merged, err := mergeTileIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	if got := tileID(t, merged, "A"); got != "id-a" {
		t.Errorf("reordered tile A id = %q, want id-a (index match would give id-b)", got)
	}
	if got := tileID(t, merged, "B"); got != "id-b" {
		t.Errorf("reordered tile B id = %q, want id-b", got)
	}
}

func TestMergeTileIDs_MiddleRemoved(t *testing.T) {
	t.Parallel()
	// Author removes tile B. Surviving A and C must keep THEIR ids — an index
	// match would shift C onto B's old id and orphan C's alert.
	authored := json.RawMessage(`{"tiles":[{"name":"A"},{"name":"C"}]}`)
	prior := json.RawMessage(`{"tiles":[{"id":"id-a","name":"A"},{"id":"id-b","name":"B"},{"id":"id-c","name":"C"}]}`)

	merged, err := mergeTileIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	if got := tileID(t, merged, "A"); got != "id-a" {
		t.Errorf("tile A id = %q, want id-a", got)
	}
	if got := tileID(t, merged, "C"); got != "id-c" {
		t.Errorf("tile C id = %q, want id-c (index fallback would wrongly give id-b)", got)
	}
}

func TestMergeTileIDs_AuthorPinnedIDUnchanged(t *testing.T) {
	t.Parallel()
	authored := json.RawMessage(`{"tiles":[{"id":"pinned","name":"A"}]}`)
	prior := json.RawMessage(`{"tiles":[{"id":"id-a","name":"A"}]}`)

	merged, err := mergeTileIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	if got := tileID(t, merged, "A"); got != "pinned" {
		t.Errorf("author-pinned id overwritten: got %q, want pinned", got)
	}
}

func TestMergeTileIDs_NewTileGetsNoID(t *testing.T) {
	t.Parallel()
	// A genuinely new tile (name not in prior) is left without an id so the
	// server assigns one.
	authored := json.RawMessage(`{"tiles":[{"name":"A"},{"name":"NEW"}]}`)
	prior := json.RawMessage(`{"tiles":[{"id":"id-a","name":"A"}]}`)

	merged, err := mergeTileIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	if got := tileID(t, merged, "A"); got != "id-a" {
		t.Errorf("tile A id = %q, want id-a", got)
	}
	if got := tileID(t, merged, "NEW"); got != "" {
		t.Errorf("new tile should have no id, got %q", got)
	}
}

func TestMergeTileIDs_DuplicateNamesFallBackToIndex(t *testing.T) {
	t.Parallel()
	authored := json.RawMessage(`{"tiles":[{"name":"dup"},{"name":"dup"}]}`)
	prior := json.RawMessage(`{"tiles":[{"id":"id-0","name":"dup"},{"id":"id-1","name":"dup"}]}`)

	merged, err := mergeTileIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	var doc struct {
		Tiles []map[string]any `json:"tiles"`
	}
	if err := json.Unmarshal(merged, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.Tiles[0]["id"] != "id-0" || doc.Tiles[1]["id"] != "id-1" {
		t.Errorf("duplicate names should index-match: got %v, %v", doc.Tiles[0]["id"], doc.Tiles[1]["id"])
	}
}

func TestMergeTileIDs_NoPriorTilesUnchanged(t *testing.T) {
	t.Parallel()
	authored := json.RawMessage(`{"tiles":[{"name":"A"}]}`)
	merged, err := mergeTileIDs(authored, json.RawMessage(`{"name":"D"}`))
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	if got := tileID(t, merged, "A"); got != "" {
		t.Errorf("expected no id merged when prior has no tiles, got %q", got)
	}
}

func TestMergeTileIDs_MalformedAuthoredErrors(t *testing.T) {
	t.Parallel()
	if _, err := mergeTileIDs(json.RawMessage(`{bad`), json.RawMessage(`{}`)); err == nil {
		t.Error("expected error for malformed authored JSON")
	}
}

// tileIDs returns the id of every tile in a dashboard body, in order (empty
// string for tiles without an id).
func tileIDs(t *testing.T, body json.RawMessage) []string {
	t.Helper()
	var doc struct {
		Tiles []map[string]any `json:"tiles"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ids := make([]string, len(doc.Tiles))
	for i, tile := range doc.Tiles {
		ids[i], _ = tile["id"].(string)
	}
	return ids
}

// assertNoDuplicateIDs fails if any non-empty id appears on more than one tile —
// a duplicate id in the payload lets the server bind two tiles to one id and
// delete the shadowed tile's alert.
func assertNoDuplicateIDs(t *testing.T, body json.RawMessage) {
	t.Helper()
	seen := map[string]bool{}
	for _, id := range tileIDs(t, body) {
		if id == "" {
			continue
		}
		if seen[id] {
			t.Errorf("duplicate tile id %q in merged body: %s", id, body)
		}
		seen[id] = true
	}
}

func TestMergeTileIDs_ReorderWithBlankNameNoDuplicate(t *testing.T) {
	t.Parallel()
	// Blank-named tile reordered ahead of a named one whose id it would collide
	// with via index fallback. The merge must not emit that id twice.
	authored := json.RawMessage(`{"tiles":[{"name":""},{"name":"A"}]}`)
	prior := json.RawMessage(`{"tiles":[{"id":"id-a","name":"A"},{"id":"id-b","name":""}]}`)

	merged, err := mergeTileIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	assertNoDuplicateIDs(t, merged)
}

func TestMergeTileIDs_AuthorDuplicateNamesNoDoubleStamp(t *testing.T) {
	t.Parallel()
	// Two authored tiles share a name that is unique in prior; only one may take
	// the prior id, the other is left id-less (server assigns fresh).
	authored := json.RawMessage(`{"tiles":[{"name":"X"},{"name":"X"}]}`)
	prior := json.RawMessage(`{"tiles":[{"id":"id-x","name":"X"}]}`)

	merged, err := mergeTileIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	assertNoDuplicateIDs(t, merged)
	ids := tileIDs(t, merged)
	got := 0
	for _, id := range ids {
		if id == "id-x" {
			got++
		}
	}
	if got != 1 {
		t.Errorf("expected id-x on exactly one tile, got %d (ids=%v)", got, ids)
	}
}

func TestMergeTileIDs_PinnedIDNotReusedByIndexFallback(t *testing.T) {
	t.Parallel()
	// An author-pinned id must not be re-stamped onto a blank-named tile via the
	// index fallback.
	authored := json.RawMessage(`{"tiles":[{"id":"id-a","name":"A"},{"name":""}]}`)
	prior := json.RawMessage(`{"tiles":[{"id":"id-a","name":"A"},{"id":"id-a","name":"B"}]}`)

	merged, err := mergeTileIDs(authored, prior)
	if err != nil {
		t.Fatalf("mergeTileIDs: %v", err)
	}
	assertNoDuplicateIDs(t, merged)
}

func TestMergeTileIDs_MalformedPriorIsNoOp(t *testing.T) {
	t.Parallel()
	authored := json.RawMessage(`{"tiles":[{"name":"A"}]}`)
	merged, err := mergeTileIDs(authored, json.RawMessage(`{bad`))
	if err != nil {
		t.Fatalf("expected no error on malformed prior, got %v", err)
	}
	if tileID(t, merged, "A") != "" {
		t.Error("expected no id merged from malformed prior")
	}
}
