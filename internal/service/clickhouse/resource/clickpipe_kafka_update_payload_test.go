package resource

import (
	"context"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func kafkaUpdateModel(caCertificate types.String, kafkaPassword string) models.ClickPipeResourceModel {
	mainCredAttrs := map[string]attr.Value{
		"username":            types.StringValue("kuser"),
		"password":            types.StringValue(kafkaPassword),
		"password_wo":         types.StringNull(),
		"password_wo_version": types.Int64Null(),
		"access_key_id":       types.StringNull(),
		"secret_key":          types.StringNull(),
		"connection_string":   types.StringNull(),
		"certificate":         types.StringNull(),
		"private_key":         types.StringNull(),
	}
	srCredAttrs := map[string]attr.Value{
		"username":            types.StringValue("sruser"),
		"password":            types.StringValue("sr-pass"),
		"password_wo":         types.StringNull(),
		"password_wo_version": types.Int64Null(),
	}
	srAttrs := map[string]attr.Value{
		"url":            types.StringValue("https://schema-registry.example.com"),
		"authentication": types.StringValue("PLAIN"),
		"credentials":    types.ObjectValueMust(models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes, srCredAttrs),
	}
	kafkaAttrs := map[string]attr.Value{
		"type":                         types.StringValue("kafka"),
		"format":                       types.StringValue("AvroConfluent"),
		"brokers":                      types.StringValue("broker:9092"),
		"topics":                       types.StringValue("test-topic"),
		"consumer_group":               types.StringValue("clickpipes-test"),
		"offset":                       types.ObjectNull(models.ClickPipeKafkaOffsetModel{}.ObjectType().AttrTypes),
		"schema_registry":              types.ObjectValueMust(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes, srAttrs),
		"protobuf_schema":              types.StringNull(),
		"authentication":               types.StringValue("PLAIN"),
		"credentials":                  types.ObjectValueMust(models.ClickPipeKafkaSourceCredentialsModel{}.ObjectType().AttrTypes, mainCredAttrs),
		"iam_role":                     types.StringNull(),
		"ca_certificate":               caCertificate,
		"reverse_private_endpoint_ids": types.ListNull(types.StringType),
		"exactly_once":                 types.BoolNull(),
	}
	src := models.ClickPipeSourceModel{
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
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-kafka-sr-pipe"),
		Scaling:   types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes),
		State:     types.StringValue(api.ClickPipeRunningState),
		Stopped:   types.BoolValue(false),
		Source:    src.ObjectValue(),
		Destination: types.ObjectValueMust(
			models.ClickPipeDestinationModel{}.ObjectType().AttrTypes,
			map[string]attr.Value{
				"database":         types.StringValue("default"),
				"table":            types.StringNull(),
				"managed_table":    types.BoolNull(),
				"table_definition": types.ObjectNull(models.ClickPipeDestinationTableDefinitionModel{}.ObjectType().AttrTypes),
				"columns":          types.ListNull(models.ClickPipeDestinationColumnModel{}.ObjectType()),
				"roles":            types.ListNull(types.StringType),
			},
		),
		FieldMappings: types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType()),
		Settings:      types.DynamicNull(),
		TriggerResync: types.BoolNull(),
	}
}

func TestClickPipeUpdate_KafkaOmitsImmutableFields(t *testing.T) {
	ctx := context.Background()
	state := kafkaUpdateModel(types.StringNull(), "main-pass")
	plan := kafkaUpdateModel(types.StringValue("PEM-CA"), "main-pass") // mutable-field change drives the source PATCH
	apiPipe := &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-pipe",
		State: api.ClickPipeRunningState,
		Source: api.ClickPipeSource{
			Kafka: &api.ClickPipeKafkaSource{
				Type:           "kafka",
				Format:         "AvroConfluent",
				Brokers:        "broker:9092",
				Topics:         "test-topic",
				Authentication: "PLAIN",
				SchemaRegistry: &api.ClickPipeKafkaSchemaRegistry{
					URL:            "https://schema-registry.example.com",
					Authentication: "PLAIN",
				},
			},
		},
		Destination: api.ClickPipeDestination{Database: "default"},
	}

	mc := minimock.NewController(t)
	var captured *api.ClickPipeUpdate
	mock := api.NewClientMock(mc)
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
		captured = &update
		return apiPipe, nil
	})
	mock.WaitForClickPipeStateMock.Set(func(_ context.Context, _, _ string, _ func(string) bool, _ time.Duration) (*api.ClickPipe, error) {
		return apiPipe, nil
	})
	mock.GetClickPipeMock.Set(func(_ context.Context, _, _ string) (*api.ClickPipe, error) {
		return apiPipe, nil
	})

	r := &ClickPipeResource{client: mock}
	resp := driveClickPipeUpdate(ctx, t, r, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update failed: %v", resp.Diagnostics.Errors())

	require.NotNil(t, captured, "UpdateClickPipe was not called")
	require.NotNil(t, captured.Source, "update payload carries no source")
	kafka := captured.Source.Kafka
	require.NotNil(t, kafka, "update payload carries no kafka source")

	assert.Nil(t, kafka.SchemaRegistry, "schema registry must never be sent on update")
	assert.Nil(t, kafka.ProtobufSchema, "protobuf schema must never be sent on update")
	assert.Nil(t, kafka.Credentials, "unchanged credentials must be omitted")
	assert.Empty(t, kafka.Type, "immutable type must be omitted")
	assert.Empty(t, kafka.Format, "immutable format must be omitted")
	assert.Empty(t, kafka.Brokers, "immutable brokers must be omitted")
	assert.Empty(t, kafka.Topics, "immutable topics must be omitted")
	assert.Nil(t, kafka.ConsumerGroup, "immutable consumer group must be omitted")
	assert.Nil(t, kafka.Offset, "immutable offset must be omitted")

	require.NotNil(t, kafka.CACertificate, "the mutable ca_certificate change must be carried")
	assert.Equal(t, "PEM-CA", *kafka.CACertificate)
}

// Tests a Kafka source credential rotation IS carried, while the immutable fields stay omitted.
func TestClickPipeUpdate_KafkaSendsChangedKafkaCredentials(t *testing.T) {
	ctx := context.Background()
	state := kafkaUpdateModel(types.StringNull(), "main-pass")
	plan := kafkaUpdateModel(types.StringNull(), "rotated-pass")
	apiPipe := &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-pipe",
		State: api.ClickPipeRunningState,
		Source: api.ClickPipeSource{
			Kafka: &api.ClickPipeKafkaSource{
				Type:           "kafka",
				Format:         "AvroConfluent",
				Brokers:        "broker:9092",
				Topics:         "test-topic",
				Authentication: "PLAIN",
				SchemaRegistry: &api.ClickPipeKafkaSchemaRegistry{
					URL:            "https://schema-registry.example.com",
					Authentication: "PLAIN",
				},
			},
		},
		Destination: api.ClickPipeDestination{Database: "default"},
	}

	mc := minimock.NewController(t)
	var captured *api.ClickPipeUpdate
	mock := api.NewClientMock(mc)
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
		captured = &update
		return apiPipe, nil
	})
	mock.WaitForClickPipeStateMock.Set(func(_ context.Context, _, _ string, _ func(string) bool, _ time.Duration) (*api.ClickPipe, error) {
		return apiPipe, nil
	})
	mock.GetClickPipeMock.Set(func(_ context.Context, _, _ string) (*api.ClickPipe, error) {
		return apiPipe, nil
	})

	r := &ClickPipeResource{client: mock}
	resp := driveClickPipeUpdate(ctx, t, r, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update failed: %v", resp.Diagnostics.Errors())

	require.NotNil(t, captured, "UpdateClickPipe was not called")
	require.NotNil(t, captured.Source, "update payload carries no source")
	kafka := captured.Source.Kafka
	require.NotNil(t, kafka, "update payload carries no kafka source")

	require.NotNil(t, kafka.Credentials, "rotated credentials must be carried")
	require.NotNil(t, kafka.Credentials.ClickPipeSourceCredentials, "rotated credentials must carry username/password")
	assert.Equal(t, "kuser", kafka.Credentials.Username)
	assert.Equal(t, "rotated-pass", kafka.Credentials.Password)

	assert.Nil(t, kafka.SchemaRegistry, "schema registry must never be sent on update")
	assert.Empty(t, kafka.Brokers, "immutable brokers must be omitted")
	assert.Nil(t, kafka.ConsumerGroup, "immutable consumer group must be omitted")
}
