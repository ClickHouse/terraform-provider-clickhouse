package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const sourcesPath = "/api/v2/sources"

// SourceFrom is the database/table location of a source. TableName is always
// serialized (not omitempty): the API requires the `tableName` key to be
// present for every kind, including metric sources, which locate tables via
// MetricTables and pass an empty string here.
type SourceFrom struct {
	DatabaseName string `json:"databaseName"`
	TableName    string `json:"tableName"`
}

// QuerySetting is a ClickHouse setting applied when querying a source.
type QuerySetting struct {
	Setting string `json:"setting"`
	Value   string `json:"value"`
}

// MetricTables maps metric data types to their table names. The
// exponential_histogram JSON key intentionally contains a space, matching the
// API contract.
type MetricTables struct {
	Gauge                *string `json:"gauge,omitempty"`
	Histogram            *string `json:"histogram,omitempty"`
	Sum                  *string `json:"sum,omitempty"`
	Summary              *string `json:"summary,omitempty"`
	ExponentialHistogram *string `json:"exponential histogram,omitempty"`
}

// HighlightedAttributeExpression is a highlighted attribute shown in trace/row views.
type HighlightedAttributeExpression struct {
	SQLExpression    string  `json:"sqlExpression"`
	LuceneExpression *string `json:"luceneExpression,omitempty"`
	Alias            *string `json:"alias,omitempty"`
}

// AggregatedColumn is a pre-aggregated column in a materialized view.
type AggregatedColumn struct {
	SourceColumn *string `json:"sourceColumn,omitempty"`
	AggFn        string  `json:"aggFn"`
	MVColumn     string  `json:"mvColumn"`
}

// MaterializedView configures a materialized view for query optimization.
// MinGranularity uses the API's short form (e.g. "5m", "15s").
type MaterializedView struct {
	DatabaseName      string             `json:"databaseName"`
	TableName         string             `json:"tableName"`
	DimensionColumns  string             `json:"dimensionColumns"`
	MinGranularity    string             `json:"minGranularity"`
	MinDate           *string            `json:"minDate,omitempty"`
	TimestampColumn   string             `json:"timestampColumn"`
	AggregatedColumns []AggregatedColumn `json:"aggregatedColumns"`
}

// MetadataMaterializedViews configures rollup tables for field/value discovery.
// Granularity uses the API's short form (e.g. "15m").
type MetadataMaterializedViews struct {
	KeyRollupTable string `json:"keyRollupTable"`
	KVRollupTable  string `json:"kvRollupTable"`
	Granularity    string `json:"granularity"`
}

// Source is a ClickStack source as returned by the v2 API. It is the union of
// all source kinds; kind-specific fields are pointers and omitted when nil.
type Source struct {
	ID         string     `json:"id,omitempty"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Connection string     `json:"connection"`
	From       SourceFrom `json:"from"`
	Section    *string    `json:"section,omitempty"`
	Disabled   *bool      `json:"disabled,omitempty"`

	QuerySettings            []QuerySetting `json:"querySettings,omitempty"`
	TimestampValueExpression string         `json:"timestampValueExpression"`

	DefaultTableSelectExpression      *string `json:"defaultTableSelectExpression,omitempty"`
	ServiceNameExpression             *string `json:"serviceNameExpression,omitempty"`
	SeverityTextExpression            *string `json:"severityTextExpression,omitempty"`
	BodyExpression                    *string `json:"bodyExpression,omitempty"`
	EventAttributesExpression         *string `json:"eventAttributesExpression,omitempty"`
	ResourceAttributesExpression      *string `json:"resourceAttributesExpression,omitempty"`
	DisplayedTimestampValueExpression *string `json:"displayedTimestampValueExpression,omitempty"`
	MetricSourceID                    *string `json:"metricSourceId,omitempty"`
	TraceSourceID                     *string `json:"traceSourceId,omitempty"`
	LogSourceID                       *string `json:"logSourceId,omitempty"`
	SessionSourceID                   *string `json:"sessionSourceId,omitempty"`
	TraceIDExpression                 *string `json:"traceIdExpression,omitempty"`
	SpanIDExpression                  *string `json:"spanIdExpression,omitempty"`
	ImplicitColumnExpression          *string `json:"implicitColumnExpression,omitempty"`
	KnownColumnsListExpression        *string `json:"knownColumnsListExpression,omitempty"`
	OrderByExpression                 *string `json:"orderByExpression,omitempty"`
	UseTextIndexForImplicitColumn     *string `json:"useTextIndexForImplicitColumn,omitempty"`

	DurationExpression        *string `json:"durationExpression,omitempty"`
	DurationPrecision         *int    `json:"durationPrecision,omitempty"`
	ParentSpanIDExpression    *string `json:"parentSpanIdExpression,omitempty"`
	SpanNameExpression        *string `json:"spanNameExpression,omitempty"`
	SpanKindExpression        *string `json:"spanKindExpression,omitempty"`
	SampleRateExpression      *string `json:"sampleRateExpression,omitempty"`
	StatusCodeExpression      *string `json:"statusCodeExpression,omitempty"`
	StatusMessageExpression   *string `json:"statusMessageExpression,omitempty"`
	SpanEventsValueExpression *string `json:"spanEventsValueExpression,omitempty"`

	MetricTables *MetricTables `json:"metricTables,omitempty"`

	HighlightedTraceAttributeExpressions []HighlightedAttributeExpression `json:"highlightedTraceAttributeExpressions,omitempty"`
	HighlightedRowAttributeExpressions   []HighlightedAttributeExpression `json:"highlightedRowAttributeExpressions,omitempty"`
	MaterializedViews                    []MaterializedView               `json:"materializedViews,omitempty"`
	MetadataMaterializedViews            *MetadataMaterializedViews       `json:"metadataMaterializedViews,omitempty"`
}

// sourceEnvelope wraps single-source API responses.
type sourceEnvelope struct {
	Data Source `json:"data"`
}

// sourceListEnvelope wraps source-list API responses.
type sourceListEnvelope struct {
	Data []Source `json:"data"`
}

// CreateSource creates a source and returns it as stored by the API. Any id in
// the input is ignored by the API.
func (c *Client) CreateSource(ctx context.Context, input Source) (*Source, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode source: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPost, sourcesPath, body)
	if err != nil {
		return nil, err
	}

	var resp sourceEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode source: %w", err)
	}
	return &resp.Data, nil
}

// GetSource fetches a source by ID. It returns an error wrapping ErrNotFound
// when the source does not exist.
func (c *Client) GetSource(ctx context.Context, id string) (*Source, error) {
	raw, err := c.do(ctx, http.MethodGet, sourcesPath+"/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}

	var resp sourceEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode source: %w", err)
	}
	return &resp.Data, nil
}

// ListSources fetches all sources for the authenticated team.
func (c *Client) ListSources(ctx context.Context) ([]Source, error) {
	raw, err := c.do(ctx, http.MethodGet, sourcesPath, nil)
	if err != nil {
		return nil, err
	}

	var resp sourceListEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode sources: %w", err)
	}
	return resp.Data, nil
}

// UpdateSource replaces a source by ID (the API PUT is a full replace) and
// returns the updated source. It returns an error wrapping ErrNotFound when the
// source does not exist.
func (c *Client) UpdateSource(ctx context.Context, id string, input Source) (*Source, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode source: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPut, sourcesPath+"/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}

	var resp sourceEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode source: %w", err)
	}
	return &resp.Data, nil
}

// DeleteSource deletes a source by ID. It returns an error wrapping ErrNotFound
// when the source does not exist.
func (c *Client) DeleteSource(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, sourcesPath+"/"+url.PathEscape(id), nil)
	return err
}
