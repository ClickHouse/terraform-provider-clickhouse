package clickstack

import (
	"encoding/json"
	"testing"
)

// TestDropInvalidSelectFields covers the bodies the API exports but rejects on
// write: a level carried over from a quantile aggregation, and a valueExpression
// left behind when a tile switched to count.
func TestDropInvalidSelectFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body, want string
	}{
		{
			name: "level dropped on non-quantile aggregation",
			body: `{"tiles":[{"config":{"select":[{"aggFn":"count","level":0.9,"alias":"p90"}]}}]}`,
			want: `{"tiles":[{"config":{"select":[{"aggFn":"count","alias":"p90"}]}}]}`,
		},
		{
			name: "level kept on quantile aggregation",
			body: `{"tiles":[{"config":{"select":[{"aggFn":"quantile","level":0.9,"valueExpression":"Duration"}]}}]}`,
			want: `{"tiles":[{"config":{"select":[{"aggFn":"quantile","level":0.9,"valueExpression":"Duration"}]}}]}`,
		},
		{
			name: "stale valueExpression dropped on count",
			body: `{"tiles":[{"config":{"select":[{"aggFn":"count","valueExpression":"Duration","alias":"2xx"}]}}]}`,
			want: `{"tiles":[{"config":{"select":[{"aggFn":"count","alias":"2xx"}]}}]}`,
		},
		{
			name: "empty valueExpression dropped on count",
			body: `{"tiles":[{"config":{"select":[{"aggFn":"count","valueExpression":""}]}}]}`,
			want: `{"tiles":[{"config":{"select":[{"aggFn":"count"}]}}]}`,
		},
		{
			name: "valueExpression kept on non-count aggregation",
			body: `{"tiles":[{"config":{"select":[{"aggFn":"sum","valueExpression":"Value"}]}}]}`,
			want: `{"tiles":[{"config":{"select":[{"aggFn":"sum","valueExpression":"Value"}]}}]}`,
		},
		{
			name: "select given as a plain string is untouched",
			body: `{"tiles":[{"config":{"select":"Timestamp, Body"}}]}`,
			want: `{"tiles":[{"config":{"select":"Timestamp, Body"}}]}`,
		},
		{
			name: "select nested anywhere else is cleaned too",
			body: `{"tiles":[{"alert":{"select":[{"aggFn":"count","level":0.5}]}}]}`,
			want: `{"tiles":[{"alert":{"select":[{"aggFn":"count"}]}}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := dropInvalidSelectFields(json.RawMessage(tc.body))
			if err != nil {
				t.Fatalf("dropInvalidSelectFields: %v", err)
			}
			if !jsonEqual(t, got, json.RawMessage(tc.want)) {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDropInvalidSelectFieldsInvalidJSON(t *testing.T) {
	t.Parallel()
	if _, err := dropInvalidSelectFields(json.RawMessage(`{"tiles":`)); err == nil {
		t.Fatal("expected an error for a malformed body")
	}
}

// jsonEqual compares two JSON documents ignoring key order and formatting.
func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	ca, err := canonicalizeJSON(string(a))
	if err != nil {
		t.Fatalf("canonicalize %s: %v", a, err)
	}
	cb, err := canonicalizeJSON(string(b))
	if err != nil {
		t.Fatalf("canonicalize %s: %v", b, err)
	}
	return ca == cb
}
