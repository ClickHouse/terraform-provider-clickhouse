package resource

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
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

// kinesisProtobufModel returns the existing Kinesis fixture with the supplied format and schema.
// It uses t to report fixture conversion errors before the test can make misleading assertions.
func kinesisProtobufModel(t *testing.T, format, protobufSchema types.String) models.ClickPipeResourceModel {
	t.Helper()
	ctx := t.Context()
	model := getKinesisSyncState(
		api.ClickPipeAuthenticationIAMRole,
		types.ObjectNull(models.ClickPipeSourceAccessKeyModel{}.ObjectType().AttrTypes),
		types.StringValue("arn:aws:iam::123456789012:role/clickpipes"),
	)
	var source models.ClickPipeSourceModel
	require.False(t, model.Source.As(ctx, &source, basetypes.ObjectAsOptions{}).HasError())
	var kinesis models.ClickPipeKinesisSourceModel
	require.False(t, source.Kinesis.As(ctx, &kinesis, basetypes.ObjectAsOptions{}).HasError())
	kinesis.Format = format
	kinesis.ProtobufSchema = protobufSchema
	source.Kinesis = kinesis.ObjectValue()
	model.Source = source.ObjectValue()
	model.Scaling = types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes)
	model.FieldMappings = types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType())
	model.Settings = types.DynamicNull()
	return model
}

// validateKinesisProtobufConfig returns registered validator diagnostics for model.
// It uses t to fail immediately if Terraform cannot encode the test configuration.
func validateKinesisProtobufConfig(t *testing.T, model models.ClickPipeResourceModel) diag.Diagnostics {
	t.Helper()
	ctx := t.Context()
	clickPipeResource := &ClickPipeResource{}
	schemaResponse := &resource.SchemaResponse{}
	clickPipeResource.Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError())
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	require.Empty(t, plan.Set(ctx, &model))
	request := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schemaResponse.Schema, Raw: plan.Raw}}
	response := &resource.ValidateConfigResponse{}
	for _, validator := range clickPipeResource.ConfigValidators(ctx) {
		validator.ValidateResource(ctx, request, response)
	}
	return response.Diagnostics
}

func TestClickPipeResource_ValidatesKinesisProtobufSchemaConfiguration(t *testing.T) {
	validSchema := base64.StdEncoding.EncodeToString([]byte(`syntax = "proto3"; message Event { string id = 1; }`))
	invalidSchemaMessage := "protobuf_schema must contain valid base64 data and must not exceed 1 MiB."
	tests := []struct {
		name           string
		format         types.String
		schema         types.String
		expectedDetail string
	}{
		{
			name:   "accepts an uploaded schema",
			format: types.StringValue(api.ClickPipeProtobufFormat),
			schema: types.StringValue(validSchema),
		},
		{
			name:   "accepts the maximum encoded size",
			format: types.StringValue(api.ClickPipeProtobufFormat),
			schema: types.StringValue(strings.Repeat("A", maxClickPipeProtobufSchemaEncodedSize)),
		},
		{
			name:   "accepts unpadded base64 with surrounding whitespace",
			format: types.StringValue(api.ClickPipeProtobufFormat),
			schema: types.StringValue(" \n" + strings.TrimRight(validSchema, "=") + "\t "),
		},
		{
			name:           "requires a schema for Protobuf",
			format:         types.StringValue(api.ClickPipeProtobufFormat),
			schema:         types.StringNull(),
			expectedDetail: "Protobuf format requires protobuf_schema.",
		},
		{
			name:           "rejects empty schemas",
			format:         types.StringValue(api.ClickPipeProtobufFormat),
			schema:         types.StringValue(""),
			expectedDetail: invalidSchemaMessage,
		},
		{
			name:           "rejects invalid base64",
			format:         types.StringValue(api.ClickPipeProtobufFormat),
			schema:         types.StringValue("not@base64"),
			expectedDetail: invalidSchemaMessage,
		},
		{
			name:           "rejects oversized schemas",
			format:         types.StringValue(api.ClickPipeProtobufFormat),
			schema:         types.StringValue(strings.Repeat("A", maxClickPipeProtobufSchemaEncodedSize+4)),
			expectedDetail: invalidSchemaMessage,
		},
		{
			name:   "defers an unknown schema",
			format: types.StringValue(api.ClickPipeProtobufFormat),
			schema: types.StringUnknown(),
		},
		{
			name:   "defers an unknown format",
			format: types.StringUnknown(),
			schema: types.StringValue(validSchema),
		},
		{
			name:           "validates base64 even when the format is unknown",
			format:         types.StringUnknown(),
			schema:         types.StringValue("not@base64"),
			expectedDetail: invalidSchemaMessage,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := kinesisProtobufModel(t, test.format, test.schema)

			diagnostics := validateKinesisProtobufConfig(t, model)

			assert.Equal(t, test.expectedDetail, diagnosticDetails(diagnostics))
		})
	}

	for _, format := range api.ClickPipeStreamingFormats {
		t.Run(format+" remains supported without a schema", func(t *testing.T) {
			model := kinesisProtobufModel(t, types.StringValue(format), types.StringNull())

			assert.Empty(t, validateKinesisProtobufConfig(t, model))
		})
		t.Run(format+" rejects an uploaded schema", func(t *testing.T) {
			model := kinesisProtobufModel(t, types.StringValue(format), types.StringValue(validSchema))

			diagnostics := validateKinesisProtobufConfig(t, model)

			assert.Equal(t, "protobuf_schema is supported only when format is Protobuf.", diagnosticDetails(diagnostics))
		})
	}
}

func TestClickPipeResource_KinesisProtobufSchemaIsSensitiveAndImmutable(t *testing.T) {
	ctx := t.Context()
	clickPipeResource := &ClickPipeResource{}
	schemaResponse := &resource.SchemaResponse{}
	clickPipeResource.Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError())
	source := schemaResponse.Schema.Attributes["source"]
	require.IsType(t, resourceschema.SingleNestedAttribute{}, source)
	kinesis := source.(resourceschema.SingleNestedAttribute).Attributes["kinesis"]
	require.IsType(t, resourceschema.SingleNestedAttribute{}, kinesis)
	kinesisAttribute := kinesis.(resourceschema.SingleNestedAttribute)
	upload := kinesisAttribute.Attributes["protobuf_schema"]
	require.IsType(t, resourceschema.StringAttribute{}, upload)
	schemaAttribute := upload.(resourceschema.StringAttribute)
	assert.True(t, schemaAttribute.Optional)
	assert.True(t, schemaAttribute.Sensitive)
	assert.False(t, schemaAttribute.WriteOnly)
	require.Len(t, schemaAttribute.PlanModifiers, 1)
	formatAttribute := kinesisAttribute.Attributes["format"].(resourceschema.StringAttribute)
	assert.Contains(t, formatAttribute.MarkdownDescription, "`Protobuf`")

	tests := []struct {
		name            string
		previous        types.String
		next            types.String
		requiresReplace bool
	}{
		{
			name:            "changing the schema requires replacement",
			previous:        types.StringValue("b2xk"),
			next:            types.StringValue("bmV3"),
			requiresReplace: true,
		},
		{
			name:            "adding the schema after import requires replacement",
			previous:        types.StringNull(),
			next:            types.StringValue("bmV3"),
			requiresReplace: true,
		},
		{
			name:            "removing the schema requires replacement",
			previous:        types.StringValue("b2xk"),
			next:            types.StringNull(),
			requiresReplace: true,
		},
		{
			name:     "unchanged schemas do not require replacement",
			previous: types.StringValue("b2xk"),
			next:     types.StringValue("b2xk"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateModel := kinesisProtobufModel(t, types.StringValue(api.ClickPipeProtobufFormat), test.previous)
			planModel := kinesisProtobufModel(t, types.StringValue(api.ClickPipeProtobufFormat), test.next)
			state := tfsdk.State{
				Schema: schemaResponse.Schema,
				Raw:    tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil),
			}
			plan := tfsdk.Plan{Schema: schemaResponse.Schema}
			require.Empty(t, state.Set(ctx, &stateModel))
			require.Empty(t, plan.Set(ctx, &planModel))
			request := planmodifier.StringRequest{
				Path:        path.Root("source").AtName("kinesis").AtName("protobuf_schema"),
				State:       state,
				Plan:        plan,
				StateValue:  test.previous,
				PlanValue:   test.next,
				ConfigValue: test.next,
			}
			response := &planmodifier.StringResponse{PlanValue: request.PlanValue}

			schemaAttribute.PlanModifiers[0].PlanModifyString(ctx, request, response)

			assert.False(t, response.Diagnostics.HasError())
			assert.Equal(t, test.requiresReplace, response.RequiresReplace)
		})
	}
}

func TestExtractSourceFromPlan_KinesisProtobufSchema(t *testing.T) {
	encodedSchema := base64.StdEncoding.EncodeToString([]byte(`syntax = "proto3"; message Event { string id = 1; }`))
	plan := kinesisProtobufModel(t, types.StringValue(api.ClickPipeProtobufFormat), types.StringValue(encodedSchema))
	diagnostics := diag.Diagnostics{}

	source := (&ClickPipeResource{}).extractSourceFromPlan(
		t.Context(),
		&diagnostics,
		plan,
		nil,
		false,
	)

	require.False(t, diagnostics.HasError())
	require.NotNil(t, source.Kinesis)
	require.NotNil(t, source.Kinesis.ProtobufSchema)
	assert.Equal(t, api.ClickPipeProtobufFormat, source.Kinesis.Format)
	assert.Equal(t, encodedSchema, *source.Kinesis.ProtobufSchema)
}

func TestExtractSourceFromPlan_KinesisProtobufSchemaOmittedOnUpdate(t *testing.T) {
	plan := kinesisProtobufModel(t, types.StringValue(api.ClickPipeProtobufFormat), types.StringValue("c2NoZW1h"))
	diagnostics := diag.Diagnostics{}

	source := (&ClickPipeResource{}).extractSourceFromPlan(
		t.Context(),
		&diagnostics,
		plan,
		nil,
		true,
	)

	require.False(t, diagnostics.HasError())
	require.NotNil(t, source.Kinesis)
	assert.Nil(t, source.Kinesis.ProtobufSchema)
	payload, err := json.Marshal(source)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "protobufSchema")
}

func TestClickPipeResource_SyncKinesisProtobufSchemaPreservesState(t *testing.T) {
	ctx := t.Context()
	tests := []struct {
		name   string
		schema types.String
	}{
		{name: "preserves a configured schema", schema: types.StringValue("c2NoZW1h")},
		{name: "keeps the schema null after import", schema: types.StringNull()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := kinesisProtobufModel(t, types.StringValue(api.ClickPipeProtobufFormat), test.schema)
			apiPipe := kinesisAPIResponse(
				api.ClickPipeAuthenticationIAMRole,
				strPtr("arn:aws:iam::123456789012:role/clickpipes"),
			)
			apiPipe.Source.Kinesis.Format = api.ClickPipeProtobufFormat
			client := api.NewClientMock(minimock.NewController(t)).GetClickPipeMock.
				Expect(ctx, "service-123", "test-pipe-id").Return(apiPipe, nil)

			err := (&ClickPipeResource{client: client}).syncClickPipeState(ctx, &state)

			require.NoError(t, err)
			var source models.ClickPipeSourceModel
			require.False(t, state.Source.As(ctx, &source, basetypes.ObjectAsOptions{}).HasError())
			var kinesis models.ClickPipeKinesisSourceModel
			require.False(t, source.Kinesis.As(ctx, &kinesis, basetypes.ObjectAsOptions{}).HasError())
			assert.Equal(t, test.schema, kinesis.ProtobufSchema)
		})
	}
}
