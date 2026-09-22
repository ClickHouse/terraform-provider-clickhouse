package resource

import (
	"encoding/json"
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

const glueRoleArn = "arn:aws:iam::123456789012:role/GlueRegistryAccess"

// glueRegistry returns a Glue schema registry object with an optional role override.
func glueRegistry(roleArn types.String) types.Object {
	return models.ClickPipeKinesisSchemaRegistryModel{
		Type:             types.StringValue(api.ClickPipeKinesisSchemaRegistryTypeGlue),
		GlueRegion:       types.StringValue("eu-west-1"),
		GlueRegistryName: types.StringValue("orders-registry"),
		GlueRoleArn:      roleArn,
	}.ObjectValue()
}

func nullGlueRegistry() types.Object {
	return types.ObjectNull(models.ClickPipeKinesisSchemaRegistryModel{}.ObjectType().AttrTypes)
}

// kinesisGlueModel returns the Kinesis fixture with the supplied format, uploaded schema, and registry.
// It uses t to report fixture conversion errors before the test can make misleading assertions.
func kinesisGlueModel(t *testing.T, format, protobufSchema types.String, schemaRegistry types.Object) models.ClickPipeResourceModel {
	t.Helper()
	ctx := t.Context()
	model := kinesisProtobufModel(t, format, protobufSchema)
	var source models.ClickPipeSourceModel
	require.False(t, model.Source.As(ctx, &source, basetypes.ObjectAsOptions{}).HasError())
	var kinesis models.ClickPipeKinesisSourceModel
	require.False(t, source.Kinesis.As(ctx, &kinesis, basetypes.ObjectAsOptions{}).HasError())
	kinesis.SchemaRegistry = schemaRegistry
	source.Kinesis = kinesis.ObjectValue()
	model.Source = source.ObjectValue()
	return model
}

func TestClickPipeResource_ValidatesKinesisGlueSchemaRegistryConfiguration(t *testing.T) {
	uploadedSchema := types.StringValue("c3ludGF4ID0gInByb3RvMyI7")
	tests := []struct {
		name           string
		format         types.String
		schema         types.String
		registry       types.Object
		expectedDetail string
	}{
		{
			name:     "accepts AvroConfluent with a registry",
			format:   types.StringValue(api.ClickPipeAvroConfluentFormat),
			schema:   types.StringNull(),
			registry: glueRegistry(types.StringNull()),
		},
		{
			name:     "accepts Protobuf with a registry and no upload",
			format:   types.StringValue(api.ClickPipeProtobufFormat),
			schema:   types.StringNull(),
			registry: glueRegistry(types.StringValue(glueRoleArn)),
		},
		{
			name:           "requires a registry for AvroConfluent",
			format:         types.StringValue(api.ClickPipeAvroConfluentFormat),
			schema:         types.StringNull(),
			registry:       nullGlueRegistry(),
			expectedDetail: "AvroConfluent format requires schema_registry.",
		},
		{
			name:           "rejects a registry with JSONEachRow",
			format:         types.StringValue(api.ClickPipeJSONEachRowFormat),
			schema:         types.StringNull(),
			registry:       glueRegistry(types.StringNull()),
			expectedDetail: "schema_registry is supported only when format is AvroConfluent or Protobuf.",
		},
		{
			name:           "rejects an upload combined with a registry",
			format:         types.StringValue(api.ClickPipeProtobufFormat),
			schema:         uploadedSchema,
			registry:       glueRegistry(types.StringNull()),
			expectedDetail: "protobuf_schema cannot be combined with schema_registry.",
		},
		{
			name:     "defers an unknown registry for AvroConfluent",
			format:   types.StringValue(api.ClickPipeAvroConfluentFormat),
			schema:   types.StringNull(),
			registry: types.ObjectUnknown(models.ClickPipeKinesisSchemaRegistryModel{}.ObjectType().AttrTypes),
		},
		{
			name:     "defers an unknown registry for Protobuf without an upload",
			format:   types.StringValue(api.ClickPipeProtobufFormat),
			schema:   types.StringNull(),
			registry: types.ObjectUnknown(models.ClickPipeKinesisSchemaRegistryModel{}.ObjectType().AttrTypes),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := kinesisGlueModel(t, test.format, test.schema, test.registry)

			diagnostics := validateKinesisProtobufConfig(t, model)

			assert.Equal(t, test.expectedDetail, diagnosticDetails(diagnostics))
		})
	}
}

func TestClickPipeResource_KinesisSchemaRegistrySchema(t *testing.T) {
	ctx := t.Context()
	clickPipeResource := &ClickPipeResource{}
	schemaResponse := &resource.SchemaResponse{}
	clickPipeResource.Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError())
	source := schemaResponse.Schema.Attributes["source"].(resourceschema.SingleNestedAttribute)
	kinesis := source.Attributes["kinesis"].(resourceschema.SingleNestedAttribute)
	registry, ok := kinesis.Attributes["schema_registry"].(resourceschema.SingleNestedAttribute)
	require.True(t, ok)
	assert.True(t, registry.Optional)
	assert.False(t, registry.Sensitive)
	require.Len(t, registry.PlanModifiers, 1)
	for _, name := range []string{"type", "glue_region", "glue_registry_name"} {
		assert.True(t, registry.Attributes[name].(resourceschema.StringAttribute).Required, name)
	}
	assert.True(t, registry.Attributes["glue_role_arn"].(resourceschema.StringAttribute).Optional)

	tests := []struct {
		name            string
		previous        types.Object
		next            types.Object
		requiresReplace bool
	}{
		{
			name:            "changing the registry name requires replacement",
			previous:        glueRegistry(types.StringNull()),
			next:            glueRegistryNamed("other-registry"),
			requiresReplace: true,
		},
		{
			name:            "adding a role override requires replacement",
			previous:        glueRegistry(types.StringNull()),
			next:            glueRegistry(types.StringValue(glueRoleArn)),
			requiresReplace: true,
		},
		{
			name:            "removing the registry requires replacement",
			previous:        glueRegistry(types.StringNull()),
			next:            nullGlueRegistry(),
			requiresReplace: true,
		},
		{
			name:     "an unchanged registry does not require replacement",
			previous: glueRegistry(types.StringValue(glueRoleArn)),
			next:     glueRegistry(types.StringValue(glueRoleArn)),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateModel := kinesisGlueModel(t, types.StringValue(api.ClickPipeAvroConfluentFormat), types.StringNull(), test.previous)
			planModel := kinesisGlueModel(t, types.StringValue(api.ClickPipeAvroConfluentFormat), types.StringNull(), test.next)
			state := tfsdk.State{
				Schema: schemaResponse.Schema,
				Raw:    tftypes.NewValue(schemaResponse.Schema.Type().TerraformType(ctx), nil),
			}
			plan := tfsdk.Plan{Schema: schemaResponse.Schema}
			require.Empty(t, state.Set(ctx, &stateModel))
			require.Empty(t, plan.Set(ctx, &planModel))
			request := planmodifier.ObjectRequest{
				Path:        path.Root("source").AtName("kinesis").AtName("schema_registry"),
				State:       state,
				Plan:        plan,
				StateValue:  test.previous,
				PlanValue:   test.next,
				ConfigValue: test.next,
			}
			response := &planmodifier.ObjectResponse{PlanValue: request.PlanValue}

			registry.PlanModifiers[0].PlanModifyObject(ctx, request, response)

			assert.False(t, response.Diagnostics.HasError())
			assert.Equal(t, test.requiresReplace, response.RequiresReplace)
		})
	}
}

// glueRegistryNamed returns a Glue registry object with a different registry name.
func glueRegistryNamed(name string) types.Object {
	return models.ClickPipeKinesisSchemaRegistryModel{
		Type:             types.StringValue(api.ClickPipeKinesisSchemaRegistryTypeGlue),
		GlueRegion:       types.StringValue("eu-west-1"),
		GlueRegistryName: types.StringValue(name),
		GlueRoleArn:      types.StringNull(),
	}.ObjectValue()
}

func TestExtractSourceFromPlan_KinesisGlueSchemaRegistry(t *testing.T) {
	tests := []struct {
		name         string
		roleArn      types.String
		expectedRole *string
	}{
		{name: "forwards the role override", roleArn: types.StringValue(glueRoleArn), expectedRole: strPtr(glueRoleArn)},
		{name: "omits an unset role override", roleArn: types.StringNull(), expectedRole: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := kinesisGlueModel(t, types.StringValue(api.ClickPipeAvroConfluentFormat), types.StringNull(), glueRegistry(test.roleArn))
			diagnostics := diag.Diagnostics{}

			source := (&ClickPipeResource{}).extractSourceFromPlan(t.Context(), &diagnostics, plan, nil, false)

			require.False(t, diagnostics.HasError())
			require.NotNil(t, source.Kinesis)
			require.NotNil(t, source.Kinesis.SchemaRegistry)
			assert.Equal(t, api.ClickPipeKinesisSchemaRegistry{
				Type:             api.ClickPipeKinesisSchemaRegistryTypeGlue,
				GlueRegion:       "eu-west-1",
				GlueRegistryName: "orders-registry",
				GlueRoleArn:      test.expectedRole,
			}, *source.Kinesis.SchemaRegistry)
			assert.Nil(t, source.Kinesis.ProtobufSchema)
		})
	}
}

func TestExtractSourceFromPlan_KinesisGlueSchemaRegistryOmittedOnUpdate(t *testing.T) {
	plan := kinesisGlueModel(t, types.StringValue(api.ClickPipeAvroConfluentFormat), types.StringNull(), glueRegistry(types.StringNull()))
	diagnostics := diag.Diagnostics{}

	source := (&ClickPipeResource{}).extractSourceFromPlan(t.Context(), &diagnostics, plan, nil, true)

	require.False(t, diagnostics.HasError())
	require.NotNil(t, source.Kinesis)
	assert.Nil(t, source.Kinesis.SchemaRegistry)
	payload, err := json.Marshal(source)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "schemaRegistry")
}

func TestClickPipeResource_SyncKinesisGlueSchemaRegistry(t *testing.T) {
	ctx := t.Context()
	tests := []struct {
		name     string
		response *api.ClickPipeKinesisSchemaRegistry
		expected types.Object
	}{
		{
			name: "reads the registry returned by the API",
			response: &api.ClickPipeKinesisSchemaRegistry{
				Type:             api.ClickPipeKinesisSchemaRegistryTypeGlue,
				GlueRegion:       "eu-west-1",
				GlueRegistryName: "orders-registry",
				GlueRoleArn:      strPtr(glueRoleArn),
			},
			expected: glueRegistry(types.StringValue(glueRoleArn)),
		},
		{
			name: "keeps the role null when the API omits it",
			response: &api.ClickPipeKinesisSchemaRegistry{
				Type:             api.ClickPipeKinesisSchemaRegistryTypeGlue,
				GlueRegion:       "eu-west-1",
				GlueRegistryName: "orders-registry",
			},
			expected: glueRegistry(types.StringNull()),
		},
		{
			name:     "clears the registry for a pipe without one",
			response: nil,
			expected: nullGlueRegistry(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := kinesisGlueModel(t, types.StringValue(api.ClickPipeAvroConfluentFormat), types.StringNull(), glueRegistry(types.StringNull()))
			apiPipe := kinesisAPIResponse(api.ClickPipeAuthenticationIAMRole, strPtr("arn:aws:iam::123456789012:role/clickpipes"))
			apiPipe.Source.Kinesis.Format = api.ClickPipeAvroConfluentFormat
			apiPipe.Source.Kinesis.SchemaRegistry = test.response
			client := api.NewClientMock(minimock.NewController(t)).GetClickPipeMock.
				Expect(ctx, "service-123", "test-pipe-id").Return(apiPipe, nil)

			err := (&ClickPipeResource{client: client}).syncClickPipeState(ctx, &state)

			require.NoError(t, err)
			var source models.ClickPipeSourceModel
			require.False(t, state.Source.As(ctx, &source, basetypes.ObjectAsOptions{}).HasError())
			var kinesis models.ClickPipeKinesisSourceModel
			require.False(t, source.Kinesis.As(ctx, &kinesis, basetypes.ObjectAsOptions{}).HasError())
			assert.Equal(t, test.expected, kinesis.SchemaRegistry)
		})
	}
}
