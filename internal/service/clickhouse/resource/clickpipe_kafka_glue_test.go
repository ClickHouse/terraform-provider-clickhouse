package resource

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// kafkaGlueRegistry returns a Kafka AWS Glue schema registry object with an optional role override.
func kafkaGlueRegistry(roleArn types.String) types.Object {
	return models.ClickPipeKafkaSchemaRegistryModel{
		Type:             types.StringValue(api.ClickPipeKafkaSchemaRegistryTypeGlue),
		URL:              types.StringNull(),
		Authentication:   types.StringNull(),
		Credentials:      types.ObjectNull(models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes),
		GlueRegion:       types.StringValue("eu-west-1"),
		GlueRegistryName: types.StringValue("orders-registry"),
		GlueRoleArn:      roleArn,
	}.ObjectValue()
}

// withKafkaRegistryAttrs returns registry with the named attributes overridden.
func withKafkaRegistryAttrs(t *testing.T, registry types.Object, overrides map[string]any) types.Object {
	t.Helper()
	attrs := registry.Attributes()
	for name, value := range overrides {
		switch v := value.(type) {
		case types.String:
			attrs[name] = v
		case types.Object:
			attrs[name] = v
		default:
			t.Fatalf("unsupported override type %T for %s", value, name)
		}
	}
	return types.ObjectValueMust(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes, attrs)
}

// kafkaGlueModel returns the AvroConfluent Kafka fixture with the supplied broker authentication and schema registry.
func kafkaGlueModel(t *testing.T, authentication types.String, schemaRegistry types.Object) models.ClickPipeResourceModel {
	t.Helper()
	return kafkaGlueModelWithFormat(t, api.ClickPipeAvroConfluentFormat, authentication, schemaRegistry)
}

// kafkaGlueModelWithFormat returns the Kafka fixture with the supplied format, broker authentication and schema registry.
func kafkaGlueModelWithFormat(t *testing.T, format string, authentication types.String, schemaRegistry types.Object) models.ClickPipeResourceModel {
	t.Helper()
	ctx := t.Context()
	model := buildKafkaProtobufPlan(format, types.StringNull(), schemaRegistry)
	var source models.ClickPipeSourceModel
	require.False(t, model.Source.As(ctx, &source, basetypes.ObjectAsOptions{}).HasError())
	var kafka models.ClickPipeKafkaSourceModel
	require.False(t, source.Kafka.As(ctx, &kafka, basetypes.ObjectAsOptions{}).HasError())
	kafka.Authentication = authentication
	source.Kafka = kafka.ObjectValue()
	model.Source = source.ObjectValue()
	return model
}

// validateKafkaSchemaRegistryConfig runs the Kafka schema registry validator against a resource model.
func validateKafkaSchemaRegistryConfig(t *testing.T, configModel models.ClickPipeResourceModel) diag.Diagnostics {
	t.Helper()
	ctx := t.Context()
	schemaResponse := &resource.SchemaResponse{}
	(&ClickPipeResource{}).Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError(), "building resource schema failed: %v", schemaResponse.Diagnostics.Errors())

	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	require.False(t, plan.Set(ctx, &configModel).HasError())
	config := tfsdk.Config{Schema: schemaResponse.Schema, Raw: plan.Raw}
	validationResponse := &resource.ValidateConfigResponse{}
	kafkaSchemaRegistryValidator{}.ValidateResource(ctx, resource.ValidateConfigRequest{Config: config}, validationResponse)

	return validationResponse.Diagnostics
}

func TestClickPipeResource_ValidatesKafkaSchemaRegistryConfiguration(t *testing.T) {
	iamRole := types.StringValue(api.ClickPipeAuthenticationIAMRole)
	plain := types.StringValue(api.ClickPipeKafkaAuthenticationPlain)
	confluentWithNullType := func(t *testing.T) types.Object {
		return withKafkaRegistryAttrs(t, kafkaSchemaRegistryValue(), map[string]any{"type": types.StringNull()})
	}
	tests := []struct {
		name           string
		authentication types.String
		registry       func(t *testing.T) types.Object
		expectedDetail string
	}{
		{
			name:           "accepts a confluent registry",
			authentication: plain,
			registry:       func(*testing.T) types.Object { return kafkaSchemaRegistryValue() },
		},
		{
			name:           "accepts a confluent registry without an explicit type",
			authentication: plain,
			registry:       confluentWithNullType,
		},
		{
			name:           "requires url for a confluent registry",
			authentication: plain,
			registry: func(t *testing.T) types.Object {
				return withKafkaRegistryAttrs(t, confluentWithNullType(t), map[string]any{"url": types.StringNull()})
			},
			expectedDetail: "url is required for a confluent schema registry.",
		},
		{
			name:           "requires credentials for a confluent registry",
			authentication: plain,
			registry: func(t *testing.T) types.Object {
				return withKafkaRegistryAttrs(t, kafkaSchemaRegistryValue(), map[string]any{
					"credentials": types.ObjectNull(models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes),
				})
			},
			expectedDetail: "credentials is required for a confluent schema registry.",
		},
		{
			name:           "rejects glue fields on a confluent registry",
			authentication: plain,
			registry: func(t *testing.T) types.Object {
				return withKafkaRegistryAttrs(t, kafkaSchemaRegistryValue(), map[string]any{"glue_region": types.StringValue("eu-west-1")})
			},
			expectedDetail: "glue_region is supported only when type is glue.",
		},
		{
			name:           "accepts a glue registry on an IAM broker without a role",
			authentication: iamRole,
			registry:       func(*testing.T) types.Object { return kafkaGlueRegistry(types.StringNull()) },
		},
		{
			name:           "accepts a glue registry with a role on a PLAIN broker",
			authentication: plain,
			registry:       func(*testing.T) types.Object { return kafkaGlueRegistry(types.StringValue(glueRoleArn)) },
		},
		{
			name:           "requires a role for a glue registry on a PLAIN broker",
			authentication: plain,
			registry:       func(*testing.T) types.Object { return kafkaGlueRegistry(types.StringNull()) },
			expectedDetail: "glue_role_arn is required for a glue schema registry unless the Kafka source uses IAM_ROLE or IAM_USER authentication.",
		},
		{
			name:           "requires a role when broker authentication defaults to PLAIN",
			authentication: types.StringNull(),
			registry:       func(*testing.T) types.Object { return kafkaGlueRegistry(types.StringNull()) },
			expectedDetail: "glue_role_arn is required for a glue schema registry unless the Kafka source uses IAM_ROLE or IAM_USER authentication.",
		},
		{
			name:           "defers the role check while broker authentication is unknown",
			authentication: types.StringUnknown(),
			registry:       func(*testing.T) types.Object { return kafkaGlueRegistry(types.StringNull()) },
		},
		{
			name:           "requires a registry name for a glue registry",
			authentication: iamRole,
			registry: func(t *testing.T) types.Object {
				return withKafkaRegistryAttrs(t, kafkaGlueRegistry(types.StringNull()), map[string]any{"glue_registry_name": types.StringNull()})
			},
			expectedDetail: "glue_registry_name is required for a glue schema registry.",
		},
		{
			name:           "rejects confluent fields on a glue registry",
			authentication: iamRole,
			registry: func(t *testing.T) types.Object {
				return withKafkaRegistryAttrs(t, kafkaGlueRegistry(types.StringNull()), map[string]any{"url": types.StringValue("https://registry.example.com")})
			},
			expectedDetail: "url is not supported for a glue schema registry, which authenticates with IAM.",
		},
		{
			name:           "defers validation while the type is unknown",
			authentication: plain,
			registry: func(t *testing.T) types.Object {
				return withKafkaRegistryAttrs(t, kafkaGlueRegistry(types.StringNull()), map[string]any{"type": types.StringUnknown()})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := validateKafkaSchemaRegistryConfig(t, kafkaGlueModel(t, test.authentication, test.registry(t)))

			if test.expectedDetail == "" {
				assert.False(t, diagnostics.HasError(), "unexpected diagnostics: %s", diagnosticDetails(diagnostics))
				return
			}
			assert.Equal(t, test.expectedDetail, diagnosticDetails(diagnostics))
		})
	}
}

func TestClickPipeResource_ValidatesKafkaGlueSchemaRegistryFormat(t *testing.T) {
	iamRole := types.StringValue(api.ClickPipeAuthenticationIAMRole)
	formatError := "a glue schema registry is supported only when format is AvroConfluent or Protobuf."
	tests := []struct {
		name           string
		format         string
		registry       types.Object
		expectedDetail string
	}{
		{name: "accepts glue with AvroConfluent", format: api.ClickPipeAvroConfluentFormat, registry: kafkaGlueRegistry(types.StringNull())},
		{name: "accepts glue with Protobuf", format: api.ClickPipeProtobufFormat, registry: kafkaGlueRegistry(types.StringNull())},
		{name: "rejects glue with JSONEachRow", format: api.ClickPipeJSONEachRowFormat, registry: kafkaGlueRegistry(types.StringNull()), expectedDetail: formatError},
		{name: "rejects glue with Avro", format: api.ClickPipeAvroFormat, registry: kafkaGlueRegistry(types.StringNull()), expectedDetail: formatError},
		// The API owns the Confluent format rule; the provider leaves it alone so imported pipes keep planning.
		{name: "leaves confluent with JSONEachRow to the API", format: api.ClickPipeJSONEachRowFormat, registry: kafkaSchemaRegistryValue()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostics := validateKafkaSchemaRegistryConfig(t, kafkaGlueModelWithFormat(t, test.format, iamRole, test.registry))

			if test.expectedDetail == "" {
				assert.False(t, diagnostics.HasError(), "unexpected diagnostics: %s", diagnosticDetails(diagnostics))
				return
			}
			assert.Equal(t, test.expectedDetail, diagnosticDetails(diagnostics))
		})
	}
}

func TestClickPipeResource_RegistersKafkaSchemaRegistryValidator(t *testing.T) {
	validators := (&ClickPipeResource{}).ConfigValidators(t.Context())

	assert.Contains(t, validators, kafkaSchemaRegistryValidator{})
}

func TestClickPipeResource_KafkaSchemaRegistrySchema(t *testing.T) {
	schemaResponse := &resource.SchemaResponse{}
	(&ClickPipeResource{}).Schema(t.Context(), resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError())

	source, ok := schemaResponse.Schema.Attributes["source"].(resourceschema.SingleNestedAttribute)
	require.True(t, ok)
	kafka, ok := source.Attributes["kafka"].(resourceschema.SingleNestedAttribute)
	require.True(t, ok)
	registry, ok := kafka.Attributes["schema_registry"].(resourceschema.SingleNestedAttribute)
	require.True(t, ok)

	registryType, ok := registry.Attributes["type"].(resourceschema.StringAttribute)
	require.True(t, ok)
	assert.True(t, registryType.Optional)
	assert.True(t, registryType.Computed)
	require.NotNil(t, registryType.Default)

	// Every type-specific attribute is optional at the schema level; kafkaSchemaRegistryValidator
	// decides which ones the chosen type requires.
	for _, name := range []string{"url", "authentication", "credentials", "glue_region", "glue_registry_name", "glue_role_arn"} {
		assert.True(t, registry.Attributes[name].IsOptional(), "%s should be optional", name)
		assert.False(t, registry.Attributes[name].IsRequired(), "%s should not be required", name)
	}
}

func TestExtractSourceFromPlan_KafkaGlueSchemaRegistry(t *testing.T) {
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
			plan := kafkaGlueModel(t, types.StringValue(api.ClickPipeAuthenticationIAMRole), kafkaGlueRegistry(test.roleArn))
			diagnostics := diag.Diagnostics{}

			source := (&ClickPipeResource{}).extractSourceFromPlan(t.Context(), &diagnostics, plan, nil, false)

			require.False(t, diagnostics.HasError(), "unexpected diagnostics: %v", diagnostics.Errors())
			require.NotNil(t, source.Kafka)
			require.NotNil(t, source.Kafka.SchemaRegistry)
			assert.Equal(t, api.ClickPipeKafkaSchemaRegistry{
				Type:             api.ClickPipeKafkaSchemaRegistryTypeGlue,
				GlueRegion:       "eu-west-1",
				GlueRegistryName: "orders-registry",
				GlueRoleArn:      test.expectedRole,
			}, *source.Kafka.SchemaRegistry)
		})
	}
}

func TestExtractSourceFromPlan_KafkaSchemaRegistryWirePayload(t *testing.T) {
	tests := []struct {
		name     string
		registry types.Object
		expected string
	}{
		{
			name:     "sends only the Glue fields for a glue registry",
			registry: kafkaGlueRegistry(types.StringValue(glueRoleArn)),
			expected: `{"type":"glue","glueRegion":"eu-west-1","glueRegistryName":"orders-registry","glueRoleArn":"` + glueRoleArn + `"}`,
		},
		{
			name:     "omits type for a confluent registry, matching the pre-Glue request",
			registry: kafkaSchemaRegistryValue(),
			expected: `{"url":"https://schema-registry.example.com","authentication":"PLAIN","credentials":{"username":"registry-user","password":"registry-password"}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := kafkaGlueModel(t, types.StringValue(api.ClickPipeAuthenticationIAMRole), test.registry)
			diagnostics := diag.Diagnostics{}

			source := (&ClickPipeResource{}).extractSourceFromPlan(t.Context(), &diagnostics, plan, nil, false)

			require.False(t, diagnostics.HasError(), "unexpected diagnostics: %v", diagnostics.Errors())
			require.NotNil(t, source.Kafka)
			payload, err := json.Marshal(source.Kafka.SchemaRegistry)
			require.NoError(t, err)
			assert.JSONEq(t, test.expected, string(payload))
		})
	}
}

func TestClickPipeResource_SyncKafkaSchemaRegistry(t *testing.T) {
	ctx := t.Context()
	tests := []struct {
		name          string
		stateRegistry types.Object
		response      *api.ClickPipeKafkaSchemaRegistry
		expected      types.Object
	}{
		{
			name:          "reads a glue registry returned by the API",
			stateRegistry: kafkaGlueRegistry(types.StringNull()),
			response: &api.ClickPipeKafkaSchemaRegistry{
				Type:             api.ClickPipeKafkaSchemaRegistryTypeGlue,
				GlueRegion:       "eu-west-1",
				GlueRegistryName: "orders-registry",
				GlueRoleArn:      strPtr(glueRoleArn),
			},
			expected: kafkaGlueRegistry(types.StringValue(glueRoleArn)),
		},
		{
			name:          "keeps the glue role null when the API omits it",
			stateRegistry: kafkaGlueRegistry(types.StringNull()),
			response: &api.ClickPipeKafkaSchemaRegistry{
				Type:             api.ClickPipeKafkaSchemaRegistryTypeGlue,
				GlueRegion:       "eu-west-1",
				GlueRegistryName: "orders-registry",
			},
			expected: kafkaGlueRegistry(types.StringNull()),
		},
		{
			name:          "records the confluent type the API omits and keeps state credentials",
			stateRegistry: withKafkaRegistryAttrs(t, kafkaSchemaRegistryValue(), map[string]any{"type": types.StringNull()}),
			response: &api.ClickPipeKafkaSchemaRegistry{
				URL:            "https://schema-registry.example.com",
				Authentication: api.ClickPipeKafkaAuthenticationPlain,
			},
			expected: kafkaSchemaRegistryValue(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := kafkaGlueModel(t, types.StringValue(api.ClickPipeAuthenticationIAMRole), test.stateRegistry)
			apiClickPipe := &api.ClickPipe{
				ID:    "pipe-id",
				Name:  "protobuf-pipe",
				State: api.ClickPipeRunningState,
				Source: api.ClickPipeSource{Kafka: &api.ClickPipeKafkaSource{
					Type:           api.ClickPipeKafkaSourceType,
					Format:         api.ClickPipeAvroConfluentFormat,
					Brokers:        "broker:9092",
					Topics:         "events",
					Authentication: api.ClickPipeAuthenticationIAMRole,
					IAMRole:        strPtr("arn:aws:iam::123456789012:role/clickpipes"),
					SchemaRegistry: test.response,
				}},
				Destination: api.ClickPipeDestination{Database: "default"},
			}
			client := api.NewClientMock(minimock.NewController(t)).GetClickPipeMock.
				Expect(ctx, "service-id", "pipe-id").Return(apiClickPipe, nil)

			err := (&ClickPipeResource{client: client}).syncClickPipeState(ctx, &state)

			require.NoError(t, err)
			var source models.ClickPipeSourceModel
			require.False(t, state.Source.As(ctx, &source, basetypes.ObjectAsOptions{}).HasError())
			var kafka models.ClickPipeKafkaSourceModel
			require.False(t, source.Kafka.As(ctx, &kafka, basetypes.ObjectAsOptions{}).HasError())
			assert.Equal(t, test.expected, kafka.SchemaRegistry)
		})
	}
}

// kafkaGlueUpdateModel returns the Kafka update fixture with a Glue registry in place of the Confluent one.
func kafkaGlueUpdateModel(t *testing.T, kafkaPassword string) models.ClickPipeResourceModel {
	t.Helper()
	model := kafkaUpdateModel(types.StringNull(), kafkaPassword)
	var source models.ClickPipeSourceModel
	require.False(t, model.Source.As(t.Context(), &source, basetypes.ObjectAsOptions{}).HasError())
	kafkaAttrs := source.Kafka.Attributes()
	kafkaAttrs["schema_registry"] = kafkaGlueRegistry(types.StringValue(glueRoleArn))
	source.Kafka = types.ObjectValueMust(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes, kafkaAttrs)
	model.Source = source.ObjectValue()
	return model
}

// Rotating broker credentials on a Glue pipe must not send the immutable registry, and the
// registry must survive in state after the update.
func TestClickPipeUpdate_KafkaGlueRegistryKeptAndNotSent(t *testing.T) {
	ctx := t.Context()
	state := kafkaGlueUpdateModel(t, "main-pass")
	plan := kafkaGlueUpdateModel(t, "rotated-pass")
	apiPipe := &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-pipe",
		State: api.ClickPipeRunningState,
		Source: api.ClickPipeSource{
			Kafka: &api.ClickPipeKafkaSource{
				Type:           "kafka",
				Format:         "AvroConfluent",
				Brokers:        "broker:9092",
				Topics:         "test-topic",
				Authentication: "PLAIN",
				SchemaRegistry: &api.ClickPipeKafkaSchemaRegistry{
					Type:             api.ClickPipeKafkaSchemaRegistryTypeGlue,
					GlueRegion:       "eu-west-1",
					GlueRegistryName: "orders-registry",
					GlueRoleArn:      strPtr(glueRoleArn),
				},
			},
		},
		Destination: api.ClickPipeDestination{Database: "default"},
	}

	var captured *api.ClickPipeUpdate
	mock := api.NewClientMock(minimock.NewController(t))
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
		captured = &update
		return apiPipe, nil
	})
	mock.WaitForClickPipeStateMock.Set(func(_ context.Context, _, _ string, _ func(string) bool, _ time.Duration) (*api.ClickPipe, error) {
		return apiPipe, nil
	})
	mock.GetClickPipeMock.Set(func(_ context.Context, _, _ string) (*api.ClickPipe, error) {
		return apiPipe, nil
	})

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update failed: %v", resp.Diagnostics.Errors())

	require.NotNil(t, captured, "UpdateClickPipe was not called")
	require.NotNil(t, captured.Source)
	kafka := captured.Source.Kafka
	require.NotNil(t, kafka)
	require.NotNil(t, kafka.Credentials, "rotated broker credentials must be carried")
	require.NotNil(t, kafka.Credentials.ClickPipeSourceCredentials)
	assert.Equal(t, "rotated-pass", kafka.Credentials.Password)
	assert.Nil(t, kafka.SchemaRegistry, "schema registry must never be sent on update")

	var updated models.ClickPipeResourceModel
	require.False(t, resp.State.Get(ctx, &updated).HasError())
	var source models.ClickPipeSourceModel
	require.False(t, updated.Source.As(ctx, &source, basetypes.ObjectAsOptions{}).HasError())
	var kafkaModel models.ClickPipeKafkaSourceModel
	require.False(t, source.Kafka.As(ctx, &kafkaModel, basetypes.ObjectAsOptions{}).HasError())
	assert.Equal(t, kafkaGlueRegistry(types.StringValue(glueRoleArn)), kafkaModel.SchemaRegistry)
}
