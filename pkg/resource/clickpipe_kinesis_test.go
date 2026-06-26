package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

// buildKinesisResourceModel builds a ClickPipeResourceModel with a Kinesis source for
// update/patch tests. authentication selects IAM_USER (access_key populated) or
// IAM_ROLE (iam_role populated). The non-credential fields are fixed; only the
// authentication, access_key and iam_role fields are patchable for Kinesis.
func buildKinesisResourceModel(authentication string, accessKeyID, secretKey, iamRole types.String) models.ClickPipeResourceModel {
	accessKey := types.ObjectNull(models.ClickPipeSourceAccessKeyModel{}.ObjectType().AttrTypes)
	if authentication == api.ClickPipeAuthenticationIAMUser {
		accessKey = types.ObjectValueMust(models.ClickPipeSourceAccessKeyModel{}.ObjectType().AttrTypes, map[string]attr.Value{
			"access_key_id": accessKeyID,
			"secret_key":    secretKey,
		})
	}

	kinesisAttrs := map[string]attr.Value{
		"format":               types.StringValue("JSONEachRow"),
		"stream_name":          types.StringValue("my-stream"),
		"region":               types.StringValue("us-east-1"),
		"iterator_type":        types.StringValue("TRIM_HORIZON"),
		"timestamp":            types.StringNull(),
		"use_enhanced_fan_out": types.BoolValue(false),
		"authentication":       types.StringValue(authentication),
		"access_key":           accessKey,
		"iam_role":             iamRole,
	}

	sourceModel := models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectValueMust(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes, kinesisAttrs),
		PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}

	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-kinesis-pipe"),
		Source:    sourceModel.ObjectValue(),
	}
}

// TestExtractSourceFromPlan_KinesisUpdate proves the Kinesis source can be built on the
// update (PATCH) path. Before this change, Kinesis had no Update dispatch branch and every
// field carried RequiresReplace, so a Kinesis pipe could never be patched.
func TestExtractSourceFromPlan_KinesisUpdate(t *testing.T) {
	ctx := context.Background()
	resource := &ClickPipeResource{}

	t.Run("IAM_USER source extracts with access_key on update", func(t *testing.T) {
		plan := buildKinesisResourceModel(api.ClickPipeAuthenticationIAMUser, types.StringValue("AKIAEXAMPLE"), types.StringValue("secret-value"), types.StringNull())
		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

		assert.False(t, diagnostics.HasError(), "Kinesis updates must be supported, got: %v", diagnostics.Errors())
		assert.NotNil(t, source)
		assert.NotNil(t, source.Kinesis)
		assert.NotNil(t, source.Kinesis.AccessKey, "IAM_USER auth must carry the access_key for credential rotation")
		assert.Equal(t, "AKIAEXAMPLE", source.Kinesis.AccessKey.AccessKeyID)
		assert.Equal(t, "secret-value", source.Kinesis.AccessKey.SecretKey)
		assert.Equal(t, api.ClickPipeAuthenticationIAMUser, source.Kinesis.Authentication)
	})

	t.Run("IAM_ROLE source extracts without access_key on update", func(t *testing.T) {
		plan := buildKinesisResourceModel(api.ClickPipeAuthenticationIAMRole, types.StringNull(), types.StringNull(), types.StringValue("arn:aws:iam::123456789012:role/clickpipes"))
		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

		assert.False(t, diagnostics.HasError(), "Kinesis IAM_ROLE updates must be supported, got: %v", diagnostics.Errors())
		assert.NotNil(t, source.Kinesis)
		assert.Nil(t, source.Kinesis.AccessKey, "IAM_ROLE auth must not carry an access_key")
		assert.NotNil(t, source.Kinesis.IAMRole)
		assert.Equal(t, "arn:aws:iam::123456789012:role/clickpipes", *source.Kinesis.IAMRole)
		assert.Equal(t, api.ClickPipeAuthenticationIAMRole, source.Kinesis.Authentication)
	})
}

// TestCredentialsObjectChanged_KinesisAccessKey covers the Update() prune decision for the
// Kinesis access_key: an unchanged key is omitted from the PATCH (avoiding pointless
// server-side re-encryption), while any rotation re-sends it. credentialsObjectChanged is
// reused for this because the access_key object has no password_wo attribute, so the
// generic attribute-wise comparison applies directly.
func TestCredentialsObjectChanged_KinesisAccessKey(t *testing.T) {
	accessKeyTypes := models.ClickPipeSourceAccessKeyModel{}.ObjectType().AttrTypes
	buildAccessKey := func(id, secret types.String) types.Object {
		return types.ObjectValueMust(accessKeyTypes, map[string]attr.Value{
			"access_key_id": id,
			"secret_key":    secret,
		})
	}
	nullAccessKey := types.ObjectNull(accessKeyTypes)

	t.Run("unchanged access_key is omitted from PATCH", func(t *testing.T) {
		plan := buildAccessKey(types.StringValue("AKIA"), types.StringValue("secret"))
		state := buildAccessKey(types.StringValue("AKIA"), types.StringValue("secret"))
		assert.False(t, credentialsObjectChanged(plan, state), "identical access_key must not be re-sent in PATCH")
	})

	t.Run("secret_key rotation is detected", func(t *testing.T) {
		plan := buildAccessKey(types.StringValue("AKIA"), types.StringValue("new-secret"))
		state := buildAccessKey(types.StringValue("AKIA"), types.StringValue("old-secret"))
		assert.True(t, credentialsObjectChanged(plan, state), "rotated secret_key must be re-sent in PATCH")
	})

	t.Run("access_key_id change is detected", func(t *testing.T) {
		plan := buildAccessKey(types.StringValue("AKIA-NEW"), types.StringValue("secret"))
		state := buildAccessKey(types.StringValue("AKIA-OLD"), types.StringValue("secret"))
		assert.True(t, credentialsObjectChanged(plan, state), "changed access_key_id must be re-sent in PATCH")
	})

	t.Run("switching auth away from IAM_USER (access_key set->null) is detected", func(t *testing.T) {
		plan := nullAccessKey
		state := buildAccessKey(types.StringValue("AKIA"), types.StringValue("secret"))
		assert.True(t, credentialsObjectChanged(plan, state), "set->null access_key transition must be detected")
	})
}

// getKinesisSyncState builds a full ClickPipeResourceModel with a Kinesis source for
// syncClickPipeState tests. When accessKey is non-null it represents an IAM_USER pipe whose
// credentials live in state (the API never returns them); iamRole represents an IAM_ROLE pipe.
func getKinesisSyncState(authentication string, accessKey types.Object, iamRole types.String) models.ClickPipeResourceModel {
	kinesisAttrs := map[string]attr.Value{
		"format":               types.StringValue("JSONEachRow"),
		"stream_name":          types.StringValue("my-stream"),
		"region":               types.StringValue("us-east-1"),
		"iterator_type":        types.StringValue("TRIM_HORIZON"),
		"timestamp":            types.StringNull(),
		"use_enhanced_fan_out": types.BoolValue(false),
		"authentication":       types.StringValue(authentication),
		"access_key":           accessKey,
		"iam_role":             iamRole,
	}
	source := models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectValueMust(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes, kinesisAttrs),
		PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-kinesis-pipe"),
		State:     types.StringValue("provisioning"),
		Source:    source.ObjectValue(),
		Destination: types.ObjectValueMust(
			models.ClickPipeDestinationModel{}.ObjectType().AttrTypes,
			map[string]attr.Value{
				"database":         types.StringValue("default"),
				"table":            types.StringValue("e2e_kinesis_table"),
				"managed_table":    types.BoolValue(true),
				"table_definition": types.ObjectNull(models.ClickPipeDestinationTableDefinitionModel{}.ObjectType().AttrTypes),
				"columns":          types.ListNull(models.ClickPipeDestinationColumnModel{}.ObjectType()),
				"roles":            types.ListNull(types.StringType),
			},
		),
	}
}

// kinesisAPIResponse builds the GET response for a Kinesis pipe. Mirroring the real API, the
// Kinesis source carries NO AccessKey — secrets are never returned — so syncClickPipeState must
// reinstate the credentials from prior state.
func kinesisAPIResponse(authentication string, iamRole *string) *api.ClickPipe {
	return &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-kinesis-pipe",
		State: "running",
		Source: api.ClickPipeSource{
			Kinesis: &api.ClickPipeKinesisSource{
				Format:         "JSONEachRow",
				StreamName:     "my-stream",
				Region:         "us-east-1",
				IteratorType:   "TRIM_HORIZON",
				Authentication: authentication,
				IAMRole:        iamRole,
				// AccessKey intentionally nil: the API never returns credentials.
			},
		},
		Destination: api.ClickPipeDestination{
			Database:     "default",
			Table:        strPtr("e2e_kinesis_table"),
			ManagedTable: boolPtr(true),
			Columns:      []api.ClickPipeDestinationColumn{{Name: "radio", Type: "String"}},
		},
	}
}

// TestClickPipeResource_syncClickPipeState_Kinesis proves the read-back preserves Kinesis
// credentials from prior state. This is the safety property that lets access_key drop
// RequiresReplace: if the read nulled the access_key (the API never returns it) every plan
// would show a spurious credential diff / rotation.
func TestClickPipeResource_syncClickPipeState_Kinesis(t *testing.T) {
	ctx := context.Background()

	t.Run("IAM_USER access_key is preserved from state across read", func(t *testing.T) {
		state := getKinesisSyncState(
			api.ClickPipeAuthenticationIAMUser,
			types.ObjectValueMust(models.ClickPipeSourceAccessKeyModel{}.ObjectType().AttrTypes, map[string]attr.Value{
				"access_key_id": types.StringValue("AKIAEXAMPLE"),
				"secret_key":    types.StringValue("secret-value"),
			}),
			types.StringNull(),
		)

		mc := minimock.NewController(t)
		apiClientMock := api.NewClientMock(mc).
			GetClickPipeMock.
			Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
			Return(kinesisAPIResponse(api.ClickPipeAuthenticationIAMUser, nil), nil)
		resource := &ClickPipeResource{client: apiClientMock}

		err := resource.syncClickPipeState(ctx, &state)
		assert.NoError(t, err)

		var sourceModel models.ClickPipeSourceModel
		state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})
		var kinesisModel models.ClickPipeKinesisSourceModel
		sourceModel.Kinesis.As(ctx, &kinesisModel, basetypes.ObjectAsOptions{})

		// Metadata reflects the API response.
		assert.Equal(t, "my-stream", kinesisModel.StreamName.ValueString())
		assert.Equal(t, "us-east-1", kinesisModel.Region.ValueString())
		// Credentials are preserved from prior state (API returned none).
		assert.False(t, kinesisModel.AccessKey.IsNull(), "access_key must be preserved from state")
		var accessKey models.ClickPipeSourceAccessKeyModel
		kinesisModel.AccessKey.As(ctx, &accessKey, basetypes.ObjectAsOptions{})
		assert.Equal(t, "AKIAEXAMPLE", accessKey.AccessKeyID.ValueString())
		assert.Equal(t, "secret-value", accessKey.SecretKey.ValueString())
	})

	t.Run("IAM_ROLE keeps access_key null and syncs iam_role", func(t *testing.T) {
		state := getKinesisSyncState(
			api.ClickPipeAuthenticationIAMRole,
			types.ObjectNull(models.ClickPipeSourceAccessKeyModel{}.ObjectType().AttrTypes),
			types.StringValue("arn:aws:iam::123456789012:role/clickpipes"),
		)

		mc := minimock.NewController(t)
		apiClientMock := api.NewClientMock(mc).
			GetClickPipeMock.
			Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
			Return(kinesisAPIResponse(api.ClickPipeAuthenticationIAMRole, strPtr("arn:aws:iam::123456789012:role/clickpipes")), nil)
		resource := &ClickPipeResource{client: apiClientMock}

		err := resource.syncClickPipeState(ctx, &state)
		assert.NoError(t, err)

		var sourceModel models.ClickPipeSourceModel
		state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})
		var kinesisModel models.ClickPipeKinesisSourceModel
		sourceModel.Kinesis.As(ctx, &kinesisModel, basetypes.ObjectAsOptions{})

		assert.True(t, kinesisModel.AccessKey.IsNull(), "IAM_ROLE pipe must keep access_key null")
		assert.Equal(t, "arn:aws:iam::123456789012:role/clickpipes", kinesisModel.IAMRole.ValueString())
	})
}
