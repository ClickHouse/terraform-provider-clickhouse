package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func workloadIdentitySourceModel() models.ClickPipeSourceModel {
	return models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}
}

func workloadIdentityKafka(sourceType string, credentials types.Object, iamRole types.String) types.Object {
	return types.ObjectValueMust(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes, map[string]attr.Value{
		"type":                         types.StringValue(sourceType),
		"format":                       types.StringValue(api.ClickPipeJSONEachRowFormat),
		"brokers":                      types.StringValue("broker:9092"),
		"topics":                       types.StringValue("events"),
		"consumer_group":               types.StringNull(),
		"offset":                       types.ObjectNull(models.ClickPipeKafkaOffsetModel{}.ObjectType().AttrTypes),
		"schema_registry":              types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes),
		"authentication":               types.StringValue(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity),
		"credentials":                  credentials,
		"iam_role":                     iamRole,
		"ca_certificate":               types.StringNull(),
		"reverse_private_endpoint_ids": types.ListNull(types.StringType),
		"exactly_once":                 types.BoolNull(),
	})
}

func TestExtractSourceFromPlan_GCMKWorkloadIdentityHasNoCredentials(t *testing.T) {
	source := workloadIdentitySourceModel()
	source.Kafka = workloadIdentityKafka(
		api.ClickPipeKafkaGCMKSourceType,
		types.ObjectNull(models.ClickPipeKafkaSourceCredentialsModel{}.ObjectType().AttrTypes),
		types.StringNull(),
	)
	plan := models.ClickPipeResourceModel{Source: source.ObjectValue()}
	diagnostics := diag.Diagnostics{}

	got := (&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, false)
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics.Errors())
	}
	if got.Kafka == nil || got.Kafka.Authentication != api.ClickPipeAuthenticationServiceAccountWorkloadIdentity {
		t.Fatalf("unexpected Kafka source: %+v", got.Kafka)
	}
	if got.Kafka.Credentials != nil || got.Kafka.IAMRole != nil {
		t.Errorf("workload identity source included customer credentials: %+v", got.Kafka)
	}
	if !clickPipeSourceUsesGCPWorkloadIdentity(got) {
		t.Error("source was not detected as using workload identity")
	}
}

func TestValidateGCPWorkloadIdentityConfiguration_KafkaRejectsConflicts(t *testing.T) {
	credentials := types.ObjectValueMust(models.ClickPipeKafkaSourceCredentialsModel{}.ObjectType().AttrTypes, map[string]attr.Value{
		"username":            types.StringValue("user"),
		"password":            types.StringValue("secret"),
		"password_wo":         types.StringNull(),
		"password_wo_version": types.Int64Null(),
		"access_key_id":       types.StringNull(),
		"secret_key":          types.StringNull(),
		"connection_string":   types.StringNull(),
		"certificate":         types.StringNull(),
		"private_key":         types.StringNull(),
	})
	source := workloadIdentitySourceModel()
	source.Kafka = workloadIdentityKafka(api.ClickPipeKafkaSourceType, credentials, types.StringValue("role"))
	diagnostics := diag.Diagnostics{}

	validateGCPWorkloadIdentityConfiguration(context.Background(), source.ObjectValue(), &diagnostics)
	if len(diagnostics.Errors()) != 3 {
		t.Fatalf("errors = %v; want source type, credentials, and IAM role errors", diagnostics.Errors())
	}
}

func workloadIdentityObjectStorage(sourceType string, accessKey types.Object, serviceAccountKey types.String) types.Object {
	model := models.ClickPipeObjectStorageSourceModel{
		Type:               types.StringValue(sourceType),
		Format:             types.StringValue(api.ClickPipeJSONEachRowFormat),
		URL:                types.StringValue("gs://bucket/events/*.json"),
		Delimiter:          types.StringNull(),
		Compression:        types.StringNull(),
		IsContinuous:       types.BoolValue(false),
		QueueURL:           types.StringNull(),
		SkipInitialLoad:    types.BoolNull(),
		StartAfter:         types.StringNull(),
		Authentication:     types.StringValue(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity),
		AccessKey:          accessKey,
		IAMRole:            types.StringNull(),
		ServiceAccountKey:  serviceAccountKey,
		ConnectionString:   types.StringNull(),
		Path:               types.StringNull(),
		AzureContainerName: types.StringNull(),
	}
	return model.ObjectValue()
}

func TestValidateGCPWorkloadIdentityConfiguration_ObjectStorage(t *testing.T) {
	t.Run("accepts credentialless GCS", func(t *testing.T) {
		source := workloadIdentitySourceModel()
		source.ObjectStorage = workloadIdentityObjectStorage(api.ClickPipeObjectStorageGCSType, types.ObjectNull(models.ClickPipeSourceAccessKeyModel{}.ObjectType().AttrTypes), types.StringNull())
		diagnostics := diag.Diagnostics{}
		plan := models.ClickPipeResourceModel{Source: source.ObjectValue()}
		got := (&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, false)
		if diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diagnostics.Errors())
		}
		if got.ObjectStorage == nil || got.ObjectStorage.Authentication == nil || *got.ObjectStorage.Authentication != api.ClickPipeAuthenticationServiceAccountWorkloadIdentity {
			t.Fatalf("unexpected object storage source: %+v", got.ObjectStorage)
		}
		if got.ObjectStorage.AccessKey != nil || got.ObjectStorage.ServiceAccountKey != nil {
			t.Errorf("workload identity source included customer credentials: %+v", got.ObjectStorage)
		}
	})

	t.Run("rejects non-GCS source and credentials", func(t *testing.T) {
		accessKey := models.ClickPipeSourceAccessKeyModel{AccessKeyID: types.StringValue("id"), SecretKey: types.StringValue("secret")}.ObjectValue()
		source := workloadIdentitySourceModel()
		source.ObjectStorage = workloadIdentityObjectStorage(api.ClickPipeObjectStorageS3Type, accessKey, types.StringValue("key"))
		diagnostics := diag.Diagnostics{}
		validateGCPWorkloadIdentityConfiguration(context.Background(), source.ObjectValue(), &diagnostics)
		if len(diagnostics.Errors()) != 3 {
			t.Fatalf("errors = %v; want source type, access key, and service account key errors", diagnostics.Errors())
		}
	})
}

func workloadIdentityPubSub(authentication string, serviceAccountKey types.Object) types.Object {
	model := models.ClickPipePubSubSourceModel{
		Format:            types.StringValue(api.ClickPipeJSONEachRowFormat),
		ProjectID:         types.StringValue("my-gcp-project"),
		Topic:             types.StringValue("events"),
		Authentication:    types.StringValue(authentication),
		SeekType:          types.StringValue(api.ClickPipePubSubSeekTypeLatest),
		SeekTimestamp:     types.StringNull(),
		Filter:            types.StringNull(),
		EnableOrdering:    types.BoolNull(),
		AckDeadline:       types.Int64Null(),
		ServiceAccountKey: serviceAccountKey,
	}
	return model.ObjectValue()
}

func TestValidateGCPWorkloadIdentityConfiguration_PubSubCredentialRules(t *testing.T) {
	nullKey := types.ObjectNull(models.ClickPipeServiceAccountModel{}.ObjectType().AttrTypes)
	key := models.ClickPipeServiceAccountModel{ServiceAccountFile: types.StringValue("base64-key")}.ObjectValue()

	t.Run("accepts credentialless workload identity", func(t *testing.T) {
		source := workloadIdentitySourceModel()
		source.PubSub = workloadIdentityPubSub(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity, nullKey)
		diagnostics := diag.Diagnostics{}
		plan := models.ClickPipeResourceModel{Source: source.ObjectValue()}
		got := (&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, false)
		if diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diagnostics.Errors())
		}
		if got.PubSub == nil || got.PubSub.Authentication != api.ClickPipeAuthenticationServiceAccountWorkloadIdentity || got.PubSub.ServiceAccountKey != nil {
			t.Fatalf("unexpected Pub/Sub source: %+v", got.PubSub)
		}
	})

	t.Run("rejects key for workload identity", func(t *testing.T) {
		source := workloadIdentitySourceModel()
		source.PubSub = workloadIdentityPubSub(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity, key)
		diagnostics := diag.Diagnostics{}
		validateGCPWorkloadIdentityConfiguration(context.Background(), source.ObjectValue(), &diagnostics)
		if !diagnostics.HasError() {
			t.Fatal("expected conflicting service account key error")
		}
	})

	t.Run("requires key for service account", func(t *testing.T) {
		source := workloadIdentitySourceModel()
		source.PubSub = workloadIdentityPubSub(api.ClickPipeAuthenticationServiceAccount, nullKey)
		diagnostics := diag.Diagnostics{}
		validateGCPWorkloadIdentityConfiguration(context.Background(), source.ObjectValue(), &diagnostics)
		if !diagnostics.HasError() {
			t.Fatal("expected missing service account key error")
		}
	})
}
