package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

type bigQueryCDCTestMapping struct {
	watermarkColumn types.String
	eventsFunction  types.String
}

func buildBigQueryCDCPlan(settings models.ClickPipeBigQuerySettingsModel, mappings ...bigQueryCDCTestMapping) models.ClickPipeResourceModel {
	mappingValues := make([]attr.Value, len(mappings))
	for i, m := range mappings {
		mappingValues[i] = models.ClickPipeBigQueryTableMappingModel{
			SourceDatasetName:       types.StringValue("dataset"),
			SourceTable:             types.StringValue("table"),
			TargetTable:             types.StringValue("target"),
			ExcludedColumns:         types.SetNull(types.StringType),
			UseCustomSortingKey:     types.BoolValue(false),
			SortingKeys:             types.ListNull(types.StringType),
			TableEngine:             types.StringNull(),
			QueryCDCWatermarkColumn: m.watermarkColumn,
			EventsFunction:          m.eventsFunction,
		}.ObjectValue()
	}

	bigQuery := models.ClickPipeBigQuerySourceModel{
		SnapshotStagingPath: types.StringValue("gs://staging-bucket/clickpipes/"),
		Authentication:      types.StringValue(api.ClickPipeAuthenticationServiceAccount),
		ProjectID:           types.StringNull(),
		Settings:            settings.ObjectValue(),
		TableMappings:       types.ListValueMust(models.ClickPipeBigQueryTableMappingModel{}.ObjectType(), mappingValues),
		Credentials: models.ClickPipeServiceAccountModel{
			ServiceAccountFile: types.StringValue("base64-key"),
		}.ObjectValue(),
	}
	sourceModel := models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      bigQuery.ObjectValue(),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-bigquery"),
		Source:    sourceModel.ObjectValue(),

		Scaling:       types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes),
		Destination:   types.ObjectNull(models.ClickPipeDestinationModel{}.ObjectType().AttrTypes),
		FieldMappings: types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType()),
		Settings:      types.DynamicNull(),
	}
}

func bigQuerySettings(mode string) models.ClickPipeBigQuerySettingsModel {
	return models.ClickPipeBigQuerySettingsModel{
		ReplicationMode:                types.StringValue(mode),
		ReplicationMethod:              types.StringNull(),
		SyncIntervalSeconds:            types.Int64Null(),
		QueryCDCPullSyncParallelism:    types.Int64Null(),
		SyncDelaySeconds:               types.Int64Null(),
		PullWindowSizeSeconds:          types.Int64Null(),
		AllowNullableColumns:           types.BoolValue(false),
		InitialLoadParallelism:         types.Int64Value(4),
		SnapshotNumRowsPerPartition:    types.Int64Value(100_000),
		SnapshotNumberOfParallelTables: types.Int64Value(1),
	}
}

func TestExtractSourceFromPlan_BigQuery_QueryBasedCDC(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	settings := bigQuerySettings(api.ClickPipeReplicationModeCDC)
	settings.ReplicationMethod = types.StringValue(api.ClickPipeBigQueryReplicationMethodQueryBased)
	settings.SyncIntervalSeconds = types.Int64Value(30)
	settings.SyncDelaySeconds = types.Int64Value(120)
	// computed attributes are unknown in the plan when omitted from the config
	settings.QueryCDCPullSyncParallelism = types.Int64Unknown()
	settings.PullWindowSizeSeconds = types.Int64Unknown()

	plan := buildBigQueryCDCPlan(settings, bigQueryCDCTestMapping{
		watermarkColumn: types.StringValue("updated_at"),
		eventsFunction:  types.StringNull(),
	})

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	require.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	require.NotNil(t, source.BigQuery)

	got := source.BigQuery.Settings
	assert.Equal(t, api.ClickPipeReplicationModeCDC, got.ReplicationMode)
	require.NotNil(t, got.ReplicationMethod)
	assert.Equal(t, api.ClickPipeBigQueryReplicationMethodQueryBased, *got.ReplicationMethod)
	require.NotNil(t, got.SyncIntervalSeconds)
	assert.Equal(t, 30, *got.SyncIntervalSeconds)
	require.NotNil(t, got.SyncDelaySeconds)
	assert.Equal(t, 120, *got.SyncDelaySeconds)
	assert.Nil(t, got.QueryCDCPullSyncParallelism)
	assert.Nil(t, got.PullWindowSizeSeconds)

	require.Len(t, source.BigQuery.Mappings, 1)
	require.NotNil(t, source.BigQuery.Mappings[0].QueryCDCWatermarkColumn)
	assert.Equal(t, "updated_at", *source.BigQuery.Mappings[0].QueryCDCWatermarkColumn)
	assert.Nil(t, source.BigQuery.Mappings[0].EventsFunction)
}

func TestExtractSourceFromPlan_BigQuery_EventsBasedCDCOnly(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	settings := bigQuerySettings(api.ClickPipeReplicationModeCDCOnly)
	settings.ReplicationMethod = types.StringValue(api.ClickPipeBigQueryReplicationMethodEventsBased)

	plan := buildBigQueryCDCPlan(settings, bigQueryCDCTestMapping{
		watermarkColumn: types.StringNull(),
		eventsFunction:  types.StringValue("changes"),
	})

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	require.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	require.NotNil(t, source.BigQuery)
	assert.Equal(t, api.ClickPipeReplicationModeCDCOnly, source.BigQuery.Settings.ReplicationMode)
	require.Len(t, source.BigQuery.Mappings, 1)
	assert.Nil(t, source.BigQuery.Mappings[0].QueryCDCWatermarkColumn)
	require.NotNil(t, source.BigQuery.Mappings[0].EventsFunction)
	assert.Equal(t, "changes", *source.BigQuery.Mappings[0].EventsFunction)
}

func TestExtractSourceFromPlan_BigQuery_SnapshotOmitsCDCFields(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	settings := bigQuerySettings(api.ClickPipeReplicationModeSnapshot)
	// computed attributes are unknown in the plan even for snapshot pipes
	settings.SyncIntervalSeconds = types.Int64Unknown()
	settings.QueryCDCPullSyncParallelism = types.Int64Unknown()
	settings.SyncDelaySeconds = types.Int64Unknown()
	settings.PullWindowSizeSeconds = types.Int64Unknown()

	plan := buildBigQueryCDCPlan(settings, bigQueryCDCTestMapping{
		watermarkColumn: types.StringNull(),
		eventsFunction:  types.StringNull(),
	})

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	require.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	got := source.BigQuery.Settings
	assert.Nil(t, got.ReplicationMethod)
	assert.Nil(t, got.SyncIntervalSeconds)
	assert.Nil(t, got.QueryCDCPullSyncParallelism)
	assert.Nil(t, got.SyncDelaySeconds)
	assert.Nil(t, got.PullWindowSizeSeconds)
}

func TestClickPipeResource_syncClickPipeState_BigQueryCDC(t *testing.T) {
	ctx := context.Background()
	state := models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("test-service-id"),
		Source:    types.ObjectNull(models.ClickPipeSourceModel{}.ObjectType().AttrTypes),
	}

	intPtr := func(v int) *int { return &v }
	strPtr := func(v string) *string { return &v }

	mc := minimock.NewController(t)
	apiClientMock := api.NewClientMock(mc).
		GetClickPipeMock.
		Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
		Return(&api.ClickPipe{
			ID:    "test-pipe-id",
			Name:  "test-pipe",
			State: api.ClickPipeRunningState,
			Source: api.ClickPipeSource{
				BigQuery: &api.ClickPipeBigQuerySource{
					SnapshotStagingPath: "gs://staging-bucket/clickpipes/",
					Authentication:      api.ClickPipeAuthenticationServiceAccount,
					Settings: api.ClickPipeBigQuerySettings{
						ReplicationMode:             api.ClickPipeReplicationModeCDC,
						ReplicationMethod:           strPtr(api.ClickPipeBigQueryReplicationMethodQueryBased),
						SyncIntervalSeconds:         intPtr(60),
						QueryCDCPullSyncParallelism: intPtr(4),
						SyncDelaySeconds:            intPtr(120),
						PullWindowSizeSeconds:       intPtr(86400),
					},
					Mappings: []api.ClickPipeBigQueryTableMapping{{
						SourceDatasetName:       "dataset",
						SourceTable:             "table",
						TargetTable:             "target",
						QueryCDCWatermarkColumn: strPtr("updated_at"),
					}},
				},
			},
			Destination: api.ClickPipeDestination{Database: "default"},
		}, nil)

	r := &ClickPipeResource{client: apiClientMock}
	require.NoError(t, r.syncClickPipeState(ctx, &state))

	var sourceModel models.ClickPipeSourceModel
	require.False(t, state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())
	var bigQueryModel models.ClickPipeBigQuerySourceModel
	require.False(t, sourceModel.BigQuery.As(ctx, &bigQueryModel, basetypes.ObjectAsOptions{}).HasError())
	var settings models.ClickPipeBigQuerySettingsModel
	require.False(t, bigQueryModel.Settings.As(ctx, &settings, basetypes.ObjectAsOptions{}).HasError())

	assert.Equal(t, api.ClickPipeReplicationModeCDC, settings.ReplicationMode.ValueString())
	assert.Equal(t, api.ClickPipeBigQueryReplicationMethodQueryBased, settings.ReplicationMethod.ValueString())
	assert.Equal(t, int64(60), settings.SyncIntervalSeconds.ValueInt64())
	assert.Equal(t, int64(4), settings.QueryCDCPullSyncParallelism.ValueInt64())
	assert.Equal(t, int64(120), settings.SyncDelaySeconds.ValueInt64())
	assert.Equal(t, int64(86400), settings.PullWindowSizeSeconds.ValueInt64())

	var mappings []models.ClickPipeBigQueryTableMappingModel
	require.False(t, bigQueryModel.TableMappings.ElementsAs(ctx, &mappings, false).HasError())
	require.Len(t, mappings, 1)
	assert.Equal(t, "updated_at", mappings[0].QueryCDCWatermarkColumn.ValueString())
	assert.True(t, mappings[0].EventsFunction.IsNull())
}

func TestClickPipeResource_syncClickPipeState_BigQuerySnapshotHasNoCDCFields(t *testing.T) {
	ctx := context.Background()
	state := models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("test-service-id"),
		Source:    types.ObjectNull(models.ClickPipeSourceModel{}.ObjectType().AttrTypes),
	}

	mc := minimock.NewController(t)
	apiClientMock := api.NewClientMock(mc).
		GetClickPipeMock.
		Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
		Return(&api.ClickPipe{
			ID:    "test-pipe-id",
			Name:  "test-pipe",
			State: api.ClickPipeCompletedState,
			Source: api.ClickPipeSource{
				BigQuery: &api.ClickPipeBigQuerySource{
					SnapshotStagingPath: "gs://staging-bucket/clickpipes/",
					Authentication:      api.ClickPipeAuthenticationServiceAccount,
					Settings:            api.ClickPipeBigQuerySettings{ReplicationMode: api.ClickPipeReplicationModeSnapshot},
					Mappings: []api.ClickPipeBigQueryTableMapping{{
						SourceDatasetName: "dataset",
						SourceTable:       "table",
						TargetTable:       "target",
					}},
				},
			},
			Destination: api.ClickPipeDestination{Database: "default"},
		}, nil)

	r := &ClickPipeResource{client: apiClientMock}
	require.NoError(t, r.syncClickPipeState(ctx, &state))

	var sourceModel models.ClickPipeSourceModel
	require.False(t, state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())
	var bigQueryModel models.ClickPipeBigQuerySourceModel
	require.False(t, sourceModel.BigQuery.As(ctx, &bigQueryModel, basetypes.ObjectAsOptions{}).HasError())
	var settings models.ClickPipeBigQuerySettingsModel
	require.False(t, bigQueryModel.Settings.As(ctx, &settings, basetypes.ObjectAsOptions{}).HasError())

	assert.True(t, settings.ReplicationMethod.IsNull())
	assert.True(t, settings.SyncIntervalSeconds.IsNull())
	assert.True(t, settings.QueryCDCPullSyncParallelism.IsNull())
	assert.True(t, settings.SyncDelaySeconds.IsNull())
	assert.True(t, settings.PullWindowSizeSeconds.IsNull())
}

func validateBigQueryCDCConfig(t *testing.T, configModel models.ClickPipeResourceModel) diag.Diagnostics {
	t.Helper()
	ctx := context.Background()
	clickPipeResource := &ClickPipeResource{}
	schemaResponse := &resource.SchemaResponse{}
	clickPipeResource.Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError(), "building resource schema failed: %v", schemaResponse.Diagnostics.Errors())

	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	setDiagnostics := plan.Set(ctx, &configModel)
	require.False(t, setDiagnostics.HasError(), "encoding config failed: %v", setDiagnostics.Errors())
	config := tfsdk.Config{Schema: schemaResponse.Schema, Raw: plan.Raw}
	validationResponse := &resource.ValidateConfigResponse{}
	bigQueryCDCValidator{}.ValidateResource(ctx, resource.ValidateConfigRequest{Config: config}, validationResponse)

	return validationResponse.Diagnostics
}

func TestClickPipeResource_ValidatesBigQueryCDCConfig(t *testing.T) {
	watermark := bigQueryCDCTestMapping{watermarkColumn: types.StringValue("updated_at"), eventsFunction: types.StringNull()}
	events := bigQueryCDCTestMapping{watermarkColumn: types.StringNull(), eventsFunction: types.StringValue("appends")}
	plain := bigQueryCDCTestMapping{watermarkColumn: types.StringNull(), eventsFunction: types.StringNull()}

	tests := []struct {
		name      string
		mode      string
		method    types.String
		tweak     func(*models.ClickPipeBigQuerySettingsModel)
		mappings  []bigQueryCDCTestMapping
		wantError string
	}{
		{
			name:     "snapshot without CDC fields",
			mode:     api.ClickPipeReplicationModeSnapshot,
			method:   types.StringNull(),
			mappings: []bigQueryCDCTestMapping{plain},
		},
		{
			name:      "snapshot rejects replication_method",
			mode:      api.ClickPipeReplicationModeSnapshot,
			method:    types.StringValue(api.ClickPipeBigQueryReplicationMethodQueryBased),
			mappings:  []bigQueryCDCTestMapping{plain},
			wantError: "replication_method must not be set",
		},
		{
			name:      "snapshot rejects sync_delay_seconds",
			mode:      api.ClickPipeReplicationModeSnapshot,
			method:    types.StringNull(),
			tweak:     func(s *models.ClickPipeBigQuerySettingsModel) { s.SyncDelaySeconds = types.Int64Value(10) },
			mappings:  []bigQueryCDCTestMapping{plain},
			wantError: "sync_delay_seconds must not be set",
		},
		{
			name:      "snapshot rejects watermark column",
			mode:      api.ClickPipeReplicationModeSnapshot,
			method:    types.StringNull(),
			mappings:  []bigQueryCDCTestMapping{watermark},
			wantError: "query_cdc_watermark_column must not be set",
		},
		{
			name:      "cdc requires replication_method",
			mode:      api.ClickPipeReplicationModeCDC,
			method:    types.StringNull(),
			mappings:  []bigQueryCDCTestMapping{plain},
			wantError: "replication_method is required",
		},
		{
			name:     "query based with watermark column",
			mode:     api.ClickPipeReplicationModeCDC,
			method:   types.StringValue(api.ClickPipeBigQueryReplicationMethodQueryBased),
			mappings: []bigQueryCDCTestMapping{watermark},
		},
		{
			name:      "query based requires watermark column",
			mode:      api.ClickPipeReplicationModeCDC,
			method:    types.StringValue(api.ClickPipeBigQueryReplicationMethodQueryBased),
			mappings:  []bigQueryCDCTestMapping{watermark, plain},
			wantError: "query_cdc_watermark_column is required",
		},
		{
			name:      "query based rejects events function",
			mode:      api.ClickPipeReplicationModeCDC,
			method:    types.StringValue(api.ClickPipeBigQueryReplicationMethodQueryBased),
			mappings:  []bigQueryCDCTestMapping{{watermarkColumn: types.StringValue("updated_at"), eventsFunction: types.StringValue("appends")}},
			wantError: "events_function is only allowed",
		},
		{
			name:     "events based cdc_only with events function",
			mode:     api.ClickPipeReplicationModeCDCOnly,
			method:   types.StringValue(api.ClickPipeBigQueryReplicationMethodEventsBased),
			mappings: []bigQueryCDCTestMapping{events},
		},
		{
			name:      "events based requires events function",
			mode:      api.ClickPipeReplicationModeCDC,
			method:    types.StringValue(api.ClickPipeBigQueryReplicationMethodEventsBased),
			mappings:  []bigQueryCDCTestMapping{plain},
			wantError: "events_function is required",
		},
		{
			name:      "events based rejects watermark column",
			mode:      api.ClickPipeReplicationModeCDC,
			method:    types.StringValue(api.ClickPipeBigQueryReplicationMethodEventsBased),
			mappings:  []bigQueryCDCTestMapping{{watermarkColumn: types.StringValue("updated_at"), eventsFunction: types.StringValue("appends")}},
			wantError: "query_cdc_watermark_column is only allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := bigQuerySettings(tt.mode)
			settings.ReplicationMethod = tt.method
			if tt.tweak != nil {
				tt.tweak(&settings)
			}

			diagnostics := validateBigQueryCDCConfig(t, buildBigQueryCDCPlan(settings, tt.mappings...))

			if tt.wantError == "" {
				assert.False(t, diagnostics.HasError(), "unexpected errors: %v", diagnostics.Errors())
				return
			}
			require.True(t, diagnostics.HasError(), "expected an error containing %q", tt.wantError)
			found := false
			for _, d := range diagnostics.Errors() {
				if strings.Contains(d.Detail(), tt.wantError) {
					found = true
				}
			}
			assert.True(t, found, "no error contains %q: %v", tt.wantError, diagnostics.Errors())
		})
	}
}
