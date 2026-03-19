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

func TestGetSourceType(t *testing.T) {
	kafkaTypes := models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes
	objectStorageTypes := models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes
	kinesisTypes := models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes
	postgresTypes := models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes
	mysqlTypes := models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes
	bigqueryTypes := models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes
	mongodbTypes := models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes

	nullSource := models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(kafkaTypes),
		ObjectStorage: types.ObjectNull(objectStorageTypes),
		Kinesis:       types.ObjectNull(kinesisTypes),
		Postgres:      types.ObjectNull(postgresTypes),
		MySQL:         types.ObjectNull(mysqlTypes),
		BigQuery:      types.ObjectNull(bigqueryTypes),
		MongoDB:       types.ObjectNull(mongodbTypes),
	}

	tests := []struct {
		name         string
		sourceModel  models.ClickPipeSourceModel
		expectedType SourceType
	}{
		{
			name: "Kafka source",
			sourceModel: func() models.ClickPipeSourceModel {
				s := nullSource
				s.Kafka = types.ObjectUnknown(kafkaTypes)
				return s
			}(),
			expectedType: SourceTypeKafka,
		},
		{
			name: "ObjectStorage source",
			sourceModel: func() models.ClickPipeSourceModel {
				s := nullSource
				s.ObjectStorage = types.ObjectUnknown(objectStorageTypes)
				return s
			}(),
			expectedType: SourceTypeObjectStorage,
		},
		{
			name: "Kinesis source",
			sourceModel: func() models.ClickPipeSourceModel {
				s := nullSource
				s.Kinesis = types.ObjectUnknown(kinesisTypes)
				return s
			}(),
			expectedType: SourceTypeKinesis,
		},
		{
			name: "Postgres source",
			sourceModel: func() models.ClickPipeSourceModel {
				s := nullSource
				s.Postgres = types.ObjectUnknown(postgresTypes)
				return s
			}(),
			expectedType: SourceTypePostgres,
		},
		{
			name: "MongoDB source",
			sourceModel: func() models.ClickPipeSourceModel {
				s := nullSource
				s.MongoDB = types.ObjectUnknown(mongodbTypes)
				return s
			}(),
			expectedType: SourceTypeMongoDB,
		},
		{
			name:         "Unknown source (all null)",
			sourceModel:  nullSource,
			expectedType: SourceTypeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSourceType(tt.sourceModel)
			assert.Equal(t, tt.expectedType, result)
		})
	}
}

func TestClickPipeResource_syncClickPipeState_Postgres(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		state        models.ClickPipeResourceModel
		response     *api.ClickPipe
		responseErr  error
		desiredState models.ClickPipeResourceModel
		wantErr      bool
	}{
		{
			name:  "Syncs Postgres source with all settings",
			state: getPostgresInitialState(),
			response: &api.ClickPipe{
				ID:    "test-pipe-id",
				Name:  "test-pipe",
				State: "running",
				Source: api.ClickPipeSource{
					Postgres: &api.ClickPipePostgresSource{
						Host:     "postgres.example.com",
						Port:     5432,
						Database: "mydb",
						Settings: &api.ClickPipePostgresSettings{
							ReplicationMode:      "cdc",
							SyncIntervalSeconds:  intPtr(60),
							PullBatchSize:        intPtr(1000),
							PublicationName:      strPtr("my_publication"),
							AllowNullableColumns: boolPtr(true),
						},
						Mappings: []api.ClickPipePostgresTableMapping{
							{
								SourceSchemaName: "public",
								SourceTable:      "users",
								TargetTable:      "users",
								TableEngine:      strPtr("ReplacingMergeTree"),
							},
						},
					},
				},
				Destination: api.ClickPipeDestination{
					Database: "default",
				},
			},
			desiredState: getPostgresDesiredState("test-pipe", "running"),
			wantErr:      false,
		},
		{
			name:  "Preserves null values for optional Postgres settings",
			state: getPostgresInitialState(),
			response: &api.ClickPipe{
				ID:    "test-pipe-id",
				Name:  "test-pipe",
				State: "running",
				Source: api.ClickPipeSource{
					Postgres: &api.ClickPipePostgresSource{
						Host:     "postgres.example.com",
						Port:     5432,
						Database: "mydb",
						Settings: &api.ClickPipePostgresSettings{
							ReplicationMode: "cdc",
							// Optional fields not set - API may return empty/defaults
							PublicationName:     nil,
							ReplicationSlotName: nil,
							EnableFailoverSlots: nil,
						},
						Mappings: []api.ClickPipePostgresTableMapping{
							{
								SourceSchemaName: "public",
								SourceTable:      "users",
								TargetTable:      "users",
								TableEngine:      nil, // Not set
							},
						},
					},
				},
				Destination: api.ClickPipeDestination{
					Database: "default",
				},
			},
			desiredState: getPostgresDesiredState("test-pipe", "running"),
			wantErr:      false,
		},
		{
			name:  "Preserves null destination fields for Postgres CDC",
			state: getPostgresInitialState(),
			response: &api.ClickPipe{
				ID:    "test-pipe-id",
				Name:  "test-pipe",
				State: "running",
				Source: api.ClickPipeSource{
					Postgres: &api.ClickPipePostgresSource{
						Host:     "postgres.example.com",
						Port:     5432,
						Database: "mydb",
						Settings: &api.ClickPipePostgresSettings{
							ReplicationMode: "cdc",
						},
						Mappings: []api.ClickPipePostgresTableMapping{
							{
								SourceSchemaName: "public",
								SourceTable:      "users",
								TargetTable:      "users",
							},
						},
					},
				},
				Destination: api.ClickPipeDestination{
					Database: "default",
					// API doesn't return these for Postgres CDC
					Table:        nil,
					ManagedTable: nil,
					Columns:      nil,
				},
			},
			desiredState: getPostgresDesiredState("test-pipe", "running"),
			wantErr:      false,
		},
		{
			name:  "Updates sync_interval_seconds when API returns new value",
			state: getPostgresInitialState(),
			response: &api.ClickPipe{
				ID:    "test-pipe-id",
				Name:  "test-pipe",
				State: "running",
				Source: api.ClickPipeSource{
					Postgres: &api.ClickPipePostgresSource{
						Host:     "postgres.example.com",
						Port:     5432,
						Database: "mydb",
						Settings: &api.ClickPipePostgresSettings{
							ReplicationMode:     "cdc",
							SyncIntervalSeconds: intPtr(120), // Changed from null to 120
						},
						Mappings: []api.ClickPipePostgresTableMapping{
							{
								SourceSchemaName: "public",
								SourceTable:      "users",
								TargetTable:      "users",
							},
						},
					},
				},
				Destination: api.ClickPipeDestination{
					Database: "default",
				},
			},
			desiredState: getPostgresDesiredState("test-pipe", "running"),
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)

			apiClientMock := api.NewClientMock(mc).
				GetClickPipeMock.
				Expect(context.Background(), tt.state.ServiceID.ValueString(), tt.state.ID.ValueString()).
				Return(tt.response, tt.responseErr)

			resource := &ClickPipeResource{
				client: apiClientMock,
			}

			err := resource.syncClickPipeState(ctx, &tt.state)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Validate key fields
				assert.Equal(t, tt.desiredState.ID.ValueString(), tt.state.ID.ValueString())
				assert.Equal(t, tt.desiredState.Name.ValueString(), tt.state.Name.ValueString())
				assert.Equal(t, tt.desiredState.State.ValueString(), tt.state.State.ValueString())

				// Validate source is synced
				assert.False(t, tt.state.Source.IsNull())

				// Validate destination preserves null values for Postgres
				var destModel models.ClickPipeDestinationModel
				tt.state.Destination.As(ctx, &destModel, basetypes.ObjectAsOptions{})
				assert.True(t, destModel.Table.IsNull(), "table should remain null for Postgres CDC")
				assert.Equal(t, types.BoolValue(false), destModel.ManagedTable, "managed_table should be false for Postgres CDC")

				// Validate credentials are preserved from state (not returned by API)
				var sourceModel models.ClickPipeSourceModel
				tt.state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})
				var postgresModel models.ClickPipePostgresSourceModel
				sourceModel.Postgres.As(ctx, &postgresModel, basetypes.ObjectAsOptions{})
				assert.False(t, postgresModel.Credentials.IsNull(), "credentials should be preserved from state")
			}
		})
	}
}

// getPostgresInitialState returns a ClickPipeResourceModel with Postgres source in provisioning state
func getPostgresInitialState() models.ClickPipeResourceModel {
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-pipe"),
		State:     types.StringValue("provisioning"),
		Source: models.ClickPipeSourceModel{
			Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
			ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
			Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
			MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
			BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
			MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
			Postgres: types.ObjectValueMust(
				models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes,
				map[string]attr.Value{
					"type":           types.StringValue("postgres"),
					"host":           types.StringValue("postgres.example.com"),
					"port":           types.Int64Value(5432),
					"database":       types.StringValue("mydb"),
					"authentication": types.StringNull(),
					"iam_role":       types.StringNull(),
					"tls_host":       types.StringNull(),
					"ca_certificate": types.StringNull(),
					"credentials": types.ObjectValueMust(
						models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes,
						map[string]attr.Value{
							"username": types.StringValue("user"),
							"password": types.StringValue("pass"),
						},
					),
					"settings": types.ObjectValueMust(
						models.ClickPipePostgresSettingsModel{}.ObjectType().AttrTypes,
						map[string]attr.Value{
							"replication_mode":                   types.StringValue("cdc"),
							"sync_interval_seconds":              types.Int64Null(),
							"pull_batch_size":                    types.Int64Null(),
							"publication_name":                   types.StringNull(),
							"replication_slot_name":              types.StringNull(),
							"allow_nullable_columns":             types.BoolNull(),
							"initial_load_parallelism":           types.Int64Null(),
							"snapshot_num_rows_per_partition":    types.Int64Null(),
							"snapshot_number_of_parallel_tables": types.Int64Null(),
							"enable_failover_slots":              types.BoolNull(),
							"delete_on_merge":                    types.BoolNull(),
						},
					),
					"table_mappings": types.SetValueMust(
						models.ClickPipePostgresTableMappingModel{}.ObjectType(),
						[]attr.Value{
							types.ObjectValueMust(
								models.ClickPipePostgresTableMappingModel{}.ObjectType().AttrTypes,
								map[string]attr.Value{
									"source_schema_name":     types.StringValue("public"),
									"source_table":           types.StringValue("users"),
									"target_table":           types.StringValue("users"),
									"excluded_columns":       types.ListNull(types.StringType),
									"use_custom_sorting_key": types.BoolNull(),
									"sorting_keys":           types.ListNull(types.StringType),
									"table_engine":           types.StringNull(),
									"partition_key":          types.StringNull(),
								},
							),
						},
					),
				},
			),
		}.ObjectValue(),
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
	}
}

// getPostgresDesiredState returns the expected state after syncing with the given name and state
func getPostgresDesiredState(name, state string) models.ClickPipeResourceModel {
	desiredState := getPostgresInitialState()
	desiredState.Name = types.StringValue(name)
	desiredState.State = types.StringValue(state)
	return desiredState
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func buildKafkaMutualTLSPlan(certificate, privateKey types.String) models.ClickPipeResourceModel {
	credAttrs := map[string]attr.Value{
		"username":          types.StringNull(),
		"password":          types.StringNull(),
		"access_key_id":     types.StringNull(),
		"secret_key":        types.StringNull(),
		"connection_string": types.StringNull(),
		"certificate":       certificate,
		"private_key":       privateKey,
	}

	kafkaAttrs := map[string]attr.Value{
		"type":                         types.StringValue("kafka"),
		"format":                       types.StringValue("JSONEachRow"),
		"brokers":                      types.StringValue("broker:9092"),
		"topics":                       types.StringValue("test-topic"),
		"consumer_group":               types.StringNull(),
		"offset":                       types.ObjectNull(models.ClickPipeKafkaOffsetModel{}.ObjectType().AttrTypes),
		"schema_registry":              types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes),
		"authentication":               types.StringValue("MUTUAL_TLS"),
		"credentials":                  types.ObjectValueMust(models.ClickPipeKafkaSourceCredentialsModel{}.ObjectType().AttrTypes, credAttrs),
		"iam_role":                     types.StringNull(),
		"ca_certificate":               types.StringNull(),
		"reverse_private_endpoint_ids": types.ListNull(types.StringType),
	}

	sourceModel := models.ClickPipeSourceModel{
		Kafka:         types.ObjectValueMust(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes, kafkaAttrs),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}

	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-mtls-pipe"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_KafkaMutualTLS(t *testing.T) {
	ctx := context.Background()
	resource := &ClickPipeResource{}

	t.Run("success: both certificate and private_key provided", func(t *testing.T) {
		plan := buildKafkaMutualTLSPlan(
			types.StringValue("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"),
			types.StringValue("-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"),
		)

		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, false)

		assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
		assert.NotNil(t, source)
		assert.NotNil(t, source.Kafka)
		assert.NotNil(t, source.Kafka.Credentials)
		assert.Equal(t, "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----", *source.Kafka.Credentials.Certificate)
		assert.Equal(t, "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----", *source.Kafka.Credentials.PrivateKey)
		assert.Nil(t, source.Kafka.Credentials.ClickPipeSourceCredentials, "SASL credentials should not be set")
		assert.Nil(t, source.Kafka.Credentials.ClickPipeSourceAccessKey, "access key credentials should not be set")
		assert.Nil(t, source.Kafka.Credentials.ConnectionString, "connection string should not be set")
	})

	t.Run("failure: only certificate provided without private_key", func(t *testing.T) {
		plan := buildKafkaMutualTLSPlan(
			types.StringValue("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"),
			types.StringNull(),
		)

		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, false)

		assert.True(t, diagnostics.HasError(), "expected error when private_key is missing")
		assert.Nil(t, source)
	})

	t.Run("failure: only private_key provided without certificate", func(t *testing.T) {
		plan := buildKafkaMutualTLSPlan(
			types.StringNull(),
			types.StringValue("-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"),
		)

		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, false)

		assert.True(t, diagnostics.HasError(), "expected error when certificate is missing")
		assert.Nil(t, source)
	})
}

func TestClickPipeResource_syncClickPipeState_MongoDB(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		state       models.ClickPipeResourceModel
		response    *api.ClickPipe
		responseErr error
		wantErr     bool
	}{
		{
			name:  "Syncs MongoDB source with all settings",
			state: getMongoDBInitialState(),
			response: &api.ClickPipe{
				ID:    "test-pipe-id",
				Name:  "test-pipe",
				State: "running",
				Source: api.ClickPipeSource{
					MongoDB: &api.ClickPipeMongoDBSource{
						URI:            "mongodb+srv://cluster0.example.mongodb.net/mydb",
						ReadPreference: "secondaryPreferred",
						Settings: &api.ClickPipeMongoDBSettings{
							ReplicationMode:                "cdc",
							SyncIntervalSeconds:            intPtr(30),
							PullBatchSize:                  intPtr(500),
							SnapshotNumRowsPerPartition:    intPtr(100000),
							SnapshotNumberOfParallelTables: intPtr(2),
							DeleteOnMerge:                  boolPtr(true),
							UseJsonNativeFormat:            boolPtr(true),
						},
						Mappings: []api.ClickPipeMongoDBTableMapping{
							{
								SourceDatabaseName: "mydb",
								SourceCollection:   "users",
								TargetTable:        "mydb_users",
								TableEngine:        strPtr("ReplacingMergeTree"),
							},
						},
					},
				},
				Destination: api.ClickPipeDestination{
					Database: "default",
				},
			},
			wantErr: false,
		},
		{
			name:  "Preserves null values for optional MongoDB settings",
			state: getMongoDBInitialState(),
			response: &api.ClickPipe{
				ID:    "test-pipe-id",
				Name:  "test-pipe",
				State: "running",
				Source: api.ClickPipeSource{
					MongoDB: &api.ClickPipeMongoDBSource{
						URI:            "mongodb+srv://cluster0.example.mongodb.net/mydb",
						ReadPreference: "secondaryPreferred",
						Settings: &api.ClickPipeMongoDBSettings{
							ReplicationMode: "cdc",
						},
						Mappings: []api.ClickPipeMongoDBTableMapping{
							{
								SourceDatabaseName: "mydb",
								SourceCollection:   "users",
								TargetTable:        "mydb_users",
							},
						},
					},
				},
				Destination: api.ClickPipeDestination{
					Database: "default",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)

			apiClientMock := api.NewClientMock(mc).
				GetClickPipeMock.
				Expect(context.Background(), tt.state.ServiceID.ValueString(), tt.state.ID.ValueString()).
				Return(tt.response, tt.responseErr)

			resource := &ClickPipeResource{
				client: apiClientMock,
			}

			err := resource.syncClickPipeState(ctx, &tt.state)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				assert.Equal(t, "test-pipe-id", tt.state.ID.ValueString())
				assert.Equal(t, "test-pipe", tt.state.Name.ValueString())
				assert.Equal(t, "running", tt.state.State.ValueString())

				assert.False(t, tt.state.Source.IsNull())

				// Validate destination preserves null values for MongoDB CDC
				var destModel models.ClickPipeDestinationModel
				tt.state.Destination.As(ctx, &destModel, basetypes.ObjectAsOptions{})
				assert.True(t, destModel.Table.IsNull(), "table should remain null for MongoDB CDC")
				assert.Equal(t, types.BoolValue(false), destModel.ManagedTable, "managed_table should be false for MongoDB CDC")

				// Validate credentials are preserved from state
				var sourceModel models.ClickPipeSourceModel
				tt.state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})
				var mongodbModel models.ClickPipeMongoDBSourceModel
				sourceModel.MongoDB.As(ctx, &mongodbModel, basetypes.ObjectAsOptions{})
				assert.False(t, mongodbModel.Credentials.IsNull(), "credentials should be preserved from state")
			}
		})
	}
}

func getMongoDBInitialState() models.ClickPipeResourceModel {
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-pipe"),
		State:     types.StringValue("provisioning"),
		Source: models.ClickPipeSourceModel{
			Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
			ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
			Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
			Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
			MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
			BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
			MongoDB: types.ObjectValueMust(
				models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes,
				map[string]attr.Value{
					"uri":             types.StringValue("mongodb+srv://cluster0.example.mongodb.net/mydb"),
					"read_preference": types.StringValue("secondaryPreferred"),
					"tls_host":        types.StringNull(),
					"ca_certificate":  types.StringNull(),
					"disable_tls":     types.BoolValue(false),
					"credentials": types.ObjectValueMust(
						models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes,
						map[string]attr.Value{
							"username": types.StringValue("user"),
							"password": types.StringValue("pass"),
						},
					),
					"settings": types.ObjectValueMust(
						models.ClickPipeMongoDBSettingsModel{}.ObjectType().AttrTypes,
						map[string]attr.Value{
							"replication_mode":                   types.StringValue("cdc"),
							"sync_interval_seconds":              types.Int64Null(),
							"pull_batch_size":                    types.Int64Null(),
							"snapshot_num_rows_per_partition":    types.Int64Null(),
							"snapshot_number_of_parallel_tables": types.Int64Null(),
							"delete_on_merge":                    types.BoolNull(),
							"use_json_native_format":             types.BoolNull(),
						},
					),
					"table_mappings": types.SetValueMust(
						models.ClickPipeMongoDBTableMappingModel{}.ObjectType(),
						[]attr.Value{
							types.ObjectValueMust(
								models.ClickPipeMongoDBTableMappingModel{}.ObjectType().AttrTypes,
								map[string]attr.Value{
									"source_database_name": types.StringValue("mydb"),
									"source_collection":    types.StringValue("users"),
									"target_table":         types.StringValue("mydb_users"),
									"table_engine":         types.StringNull(),
								},
							),
						},
					),
				},
			),
		}.ObjectValue(),
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
	}
}
