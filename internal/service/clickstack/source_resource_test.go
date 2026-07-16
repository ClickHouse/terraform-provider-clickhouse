package clickstack

import (
	"context"
	"reflect"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

// TestSourceModel_RoundTrip locks the toClient/applySource mapping in both
// directions: a fully-populated client.Source mapped into the model and back
// must be unchanged. This is the CI-runnable guard for the mapping code, which
// the TF_ACC test cannot cover. Fields from several kinds are populated at once
// — the flat union schema maps them independently of kind.
func TestSourceModel_RoundTrip(t *testing.T) {
	t.Parallel()

	ptr := func(s string) *string { return &s }
	dp := 9
	disabled := true

	orig := client.Source{
		Name:                         "src",
		Kind:                         "trace",
		Connection:                   "conn1",
		From:                         client.SourceFrom{DatabaseName: "otel", TableName: "otel_traces"},
		Section:                      ptr("billing"),
		Disabled:                     &disabled,
		TimestampValueExpression:     "Timestamp",
		QuerySettings:                []client.QuerySetting{{Setting: "max_threads", Value: "4"}},
		DefaultTableSelectExpression: ptr("Timestamp, SpanName"),
		ServiceNameExpression:        ptr("ServiceName"),
		ResourceAttributesExpression: ptr("ResourceAttributes"),
		DurationExpression:           ptr("Duration"),
		DurationPrecision:            &dp,
		TraceIDExpression:            ptr("TraceId"),
		SpanIDExpression:             ptr("SpanId"),
		ParentSpanIDExpression:       ptr("ParentSpanId"),
		SpanNameExpression:           ptr("SpanName"),
		SpanKindExpression:           ptr("SpanKind"),
		MetricTables:                 &client.MetricTables{Gauge: ptr("g"), ExponentialHistogram: ptr("eh")},
		HighlightedTraceAttributeExpressions: []client.HighlightedAttributeExpression{
			{SQLExpression: "a", LuceneExpression: ptr("l"), Alias: ptr("al")},
		},
		HighlightedRowAttributeExpressions: []client.HighlightedAttributeExpression{
			{SQLExpression: "b"},
		},
		MaterializedViews: []client.MaterializedView{{
			DatabaseName:     "otel",
			TableName:        "mv",
			DimensionColumns: "ServiceName",
			MinGranularity:   "5m",
			MinDate:          ptr("2025-01-01T00:00:00Z"),
			TimestampColumn:  "Timestamp",
			AggregatedColumns: []client.AggregatedColumn{
				{SourceColumn: ptr("Duration"), AggFn: "sum", MVColumn: "sum__Duration"},
				{AggFn: "count", MVColumn: "count"},
			},
		}},
		MetadataMaterializedViews: &client.MetadataMaterializedViews{
			KeyRollupTable: "k", KVRollupTable: "kv", Granularity: "15m",
		},
	}

	var m sourceResourceModel
	m.applySource(&orig)
	got := m.toClient()

	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round-trip mismatch:\n orig = %+v\n got  = %+v", orig, got)
	}
}

func TestSourceResource_Schema(t *testing.T) {
	t.Parallel()

	r := NewSourceResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}

	for _, attr := range []string{
		"id", "team", "name", "kind", "connection_id", "from",
		"timestamp_value_expression", "duration_precision", "metric_tables",
		"materialized_views", "query_settings",
	} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected resource schema to contain attribute %q", attr)
		}
	}
}
