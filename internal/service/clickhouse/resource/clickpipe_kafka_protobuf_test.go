package resource

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// buildKafkaProtobufPlan returns a minimal Kafka ClickPipe model for Protobuf schema tests.
func buildKafkaProtobufPlan(format string, protobufSchema types.String, schemaRegistry types.Object) models.ClickPipeResourceModel {
	credentialAttrs := map[string]attr.Value{
		"username":            types.StringValue("user"),
		"password":            types.StringValue("password"),
		"password_wo":         types.StringNull(),
		"password_wo_version": types.Int64Null(),
		"access_key_id":       types.StringNull(),
		"secret_key":          types.StringNull(),
		"connection_string":   types.StringNull(),
		"certificate":         types.StringNull(),
		"private_key":         types.StringNull(),
	}
	kafkaAttrs := map[string]attr.Value{
		"type":                         types.StringValue(api.ClickPipeKafkaSourceType),
		"format":                       types.StringValue(format),
		"protobuf_schema":              protobufSchema,
		"brokers":                      types.StringValue("broker:9092"),
		"topics":                       types.StringValue("events"),
		"consumer_group":               types.StringNull(),
		"offset":                       types.ObjectNull(models.ClickPipeKafkaOffsetModel{}.ObjectType().AttrTypes),
		"schema_registry":              schemaRegistry,
		"authentication":               types.StringValue(api.ClickPipeKafkaAuthenticationPlain),
		"credentials":                  types.ObjectValueMust(models.ClickPipeKafkaSourceCredentialsModel{}.ObjectType().AttrTypes, credentialAttrs),
		"iam_role":                     types.StringNull(),
		"ca_certificate":               types.StringNull(),
		"reverse_private_endpoint_ids": types.ListNull(types.StringType),
		"exactly_once":                 types.BoolNull(),
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
		ID:            types.StringValue("pipe-id"),
		ServiceID:     types.StringValue("service-id"),
		Name:          types.StringValue("protobuf-pipe"),
		Scaling:       types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes),
		State:         types.StringNull(),
		Stopped:       types.BoolNull(),
		Source:        sourceModel.ObjectValue(),
		Destination:   types.ObjectNull(models.ClickPipeDestinationModel{}.ObjectType().AttrTypes),
		FieldMappings: types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType()),
		Settings:      types.DynamicNull(),
		TriggerResync: types.BoolNull(),
	}
}

// kafkaSchemaRegistryValue returns a populated schema registry model for validation tests.
func kafkaSchemaRegistryValue() types.Object {
	credentialAttrs := map[string]attr.Value{
		"username":            types.StringValue("registry-user"),
		"password":            types.StringValue("registry-password"),
		"password_wo":         types.StringNull(),
		"password_wo_version": types.Int64Null(),
	}
	registryAttrs := map[string]attr.Value{
		"url":            types.StringValue("https://schema-registry.example.com"),
		"authentication": types.StringValue(api.ClickPipeKafkaAuthenticationPlain),
		"credentials":    types.ObjectValueMust(models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes, credentialAttrs),
	}

	return types.ObjectValueMust(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes, registryAttrs)
}

// validateKafkaProtobufConfig runs the Kafka Protobuf validator against a resource model.
func validateKafkaProtobufConfig(t *testing.T, configModel models.ClickPipeResourceModel) diag.Diagnostics {
	t.Helper()
	ctx := context.Background()
	clickPipeResource := &ClickPipeResource{}
	schemaResponse := &resource.SchemaResponse{}
	clickPipeResource.Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError(), "building resource schema failed: %v", schemaResponse.Diagnostics.Errors())

	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	setDiagnostics := plan.Set(ctx, &configModel)
	require.False(t, setDiagnostics.HasError(), "encoding config failed: %v", setDiagnostics.Errors())
	config := tfsdk.Config{Schema: schemaResponse.Schema, Raw: plan.Raw}
	validationResponse := &resource.ValidateConfigResponse{}
	kafkaProtobufSchemaValidator{}.ValidateResource(ctx, resource.ValidateConfigRequest{Config: config}, validationResponse)

	return validationResponse.Diagnostics
}

// diagnosticDetails joins diagnostic details to make validation assertions readable.
func diagnosticDetails(diagnostics diag.Diagnostics) string {
	details := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		details = append(details, diagnostic.Detail())
	}

	return strings.Join(details, "\n")
}

func TestExtractSourceFromPlan_KafkaProtobufSchema(t *testing.T) {
	encodedSchema := base64.StdEncoding.EncodeToString([]byte(`syntax = "proto3"; message Event { string id = 1; }`))
	plan := buildKafkaProtobufPlan(
		api.ClickPipeProtobufFormat,
		types.StringValue(encodedSchema),
		types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes),
	)
	diagnostics := diag.Diagnostics{}

	source := (&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, false)

	require.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	require.NotNil(t, source.Kafka)
	require.NotNil(t, source.Kafka.ProtobufSchema)
	assert.Equal(t, encodedSchema, *source.Kafka.ProtobufSchema)
}

func TestExtractSourceFromPlan_KafkaProtobufSchemaOmittedOnUpdate(t *testing.T) {
	encodedSchema := base64.StdEncoding.EncodeToString([]byte(`syntax = "proto3"; message Event { string id = 1; }`))
	plan := buildKafkaProtobufPlan(
		api.ClickPipeProtobufFormat,
		types.StringValue(encodedSchema),
		types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes),
	)
	diagnostics := diag.Diagnostics{}

	source := (&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, true)

	require.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	require.NotNil(t, source.Kafka)
	assert.Nil(t, source.Kafka.ProtobufSchema)
}

func TestClickPipeResource_SyncKafkaProtobufSchemaPreservesState(t *testing.T) {
	ctx := context.Background()
	encodedSchema := base64.StdEncoding.EncodeToString([]byte(`syntax = "proto3"; message Event { string id = 1; }`))
	state := buildKafkaProtobufPlan(
		api.ClickPipeProtobufFormat,
		types.StringValue(encodedSchema),
		types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes),
	)
	apiClickPipe := &api.ClickPipe{
		ID:    "pipe-id",
		Name:  "protobuf-pipe",
		State: api.ClickPipeRunningState,
		Source: api.ClickPipeSource{Kafka: &api.ClickPipeKafkaSource{
			Type:           api.ClickPipeKafkaSourceType,
			Format:         api.ClickPipeProtobufFormat,
			Brokers:        "broker:9092",
			Topics:         "events",
			Authentication: api.ClickPipeKafkaAuthenticationPlain,
		}},
		Destination: api.ClickPipeDestination{Database: "default"},
	}
	mockController := minimock.NewController(t)
	apiClient := api.NewClientMock(mockController).
		GetClickPipeMock.
		Expect(ctx, "service-id", "pipe-id").
		Return(apiClickPipe, nil)

	err := (&ClickPipeResource{client: apiClient}).syncClickPipeState(ctx, &state)

	require.NoError(t, err)
	var sourceModel models.ClickPipeSourceModel
	require.False(t, state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())
	var kafkaModel models.ClickPipeKafkaSourceModel
	require.False(t, sourceModel.Kafka.As(ctx, &kafkaModel, basetypes.ObjectAsOptions{}).HasError())
	assert.Equal(t, encodedSchema, kafkaModel.ProtobufSchema.ValueString())
}

func TestClickPipeResource_ValidatesKafkaProtobufSchemaConfiguration(t *testing.T) {
	validSchema := base64.StdEncoding.EncodeToString([]byte(`syntax = "proto3"; message Event { string id = 1; }`))
	nullRegistry := types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes)

	tests := []struct {
		name              string
		format            string
		protobufSchema    types.String
		schemaRegistry    types.Object
		expectedErrors    int
		expectedErrorText string
	}{
		{
			name:              "accepts an uploaded Protobuf schema",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringValue(validSchema),
			schemaRegistry:    nullRegistry,
			expectedErrorText: "",
		},
		{
			name:              "accepts a schema registry for Protobuf",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringNull(),
			schemaRegistry:    kafkaSchemaRegistryValue(),
			expectedErrorText: "",
		},
		{
			name:              "accepts an encoded schema at the size limit",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringValue(strings.Repeat("A", maxClickPipeProtobufSchemaEncodedSize)),
			schemaRegistry:    nullRegistry,
			expectedErrorText: "",
		},
		{
			name:              "accepts unpadded base64",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringValue(strings.TrimRight(validSchema, "=")),
			schemaRegistry:    nullRegistry,
			expectedErrorText: "",
		},
		{
			name:              "accepts surrounding whitespace",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringValue("\n" + validSchema + " \t"),
			schemaRegistry:    nullRegistry,
			expectedErrorText: "",
		},
		{
			name:              "defers validation for an unknown uploaded schema",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringUnknown(),
			schemaRegistry:    nullRegistry,
			expectedErrorText: "",
		},
		{
			name:              "defers validation for an unknown schema registry",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringNull(),
			schemaRegistry:    types.ObjectUnknown(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes),
			expectedErrorText: "",
		},
		{
			name:              "requires a schema source for Protobuf",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringNull(),
			schemaRegistry:    nullRegistry,
			expectedErrors:    1,
			expectedErrorText: "Protobuf format requires either protobuf_schema or schema_registry.",
		},
		{
			name:              "rejects both schema sources",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringValue(validSchema),
			schemaRegistry:    kafkaSchemaRegistryValue(),
			expectedErrors:    1,
			expectedErrorText: "protobuf_schema cannot be combined with schema_registry.",
		},
		{
			name:              "rejects an uploaded schema for another format",
			format:            api.ClickPipeJSONEachRowFormat,
			protobufSchema:    types.StringValue(validSchema),
			schemaRegistry:    nullRegistry,
			expectedErrors:    1,
			expectedErrorText: "protobuf_schema is supported only when format is Protobuf.",
		},
		{
			name:              "rejects an empty schema",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringValue(""),
			schemaRegistry:    nullRegistry,
			expectedErrors:    1,
			expectedErrorText: "protobuf_schema must contain valid base64 data and must not exceed 1 MiB.",
		},
		{
			name:              "rejects a whitespace-only schema",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringValue(" \n\t"),
			schemaRegistry:    nullRegistry,
			expectedErrors:    1,
			expectedErrorText: "protobuf_schema must contain valid base64 data and must not exceed 1 MiB.",
		},
		{
			name:              "rejects invalid base64",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringValue("not-base64"),
			schemaRegistry:    nullRegistry,
			expectedErrors:    1,
			expectedErrorText: "protobuf_schema must contain valid base64 data and must not exceed 1 MiB.",
		},
		{
			name:              "rejects an oversized schema",
			format:            api.ClickPipeProtobufFormat,
			protobufSchema:    types.StringValue(strings.Repeat("A", maxClickPipeProtobufSchemaEncodedSize+1)),
			schemaRegistry:    nullRegistry,
			expectedErrors:    1,
			expectedErrorText: "protobuf_schema must contain valid base64 data and must not exceed 1 MiB.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := buildKafkaProtobufPlan(test.format, test.protobufSchema, test.schemaRegistry)

			diagnostics := validateKafkaProtobufConfig(t, config)
			details := diagnosticDetails(diagnostics)

			assert.Len(t, diagnostics.Errors(), test.expectedErrors)
			assert.Equal(t, test.expectedErrorText, details)
		})
	}
}

func TestClickPipeResource_RegistersKafkaProtobufSchemaValidator(t *testing.T) {
	validators := (&ClickPipeResource{}).ConfigValidators(context.Background())

	assert.Contains(t, validators, kafkaProtobufSchemaValidator{})
}

func TestClickPipeResource_KafkaProtobufSchemaIsSensitiveAndImmutable(t *testing.T) {
	ctx := context.Background()
	clickPipeResource := &ClickPipeResource{}
	schemaResponse := &resource.SchemaResponse{}
	clickPipeResource.Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError(), "building resource schema failed: %v", schemaResponse.Diagnostics.Errors())

	sourceAttribute := schemaResponse.Schema.Attributes["source"]
	require.IsType(t, resourceschema.SingleNestedAttribute{}, sourceAttribute)
	sourceNestedAttribute := sourceAttribute.(resourceschema.SingleNestedAttribute)
	kafkaAttribute := sourceNestedAttribute.Attributes["kafka"]
	require.IsType(t, resourceschema.SingleNestedAttribute{}, kafkaAttribute)
	kafkaNestedAttribute := kafkaAttribute.(resourceschema.SingleNestedAttribute)
	protobufSchemaAttribute := kafkaNestedAttribute.Attributes["protobuf_schema"]
	require.IsType(t, resourceschema.StringAttribute{}, protobufSchemaAttribute)
	protobufSchemaStringAttribute := protobufSchemaAttribute.(resourceschema.StringAttribute)
	assert.True(t, protobufSchemaStringAttribute.Sensitive)
	require.Len(t, protobufSchemaStringAttribute.PlanModifiers, 1)

	nullRegistry := types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes)
	stateModel := buildKafkaProtobufPlan(api.ClickPipeProtobufFormat, types.StringValue("b2xk"), nullRegistry)
	planModel := buildKafkaProtobufPlan(api.ClickPipeProtobufFormat, types.StringValue("bmV3"), nullRegistry)
	state := tfsdk.State{Schema: schemaResponse.Schema, Raw: tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil)}
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	require.False(t, state.Set(ctx, &stateModel).HasError())
	require.False(t, plan.Set(ctx, &planModel).HasError())
	modifierRequest := planmodifier.StringRequest{
		Path:        path.Root("source").AtName("kafka").AtName("protobuf_schema"),
		State:       state,
		Plan:        plan,
		StateValue:  types.StringValue("b2xk"),
		PlanValue:   types.StringValue("bmV3"),
		ConfigValue: types.StringValue("bmV3"),
	}
	modifierResponse := &planmodifier.StringResponse{PlanValue: modifierRequest.PlanValue}

	protobufSchemaStringAttribute.PlanModifiers[0].PlanModifyString(ctx, modifierRequest, modifierResponse)

	assert.True(t, modifierResponse.RequiresReplace)
}

func TestClickPipeResource_ImportWarnsAboutUnrecoverableProtobufSchema(t *testing.T) {
	ctx := context.Background()
	clickPipeResource := &ClickPipeResource{}
	schemaResponse := &resource.SchemaResponse{}
	clickPipeResource.Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError(), "building resource schema failed: %v", schemaResponse.Diagnostics.Errors())
	importResponse := &resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: schemaResponse.Schema,
			Raw:    tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil),
		},
	}

	clickPipeResource.ImportState(ctx, resource.ImportStateRequest{ID: "service-id:pipe-id"}, importResponse)

	require.False(t, importResponse.Diagnostics.HasError(), "import failed: %v", importResponse.Diagnostics.Errors())
	warnings := importResponse.Diagnostics.Warnings()
	require.Len(t, warnings, 1)
	assert.Equal(t, "Sensitive values are not imported", warnings[0].Summary())
	assert.Contains(t, warnings[0].Detail(), "Protobuf schemas aren't imported")
	assert.Contains(t, warnings[0].Detail(), "Adding `protobuf_schema` after import forces ClickPipe replacement")
}
