package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

func int64Ptr(i int64) *int64 { return &i }

func TestClickPipeResource_syncClickPipeState_KafkaSchemaRegistryImport(t *testing.T) {
	ctx := context.Background()

	state := models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("test-service-id"),
		Source:    types.ObjectNull(models.ClickPipeSourceModel{}.ObjectType().AttrTypes),
	}

	mc := minimock.NewController(t)
	apiClientMock := api.NewClientMock(mc).
		GetClickPipeMock.
		Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
		Return(&api.ClickPipe{
			ID:    "test-pipe-id",
			Name:  "test-pipe",
			State: "Running",
			Source: api.ClickPipeSource{
				Kafka: &api.ClickPipeKafkaSource{
					Type:           "confluent",
					Format:         "Protobuf",
					Brokers:        "broker:9092",
					Topics:         "test-topic",
					Authentication: "PLAIN",
					SchemaRegistry: &api.ClickPipeKafkaSchemaRegistry{
						URL:            "https://schema-registry.example/schemas/ids/1",
						Authentication: "PLAIN",
					},
				},
			},
			Destination: api.ClickPipeDestination{Database: "default"},
		}, nil)

	r := &ClickPipeResource{client: apiClientMock}

	err := r.syncClickPipeState(ctx, &state)
	assert.NoError(t, err)

	var sourceModel models.ClickPipeSourceModel
	state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})
	var kafkaModel models.ClickPipeKafkaSourceModel
	sourceModel.Kafka.As(ctx, &kafkaModel, basetypes.ObjectAsOptions{})
	assert.False(t, kafkaModel.SchemaRegistry.IsNull(), "schema_registry should be populated after sync")
	var srModel models.ClickPipeKafkaSchemaRegistryModel
	kafkaModel.SchemaRegistry.As(ctx, &srModel, basetypes.ObjectAsOptions{})
	assert.Equal(t, "https://schema-registry.example/schemas/ids/1", srModel.URL.ValueString())
	assert.Equal(t, "PLAIN", srModel.Authentication.ValueString())
	assert.True(t, srModel.Credentials.IsNull(), "credentials should be a typed null (not present in API response)")
}

// ============================================================================
// Issue #528 — Postgres/MySQL credentials become "undefined" when
// lifecycle.ignore_changes hides them during update.
//
// Fix: extractSourceFromPlan now treats a null/unknown credentials block as
// "omit credentials from the PATCH" instead of serializing empty strings that
// the API rejects as the literal text "undefined".
// ============================================================================

// buildPostgresPlanWithCredentials returns a minimal Postgres-source plan model.
// Pass any credentials Object (null, unknown, or populated) to exercise the
// extractSourceFromPlan branches that the issue-#528 fix introduced.
func buildPostgresPlanWithCredentials(credentials types.Object) models.ClickPipeResourceModel {
	settingsAttrs := map[string]attr.Value{
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
	}
	tableMappingAttrs := map[string]attr.Value{
		"source_schema_name":     types.StringValue("public"),
		"source_table":           types.StringValue("users"),
		"target_table":           types.StringValue("users"),
		"excluded_columns":       types.SetNull(types.StringType),
		"use_custom_sorting_key": types.BoolNull(),
		"sorting_keys":           types.ListNull(types.StringType),
		"table_engine":           types.StringNull(),
		"partition_key":          types.StringNull(),
	}
	pgAttrs := map[string]attr.Value{
		"type":           types.StringValue("postgres"),
		"host":           types.StringValue("postgres.example.com"),
		"port":           types.Int64Value(5432),
		"database":       types.StringValue("mydb"),
		"authentication": types.StringNull(),
		"iam_role":       types.StringNull(),
		"tls_host":       types.StringNull(),
		"ca_certificate": types.StringNull(),
		"credentials":    credentials,
		"settings":       types.ObjectValueMust(models.ClickPipePostgresSettingsModel{}.ObjectType().AttrTypes, settingsAttrs),
		"table_mappings": types.SetValueMust(models.ClickPipePostgresTableMappingModel{}.ObjectType(), []attr.Value{
			types.ObjectValueMust(models.ClickPipePostgresTableMappingModel{}.ObjectType().AttrTypes, tableMappingAttrs),
		}),
	}
	sourceModel := models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
		Postgres:      types.ObjectValueMust(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes, pgAttrs),
		MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-pg-pipe"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_PostgresIgnoreChangesCredentials(t *testing.T) {
	// Regression scenarios for issue #528 — when ignore_changes = [credentials]
	// is in play, an update to an unrelated field (e.g. table_mappings) must NOT
	// send credentials in the PATCH body. The pre-fix provider serialized
	// credentials.username/password as "undefined", which the API rejects with 400.
	ctx := context.Background()
	resource := &ClickPipeResource{}
	credentialsType := models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes

	t.Run("null credentials block omits credentials from API source", func(t *testing.T) {
		plan := buildPostgresPlanWithCredentials(types.ObjectNull(credentialsType))
		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

		assert.False(t, diagnostics.HasError(), "credential-shape validation must be skipped when block is null: %v", diagnostics.Errors())
		assert.NotNil(t, source)
		assert.NotNil(t, source.Postgres)
		assert.Nil(t, source.Postgres.Credentials, "credentials must be nil so JSON serialization omits the field entirely (issue #528)")
	})

	t.Run("unknown credentials block omits credentials from API source", func(t *testing.T) {
		plan := buildPostgresPlanWithCredentials(types.ObjectUnknown(credentialsType))
		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

		assert.False(t, diagnostics.HasError(), "validation must be skipped when credentials are unknown: %v", diagnostics.Errors())
		assert.NotNil(t, source.Postgres)
		assert.Nil(t, source.Postgres.Credentials, "credentials must be nil when block is unknown")
	})

	t.Run("empty username inside credentials block omits credentials from API source", func(t *testing.T) {
		// Defensive: a state corruption could leave the block populated but with
		// blank values. The fix guards against `Username == ""` so we never send
		// the literal "undefined" to the API.
		emptyCreds := types.ObjectValueMust(credentialsType, map[string]attr.Value{
			"username":            types.StringValue(""),
			"password":            types.StringNull(),
			"password_wo":         types.StringNull(),
			"password_wo_version": types.Int64Null(),
		})
		plan := buildPostgresPlanWithCredentials(emptyCreds)
		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

		assert.NotNil(t, source.Postgres)
		assert.Nil(t, source.Postgres.Credentials, "credentials must be omitted when username is the empty string")
	})

	t.Run("populated credentials forwarded normally (control)", func(t *testing.T) {
		validCreds := types.ObjectValueMust(credentialsType, map[string]attr.Value{
			"username":            types.StringValue("alice"),
			"password":            types.StringValue("secret"),
			"password_wo":         types.StringNull(),
			"password_wo_version": types.Int64Null(),
		})
		plan := buildPostgresPlanWithCredentials(validCreds)
		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

		assert.False(t, diagnostics.HasError())
		assert.NotNil(t, source.Postgres.Credentials)
		assert.Equal(t, "alice", source.Postgres.Credentials.Username)
		assert.Equal(t, "secret", source.Postgres.Credentials.Password)
	})
}

// buildMySQLPlanWithCredentials returns a minimal MySQL-source plan model.
func buildMySQLPlanWithCredentials(credentials types.Object) models.ClickPipeResourceModel {
	settingsAttrs := map[string]attr.Value{
		"replication_mode":                   types.StringValue("cdc"),
		"sync_interval_seconds":              types.Int64Null(),
		"pull_batch_size":                    types.Int64Null(),
		"replication_mechanism":              types.StringNull(),
		"use_compression":                    types.BoolNull(),
		"allow_nullable_columns":             types.BoolNull(),
		"initial_load_parallelism":           types.Int64Null(),
		"snapshot_num_rows_per_partition":    types.Int64Null(),
		"snapshot_number_of_parallel_tables": types.Int64Null(),
		"delete_on_merge":                    types.BoolNull(),
	}
	tableMappingAttrs := map[string]attr.Value{
		"source_schema_name":     types.StringValue("public"),
		"source_table":           types.StringValue("users"),
		"target_table":           types.StringValue("users"),
		"excluded_columns":       types.SetNull(types.StringType),
		"use_custom_sorting_key": types.BoolNull(),
		"sorting_keys":           types.ListNull(types.StringType),
		"table_engine":           types.StringNull(),
		"partition_key":          types.StringNull(),
	}
	mysqlAttrs := map[string]attr.Value{
		"type":                   types.StringValue("mysql"),
		"host":                   types.StringValue("mysql.example.com"),
		"port":                   types.Int64Value(3306),
		"authentication":         types.StringNull(),
		"iam_role":               types.StringNull(),
		"tls_host":               types.StringNull(),
		"ca_certificate":         types.StringNull(),
		"disable_tls":            types.BoolNull(),
		"skip_cert_verification": types.BoolNull(),
		"credentials":            credentials,
		"settings":               types.ObjectValueMust(models.ClickPipeMySQLSettingsModel{}.ObjectType().AttrTypes, settingsAttrs),
		"table_mappings": types.SetValueMust(models.ClickPipeMySQLTableMappingModel{}.ObjectType(), []attr.Value{
			types.ObjectValueMust(models.ClickPipeMySQLTableMappingModel{}.ObjectType().AttrTypes, tableMappingAttrs),
		}),
	}
	sourceModel := models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
		Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		MySQL:         types.ObjectValueMust(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes, mysqlAttrs),
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-mysql-pipe"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_MySQLIgnoreChangesCredentials(t *testing.T) {
	// Mirror of the Postgres scenarios above. The MySQL extractSourceFromPlan
	// branch received the same #528 fix.
	ctx := context.Background()
	resource := &ClickPipeResource{}
	credentialsType := models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes

	t.Run("null credentials block omits credentials from API source", func(t *testing.T) {
		plan := buildMySQLPlanWithCredentials(types.ObjectNull(credentialsType))
		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

		assert.False(t, diagnostics.HasError(), "validation must be skipped when block is null: %v", diagnostics.Errors())
		assert.NotNil(t, source.MySQL)
		assert.Nil(t, source.MySQL.Credentials, "credentials must be nil so JSON serialization omits the field (issue #528)")
	})

	t.Run("unknown credentials block omits credentials from API source", func(t *testing.T) {
		plan := buildMySQLPlanWithCredentials(types.ObjectUnknown(credentialsType))
		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

		assert.False(t, diagnostics.HasError())
		assert.Nil(t, source.MySQL.Credentials)
	})

	t.Run("populated credentials forwarded normally (control)", func(t *testing.T) {
		validCreds := types.ObjectValueMust(credentialsType, map[string]attr.Value{
			"username":            types.StringValue("bob"),
			"password":            types.StringValue("hunter2"),
			"password_wo":         types.StringNull(),
			"password_wo_version": types.Int64Null(),
		})
		plan := buildMySQLPlanWithCredentials(validCreds)
		diagnostics := diag.Diagnostics{}
		source := resource.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

		assert.False(t, diagnostics.HasError())
		assert.NotNil(t, source.MySQL.Credentials)
		assert.Equal(t, "bob", source.MySQL.Credentials.Username)
		assert.Equal(t, "hunter2", source.MySQL.Credentials.Password)
	})
}

// ============================================================================
// Issue #513 — Provider produces inconsistent result after apply when
// scaling.replica_cpu_millicores / replica_memory_gb come back as 0 from the
// API immediately after a create or scaling PATCH (transient propagation
// delay before the API surfaces the real values).
//
// Fix: syncClickPipeState rejects sub-threshold API values (cpu < 125, mem <
// 0.5) and falls back to the prior planned values in state.
// ============================================================================

// getPostgresStateWithScaling extends the Postgres initial state helper with a
// `scaling` block. Scaling logic in syncClickPipeState is source-agnostic, so
// reusing the Postgres fixture keeps the test compact.
func getPostgresStateWithScaling(replicas int64, cpu int64, mem float64) models.ClickPipeResourceModel {
	state := getPostgresInitialState()
	state.Scaling = types.ObjectValueMust(models.ClickPipeScalingModel{}.ObjectType().AttrTypes, map[string]attr.Value{
		"replicas":               types.Int64Value(replicas),
		"replica_cpu_millicores": types.Int64Value(cpu),
		"replica_memory_gb":      types.Float64Value(mem),
	})
	return state
}

func TestClickPipeResource_syncClickPipeState_ScalingPropagation(t *testing.T) {
	// Regression scenarios for issue #513. Building blocks:
	//   - prior state always carries cpu=125, mem=0.5 (the user's valid plan values)
	//   - response.Scaling varies (0 = transient/un-propagated, 125/0.5 = settled)
	//   - assertion: the state after sync reflects the user's planned values whenever
	//     the API value is sub-threshold; the API value is used only when valid.
	ctx := context.Background()

	tests := []struct {
		name          string
		state         models.ClickPipeResourceModel
		responseCpu   interface{} // value to put into ClickPipeScaling.ReplicaCpuMillicores
		responseMem   interface{} // value to put into ClickPipeScaling.ReplicaMemoryGb
		expectedCpu   int64
		expectedMem   float64
		preservedNote string
	}{
		{
			name:          "valid scaling from API is used as-is (control)",
			state:         getPostgresStateWithScaling(2, 125, 0.5),
			responseCpu:   float64(250),
			responseMem:   float64(1.0),
			expectedCpu:   250,
			expectedMem:   1.0,
			preservedNote: "API supplied valid values; sync should adopt them",
		},
		{
			name:          "transient zero CPU falls back to prior planned value",
			state:         getPostgresStateWithScaling(2, 125, 0.5),
			responseCpu:   float64(0),
			responseMem:   float64(0.5),
			expectedCpu:   125,
			expectedMem:   0.5,
			preservedNote: "CPU < 125 threshold → preserve prior; memory at threshold → adopt",
		},
		{
			name:          "transient zero memory falls back to prior planned value",
			state:         getPostgresStateWithScaling(2, 125, 0.5),
			responseCpu:   float64(125),
			responseMem:   float64(0),
			expectedCpu:   125,
			expectedMem:   0.5,
			preservedNote: "CPU at threshold → adopt; memory < 0.5 threshold → preserve prior",
		},
		{
			name:          "both transient zeros fall back to prior values (#513 main repro)",
			state:         getPostgresStateWithScaling(2, 125, 0.5),
			responseCpu:   float64(0),
			responseMem:   float64(0),
			expectedCpu:   125,
			expectedMem:   0.5,
			preservedNote: "matches the exact symptom in issue #513",
		},
		{
			name:          "below-threshold non-zero values still fall back",
			state:         getPostgresStateWithScaling(2, 125, 0.5),
			responseCpu:   float64(124),
			responseMem:   float64(0.4),
			expectedCpu:   125,
			expectedMem:   0.5,
			preservedNote: "guard is strictly ≥125 / ≥0.5; just-under should still preserve",
		},
		{
			name:          "boundary threshold values are adopted (not preserved)",
			state:         getPostgresStateWithScaling(2, 200, 0.7),
			responseCpu:   float64(125),
			responseMem:   float64(0.5),
			expectedCpu:   125,
			expectedMem:   0.5,
			preservedNote: "values exactly at threshold are valid; prior should NOT win",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)
			apiClientMock := api.NewClientMock(mc).
				GetClickPipeMock.
				Expect(context.Background(), tt.state.ServiceID.ValueString(), tt.state.ID.ValueString()).
				Return(&api.ClickPipe{
					ID:    "test-pipe-id",
					Name:  "test-pipe",
					State: "running",
					Source: api.ClickPipeSource{
						Postgres: &api.ClickPipePostgresSource{
							Host:     "postgres.example.com",
							Port:     5432,
							Database: "mydb",
							Settings: &api.ClickPipePostgresSettings{ReplicationMode: "cdc"},
							Mappings: []api.ClickPipePostgresTableMapping{{
								SourceSchemaName: "public",
								SourceTable:      "users",
								TargetTable:      "users",
							}},
						},
					},
					Destination: api.ClickPipeDestination{Database: "default"},
					Scaling: &api.ClickPipeScaling{
						Replicas:             int64Ptr(2),
						ReplicaCpuMillicores: tt.responseCpu,
						ReplicaMemoryGb:      tt.responseMem,
					},
				}, nil)

			resource := &ClickPipeResource{client: apiClientMock}

			err := resource.syncClickPipeState(ctx, &tt.state)
			assert.NoError(t, err)

			var scalingModel models.ClickPipeScalingModel
			tt.state.Scaling.As(ctx, &scalingModel, basetypes.ObjectAsOptions{})

			assert.Equal(t, tt.expectedCpu, scalingModel.ReplicaCpuMillicores.ValueInt64(),
				"CPU millicores expectation — %s", tt.preservedNote)
			assert.Equal(t, tt.expectedMem, scalingModel.ReplicaMemoryGb.ValueFloat64(),
				"Memory GB expectation — %s", tt.preservedNote)
		})
	}
}

// TestClickPipeResource_syncClickPipeState_ReplicasPropagation covers the same
// transient-propagation guard as the CPU/memory cases above, applied to
// `replicas`. Issue #513 only surfaced cpu/memory going to 0, but a nil or 0
// replicas reading from the API would trip the identical consistency check, so
// the fallback is mirrored defensively.
func TestClickPipeResource_syncClickPipeState_ReplicasPropagation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		responseReplicas *int64
		expectedReplicas int64
		note             string
	}{
		{
			name:             "valid replicas from API is used as-is (control)",
			responseReplicas: int64Ptr(3),
			expectedReplicas: 3,
			note:             "API supplied a valid replica count; sync should adopt it",
		},
		{
			name:             "transient zero replicas falls back to prior planned value",
			responseReplicas: int64Ptr(0),
			expectedReplicas: 2,
			note:             "replicas < 1 → not propagated yet → preserve prior",
		},
		{
			name:             "nil replicas falls back to prior planned value",
			responseReplicas: nil,
			expectedReplicas: 2,
			note:             "missing replicas in response → preserve prior",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := getPostgresStateWithScaling(2, 125, 0.5)

			mc := minimock.NewController(t)
			apiClientMock := api.NewClientMock(mc).
				GetClickPipeMock.
				Expect(context.Background(), state.ServiceID.ValueString(), state.ID.ValueString()).
				Return(&api.ClickPipe{
					ID:    "test-pipe-id",
					Name:  "test-pipe",
					State: "running",
					Source: api.ClickPipeSource{
						Postgres: &api.ClickPipePostgresSource{
							Host:     "postgres.example.com",
							Port:     5432,
							Database: "mydb",
							Settings: &api.ClickPipePostgresSettings{ReplicationMode: "cdc"},
							Mappings: []api.ClickPipePostgresTableMapping{{
								SourceSchemaName: "public",
								SourceTable:      "users",
								TargetTable:      "users",
							}},
						},
					},
					Destination: api.ClickPipeDestination{Database: "default"},
					Scaling: &api.ClickPipeScaling{
						Replicas:             tt.responseReplicas,
						ReplicaCpuMillicores: float64(125),
						ReplicaMemoryGb:      float64(0.5),
					},
				}, nil)

			resource := &ClickPipeResource{client: apiClientMock}

			err := resource.syncClickPipeState(ctx, &state)
			assert.NoError(t, err)

			var scalingModel models.ClickPipeScalingModel
			state.Scaling.As(ctx, &scalingModel, basetypes.ObjectAsOptions{})

			assert.Equal(t, tt.expectedReplicas, scalingModel.Replicas.ValueInt64(), tt.note)
		})
	}
}

// ============================================================================
// Pause/Paused state recognition — supporting the issue #529 fix.
//
// CDC pipes (Postgres / MySQL / MongoDB) settle in `Paused`, not `Stopped`,
// when paused via the API. getStateCheckFunc must accept both terminal states
// when stopped=true, otherwise the Update flow's wait loop times out and
// surfaces a spurious "didn't reach desired state" warning.
// ============================================================================

func TestGetStateCheckFunc_AcceptsPausedAndStopped(t *testing.T) {
	resource := &ClickPipeResource{}

	// Minimal plan: only `stopped` matters for the early-return branch of
	// getStateCheckFunc. Source can be entirely null.
	plan := models.ClickPipeResourceModel{
		Stopped: types.BoolValue(true),
		Source: models.ClickPipeSourceModel{
			Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
			ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
			Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
			PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
			Postgres:      types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
			MySQL:         types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
			BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
			MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
		}.ObjectValue(),
	}

	checker := resource.getStateCheckFunc(context.Background(), plan)

	assert.True(t, checker(api.ClickPipeStoppedState), "streaming pipes settle in Stopped — must be accepted")
	assert.True(t, checker(api.ClickPipePausedState), "CDC pipes settle in Paused — must be accepted")
	assert.False(t, checker(api.ClickPipeRunningState), "Running must NOT match when stopped=true")
	assert.False(t, checker(api.ClickPipeSnapShotState), "Snapshot is transient, not terminal")
	assert.False(t, checker(api.ClickPipeProvisioningState), "Provisioning is transient, not terminal")
}

// Issue #558 — reordering excluded_columns (now a Set, was an ordered List) must not trip the "table mappings are immutable" ModifyPlan guard.

// postgresPlanWithExcludedColumns clones the complete Postgres fixture and
// overrides the single table mapping's excluded_columns (as a Set) and
// target_table. Everything else is identical between the state and plan we build
// from it, so ModifyPlan sees no change other than what the test varies.
func postgresPlanWithExcludedColumns(ctx context.Context, excludedColumns []string, targetTable string) models.ClickPipeResourceModel {
	state := getPostgresInitialState()

	// getPostgresInitialState leaves these complex-typed fields as zero values,
	// which carry no element/attribute types. tfsdk.State.Set rejects that, so
	// fill them with correctly-typed nulls before encoding against the schema.
	state.Scaling = types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes)
	state.FieldMappings = types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType())
	state.Settings = types.DynamicNull()
	state.Stopped = types.BoolNull()
	state.TriggerResync = types.BoolNull()

	excludedVals := make([]attr.Value, len(excludedColumns))
	for i, c := range excludedColumns {
		excludedVals[i] = types.StringValue(c)
	}
	mappingAttrs := map[string]attr.Value{
		"source_schema_name":     types.StringValue("public"),
		"source_table":           types.StringValue("users"),
		"target_table":           types.StringValue(targetTable),
		"excluded_columns":       types.SetValueMust(types.StringType, excludedVals),
		"use_custom_sorting_key": types.BoolNull(),
		"sorting_keys":           types.ListNull(types.StringType), // sorting_keys stays an ordered List
		"table_engine":           types.StringNull(),
		"partition_key":          types.StringNull(),
	}

	var src models.ClickPipeSourceModel
	state.Source.As(ctx, &src, basetypes.ObjectAsOptions{})
	var pg models.ClickPipePostgresSourceModel
	src.Postgres.As(ctx, &pg, basetypes.ObjectAsOptions{})
	pg.TableMappings = types.SetValueMust(
		models.ClickPipePostgresTableMappingModel{}.ObjectType(),
		[]attr.Value{
			types.ObjectValueMust(models.ClickPipePostgresTableMappingModel{}.ObjectType().AttrTypes, mappingAttrs),
		},
	)
	src.Postgres = pg.ObjectValue()
	state.Source = src.ObjectValue()
	return state
}

func TestClickPipeResource_ModifyPlan_ExcludedColumnsReorder_Issue558(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building resource schema failed: %v", schemaResp.Diagnostics.Errors())
	}
	sch := schemaResp.Schema

	// run drives ModifyPlan for an update (non-null state + plan) and returns the
	// resulting diagnostics.
	run := func(t *testing.T, stateModel, planModel models.ClickPipeResourceModel) diag.Diagnostics {
		t.Helper()
		stateVal := tfsdk.State{Schema: sch}
		if d := stateVal.Set(ctx, &stateModel); d.HasError() {
			t.Fatalf("encoding prior state failed: %v", d.Errors())
		}
		planVal := tfsdk.Plan{Schema: sch}
		if d := planVal.Set(ctx, &planModel); d.HasError() {
			t.Fatalf("encoding plan failed: %v", d.Errors())
		}
		req := resource.ModifyPlanRequest{
			State:  stateVal,
			Plan:   planVal,
			Config: tfsdk.Config{Schema: sch, Raw: planVal.Raw},
		}
		resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: sch, Raw: planVal.Raw}}
		r.ModifyPlan(ctx, req, resp)
		return resp.Diagnostics
	}

	// detailContains reports whether any error diagnostic's detail contains substr.
	// We assert on the specific immutability message rather than HasError() so the
	// test is robust against unrelated ModifyPlan diagnostics.
	detailContains := func(diags diag.Diagnostics, substr string) bool {
		for _, d := range diags.Errors() {
			if strings.Contains(d.Detail(), substr) {
				return true
			}
		}
		return false
	}

	t.Run("reordered excluded_columns is not a forbidden modification", func(t *testing.T) {
		// state = order a refresh wrote (API order); plan = config order. Same set.
		state := postgresPlanWithExcludedColumns(ctx, []string{"status", "status_source", "created_at", "updated_at"}, "users")
		plan := postgresPlanWithExcludedColumns(ctx, []string{"updated_at", "created_at", "status", "status_source"}, "users")

		diags := run(t, state, plan)

		assert.False(t, detailContains(diags, "Cannot modify excluded_columns"),
			"#558: reordering excluded_columns must NOT be flagged as a mapping modification; got: %v", diags.Errors())
	})

	t.Run("changing target_table is still rejected (guard intact)", func(t *testing.T) {
		// Control: the immutability guard must still fire on a genuine change.
		state := postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users")
		plan := postgresPlanWithExcludedColumns(ctx, []string{"status"}, "users_renamed")

		diags := run(t, state, plan)

		assert.True(t, detailContains(diags, "Cannot modify target_table"),
			"a real target_table change on an existing mapping must still be rejected; got: %v", diags.Errors())
	})
}

// Issue #571 — ModifyPlan must reject destination.table_definition on a CDC pipe (it caused "inconsistent result after apply"), and must not fire when it's omitted.

// postgresPlanWithTableDefinition clones the Postgres fixture, optionally adding a minimal destination.table_definition (MergeTree).
func postgresPlanWithTableDefinition(withTableDef bool) models.ClickPipeResourceModel {
	plan := getPostgresInitialState()

	plan.Scaling = types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes)
	plan.FieldMappings = types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType())
	plan.Settings = types.DynamicNull()
	plan.Stopped = types.BoolValue(false)
	plan.TriggerResync = types.BoolNull()

	tableDef := types.ObjectNull(models.ClickPipeDestinationTableDefinitionModel{}.ObjectType().AttrTypes)
	if withTableDef {
		tableDef = models.ClickPipeDestinationTableDefinitionModel{
			Engine: models.ClickPipeDestinationTableEngineModel{
				Type:            types.StringValue("MergeTree"),
				VersionColumnID: types.StringNull(),
				ColumnIDs:       types.ListNull(types.StringType),
			}.ObjectValue(),
			SortingKey:  types.ListValueMust(types.StringType, []attr.Value{}),
			PartitionBy: types.StringNull(),
			PrimaryKey:  types.StringNull(),
		}.ObjectValue()
	}

	plan.Destination = models.ClickPipeDestinationModel{
		Database:        types.StringValue("default"),
		Table:           types.StringNull(),
		ManagedTable:    types.BoolNull(),
		TableDefinition: tableDef,
		Columns:         types.ListNull(models.ClickPipeDestinationColumnModel{}.ObjectType()),
		Roles:           types.ListNull(types.StringType),
	}.ObjectValue()

	return plan
}

func TestClickPipeResource_ModifyPlan_RejectsTableDefinitionForCDC_Issue571(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building resource schema failed: %v", schemaResp.Diagnostics.Errors())
	}
	sch := schemaResp.Schema

	// runCreate drives ModifyPlan for a create (null prior state) and returns the diagnostics.
	runCreate := func(t *testing.T, planModel models.ClickPipeResourceModel) diag.Diagnostics {
		t.Helper()
		planVal := tfsdk.Plan{Schema: sch}
		if d := planVal.Set(ctx, &planModel); d.HasError() {
			t.Fatalf("encoding plan failed: %v", d.Errors())
		}
		req := resource.ModifyPlanRequest{
			State:  tfsdk.State{Schema: sch}, // null Raw => create
			Plan:   planVal,
			Config: tfsdk.Config{Schema: sch, Raw: planVal.Raw},
		}
		resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: sch, Raw: planVal.Raw}}
		r.ModifyPlan(ctx, req, resp)
		return resp.Diagnostics
	}

	detailContains := func(diags diag.Diagnostics, substr string) bool {
		for _, d := range diags.Errors() {
			if strings.Contains(d.Detail(), substr) {
				return true
			}
		}
		return false
	}

	t.Run("table_definition on a CDC pipe is rejected at plan time", func(t *testing.T) {
		diags := runCreate(t, postgresPlanWithTableDefinition(true))

		assert.True(t, detailContains(diags, "destination.table_definition cannot be used"),
			"#571: a table_definition on a Postgres CDC pipe must be rejected at plan time; got: %v", diags.Errors())
	})

	t.Run("omitting table_definition on a CDC pipe is allowed", func(t *testing.T) {
		// Control: the guard must not over-fire when table_definition is absent.
		diags := runCreate(t, postgresPlanWithTableDefinition(false))

		assert.False(t, detailContains(diags, "destination.table_definition cannot be used"),
			"a Postgres CDC pipe without table_definition must not be rejected; got: %v", diags.Errors())
	})
}
