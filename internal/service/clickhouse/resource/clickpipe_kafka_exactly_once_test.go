package resource

import (
	"context"
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

// buildKafkaDeliveryPlan returns a minimal IAM_ROLE-authenticated Kafka plan
// with the given create-only delivery settings.
func buildKafkaDeliveryPlan(exactlyOnce types.Bool, tombstoneMode types.String) models.ClickPipeResourceModel {
	kafkaAttrs := map[string]attr.Value{
		"ssh_key_resource_id":          types.StringNull(),
		"type":                         types.StringValue("msk"),
		"format":                       types.StringValue(api.ClickPipeJSONEachRowFormat),
		"brokers":                      types.StringValue("broker:9092"),
		"topics":                       types.StringValue("test-topic"),
		"consumer_group":               types.StringNull(),
		"offset":                       types.ObjectNull(models.ClickPipeKafkaOffsetModel{}.ObjectType().AttrTypes),
		"schema_registry":              types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes),
		"protobuf_schema":              types.StringNull(),
		"authentication":               types.StringValue(api.ClickPipeAuthenticationIAMRole),
		"credentials":                  types.ObjectNull(models.ClickPipeKafkaSourceCredentialsModel{}.ObjectType().AttrTypes),
		"iam_role":                     types.StringValue("arn:aws:iam::123456789012:role/MyRole"),
		"ca_certificate":               types.StringNull(),
		"reverse_private_endpoint_ids": types.ListNull(types.StringType),
		"exactly_once":                 exactlyOnce,
		"tombstone_mode":               tombstoneMode,
	}
	sourceModel := models.ClickPipeSourceModel{
		Kafka:         types.ObjectValueMust(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes, kafkaAttrs),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}
	return models.ClickPipeResourceModel{
		ID:            types.StringValue("test-pipe-id"),
		ServiceID:     types.StringValue("service-123"),
		Name:          types.StringValue("test-kafka-eos"),
		Scaling:       types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes),
		State:         types.StringNull(),
		Stopped:       types.BoolNull(),
		Source:        sourceModel.ObjectValue(),
		Destination:   types.ObjectNull(models.ClickPipeDestinationModel{}.ObjectType().AttrTypes),
		FieldMappings: types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType()),
		Settings:      types.DynamicNull(),
		TriggerResync: types.BoolNull(),
	}
}

func TestExtractSourceFromPlan_Kafka_ExactlyOnceEnabled(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaDeliveryPlan(types.BoolValue(true), types.StringNull())

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.NotNil(t, source.Kafka.ExactlyOnce)
	assert.True(t, *source.Kafka.ExactlyOnce)
}

func TestExtractSourceFromPlan_Kafka_ExactlyOnceNull(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaDeliveryPlan(types.BoolNull(), types.StringNull())

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.Nil(t, source.Kafka.ExactlyOnce)
}

// exactly_once is create-only: it must not be included in an update (PATCH) payload.
func TestExtractSourceFromPlan_Kafka_ExactlyOnceOmittedOnUpdate(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaDeliveryPlan(types.BoolValue(true), types.StringNull())

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.Nil(t, source.Kafka.ExactlyOnce)
}

func TestExtractSourceFromPlan_Kafka_TombstoneModeDelete(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaDeliveryPlan(types.BoolValue(true), types.StringValue(api.ClickPipeKafkaTombstoneModeDelete))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.NotNil(t, source.Kafka.TombstoneMode)
	assert.Equal(t, api.ClickPipeKafkaTombstoneModeDelete, *source.Kafka.TombstoneMode)
}

func TestExtractSourceFromPlan_Kafka_TombstoneModeNull(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaDeliveryPlan(types.BoolNull(), types.StringNull())

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.Nil(t, source.Kafka.TombstoneMode)
}

// tombstone_mode is create-only: it must not be included in an update (PATCH) payload.
func TestExtractSourceFromPlan_Kafka_TombstoneModeOmittedOnUpdate(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaDeliveryPlan(types.BoolValue(true), types.StringValue(api.ClickPipeKafkaTombstoneModeDelete))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.Nil(t, source.Kafka.TombstoneMode)
}

func TestClickPipeResource_syncClickPipeState_KafkaTombstoneMode(t *testing.T) {
	ctx := context.Background()
	state := models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("test-service-id"),
		Source:    types.ObjectNull(models.ClickPipeSourceModel{}.ObjectType().AttrTypes),
	}
	tombstoneMode := api.ClickPipeKafkaTombstoneModeDelete

	mc := minimock.NewController(t)
	apiClientMock := api.NewClientMock(mc).
		GetClickPipeMock.
		Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
		Return(&api.ClickPipe{
			ID:    "test-pipe-id",
			Name:  "test-pipe",
			State: api.ClickPipeRunningState,
			Source: api.ClickPipeSource{
				Kafka: &api.ClickPipeKafkaSource{
					Type:           api.ClickPipeKafkaSourceType,
					Format:         api.ClickPipeJSONEachRowFormat,
					Brokers:        "broker:9092",
					Topics:         "test-topic",
					Authentication: api.ClickPipeKafkaAuthenticationPlain,
					TombstoneMode:  &tombstoneMode,
				},
			},
			Destination: api.ClickPipeDestination{Database: "default"},
		}, nil)

	r := &ClickPipeResource{client: apiClientMock}
	assert.NoError(t, r.syncClickPipeState(ctx, &state))

	var sourceModel models.ClickPipeSourceModel
	assert.False(t, state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())
	var kafkaModel models.ClickPipeKafkaSourceModel
	assert.False(t, sourceModel.Kafka.As(ctx, &kafkaModel, basetypes.ObjectAsOptions{}).HasError())
	assert.Equal(t, api.ClickPipeKafkaTombstoneModeDelete, kafkaModel.TombstoneMode.ValueString())
}

func validateKafkaTombstoneConfig(t *testing.T, configModel models.ClickPipeResourceModel) diag.Diagnostics {
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
	kafkaTombstoneModeValidator{}.ValidateResource(ctx, resource.ValidateConfigRequest{Config: config}, validationResponse)

	return validationResponse.Diagnostics
}

func TestClickPipeResource_ValidatesKafkaTombstoneModeRequiresExactlyOnce(t *testing.T) {
	tests := []struct {
		name          string
		exactlyOnce   types.Bool
		tombstoneMode types.String
		wantError     bool
	}{
		{
			name:          "delete mode with exactly-once",
			exactlyOnce:   types.BoolValue(true),
			tombstoneMode: types.StringValue(api.ClickPipeKafkaTombstoneModeDelete),
		},
		{
			name:          "delete mode with exactly-once disabled",
			exactlyOnce:   types.BoolValue(false),
			tombstoneMode: types.StringValue(api.ClickPipeKafkaTombstoneModeDelete),
			wantError:     true,
		},
		{
			name:          "delete mode without exactly-once",
			exactlyOnce:   types.BoolNull(),
			tombstoneMode: types.StringValue(api.ClickPipeKafkaTombstoneModeDelete),
			wantError:     true,
		},
		{
			name:          "unknown exactly-once is deferred",
			exactlyOnce:   types.BoolUnknown(),
			tombstoneMode: types.StringValue(api.ClickPipeKafkaTombstoneModeDelete),
		},
		{
			name:          "tombstone mode omitted",
			exactlyOnce:   types.BoolNull(),
			tombstoneMode: types.StringNull(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := validateKafkaTombstoneConfig(t, buildKafkaDeliveryPlan(test.exactlyOnce, test.tombstoneMode))
			assert.Equal(t, test.wantError, diagnostics.HasError())
			if test.wantError {
				assert.Contains(t, diagnostics.Errors()[0].Detail(), "tombstone_mode requires exactly_once = true")
			}
		})
	}
}

func TestClickPipeResource_RegistersKafkaTombstoneModeValidator(t *testing.T) {
	validators := (&ClickPipeResource{}).ConfigValidators(context.Background())

	assert.Contains(t, validators, kafkaTombstoneModeValidator{})
}
