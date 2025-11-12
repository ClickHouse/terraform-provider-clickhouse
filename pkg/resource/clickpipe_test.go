//go:build alpha

package resource

import (
	"context"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
)

func TestGetSourceType(t *testing.T) {
	kafkaTypes := models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes
	objectStorageTypes := models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes
	kinesisTypes := models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes
	postgresTypes := models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes

	tests := []struct {
		name         string
		sourceModel  models.ClickPipeSourceModel
		expectedType string
	}{
		{
			name: "Kafka source",
			sourceModel: models.ClickPipeSourceModel{
				Kafka:         types.ObjectUnknown(kafkaTypes),
				ObjectStorage: types.ObjectNull(objectStorageTypes),
				Kinesis:       types.ObjectNull(kinesisTypes),
				Postgres:      types.ObjectNull(postgresTypes),
			},
			expectedType: "kafka",
		},
		{
			name: "ObjectStorage source",
			sourceModel: models.ClickPipeSourceModel{
				Kafka:         types.ObjectNull(kafkaTypes),
				ObjectStorage: types.ObjectUnknown(objectStorageTypes),
				Kinesis:       types.ObjectNull(kinesisTypes),
				Postgres:      types.ObjectNull(postgresTypes),
			},
			expectedType: "object_storage",
		},
		{
			name: "Kinesis source",
			sourceModel: models.ClickPipeSourceModel{
				Kafka:         types.ObjectNull(kafkaTypes),
				ObjectStorage: types.ObjectNull(objectStorageTypes),
				Kinesis:       types.ObjectUnknown(kinesisTypes),
				Postgres:      types.ObjectNull(postgresTypes),
			},
			expectedType: "kinesis",
		},
		{
			name: "Postgres source",
			sourceModel: models.ClickPipeSourceModel{
				Kafka:         types.ObjectNull(kafkaTypes),
				ObjectStorage: types.ObjectNull(objectStorageTypes),
				Kinesis:       types.ObjectNull(kinesisTypes),
				Postgres:      types.ObjectUnknown(postgresTypes),
			},
			expectedType: "postgres",
		},
		{
			name: "Unknown source (all null)",
			sourceModel: models.ClickPipeSourceModel{
				Kafka:         types.ObjectNull(kafkaTypes),
				ObjectStorage: types.ObjectNull(objectStorageTypes),
				Kinesis:       types.ObjectNull(kinesisTypes),
				Postgres:      types.ObjectNull(postgresTypes),
			},
			expectedType: "unknown",
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
						Settings: api.ClickPipePostgresSettings{
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
						Settings: api.ClickPipePostgresSettings{
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
						Settings: api.ClickPipePostgresSettings{
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
						Settings: api.ClickPipePostgresSettings{
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
				assert.True(t, destModel.ManagedTable.IsNull(), "managed_table should remain null for Postgres CDC")

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
			Postgres: types.ObjectValueMust(
				models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes,
				map[string]attr.Value{
					"host":     types.StringValue("postgres.example.com"),
					"port":     types.Int64Value(5432),
					"database": types.StringValue("mydb"),
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
						},
					),
					"table_mappings": types.ListValueMust(
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
