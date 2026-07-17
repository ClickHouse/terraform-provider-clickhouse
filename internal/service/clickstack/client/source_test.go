package client

import (
	"encoding/json"
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
