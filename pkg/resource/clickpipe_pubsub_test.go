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

// buildPubSubPlan returns a plan model with a Pub/Sub source populated from the
// supplied params. Optional values (filter, ack_deadline, etc.) are passed through
// as-is so tests can exercise null vs. set behavior.
func buildPubSubPlan(
	format, projectID, topic, seekType string,
	seekTimestamp, seekSnapshot, filter types.String,
	enableOrdering types.Bool,
	ackDeadline types.Int64,
	serviceAccountFile types.String,
) models.ClickPipeResourceModel {
	keyAttrs := map[string]attr.Value{
		"service_account_file": serviceAccountFile,
	}
	pubsubAttrs := map[string]attr.Value{
		"format":              types.StringValue(format),
		"project_id":          types.StringValue(projectID),
		"topic":               types.StringValue(topic),
		"authentication":      types.StringValue(api.ClickPipeAuthenticationServiceAccount),
		"seek_type":           types.StringValue(seekType),
		"seek_timestamp":      seekTimestamp,
		"seek_snapshot":       seekSnapshot,
		"filter":              filter,
		"enable_ordering":     enableOrdering,
		"ack_deadline":        ackDeadline,
		"service_account_key": types.ObjectValueMust(models.ClickPipeServiceAccountModel{}.ObjectType().AttrTypes, keyAttrs),
	}
	sourceModel := models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		PubSub:        types.ObjectValueMust(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes, pubsubAttrs),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-pubsub"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_PubSub_LatestCreate(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildPubSubPlan(
		api.ClickPipeJSONEachRowFormat,
		"my-gcp-project",
		"events",
		api.ClickPipePubSubSeekTypeLatest,
		types.StringNull(),
		types.StringNull(),
		types.StringNull(),
		types.BoolNull(),
		types.Int64Null(),
		types.StringValue("base64-encoded-key"),
	)

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source)
	assert.NotNil(t, source.PubSub)
	assert.Equal(t, api.ClickPipeJSONEachRowFormat, source.PubSub.Format)
	assert.Equal(t, "my-gcp-project", source.PubSub.ProjectID)
	assert.Equal(t, "events", source.PubSub.Topic)
	assert.Equal(t, api.ClickPipeAuthenticationServiceAccount, source.PubSub.Authentication)
	assert.Equal(t, api.ClickPipePubSubSeekTypeLatest, source.PubSub.SeekType)
	assert.Nil(t, source.PubSub.SeekTimestamp)
	assert.Nil(t, source.PubSub.SeekSnapshot)
	assert.Nil(t, source.PubSub.Filter)
	assert.Nil(t, source.PubSub.EnableOrdering)
	assert.Nil(t, source.PubSub.AckDeadline)
	assert.NotNil(t, source.PubSub.ServiceAccountKey)
	assert.Equal(t, "base64-encoded-key", source.PubSub.ServiceAccountKey.ServiceAccountFile)
}

func TestExtractSourceFromPlan_PubSub_TimestampWithOptionalFields(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildPubSubPlan(
		api.ClickPipeAvroFormat,
		"my-gcp-project",
		"events",
		api.ClickPipePubSubSeekTypeTimestamp,
		types.StringValue("2026-04-10T12:00:00Z"),
		types.StringNull(),
		types.StringValue(`attributes.env = "prod"`),
		types.BoolValue(true),
		types.Int64Value(120),
		types.StringValue("base64-encoded-key"),
	)

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError())
	assert.NotNil(t, source.PubSub)
	assert.Equal(t, api.ClickPipePubSubSeekTypeTimestamp, source.PubSub.SeekType)
	assert.NotNil(t, source.PubSub.SeekTimestamp)
	assert.Equal(t, "2026-04-10T12:00:00Z", *source.PubSub.SeekTimestamp)
	assert.NotNil(t, source.PubSub.Filter)
	assert.Equal(t, `attributes.env = "prod"`, *source.PubSub.Filter)
	assert.NotNil(t, source.PubSub.EnableOrdering)
	assert.True(t, *source.PubSub.EnableOrdering)
	assert.NotNil(t, source.PubSub.AckDeadline)
	assert.Equal(t, int64(120), *source.PubSub.AckDeadline)
}

func TestExtractSourceFromPlan_PubSub_UpdateIncludesAllFields(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildPubSubPlan(
		api.ClickPipeJSONEachRowFormat,
		"my-gcp-project",
		"events",
		api.ClickPipePubSubSeekTypeLatest,
		types.StringNull(),
		types.StringNull(),
		types.StringValue("attributes.env = \"prod\""),
		types.BoolValue(false),
		types.Int64Value(60),
		types.StringValue("rotated-key"),
	)

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

	assert.False(t, diagnostics.HasError())
	assert.NotNil(t, source.PubSub)
	// Immutable (RequiresReplace) fields still populated on update — values match state
	// because the framework would have forced replacement if they had changed.
	assert.Equal(t, api.ClickPipeJSONEachRowFormat, source.PubSub.Format)
	assert.Equal(t, "my-gcp-project", source.PubSub.ProjectID)
	assert.Equal(t, "events", source.PubSub.Topic)
	assert.Equal(t, api.ClickPipePubSubSeekTypeLatest, source.PubSub.SeekType)
	assert.NotNil(t, source.PubSub.Filter)
	assert.NotNil(t, source.PubSub.EnableOrdering)
	assert.NotNil(t, source.PubSub.AckDeadline)
	assert.NotNil(t, source.PubSub.ServiceAccountKey)
	assert.Equal(t, "rotated-key", source.PubSub.ServiceAccountKey.ServiceAccountFile)
}
