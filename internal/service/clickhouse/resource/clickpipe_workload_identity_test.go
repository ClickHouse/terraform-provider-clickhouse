package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

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
		"ssh_key_resource_id":          types.StringNull(),
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

func workloadIdentityBigQuery(authentication string, projectID types.String, credentials types.Object) types.Object {
	settings := models.ClickPipeBigQuerySettingsModel{
		ReplicationMode:                types.StringValue(api.ClickPipeReplicationModeSnapshot),
		AllowNullableColumns:           types.BoolValue(false),
		InitialLoadParallelism:         types.Int64Value(4),
		SnapshotNumRowsPerPartition:    types.Int64Value(100_000),
		SnapshotNumberOfParallelTables: types.Int64Value(1),
	}
	mapping := models.ClickPipeBigQueryTableMappingModel{
		SourceDatasetName:   types.StringValue("events"),
		SourceTable:         types.StringValue("source"),
		TargetTable:         types.StringValue("target"),
		ExcludedColumns:     types.SetNull(types.StringType),
		UseCustomSortingKey: types.BoolValue(false),
		SortingKeys:         types.ListNull(types.StringType),
		TableEngine:         types.StringNull(),
	}
	model := models.ClickPipeBigQuerySourceModel{
		SnapshotStagingPath: types.StringValue("gs://staging-bucket/clickpipes/"),
		Authentication:      types.StringValue(authentication),
		ProjectID:           projectID,
		Settings:            settings.ObjectValue(),
		TableMappings:       types.ListValueMust(models.ClickPipeBigQueryTableMappingModel{}.ObjectType(), []attr.Value{mapping.ObjectValue()}),
		Credentials:         credentials,
	}
	return model.ObjectValue()
}

func TestValidateGCPWorkloadIdentityConfiguration_BigQueryCredentialRules(t *testing.T) {
	nullCredentials := types.ObjectNull(models.ClickPipeServiceAccountModel{}.ObjectType().AttrTypes)
	credentials := models.ClickPipeServiceAccountModel{ServiceAccountFile: types.StringValue("base64-key")}.ObjectValue()

	t.Run("accepts credentialless workload identity", func(t *testing.T) {
		source := workloadIdentitySourceModel()
		source.BigQuery = workloadIdentityBigQuery(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity, types.StringValue("my-gcp-project"), nullCredentials)
		diagnostics := diag.Diagnostics{}
		plan := models.ClickPipeResourceModel{Source: source.ObjectValue()}

		got := (&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, false)

		if diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diagnostics.Errors())
		}
		if got.BigQuery == nil || got.BigQuery.Authentication != api.ClickPipeAuthenticationServiceAccountWorkloadIdentity {
			t.Fatalf("unexpected BigQuery source: %+v", got.BigQuery)
		}
		if got.BigQuery.ProjectID == nil || *got.BigQuery.ProjectID != "my-gcp-project" {
			t.Fatalf("unexpected BigQuery project ID: %+v", got.BigQuery.ProjectID)
		}
		if got.BigQuery.Credentials != nil {
			t.Errorf("workload identity source included customer credentials: %+v", got.BigQuery)
		}
		if !clickPipeSourceUsesGCPWorkloadIdentity(got) {
			t.Error("source was not detected as using workload identity")
		}
	})

	t.Run("rejects credentials for workload identity", func(t *testing.T) {
		source := workloadIdentitySourceModel()
		source.BigQuery = workloadIdentityBigQuery(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity, types.StringValue("my-gcp-project"), credentials)
		diagnostics := diag.Diagnostics{}

		validateGCPWorkloadIdentityConfiguration(context.Background(), source.ObjectValue(), &diagnostics)

		if !diagnostics.HasError() {
			t.Fatal("expected conflicting credentials error")
		}
	})

	t.Run("requires project ID for workload identity", func(t *testing.T) {
		source := workloadIdentitySourceModel()
		source.BigQuery = workloadIdentityBigQuery(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity, types.StringNull(), nullCredentials)
		diagnostics := diag.Diagnostics{}

		validateGCPWorkloadIdentityConfiguration(context.Background(), source.ObjectValue(), &diagnostics)

		if !diagnostics.HasError() {
			t.Fatal("expected missing project_id error")
		}
	})

	t.Run("requires credentials for service account", func(t *testing.T) {
		source := workloadIdentitySourceModel()
		source.BigQuery = workloadIdentityBigQuery(api.ClickPipeAuthenticationServiceAccount, types.StringNull(), nullCredentials)
		diagnostics := diag.Diagnostics{}

		validateGCPWorkloadIdentityConfiguration(context.Background(), source.ObjectValue(), &diagnostics)

		if !diagnostics.HasError() {
			t.Fatal("expected missing credentials error")
		}
	})

	t.Run("keeps service account credentials", func(t *testing.T) {
		source := workloadIdentitySourceModel()
		source.BigQuery = workloadIdentityBigQuery(api.ClickPipeAuthenticationServiceAccount, types.StringNull(), credentials)
		diagnostics := diag.Diagnostics{}
		plan := models.ClickPipeResourceModel{Source: source.ObjectValue()}

		got := (&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, false)

		if diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diagnostics.Errors())
		}
		if got.BigQuery == nil || got.BigQuery.Authentication != api.ClickPipeAuthenticationServiceAccount || got.BigQuery.Credentials == nil {
			t.Fatalf("unexpected BigQuery service account source: %+v", got.BigQuery)
		}
	})
}

func TestSyncClickPipeState_BigQueryWorkloadIdentity(t *testing.T) {
	ctx := context.Background()
	nullCredentials := types.ObjectNull(models.ClickPipeServiceAccountModel{}.ObjectType().AttrTypes)
	source := workloadIdentitySourceModel()
	source.BigQuery = workloadIdentityBigQuery(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity, types.StringValue("my-gcp-project"), nullCredentials)
	state := models.ClickPipeResourceModel{
		ID:        types.StringValue("pipe-id"),
		ServiceID: types.StringValue("service-id"),
		Name:      types.StringValue("BigQuery workload identity"),
		State:     types.StringValue("provisioning"),
		Source:    source.ObjectValue(),
		Destination: types.ObjectValueMust(models.ClickPipeDestinationModel{}.ObjectType().AttrTypes, map[string]attr.Value{
			"database":         types.StringValue("default"),
			"table":            types.StringNull(),
			"managed_table":    types.BoolNull(),
			"table_definition": types.ObjectNull(models.ClickPipeDestinationTableDefinitionModel{}.ObjectType().AttrTypes),
			"columns":          types.ListNull(models.ClickPipeDestinationColumnModel{}.ObjectType()),
			"roles":            types.ListNull(types.StringType),
		}),
	}
	projectID := "my-gcp-project"
	response := &api.ClickPipe{
		ID:    "pipe-id",
		Name:  "BigQuery workload identity",
		State: "running",
		Source: api.ClickPipeSource{BigQuery: &api.ClickPipeBigQuerySource{
			SnapshotStagingPath: "gs://staging-bucket/clickpipes/",
			Authentication:      api.ClickPipeAuthenticationServiceAccountWorkloadIdentity,
			ProjectID:           &projectID,
			Settings: api.ClickPipeBigQuerySettings{
				ReplicationMode: api.ClickPipeReplicationModeSnapshot,
			},
			Mappings: []api.ClickPipeBigQueryTableMapping{{
				SourceDatasetName: "events",
				SourceTable:       "source",
				TargetTable:       "target",
			}},
		}},
		Destination: api.ClickPipeDestination{Database: "default"},
	}
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc).
		GetClickPipeMock.
		Expect(ctx, "service-id", "pipe-id").
		Return(response, nil)

	err := (&ClickPipeResource{client: client}).syncClickPipeState(ctx, &state)
	if err != nil {
		t.Fatalf("sync state: %v", err)
	}
	var syncedSource models.ClickPipeSourceModel
	if diagnostics := state.Source.As(ctx, &syncedSource, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		t.Fatalf("read source state: %v", diagnostics.Errors())
	}
	var bigQuery models.ClickPipeBigQuerySourceModel
	if diagnostics := syncedSource.BigQuery.As(ctx, &bigQuery, basetypes.ObjectAsOptions{}); diagnostics.HasError() {
		t.Fatalf("read BigQuery state: %v", diagnostics.Errors())
	}
	if bigQuery.Authentication.ValueString() != api.ClickPipeAuthenticationServiceAccountWorkloadIdentity {
		t.Errorf("authentication = %q", bigQuery.Authentication.ValueString())
	}
	if bigQuery.ProjectID.ValueString() != projectID {
		t.Errorf("project_id = %q", bigQuery.ProjectID.ValueString())
	}
	if !bigQuery.Credentials.IsNull() {
		t.Error("credentials must remain null for workload identity")
	}
}
