package clickstack

import (
	"context"
	"reflect"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

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

// TestSourceModel_ApplySourceEmptyStrings covers the import path: the API echoes
// unset optional expressions back as "", and writing those into state made every
// plan show a no-op update — and, once a plan modifier tried to hide it, made
// Terraform reject the plan ("planned value for a non-computed attribute").
func TestSourceModel_ApplySourceEmptyStrings(t *testing.T) {
	t.Parallel()

	const eventAttrsExpr = "event_attributes_expression"

	ptr := func(s string) *string { return &s }
	src := client.Source{
		Name:                      "src",
		Kind:                      "log",
		Connection:                "conn1",
		From:                      client.SourceFrom{DatabaseName: "sysex", TableName: "query_log"},
		TimestampValueExpression:  "event_time",
		EventAttributesExpression: ptr(""),
		BodyExpression:            ptr(""),
		Section:                   ptr(""),
		MetricTables:              &client.MetricTables{Gauge: ptr("g"), Sum: ptr("")},
		HighlightedRowAttributeExpressions: []client.HighlightedAttributeExpression{
			{SQLExpression: "ServiceName", LuceneExpression: ptr(""), Alias: ptr("")},
		},
		MaterializedViews: []client.MaterializedView{{
			DatabaseName:     "sysex",
			TableName:        "mv",
			DimensionColumns: "database",
			MinGranularity:   "5m",
			MinDate:          ptr(""),
			TimestampColumn:  "event_time",
			AggregatedColumns: []client.AggregatedColumn{
				{SourceColumn: ptr(""), AggFn: "count", MVColumn: "count"},
			},
		}},
	}

	// Import/refresh with the attributes omitted from config: "" collapses to null.
	var imported sourceResourceModel
	imported.applySource(&src)
	for name, got := range map[string]types.String{
		eventAttrsExpr:      imported.EventAttributesExpression,
		"body_expression":   imported.BodyExpression,
		"section":           imported.Section,
		"metric_tables.sum": imported.MetricTables.Sum,
		// List-nested optional strings echo "" too, and the deleted plan
		// modifier used to cover them because it hung off the schema attribute.
		"highlighted_row.lucene_expression":  imported.HighlightedRowAttributeExpressions[0].LuceneExpression,
		"highlighted_row.alias":              imported.HighlightedRowAttributeExpressions[0].Alias,
		"materialized_views.min_date":        imported.MaterializedViews[0].MinDate,
		"aggregated_columns.0.source_column": imported.MaterializedViews[0].AggregatedColumns[0].SourceColumn,
	} {
		if !got.IsNull() {
			t.Errorf("%s = %v, want null", name, got)
		}
	}

	// An explicit "" in config is preserved, so apply stays consistent with plan.
	// A config that spells out "" keeps it, so apply stays consistent with plan.
	// The ClickStack export generates configs straight from the API, so an
	// explicit `alias = ""` really does show up in practitioners' files.
	authored := sourceResourceModel{
		From:                      &sourceFromModel{TableName: types.StringValue("")},
		EventAttributesExpression: types.StringValue(""),
		MetricTables:              &metricTablesModel{Sum: types.StringValue("")},
		HighlightedRowAttributeExpressions: []highlightedAttrModel{
			{SQLExpression: types.StringValue("ServiceName"), Alias: types.StringValue("")},
		},
	}
	emptyTable := src
	emptyTable.From = client.SourceFrom{DatabaseName: "sysex"}
	authored.applySource(&emptyTable)
	for name, got := range map[string]types.String{
		eventAttrsExpr:          authored.EventAttributesExpression,
		"metric_tables.sum":     authored.MetricTables.Sum,
		"highlighted_row.alias": authored.HighlightedRowAttributeExpressions[0].Alias,
		"from.table_name":       authored.From.TableName,
	} {
		if got.IsNull() || got.ValueString() != "" {
			t.Errorf("%s = %v, want an explicit empty string", name, got)
		}
	}
}

// TestSourceResource_OptionalAttributesPlanToConfig guards the bug class
// keepUnset exists to replace: an attribute that is Optional but not Computed
// must plan to exactly its config value, so a plan modifier that substitutes one
// makes Terraform reject the whole plan ("planned value ... for a non-computed
// attribute"). Only the acceptance tests exercise a real plan, so without this
// the next such modifier ships unnoticed. Modifiers that leave the value alone,
// like RequiresReplace, pass.
func TestSourceResource_OptionalAttributesPlanToConfig(t *testing.T) {
	t.Parallel()

	r := NewSourceResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	// The shape that broke: config omits the attribute, state holds the API's
	// echoed "". The plan must stay null.
	var check func(prefix string, attrs map[string]schema.Attribute)
	check = func(prefix string, attrs map[string]schema.Attribute) {
		for name, attr := range attrs {
			path := prefix + name
			if sa, ok := attr.(schema.StringAttribute); ok && sa.Optional && !sa.Computed {
				for _, pm := range sa.PlanModifiers {
					out := planmodifier.StringResponse{PlanValue: types.StringNull()}
					pm.PlanModifyString(context.Background(), planmodifier.StringRequest{
						ConfigValue: types.StringNull(),
						StateValue:  types.StringValue(""),
						PlanValue:   types.StringNull(),
					}, &out)
					if !out.PlanValue.IsNull() {
						t.Errorf("%s: %T planned %v against a null config; an optional, "+
							"non-computed attribute must plan to its config value "+
							"(normalize in applySource instead)", path, pm, out.PlanValue)
					}
				}
			}
			switch nested := attr.(type) {
			case schema.SingleNestedAttribute:
				check(path+".", nested.Attributes)
			case schema.ListNestedAttribute:
				check(path+".", nested.NestedObject.Attributes)
			}
		}
	}
	check("", resp.Schema.Attributes)
}
