package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// TestMetricTablesJSONKey guards the intentional space in the exponential
// histogram JSON key. The API expects "exponential histogram" (with a space);
// a rename to a conventional identifier would compile and pass every other test
// while silently breaking metric sources against the real API.
func TestMetricTablesJSONKey(t *testing.T) {
	t.Parallel()

	eh := "otel_metrics_exponential_histogram"
	b, err := json.Marshal(MetricTables{ExponentialHistogram: &eh})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"exponential histogram"`) {
		t.Errorf(`expected JSON to contain "exponential histogram" key, got: %s`, b)
	}
}

// TestSourceFromTableNameAlwaysSent guards that from.tableName is serialized
// even when empty — the API requires the key present for every kind, including
// metric sources that leave it blank.
func TestSourceFromTableNameAlwaysSent(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(Source{Name: "m", Kind: "metric", From: SourceFrom{DatabaseName: "otel"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"tableName":""`) {
		t.Errorf(`expected from.tableName to be sent as "", got: %s`, b)
	}
}

// TestSetSourceLinks_PreservesUnmodeledFields guards the reason SetSourceLinks
// works on raw JSON: the PUT is a full replace, so a field the Source struct
// does not model must be sent back as the GET returned it, or the API drops it.
func TestSetSourceLinks_PreservesUnmodeledFields(t *testing.T) {
	t.Parallel()

	var put map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/sources/t1" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"data":{"id":"t1","kind":"trace","name":"traces",`+
				`"serviceVersionExpression":"ServiceVersion","metricSourceId":"m1","sessionSourceId":"s1"}}`)
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&put); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			_, _ = io.WriteString(w, `{"data":{"id":"t1","kind":"trace","name":"traces","logSourceId":"l1","metricSourceId":"m1"}}`)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})

	l1 := "l1"
	src, err := c.SetSourceLinks(context.Background(), "t1", map[string]*string{
		LinkKeyLogSource:     &l1,
		LinkKeySessionSource: nil,
	})
	if err != nil {
		t.Fatalf("SetSourceLinks: %v", err)
	}

	want := map[string]any{
		"id": "t1", "kind": "trace", "name": "traces",
		"serviceVersionExpression": "ServiceVersion",
		"metricSourceId":           "m1",
		"logSourceId":              "l1",
	}
	if !reflect.DeepEqual(put, want) {
		t.Errorf("PUT body = %v, want %v", put, want)
	}
	if src.LogSourceID == nil || *src.LogSourceID != "l1" {
		t.Errorf("returned logSourceId = %v, want l1", src.LogSourceID)
	}
}

// TestSetSourceLinks_RejectsKeyForKind guards that a link the kind does not
// support fails instead of being PUT: the API strips it silently, which would
// surface only as perpetual drift.
func TestSetSourceLinks_RejectsKeyForKind(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected %s: a rejected link must not be written", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"l1","kind":"log","name":"logs"}}`)
	})

	t1 := "t1"
	_, err := c.SetSourceLinks(context.Background(), "l1", map[string]*string{LinkKeyLogSource: &t1})
	if err == nil || !strings.Contains(err.Error(), `kind "log"`) {
		t.Fatalf("expected a kind error, got %v", err)
	}
}

func TestSetSourceLinks_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Source not found"}`)
	})

	if _, err := c.SetSourceLinks(context.Background(), "gone", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
