package clickstack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = (*sourceResource)(nil)
	_ resource.ResourceWithConfigure   = (*sourceResource)(nil)
	_ resource.ResourceWithImportState = (*sourceResource)(nil)
)

// NewSourceResource is a helper to register the resource with the provider.
func NewSourceResource() resource.Resource {
	return &sourceResource{}
}

// sourceResource manages a ClickStack source in ClickStack.
type sourceResource struct {
	client *client.Client
}

// --- models ---

type sourceFromModel struct {
	DatabaseName types.String `tfsdk:"database_name"`
	TableName    types.String `tfsdk:"table_name"`
}

type querySettingModel struct {
	Setting types.String `tfsdk:"setting"`
	Value   types.String `tfsdk:"value"`
}

type metricTablesModel struct {
	Gauge                types.String `tfsdk:"gauge"`
	Histogram            types.String `tfsdk:"histogram"`
	Sum                  types.String `tfsdk:"sum"`
	Summary              types.String `tfsdk:"summary"`
	ExponentialHistogram types.String `tfsdk:"exponential_histogram"`
}

type highlightedAttrModel struct {
	SQLExpression    types.String `tfsdk:"sql_expression"`
	LuceneExpression types.String `tfsdk:"lucene_expression"`
	Alias            types.String `tfsdk:"alias"`
}

type aggregatedColumnModel struct {
	SourceColumn types.String `tfsdk:"source_column"`
	AggFn        types.String `tfsdk:"agg_fn"`
	MVColumn     types.String `tfsdk:"mv_column"`
}

type materializedViewModel struct {
	DatabaseName      types.String            `tfsdk:"database_name"`
	TableName         types.String            `tfsdk:"table_name"`
	DimensionColumns  types.String            `tfsdk:"dimension_columns"`
	MinGranularity    types.String            `tfsdk:"min_granularity"`
	MinDate           types.String            `tfsdk:"min_date"`
	TimestampColumn   types.String            `tfsdk:"timestamp_column"`
	AggregatedColumns []aggregatedColumnModel `tfsdk:"aggregated_columns"`
}

type metadataMVModel struct {
	KeyRollupTable types.String `tfsdk:"key_rollup_table"`
	KVRollupTable  types.String `tfsdk:"kv_rollup_table"`
	Granularity    types.String `tfsdk:"granularity"`
}

// sourceResourceModel maps the resource schema data. It is the flat union of
// all source kinds; kind-specific fields are null when not applicable.
type sourceResourceModel struct {
	ID         types.String     `tfsdk:"id"`
	Team       types.String     `tfsdk:"team"`
	Name       types.String     `tfsdk:"name"`
	Kind       types.String     `tfsdk:"kind"`
	Connection types.String     `tfsdk:"connection_id"`
	From       *sourceFromModel `tfsdk:"from"`
	Section    types.String     `tfsdk:"section"`
	Disabled   types.Bool       `tfsdk:"disabled"`

	QuerySettings            []querySettingModel `tfsdk:"query_settings"`
	TimestampValueExpression types.String        `tfsdk:"timestamp_value_expression"`

	DefaultTableSelectExpression      types.String `tfsdk:"default_table_select_expression"`
	ServiceNameExpression             types.String `tfsdk:"service_name_expression"`
	SeverityTextExpression            types.String `tfsdk:"severity_text_expression"`
	BodyExpression                    types.String `tfsdk:"body_expression"`
	EventAttributesExpression         types.String `tfsdk:"event_attributes_expression"`
	ResourceAttributesExpression      types.String `tfsdk:"resource_attributes_expression"`
	DisplayedTimestampValueExpression types.String `tfsdk:"displayed_timestamp_value_expression"`
	MetricSourceID                    types.String `tfsdk:"metric_source_id"`
	TraceSourceID                     types.String `tfsdk:"trace_source_id"`
	LogSourceID                       types.String `tfsdk:"log_source_id"`
	SessionSourceID                   types.String `tfsdk:"session_source_id"`
	TraceIDExpression                 types.String `tfsdk:"trace_id_expression"`
	SpanIDExpression                  types.String `tfsdk:"span_id_expression"`
	ImplicitColumnExpression          types.String `tfsdk:"implicit_column_expression"`
	KnownColumnsListExpression        types.String `tfsdk:"known_columns_list_expression"`
	OrderByExpression                 types.String `tfsdk:"order_by_expression"`
	UseTextIndexForImplicitColumn     types.String `tfsdk:"use_text_index_for_implicit_column"`

	DurationExpression        types.String `tfsdk:"duration_expression"`
	DurationPrecision         types.Int64  `tfsdk:"duration_precision"`
	ParentSpanIDExpression    types.String `tfsdk:"parent_span_id_expression"`
	SpanNameExpression        types.String `tfsdk:"span_name_expression"`
	SpanKindExpression        types.String `tfsdk:"span_kind_expression"`
	SampleRateExpression      types.String `tfsdk:"sample_rate_expression"`
	StatusCodeExpression      types.String `tfsdk:"status_code_expression"`
	StatusMessageExpression   types.String `tfsdk:"status_message_expression"`
	SpanEventsValueExpression types.String `tfsdk:"span_events_value_expression"`

	MetricTables *metricTablesModel `tfsdk:"metric_tables"`

	HighlightedTraceAttributeExpressions []highlightedAttrModel  `tfsdk:"highlighted_trace_attribute_expressions"`
	HighlightedRowAttributeExpressions   []highlightedAttrModel  `tfsdk:"highlighted_row_attribute_expressions"`
	MaterializedViews                    []materializedViewModel `tfsdk:"materialized_views"`
	MetadataMaterializedViews            *metadataMVModel        `tfsdk:"metadata_materialized_views"`
}

func (r *sourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_source"
}

// optStr is a reusable optional string attribute.
func optStr(desc string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Description: desc}
}

func (r *sourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ClickStack source (v2 sources API). A source ties a ClickHouse " +
			"connection to a table and describes how to read one kind of data (log, trace, metric, " +
			"session, or promql). The set of applicable fields depends on `kind`; the API validates " +
			"per-kind requirements and returns an error at apply time if a required field for the " +
			"chosen kind is missing.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:      true,
				Description:   "Identifier of the source.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID to manage this source under, sent as the `x-hdx-team` header. " +
					"Defaults to the API key's team. Only honored by multi-team (EE) deployments. " +
					"Changing this forces the source to be replaced.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			nameAttr: schema.StringAttribute{
				Required:    true,
				Description: "Display name for the source.",
			},
			"kind": schema.StringAttribute{
				Required:    true,
				Description: "Source kind: one of `log`, `trace`, `metric`, `session`, `promql`.",
			},
			"connection_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the ClickHouse connection used by this source.",
			},
			"from": schema.SingleNestedAttribute{
				Required:    true,
				Description: "Database and table location of the source data.",
				Attributes: map[string]schema.Attribute{
					"database_name": schema.StringAttribute{
						Required:    true,
						Description: "ClickHouse database name.",
					},
					"table_name": schema.StringAttribute{
						Optional: true,
						Description: "ClickHouse table name. Required for all kinds except `metric` " +
							"(which locates tables via `metric_tables`).",
					},
				},
			},
			"timestamp_value_expression": schema.StringAttribute{
				Required:    true,
				Description: "DateTime column or expression that is part of the table's primary key.",
			},
			"section": optStr("Optional grouping label used to organize sources in the source selector."),
			"disabled": schema.BoolAttribute{
				Optional:      true,
				Computed:      true,
				Description:   "When true, the source is hidden from source selectors in the UI. Defaults to false.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"query_settings": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Optional ClickHouse query settings applied when querying this source.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"setting": schema.StringAttribute{Required: true, Description: "ClickHouse setting name."},
						"value":   schema.StringAttribute{Required: true, Description: "Setting value."},
					},
				},
			},

			"default_table_select_expression":      optStr("Default columns selected in search results. Required for `log` and `trace`."),
			"service_name_expression":              optStr("Expression to extract the service name."),
			"severity_text_expression":             optStr("Expression to extract the severity/log level text (log)."),
			"body_expression":                      optStr("Expression to extract the log message body (log)."),
			"event_attributes_expression":          optStr("Expression to extract event-level attributes."),
			"resource_attributes_expression":       optStr("Expression to extract resource-level attributes. Required for `metric`."),
			"displayed_timestamp_value_expression": optStr("DateTime column used to display and order search results."),
			"metric_source_id":                     optStr("Correlated metric source ID."),
			"trace_source_id":                      optStr("Correlated trace source ID. Required for `session`."),
			"log_source_id":                        optStr("Correlated log source ID."),
			"session_source_id":                    optStr("Correlated session source ID (trace)."),
			"trace_id_expression":                  optStr("Expression to extract the trace ID."),
			"span_id_expression":                   optStr("Expression to extract the span ID."),
			"implicit_column_expression":           optStr("Column used for full-text search when no property is specified in a Lucene search."),
			"known_columns_list_expression":        optStr("For distributed tables with non-matching column sets: columns supported across all target tables."),
			"order_by_expression":                  optStr("Expression used to order rows."),
			"use_text_index_for_implicit_column":   optStr("Whether to use ClickHouse text indices for the implicit column: `auto`, `enabled`, or `disabled`."),

			"duration_expression": optStr("Expression to extract span duration. Required for `trace`."),
			"duration_precision": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				Description: "Number of decimal digits in the duration value (3=ms, 6=us, 9=ns). " +
					"Defaults to 3. Applies to `trace`.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"parent_span_id_expression":    optStr("Expression to extract the parent span ID. Required for `trace`."),
			"span_name_expression":         optStr("Expression to extract the span name. Required for `trace`."),
			"span_kind_expression":         optStr("Expression to extract the span kind. Required for `trace`."),
			"sample_rate_expression":       optStr("Expression to extract the trace sample rate."),
			"status_code_expression":       optStr("Expression to extract the span status code."),
			"status_message_expression":    optStr("Expression to extract the span status message."),
			"span_events_value_expression": optStr("Expression to extract span events."),

			"metric_tables": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Mapping of metric data types to table names (metric). At least one must be set.",
				Attributes: map[string]schema.Attribute{
					"gauge":                 optStr("Table containing gauge metrics data."),
					"histogram":             optStr("Table containing histogram metrics data."),
					"sum":                   optStr("Table containing sum metrics data."),
					"summary":               optStr("Table containing summary metrics data."),
					"exponential_histogram": optStr("Table containing exponential histogram metrics data."),
				},
			},

			"highlighted_trace_attribute_expressions": highlightedAttrsAttribute(
				"Attributes displayed in the trace view for the selected trace."),
			"highlighted_row_attribute_expressions": highlightedAttrsAttribute(
				"Attributes displayed in the row side panel for the selected row."),

			"materialized_views": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Materialized views for query optimization (log, trace).",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"database_name":     schema.StringAttribute{Required: true, Description: "Materialized view database name."},
						"table_name":        schema.StringAttribute{Required: true, Description: "Materialized view table name."},
						"dimension_columns": schema.StringAttribute{Required: true, Description: "Non-aggregated columns usable for filtering and grouping."},
						"min_granularity":   schema.StringAttribute{Required: true, Description: "Timestamp granularity in short form (e.g. `5m`, `15s`, `1h`, `1d`)."},
						"min_date":          optStr("Earliest date/time (RFC3339) for which the view contains data."),
						"timestamp_column":  schema.StringAttribute{Required: true, Description: "Timestamp column name."},
						"aggregated_columns": schema.ListNestedAttribute{
							Required:    true,
							Description: "Columns pre-aggregated by the materialized view.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"source_column": optStr("Source column name (required unless agg_fn is `count`)."),
									"agg_fn":        schema.StringAttribute{Required: true, Description: "Aggregation function (e.g. count, sum, avg)."},
									"mv_column":     schema.StringAttribute{Required: true, Description: "Materialized view column name."},
								},
							},
						},
					},
				},
			},

			"metadata_materialized_views": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Materialized views for fast field discovery and value autocomplete (log, trace).",
				Attributes: map[string]schema.Attribute{
					"key_rollup_table": schema.StringAttribute{Required: true, Description: "Table name for the key rollup (field discovery)."},
					"kv_rollup_table":  schema.StringAttribute{Required: true, Description: "Table name for the key-value rollup (value autocomplete)."},
					"granularity":      schema.StringAttribute{Required: true, Description: "Granularity of the rollup tables in short form (e.g. `15m`)."},
				},
			},
		},
	}
}

func highlightedAttrsAttribute(desc string) schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Optional:    true,
		Description: desc,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"sql_expression":    schema.StringAttribute{Required: true, Description: "SQL expression for the attribute."},
				"lucene_expression": optStr("Optional Lucene version of the SQL expression."),
				"alias":             optStr("Optional alias for the attribute."),
			},
		},
	}
}

func (r *sourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("expected *service.ProviderData, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}

	if providerData.ClickStack == nil {
		resp.Diagnostics.AddError("ClickStack not configured",
			"This resource requires ClickStack credentials. For self-hosted ClickStack, set clickstack_endpoint and "+
				"clickstack_api_key on the provider (or the CLICKSTACK_ENDPOINT / CLICKSTACK_API_KEY environment variables). "+
				"For ClickStack on ClickHouse Cloud, set clickstack_service_id (or CLICKSTACK_SERVICE_ID) together with "+
				"the ClickHouse Cloud credentials (organization_id, token_key, token_secret).")
		return
	}
	r.client = providerData.ClickStack
}

func (r *sourceResource) ValidateConfig(_ context.Context, _ resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_clickstack_source", &resp.Diagnostics)
}

func (r *sourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	src, err := r.client.WithTeam(plan.Team.ValueString()).CreateSource(ctx, plan.toClient())
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Source", err.Error())
		return
	}

	plan.applySource(src)
	tflog.Trace(ctx, "created source resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	src, err := r.client.WithTeam(state.Team.ValueString()).GetSource(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Source", err.Error())
		return
	}

	state.applySource(src)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *sourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	src, err := r.client.WithTeam(plan.Team.ValueString()).UpdateSource(ctx, plan.ID.ValueString(), plan.toClient())
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Source", err.Error())
		return
	}

	plan.applySource(src)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.WithTeam(state.Team.ValueString()).DeleteSource(ctx, state.ID.ValueString()); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting Source", err.Error())
	}
}

func (r *sourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Accept "<id>" (default team) or "<team>/<id>" for a non-default team.
	if team, id, ok := strings.Cut(req.ID, "/"); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team"), team)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// --- conversion helpers ---

// strPtr converts a types.String to *string, returning nil for null/unknown.
func optStringPtr(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	return v.ValueStringPointer()
}

// emptyToNull maps the empty string to a null types.String and any other value
// to itself. Used for fields the API always echoes as "" when unset.
func emptyToNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func (m *sourceResourceModel) toClient() client.Source {
	src := client.Source{
		Name:                     m.Name.ValueString(),
		Kind:                     m.Kind.ValueString(),
		Connection:               m.Connection.ValueString(),
		TimestampValueExpression: m.TimestampValueExpression.ValueString(),
		Section:                  optStringPtr(m.Section),

		DefaultTableSelectExpression:      optStringPtr(m.DefaultTableSelectExpression),
		ServiceNameExpression:             optStringPtr(m.ServiceNameExpression),
		SeverityTextExpression:            optStringPtr(m.SeverityTextExpression),
		BodyExpression:                    optStringPtr(m.BodyExpression),
		EventAttributesExpression:         optStringPtr(m.EventAttributesExpression),
		ResourceAttributesExpression:      optStringPtr(m.ResourceAttributesExpression),
		DisplayedTimestampValueExpression: optStringPtr(m.DisplayedTimestampValueExpression),
		MetricSourceID:                    optStringPtr(m.MetricSourceID),
		TraceSourceID:                     optStringPtr(m.TraceSourceID),
		LogSourceID:                       optStringPtr(m.LogSourceID),
		SessionSourceID:                   optStringPtr(m.SessionSourceID),
		TraceIDExpression:                 optStringPtr(m.TraceIDExpression),
		SpanIDExpression:                  optStringPtr(m.SpanIDExpression),
		ImplicitColumnExpression:          optStringPtr(m.ImplicitColumnExpression),
		KnownColumnsListExpression:        optStringPtr(m.KnownColumnsListExpression),
		OrderByExpression:                 optStringPtr(m.OrderByExpression),
		UseTextIndexForImplicitColumn:     optStringPtr(m.UseTextIndexForImplicitColumn),

		DurationExpression:        optStringPtr(m.DurationExpression),
		ParentSpanIDExpression:    optStringPtr(m.ParentSpanIDExpression),
		SpanNameExpression:        optStringPtr(m.SpanNameExpression),
		SpanKindExpression:        optStringPtr(m.SpanKindExpression),
		SampleRateExpression:      optStringPtr(m.SampleRateExpression),
		StatusCodeExpression:      optStringPtr(m.StatusCodeExpression),
		StatusMessageExpression:   optStringPtr(m.StatusMessageExpression),
		SpanEventsValueExpression: optStringPtr(m.SpanEventsValueExpression),
	}

	if m.From != nil {
		// TableName is always sent (empty string when unset): the API requires
		// the tableName key for every kind. Metric sources use "" here.
		src.From = client.SourceFrom{
			DatabaseName: m.From.DatabaseName.ValueString(),
			TableName:    m.From.TableName.ValueString(),
		}
	}

	// disabled and duration_precision are Optional+Computed: omit them when
	// unknown (Create) so the server applies its default rather than receiving
	// a spurious zero value.
	if !m.Disabled.IsNull() && !m.Disabled.IsUnknown() {
		src.Disabled = m.Disabled.ValueBoolPointer()
	}
	if !m.DurationPrecision.IsNull() && !m.DurationPrecision.IsUnknown() {
		p := int(m.DurationPrecision.ValueInt64())
		src.DurationPrecision = &p
	}

	for _, qs := range m.QuerySettings {
		src.QuerySettings = append(src.QuerySettings, client.QuerySetting{
			Setting: qs.Setting.ValueString(),
			Value:   qs.Value.ValueString(),
		})
	}

	if m.MetricTables != nil {
		src.MetricTables = &client.MetricTables{
			Gauge:                optStringPtr(m.MetricTables.Gauge),
			Histogram:            optStringPtr(m.MetricTables.Histogram),
			Sum:                  optStringPtr(m.MetricTables.Sum),
			Summary:              optStringPtr(m.MetricTables.Summary),
			ExponentialHistogram: optStringPtr(m.MetricTables.ExponentialHistogram),
		}
	}

	src.HighlightedTraceAttributeExpressions = toClientHighlighted(m.HighlightedTraceAttributeExpressions)
	src.HighlightedRowAttributeExpressions = toClientHighlighted(m.HighlightedRowAttributeExpressions)

	for _, mv := range m.MaterializedViews {
		cmv := client.MaterializedView{
			DatabaseName:     mv.DatabaseName.ValueString(),
			TableName:        mv.TableName.ValueString(),
			DimensionColumns: mv.DimensionColumns.ValueString(),
			MinGranularity:   mv.MinGranularity.ValueString(),
			MinDate:          optStringPtr(mv.MinDate),
			TimestampColumn:  mv.TimestampColumn.ValueString(),
		}
		for _, ac := range mv.AggregatedColumns {
			cmv.AggregatedColumns = append(cmv.AggregatedColumns, client.AggregatedColumn{
				SourceColumn: optStringPtr(ac.SourceColumn),
				AggFn:        ac.AggFn.ValueString(),
				MVColumn:     ac.MVColumn.ValueString(),
			})
		}
		src.MaterializedViews = append(src.MaterializedViews, cmv)
	}

	if m.MetadataMaterializedViews != nil {
		src.MetadataMaterializedViews = &client.MetadataMaterializedViews{
			KeyRollupTable: m.MetadataMaterializedViews.KeyRollupTable.ValueString(),
			KVRollupTable:  m.MetadataMaterializedViews.KVRollupTable.ValueString(),
			Granularity:    m.MetadataMaterializedViews.Granularity.ValueString(),
		}
	}

	return src
}

func toClientHighlighted(in []highlightedAttrModel) []client.HighlightedAttributeExpression {
	if in == nil {
		return nil
	}
	out := make([]client.HighlightedAttributeExpression, 0, len(in))
	for _, h := range in {
		out = append(out, client.HighlightedAttributeExpression{
			SQLExpression:    h.SQLExpression.ValueString(),
			LuceneExpression: optStringPtr(h.LuceneExpression),
			Alias:            optStringPtr(h.Alias),
		})
	}
	return out
}

// applySource copies the API representation into the model.
func (m *sourceResourceModel) applySource(src *client.Source) {
	m.ID = types.StringValue(src.ID)
	m.Name = types.StringValue(src.Name)
	m.Kind = types.StringValue(src.Kind)
	m.Connection = types.StringValue(src.Connection)
	m.TimestampValueExpression = types.StringValue(src.TimestampValueExpression)
	m.Section = types.StringPointerValue(src.Section)
	m.Disabled = types.BoolPointerValue(src.Disabled)

	m.From = &sourceFromModel{
		DatabaseName: types.StringValue(src.From.DatabaseName),
		// Collapse the always-sent "" back to null so a metric source that
		// omitted table_name stays null in state (no inconsistent-result error).
		TableName: emptyToNull(src.From.TableName),
	}

	m.DefaultTableSelectExpression = types.StringPointerValue(src.DefaultTableSelectExpression)
	m.ServiceNameExpression = types.StringPointerValue(src.ServiceNameExpression)
	m.SeverityTextExpression = types.StringPointerValue(src.SeverityTextExpression)
	m.BodyExpression = types.StringPointerValue(src.BodyExpression)
	m.EventAttributesExpression = types.StringPointerValue(src.EventAttributesExpression)
	m.ResourceAttributesExpression = types.StringPointerValue(src.ResourceAttributesExpression)
	m.DisplayedTimestampValueExpression = types.StringPointerValue(src.DisplayedTimestampValueExpression)
	m.MetricSourceID = types.StringPointerValue(src.MetricSourceID)
	m.TraceSourceID = types.StringPointerValue(src.TraceSourceID)
	m.LogSourceID = types.StringPointerValue(src.LogSourceID)
	m.SessionSourceID = types.StringPointerValue(src.SessionSourceID)
	m.TraceIDExpression = types.StringPointerValue(src.TraceIDExpression)
	m.SpanIDExpression = types.StringPointerValue(src.SpanIDExpression)
	m.ImplicitColumnExpression = types.StringPointerValue(src.ImplicitColumnExpression)
	m.KnownColumnsListExpression = types.StringPointerValue(src.KnownColumnsListExpression)
	m.OrderByExpression = types.StringPointerValue(src.OrderByExpression)
	m.UseTextIndexForImplicitColumn = types.StringPointerValue(src.UseTextIndexForImplicitColumn)

	m.DurationExpression = types.StringPointerValue(src.DurationExpression)
	if src.DurationPrecision != nil {
		m.DurationPrecision = types.Int64Value(int64(*src.DurationPrecision))
	} else {
		m.DurationPrecision = types.Int64Null()
	}
	m.ParentSpanIDExpression = types.StringPointerValue(src.ParentSpanIDExpression)
	m.SpanNameExpression = types.StringPointerValue(src.SpanNameExpression)
	m.SpanKindExpression = types.StringPointerValue(src.SpanKindExpression)
	m.SampleRateExpression = types.StringPointerValue(src.SampleRateExpression)
	m.StatusCodeExpression = types.StringPointerValue(src.StatusCodeExpression)
	m.StatusMessageExpression = types.StringPointerValue(src.StatusMessageExpression)
	m.SpanEventsValueExpression = types.StringPointerValue(src.SpanEventsValueExpression)

	if len(src.QuerySettings) > 0 {
		m.QuerySettings = make([]querySettingModel, 0, len(src.QuerySettings))
		for _, qs := range src.QuerySettings {
			m.QuerySettings = append(m.QuerySettings, querySettingModel{
				Setting: types.StringValue(qs.Setting),
				Value:   types.StringValue(qs.Value),
			})
		}
	} else {
		m.QuerySettings = nil
	}

	if src.MetricTables != nil {
		m.MetricTables = &metricTablesModel{
			Gauge:                types.StringPointerValue(src.MetricTables.Gauge),
			Histogram:            types.StringPointerValue(src.MetricTables.Histogram),
			Sum:                  types.StringPointerValue(src.MetricTables.Sum),
			Summary:              types.StringPointerValue(src.MetricTables.Summary),
			ExponentialHistogram: types.StringPointerValue(src.MetricTables.ExponentialHistogram),
		}
	} else {
		m.MetricTables = nil
	}

	m.HighlightedTraceAttributeExpressions = fromClientHighlighted(src.HighlightedTraceAttributeExpressions)
	m.HighlightedRowAttributeExpressions = fromClientHighlighted(src.HighlightedRowAttributeExpressions)

	if len(src.MaterializedViews) > 0 {
		m.MaterializedViews = make([]materializedViewModel, 0, len(src.MaterializedViews))
		for _, mv := range src.MaterializedViews {
			mvm := materializedViewModel{
				DatabaseName:     types.StringValue(mv.DatabaseName),
				TableName:        types.StringValue(mv.TableName),
				DimensionColumns: types.StringValue(mv.DimensionColumns),
				MinGranularity:   types.StringValue(mv.MinGranularity),
				MinDate:          types.StringPointerValue(mv.MinDate),
				TimestampColumn:  types.StringValue(mv.TimestampColumn),
			}
			for _, ac := range mv.AggregatedColumns {
				mvm.AggregatedColumns = append(mvm.AggregatedColumns, aggregatedColumnModel{
					SourceColumn: types.StringPointerValue(ac.SourceColumn),
					AggFn:        types.StringValue(ac.AggFn),
					MVColumn:     types.StringValue(ac.MVColumn),
				})
			}
			m.MaterializedViews = append(m.MaterializedViews, mvm)
		}
	} else {
		m.MaterializedViews = nil
	}

	if src.MetadataMaterializedViews != nil {
		m.MetadataMaterializedViews = &metadataMVModel{
			KeyRollupTable: types.StringValue(src.MetadataMaterializedViews.KeyRollupTable),
			KVRollupTable:  types.StringValue(src.MetadataMaterializedViews.KVRollupTable),
			Granularity:    types.StringValue(src.MetadataMaterializedViews.Granularity),
		}
	} else {
		m.MetadataMaterializedViews = nil
	}
}

func fromClientHighlighted(in []client.HighlightedAttributeExpression) []highlightedAttrModel {
	if len(in) == 0 {
		return nil
	}
	out := make([]highlightedAttrModel, 0, len(in))
	for _, h := range in {
		out = append(out, highlightedAttrModel{
			SQLExpression:    types.StringValue(h.SQLExpression),
			LuceneExpression: types.StringPointerValue(h.LuceneExpression),
			Alias:            types.StringPointerValue(h.Alias),
		})
	}
	return out
}
