package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// buildKafkaSSHKeyResourcePlan returns a minimal IAM_ROLE-authenticated Kafka
// plan with the given ssh_key_resource_id value.
func buildKafkaSSHKeyResourcePlan(sshKeyResourceID types.String) models.ClickPipeResourceModel {
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
		"exactly_once":                 types.BoolNull(),
		"ssh_key_resource_id":          sshKeyResourceID,
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
		Name:      types.StringValue("test-kafka-ssh"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_Kafka_SSHKeyResourceIDSet(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaSSHKeyResourcePlan(types.StringValue("ssh-key-123"))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.NotNil(t, source.Kafka.SSHKeyResourceID)
	assert.Equal(t, "ssh-key-123", *source.Kafka.SSHKeyResourceID)
}

func TestExtractSourceFromPlan_Kafka_SSHKeyResourceIDNull(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaSSHKeyResourcePlan(types.StringNull())

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.Nil(t, source.Kafka.SSHKeyResourceID)
}

// ssh_key_resource_id is immutable (create-only): it must not be included in an
// update (PATCH) payload.
func TestExtractSourceFromPlan_Kafka_SSHKeyResourceIDOmittedOnUpdate(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildKafkaSSHKeyResourcePlan(types.StringValue("ssh-key-123"))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Kafka)
	assert.Nil(t, source.Kafka.SSHKeyResourceID)
}

// buildMongoDBSSHKeyResourcePlan returns a minimal MongoDB CDC plan with the
// given ssh_key_resource_id value.
func buildMongoDBSSHKeyResourcePlan(sshKeyResourceID types.String) models.ClickPipeResourceModel {
	settingsModel := models.ClickPipeMongoDBSettingsModel{
		ReplicationMode: types.StringValue("cdc"),
	}
	mongoAttrs := map[string]attr.Value{
		"uri":                    types.StringValue("mongodb+srv://cluster0.example.mongodb.net/mydb"),
		"read_preference":        types.StringValue("secondaryPreferred"),
		"tls_host":               types.StringNull(),
		"ca_certificate":         types.StringNull(),
		"disable_tls":            types.BoolNull(),
		"skip_cert_verification": types.BoolNull(),
		"credentials":            types.ObjectNull(models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes),
		"settings":               settingsModel.ObjectValue(),
		"table_mappings":         types.SetValueMust(models.ClickPipeMongoDBTableMappingModel{}.ObjectType(), []attr.Value{}),
		"ssh_key_resource_id":    sshKeyResourceID,
	}
	sourceModel := models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectValueMust(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes, mongoAttrs),
	}
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-mongodb-ssh"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_MongoDB_SSHKeyResourceIDSet(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMongoDBSSHKeyResourcePlan(types.StringValue("ssh-key-123"))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MongoDB)
	assert.NotNil(t, source.MongoDB.SSHKeyResourceID)
	assert.Equal(t, "ssh-key-123", *source.MongoDB.SSHKeyResourceID)
}

// ssh_key_resource_id is immutable (create-only): it must not be included in an
// update (PATCH) payload.
func TestExtractSourceFromPlan_MongoDB_SSHKeyResourceIDOmittedOnUpdate(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMongoDBSSHKeyResourcePlan(types.StringValue("ssh-key-123"))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MongoDB)
	assert.Nil(t, source.MongoDB.SSHKeyResourceID)
}
