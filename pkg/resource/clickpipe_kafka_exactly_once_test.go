package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

// buildKafkaExactlyOncePlan returns a minimal IAM_ROLE-authenticated Kafka plan
// with the given exactly_once value, so tests can exercise null vs. set behavior.
func buildKafkaExactlyOncePlan(exactlyOnce types.Bool) models.ClickPipeResourceModel {
	kafkaAttrs := map[string]attr.Value{
		"type":                         types.StringValue("msk"),
		"format":                       types.StringValue(api.ClickPipeJSONEachRowFormat),
		"brokers":                      types.StringValue("broker:9092"),
		"topics":                       types.StringValue("test-topic"),
		"consumer_group":               types.StringNull(),
		"offset":                       types.ObjectNull(models.ClickPipeKafkaOffsetModel{}.ObjectType().AttrTypes),
		"schema_registry":              types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes),
		"authentication":               types.StringValue(api.ClickPipeAuthenticationIAMRole),
		"credentials":                  types.ObjectNull(models.ClickPipeKafkaSourceCredentialsModel{}.ObjectType().AttrTypes),
		"iam_role":                     types.StringValue("arn:aws:iam::123456789012:role/MyRole"),
		"ca_certificate":               types.StringNull(),
		"reverse_private_endpoint_ids": types.ListNull(types.StringType),
		"exactly_once":                 exactlyOnce,
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
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-kafka-eos"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_Kafka_ExactlyOnceEnabled(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaExactlyOncePlan(types.BoolValue(true))

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

	plan := buildKafkaExactlyOncePlan(types.BoolNull())

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

	plan := buildKafkaExactlyOncePlan(types.BoolValue(true))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.Nil(t, source.Kafka.ExactlyOnce)
}
