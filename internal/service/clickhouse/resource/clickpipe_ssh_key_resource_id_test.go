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

// dbSourceModels returns a ClickPipeSourceModel with every source null except the
// one supplied by the caller, so the Postgres and MySQL builders below stay short.
func dbSourceModels(postgres, mysql types.Object) models.ClickPipeSourceModel {
	return models.ClickPipeSourceModel{
		Kafka:         types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes),
		ObjectStorage: types.ObjectNull(models.ClickPipeObjectStorageSourceModel{}.ObjectType().AttrTypes),
		Kinesis:       types.ObjectNull(models.ClickPipeKinesisSourceModel{}.ObjectType().AttrTypes),
		PubSub:        types.ObjectNull(models.ClickPipePubSubSourceModel{}.ObjectType().AttrTypes),
		Postgres:      postgres,
		MySQL:         mysql,
		BigQuery:      types.ObjectNull(models.ClickPipeBigQuerySourceModel{}.ObjectType().AttrTypes),
		MongoDB:       types.ObjectNull(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes),
	}
}

func dbSourceCredentials() types.Object {
	return models.ClickPipeSourceCredentialsModel{
		Username: types.StringValue("dbuser"),
		Password: types.StringValue("dbpass"),
	}.ObjectValue()
}

// buildPostgresSSHKeyResourcePlan returns a minimal Postgres CDC plan with the
// given ssh_key_resource_id value.
func buildPostgresSSHKeyResourcePlan(sshKeyResourceID types.String) models.ClickPipeResourceModel {
	settingsModel := models.ClickPipePostgresSettingsModel{
		ReplicationMode: types.StringValue(api.ClickPipeReplicationModeCDC),
	}
	postgresAttrs := map[string]attr.Value{
		"type":                   types.StringValue("postgres"),
		"host":                   types.StringValue("10.0.0.5"),
		"port":                   types.Int64Value(5432),
		"database":               types.StringValue("app"),
		"authentication":         types.StringValue(clickPipeAuthBasic),
		"iam_role":               types.StringNull(),
		"tls_host":               types.StringNull(),
		"ca_certificate":         types.StringNull(),
		"disable_tls":            types.BoolNull(),
		"skip_cert_verification": types.BoolNull(),
		"credentials":            dbSourceCredentials(),
		"settings":               settingsModel.ObjectValue(),
		"table_mappings":         types.SetValueMust(models.ClickPipePostgresTableMappingModel{}.ObjectType(), []attr.Value{}),
		"ssh_key_resource_id":    sshKeyResourceID,
	}
	sourceModel := dbSourceModels(
		types.ObjectValueMust(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes, postgresAttrs),
		types.ObjectNull(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes),
	)
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-postgres-ssh"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_Postgres_SSHKeyResourceIDSet(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildPostgresSSHKeyResourcePlan(types.StringValue("ssh-key-123"))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Postgres)
	assert.NotNil(t, source.Postgres.SSHKeyResourceID)
	assert.Equal(t, "ssh-key-123", *source.Postgres.SSHKeyResourceID)
}

func TestExtractSourceFromPlan_Postgres_SSHKeyResourceIDNull(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildPostgresSSHKeyResourcePlan(types.StringNull())

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Postgres)
	assert.Nil(t, source.Postgres.SSHKeyResourceID)
}

// ssh_key_resource_id is immutable (create-only): it must not be included in an
// update (PATCH) payload.
func TestExtractSourceFromPlan_Postgres_SSHKeyResourceIDOmittedOnUpdate(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildPostgresSSHKeyResourcePlan(types.StringValue("ssh-key-123"))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.Postgres)
	assert.Nil(t, source.Postgres.SSHKeyResourceID)
}

// buildMySQLSSHKeyResourcePlan returns a minimal MySQL CDC plan with the given
// ssh_key_resource_id value.
func buildMySQLSSHKeyResourcePlan(sshKeyResourceID types.String) models.ClickPipeResourceModel {
	settingsModel := models.ClickPipeMySQLSettingsModel{
		ReplicationMode: types.StringValue(api.ClickPipeReplicationModeCDC),
	}
	mysqlAttrs := map[string]attr.Value{
		"type":                   types.StringValue("mysql"),
		"host":                   types.StringValue("10.0.0.6"),
		"port":                   types.Int64Value(3306),
		"authentication":         types.StringValue(clickPipeAuthBasic),
		"iam_role":               types.StringNull(),
		"tls_host":               types.StringNull(),
		"ca_certificate":         types.StringNull(),
		"disable_tls":            types.BoolNull(),
		"skip_cert_verification": types.BoolNull(),
		"credentials":            dbSourceCredentials(),
		"settings":               settingsModel.ObjectValue(),
		"table_mappings":         types.SetValueMust(models.ClickPipeMySQLTableMappingModel{}.ObjectType(), []attr.Value{}),
		"ssh_key_resource_id":    sshKeyResourceID,
	}
	sourceModel := dbSourceModels(
		types.ObjectNull(models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes),
		types.ObjectValueMust(models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes, mysqlAttrs),
	)
	return models.ClickPipeResourceModel{
		ID:        types.StringValue("test-pipe-id"),
		ServiceID: types.StringValue("service-123"),
		Name:      types.StringValue("test-mysql-ssh"),
		Source:    sourceModel.ObjectValue(),
	}
}

func TestExtractSourceFromPlan_MySQL_SSHKeyResourceIDSet(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMySQLSSHKeyResourcePlan(types.StringValue("ssh-key-123"))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MySQL)
	assert.NotNil(t, source.MySQL.SSHKeyResourceID)
	assert.Equal(t, "ssh-key-123", *source.MySQL.SSHKeyResourceID)
}

func TestExtractSourceFromPlan_MySQL_SSHKeyResourceIDNull(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMySQLSSHKeyResourcePlan(types.StringNull())

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MySQL)
	assert.Nil(t, source.MySQL.SSHKeyResourceID)
}

// ssh_key_resource_id is immutable (create-only): it must not be included in an
// update (PATCH) payload.
func TestExtractSourceFromPlan_MySQL_SSHKeyResourceIDOmittedOnUpdate(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	plan := buildMySQLSSHKeyResourcePlan(types.StringValue("ssh-key-123"))

	diagnostics := diag.Diagnostics{}
	source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, true)

	assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	assert.NotNil(t, source.MySQL)
	assert.Nil(t, source.MySQL.SSHKeyResourceID)
}

// kafkaSSHKeyAPIResponse builds the GET response for the Kafka pipe described by
// buildKafkaSSHKeyResourcePlan, with the given sshKeyResourceId (nil = field absent).
func kafkaSSHKeyAPIResponse(sshKeyResourceID *string) *api.ClickPipe {
	return &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-kafka-ssh",
		State: "Running",
		Source: api.ClickPipeSource{
			Kafka: &api.ClickPipeKafkaSource{
				Type:             "msk",
				Format:           api.ClickPipeJSONEachRowFormat,
				Brokers:          "broker:9092",
				Topics:           "test-topic",
				Authentication:   api.ClickPipeAuthenticationIAMRole,
				IAMRole:          strPtr("arn:aws:iam::123456789012:role/MyRole"),
				SSHKeyResourceID: sshKeyResourceID,
			},
		},
		Destination: api.ClickPipeDestination{Database: "default"},
	}
}

// TestClickPipeResource_syncClickPipeState_SSHKeyResourceID covers the three
// branches of the ssh_key_resource_id read-back: the API value wins when present;
// when the API omits the field a value already in state is preserved (so a plan
// after import or on an older API never shows a spurious replacement); and when
// neither side has a value the attribute stays null.
func TestClickPipeResource_syncClickPipeState_SSHKeyResourceID(t *testing.T) {
	ctx := context.Background()

	readBack := func(t *testing.T, state models.ClickPipeResourceModel) types.String {
		var sourceModel models.ClickPipeSourceModel
		assert.False(t, state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())
		var kafkaModel models.ClickPipeKafkaSourceModel
		assert.False(t, sourceModel.Kafka.As(ctx, &kafkaModel, basetypes.ObjectAsOptions{}).HasError())
		return kafkaModel.SSHKeyResourceID
	}

	tests := []struct {
		name     string
		apiValue *string
		state    types.String
		want     types.String
	}{
		{name: "API value wins", apiValue: strPtr("ssh-key-from-api"), state: types.StringValue("ssh-key-from-state"), want: types.StringValue("ssh-key-from-api")},
		{name: "API nil, prior state set, value preserved", apiValue: nil, state: types.StringValue("ssh-key-from-state"), want: types.StringValue("ssh-key-from-state")},
		{name: "API nil, prior state null, stays null", apiValue: nil, state: types.StringNull(), want: types.StringNull()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := buildKafkaSSHKeyResourcePlan(tc.state)

			mc := minimock.NewController(t)
			apiClientMock := api.NewClientMock(mc).
				GetClickPipeMock.
				Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
				Return(kafkaSSHKeyAPIResponse(tc.apiValue), nil)
			r := &ClickPipeResource{client: apiClientMock}

			err := r.syncClickPipeState(ctx, &state)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, readBack(t, state))
		})
	}
}
