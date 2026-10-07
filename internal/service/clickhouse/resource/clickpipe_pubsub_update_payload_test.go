package resource

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// pubsubUpdateModel builds a ClickPipe model with a Pub/Sub source. The
// immutable attributes are fixed, so a state/plan pair differs only in the
// three attributes that can legitimately reach Update: authentication,
// service_account_key and ack_deadline.
func pubsubUpdateModel(authentication string, serviceAccountFile types.String, ackDeadline types.Int64) models.ClickPipeResourceModel {
	keyObject := types.ObjectNull(models.ClickPipeServiceAccountModel{}.ObjectType().AttrTypes)
	if !serviceAccountFile.IsNull() {
		keyObject = types.ObjectValueMust(models.ClickPipeServiceAccountModel{}.ObjectType().AttrTypes, map[string]attr.Value{
			"service_account_file": serviceAccountFile,
		})
	}
	pubsubAttrs := map[string]attr.Value{
		"format":              types.StringValue(api.ClickPipeJSONEachRowFormat),
		"project_id":          types.StringValue("my-gcp-project"),
		"topic":               types.StringValue("events"),
		"authentication":      types.StringValue(authentication),
		"seek_type":           types.StringValue(api.ClickPipePubSubSeekTypeLatest),
		"seek_timestamp":      types.StringNull(),
		"filter":              types.StringNull(),
		"enable_ordering":     types.BoolNull(),
		"ack_deadline":        ackDeadline,
		"service_account_key": keyObject,
	}
	src := models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		PubSub:        types.ObjectValueMust(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes, pubsubAttrs),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}
	// The destination and the remaining attributes are source-agnostic.
	m := kafkaUpdateModel(types.StringNull(), "")
	m.Name = types.StringValue("test-pubsub-pipe")
	m.Source = src.ObjectValue()
	return m
}

// driveClickPipePubSubUpdate runs Update for a Pub/Sub state/plan pair and
// returns the PATCH body that would reach the API, as JSON.
func driveClickPipePubSubUpdate(t *testing.T, state, plan models.ClickPipeResourceModel) string {
	t.Helper()
	ctx := context.Background()
	apiPipe := &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-pubsub-pipe",
		State: api.ClickPipeRunningState,
		Source: api.ClickPipeSource{
			PubSub: &api.ClickPipePubSubSource{
				Format:         api.ClickPipeJSONEachRowFormat,
				ProjectID:      "my-gcp-project",
				Topic:          "events",
				Authentication: api.ClickPipeAuthenticationServiceAccount,
				SeekType:       api.ClickPipePubSubSeekTypeLatest,
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
	mock.WaitForClickPipesGCPWorkloadIdentityMock.Optional().Set(func(_ context.Context, _ string, _ time.Duration) (*api.ClickPipesGCPWorkloadIdentityContext, error) {
		return &api.ClickPipesGCPWorkloadIdentityContext{}, nil
	})

	r := &ClickPipeResource{client: mock}
	resp := driveClickPipeUpdate(ctx, t, r, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update failed: %v", resp.Diagnostics.Errors())
	require.NotNil(t, captured, "UpdateClickPipe was not called")

	body, err := json.Marshal(captured)
	require.NoError(t, err)
	return string(body)
}

// Switching the authentication to workload identity must PATCH the new
// authentication only, never the immutable format/project/topic/seek type
// (the API rejects them with "format is immutable for Pub/Sub sources").
func TestClickPipeUpdate_PubSubSwitchToWorkloadIdentity(t *testing.T) {
	state := pubsubUpdateModel(api.ClickPipeAuthenticationServiceAccount, types.StringValue("old-key"), types.Int64Null())
	plan := pubsubUpdateModel(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity, types.StringNull(), types.Int64Null())

	body := driveClickPipePubSubUpdate(t, state, plan)

	assert.JSONEq(t, `{"source":{"pubsub":{"authentication":"SERVICE_ACCOUNT_WORKLOAD_IDENTITY"}}}`, body)
}

// Rotating the key must carry the new key next to the authentication, and
// still none of the immutable attributes.
func TestClickPipeUpdate_PubSubKeyRotation(t *testing.T) {
	state := pubsubUpdateModel(api.ClickPipeAuthenticationServiceAccount, types.StringValue("old-key"), types.Int64Null())
	plan := pubsubUpdateModel(api.ClickPipeAuthenticationServiceAccount, types.StringValue("new-key"), types.Int64Null())

	body := driveClickPipePubSubUpdate(t, state, plan)

	assert.JSONEq(t, `{"source":{"pubsub":{"authentication":"SERVICE_ACCOUNT","serviceAccountKey":{"serviceAccountFile":"new-key"}}}}`, body)
}

// Changing only ack_deadline must carry it without re-sending the unchanged
// key or any immutable attribute.
func TestClickPipeUpdate_PubSubAckDeadlineOnly(t *testing.T) {
	state := pubsubUpdateModel(api.ClickPipeAuthenticationServiceAccount, types.StringValue("same-key"), types.Int64Value(60))
	plan := pubsubUpdateModel(api.ClickPipeAuthenticationServiceAccount, types.StringValue("same-key"), types.Int64Value(120))

	body := driveClickPipePubSubUpdate(t, state, plan)

	assert.JSONEq(t, `{"source":{"pubsub":{"authentication":"SERVICE_ACCOUNT","ackDeadline":120}}}`, body)
}

// Create must keep sending every required field: the omitempty tags that keep
// the immutable fields out of PATCH must not drop them from POST.
func TestExtractSourceFromPlan_PubSub_CreateBodyKeepsRequiredFields(t *testing.T) {
	plan := pubsubUpdateModel(api.ClickPipeAuthenticationServiceAccount, types.StringValue("the-key"), types.Int64Null())

	diagnostics := diag.Diagnostics{}
	source := (&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, false)
	require.False(t, diagnostics.HasError(), "unexpected errors: %v", diagnostics.Errors())

	body, err := json.Marshal(source)
	require.NoError(t, err)
	assert.JSONEq(t, `{"pubsub":{"format":"JSONEachRow","projectId":"my-gcp-project","topic":"events","authentication":"SERVICE_ACCOUNT","seekType":"latest","serviceAccountKey":{"serviceAccountFile":"the-key"}}}`, string(body))
}
