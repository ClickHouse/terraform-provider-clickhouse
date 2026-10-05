package clickstack

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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
	bptr := func(b bool) *bool { return &b }
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
		FilterSettings: &client.FilterSettings{
			DatabaseName: "otel",
			TableName:    "k8s_objects_dict",
			Columns: []client.FilterSettingColumn{
				{Name: "region", Label: ptr("Region"), AllowAll: bptr(false)},
				{Name: "namespace", AllowAll: bptr(true)},
			},
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
		"materialized_views", "query_settings", "filter_settings",
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

// TestSourceModel_LinksLeftUnsetAreNotManaged guards the split with
// clickhouse_clickstack_source_links: a link null in config must stay null in
// state whatever the server holds, or every plan would remove a link the links
// resource set. A link the config does set still reports drift.
func TestSourceModel_LinksLeftUnsetAreNotManaged(t *testing.T) {
	t.Parallel()

	m := sourceResourceModel{TraceSourceID: types.StringValue("t1")}
	m.applySource(&client.Source{Kind: "log", TraceSourceID: strPtr("t2"), MetricSourceID: strPtr("m1")})
	if !m.MetricSourceID.IsNull() {
		t.Errorf("metric_source_id = %v, want null: config does not set it", m.MetricSourceID)
	}
	if m.TraceSourceID.ValueString() != "t2" {
		t.Errorf("trace_source_id = %v, want the server's t2", m.TraceSourceID)
	}
}

// TestSourceResource_UpdateKeepsUnmanagedLinks guards that an update, which is
// a full replace, sends back the links this resource does not manage, and
// still clears a link removed from config.
func TestSourceResource_UpdateKeepsUnmanagedLinks(t *testing.T) {
	t.Parallel()

	var put client.Source
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = io.WriteString(w, `{"data":{"id":"t1","kind":"trace","name":"traces",`+
				`"logSourceId":"l1","sessionSourceId":"s1","metricSourceId":"m1"}}`)
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&put); err != nil {
				t.Errorf("decode PUT body: %v", err)
			}
			put.ID = "t1"
			_ = json.NewEncoder(w).Encode(map[string]any{"data": put})
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL, "test-key", srv.Client())
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	schResp := &fwresource.SchemaResponse{}
	NewSourceResource().Schema(context.Background(), fwresource.SchemaRequest{}, schResp)
	sch := schResp.Schema

	base := sourceResourceModel{
		ID:                       types.StringValue("t1"),
		Name:                     types.StringValue("traces"),
		Kind:                     types.StringValue("trace"),
		Connection:               types.StringValue("conn1"),
		From:                     &sourceFromModel{DatabaseName: types.StringValue("otel"), TableName: types.StringValue("otel_traces")},
		TimestampValueExpression: types.StringValue("Timestamp"),
		Disabled:                 types.BoolValue(false),
		DurationPrecision:        types.Int64Value(3),
	}
	// Prior state: metric_source_id was set inline and is now removed from
	// config; log and session are owned by the links resource.
	prior := base
	prior.MetricSourceID = types.StringValue("m1")

	state := tfsdk.State{Schema: sch}
	plan := tfsdk.State{Schema: sch}
	if diags := state.Set(context.Background(), &prior); diags.HasError() {
		t.Fatalf("set state: %s", diags)
	}
	if diags := plan.Set(context.Background(), &base); diags.HasError() {
		t.Fatalf("set plan: %s", diags)
	}

	resp := &fwresource.UpdateResponse{State: tfsdk.State{Schema: sch}}
	(&sourceResource{client: c}).Update(context.Background(), fwresource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: sch, Raw: plan.Raw},
		State: state,
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update: %s", resp.Diagnostics)
	}

	if put.LogSourceID == nil || *put.LogSourceID != "l1" || put.SessionSourceID == nil || *put.SessionSourceID != "s1" {
		t.Errorf("unmanaged links not sent back: log=%v session=%v", put.LogSourceID, put.SessionSourceID)
	}
	if put.MetricSourceID != nil {
		t.Errorf("metric_source_id = %v, want it cleared: it was removed from config", *put.MetricSourceID)
	}

	var got sourceResourceModel
	resp.Diagnostics.Append(resp.State.Get(context.Background(), &got)...)
	if !got.LogSourceID.IsNull() || !got.SessionSourceID.IsNull() {
		t.Errorf("unmanaged links leaked into state: log=%v session=%v", got.LogSourceID, got.SessionSourceID)
	}
}
