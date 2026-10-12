package resource

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

const maxClickPipeProtobufSchemaEncodedSize = 1 << 20 // 1 MiB in bytes

// kafkaTombstoneModeValidator enforces the hard-delete exactly-once requirement exposed by the OpenAPI.
type kafkaTombstoneModeValidator struct{}

func (v kafkaTombstoneModeValidator) Description(_ context.Context) string {
	return "Validates Kafka tombstone handling configuration."
}

func (v kafkaTombstoneModeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v kafkaTombstoneModeValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.ClickPipeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data.Source.IsNull() || data.Source.IsUnknown() {
		return
	}

	var sourceModel models.ClickPipeSourceModel
	resp.Diagnostics.Append(data.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() || sourceModel.Kafka.IsNull() || sourceModel.Kafka.IsUnknown() {
		return
	}

	var kafkaModel models.ClickPipeKafkaSourceModel
	resp.Diagnostics.Append(sourceModel.Kafka.As(ctx, &kafkaModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() || kafkaModel.TombstoneMode.IsNull() || kafkaModel.TombstoneMode.IsUnknown() {
		return
	}

	if kafkaModel.TombstoneMode.ValueString() != api.ClickPipeKafkaTombstoneModeDelete || kafkaModel.ExactlyOnce.IsUnknown() {
		return
	}

	if kafkaModel.ExactlyOnce.IsNull() || !kafkaModel.ExactlyOnce.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("source").AtName("kafka").AtName("tombstone_mode"),
			"Invalid Kafka tombstone configuration",
			"tombstone_mode = \"delete\" requires exactly_once = true.",
		)
	}
}

// kafkaProtobufSchemaValidator enforces the Kafka Protobuf schema rules exposed by the OpenAPI.
type kafkaProtobufSchemaValidator struct{}

// Description returns a plain-text summary of the Kafka Protobuf schema validation.
func (v kafkaProtobufSchemaValidator) Description(_ context.Context) string {
	return "Validates direct Kafka Protobuf schema configuration."
}

// MarkdownDescription returns the Kafka Protobuf schema validation summary as Markdown.
func (v kafkaProtobufSchemaValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource validates the uploaded schema and its relationship with the Kafka format and schema registry.
// Validation is deferred when Terraform has not resolved the relevant configuration values.
func (v kafkaProtobufSchemaValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.ClickPipeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data.Source.IsNull() || data.Source.IsUnknown() {
		return
	}

	sourceModel := models.ClickPipeSourceModel{}
	resp.Diagnostics.Append(data.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() || sourceModel.Kafka.IsNull() || sourceModel.Kafka.IsUnknown() {
		return
	}

	kafkaModel := models.ClickPipeKafkaSourceModel{}
	resp.Diagnostics.Append(sourceModel.Kafka.As(ctx, &kafkaModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	protobufSchemaPath := path.Root("source").AtName("kafka").AtName("protobuf_schema")
	schemaRegistryPath := path.Root("source").AtName("kafka").AtName("schema_registry")
	formatPath := path.Root("source").AtName("kafka").AtName("format")
	protobufSchemaKnown := !kafkaModel.ProtobufSchema.IsUnknown()
	schemaRegistryKnown := !kafkaModel.SchemaRegistry.IsUnknown()
	protobufSchemaSet := protobufSchemaKnown && !kafkaModel.ProtobufSchema.IsNull()
	schemaRegistrySet := schemaRegistryKnown && !kafkaModel.SchemaRegistry.IsNull()

	if protobufSchemaSet {
		encodedSchema := strings.TrimSpace(kafkaModel.ProtobufSchema.ValueString())
		if !isValidProtobufSchemaBase64(encodedSchema) {
			resp.Diagnostics.AddAttributeError(
				protobufSchemaPath,
				"Invalid Kafka Protobuf schema",
				"protobuf_schema must contain valid base64 data and must not exceed 1 MiB.",
			)
		}

		if schemaRegistrySet {
			resp.Diagnostics.AddAttributeError(
				schemaRegistryPath,
				"Invalid Kafka Protobuf schema configuration",
				"protobuf_schema cannot be combined with schema_registry.",
			)
		}
	}

	if kafkaModel.Format.IsNull() || kafkaModel.Format.IsUnknown() {
		return
	}

	format := kafkaModel.Format.ValueString()
	if protobufSchemaSet && format != api.ClickPipeProtobufFormat {
		resp.Diagnostics.AddAttributeError(
			formatPath,
			"Invalid Kafka Protobuf schema configuration",
			"protobuf_schema is supported only when format is Protobuf.",
		)
	}

	if format == api.ClickPipeProtobufFormat && protobufSchemaKnown && schemaRegistryKnown && !protobufSchemaSet && !schemaRegistrySet {
		resp.Diagnostics.AddAttributeError(
			protobufSchemaPath,
			"Missing Kafka Protobuf schema",
			"Protobuf format requires either protobuf_schema or schema_registry.",
		)
	}
}

// schemaRegistryField pairs a schema registry attribute name with its configured value.
type schemaRegistryField struct {
	name  string
	value attr.Value
}

// kafkaSchemaRegistryValidator enforces the per-type Kafka schema registry rules exposed by the OpenAPI:
// a Confluent registry needs url, authentication and credentials, while an AWS Glue registry needs a glue
// block and authenticates with IAM instead.
type kafkaSchemaRegistryValidator struct{}

// Description returns a plain-text summary of the Kafka schema registry validation.
func (v kafkaSchemaRegistryValidator) Description(_ context.Context) string {
	return "Validates Kafka Confluent and AWS Glue schema registry configuration."
}

// MarkdownDescription returns the Kafka schema registry validation summary as Markdown.
func (v kafkaSchemaRegistryValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource checks that the schema registry carries exactly the fields of its type.
// Checks on values Terraform has not resolved yet are deferred.
func (v kafkaSchemaRegistryValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.ClickPipeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data.Source.IsNull() || data.Source.IsUnknown() {
		return
	}

	var sourceModel models.ClickPipeSourceModel
	resp.Diagnostics.Append(data.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() || sourceModel.Kafka.IsNull() || sourceModel.Kafka.IsUnknown() {
		return
	}

	var kafkaModel models.ClickPipeKafkaSourceModel
	resp.Diagnostics.Append(sourceModel.Kafka.As(ctx, &kafkaModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() || kafkaModel.SchemaRegistry.IsNull() || kafkaModel.SchemaRegistry.IsUnknown() {
		return
	}

	var registryModel models.ClickPipeKafkaSchemaRegistryModel
	resp.Diagnostics.Append(kafkaModel.SchemaRegistry.As(ctx, &registryModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() || registryModel.Type.IsUnknown() {
		return
	}

	registryPath := path.Root("source").AtName("kafka").AtName("schema_registry")
	isSet := func(value attr.Value) bool { return !value.IsUnknown() && !value.IsNull() }
	isMissing := func(value attr.Value) bool { return !value.IsUnknown() && value.IsNull() }

	// Config sees the raw value, before the `confluent` default is applied.
	if registryModel.Type.ValueString() != api.ClickPipeKafkaSchemaRegistryTypeGlue {
		for _, field := range []schemaRegistryField{
			{"url", registryModel.URL},
			{"authentication", registryModel.Authentication},
			{"credentials", registryModel.Credentials},
		} {
			if isMissing(field.value) {
				resp.Diagnostics.AddAttributeError(
					registryPath.AtName(field.name),
					"Missing Kafka schema registry attribute",
					fmt.Sprintf("%s is required for a confluent schema registry.", field.name),
				)
			}
		}
		if isSet(registryModel.Glue) {
			resp.Diagnostics.AddAttributeError(
				registryPath.AtName("glue"),
				"Invalid Kafka schema registry attribute",
				"glue is supported only when type is glue.",
			)
		}
		return
	}

	// The schema requires region and registry_name inside the block, so only its presence is checked here.
	if isMissing(registryModel.Glue) {
		resp.Diagnostics.AddAttributeError(
			registryPath.AtName("glue"),
			"Missing Kafka schema registry attribute",
			"glue is required for a glue schema registry.",
		)
	}
	for _, field := range []schemaRegistryField{
		{"url", registryModel.URL},
		{"authentication", registryModel.Authentication},
		{"credentials", registryModel.Credentials},
	} {
		if isSet(field.value) {
			resp.Diagnostics.AddAttributeError(
				registryPath.AtName(field.name),
				"Invalid Kafka schema registry attribute",
				fmt.Sprintf("%s is not supported for a glue schema registry, which authenticates with IAM.", field.name),
			)
		}
	}

	// The platform only strips the Glue wire header on its Avro and Protobuf record paths.
	if !kafkaModel.Format.IsNull() && !kafkaModel.Format.IsUnknown() {
		format := kafkaModel.Format.ValueString()
		if format != api.ClickPipeAvroConfluentFormat && format != api.ClickPipeProtobufFormat {
			resp.Diagnostics.AddAttributeError(
				registryPath,
				"Invalid Kafka schema registry configuration",
				"a glue schema registry is supported only when format is AvroConfluent or Protobuf.",
			)
		}
	}

	if !isSet(registryModel.Glue) {
		return
	}
	var glueModel models.ClickPipeKafkaGlueSchemaRegistryModel
	resp.Diagnostics.Append(registryModel.Glue.As(ctx, &glueModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Glue defaults to the broker's IAM identity, so a non-IAM broker must name a role.
	// A null authentication means the PLAIN default.
	if !isMissing(glueModel.RoleArn) || kafkaModel.Authentication.IsUnknown() {
		return
	}
	authentication := kafkaModel.Authentication.ValueString()
	if authentication != api.ClickPipeAuthenticationIAMRole && authentication != api.ClickPipeAuthenticationIAMUser {
		resp.Diagnostics.AddAttributeError(
			registryPath.AtName("glue").AtName("role_arn"),
			"Missing Kafka schema registry attribute",
			"glue.role_arn is required for a glue schema registry unless the Kafka source uses IAM_ROLE or IAM_USER authentication.",
		)
	}
}

// kinesisProtobufSchemaValidator enforces the Kinesis schema source rules exposed by the OpenAPI:
// an uploaded Protobuf schema and an AWS Glue schema registry are alternatives, each tied to specific formats.
type kinesisProtobufSchemaValidator struct{}

// Description returns a plain-text summary of the Kinesis schema source validation.
func (v kinesisProtobufSchemaValidator) Description(_ context.Context) string {
	return "Validates Kinesis Protobuf schema and AWS Glue schema registry configuration."
}

// MarkdownDescription returns the validation summary for the supplied context.
func (v kinesisProtobufSchemaValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateResource reads req.Config in ctx and adds Kinesis schema errors to resp.
// Cross-field validation is deferred until the relevant Terraform values are known.
func (v kinesisProtobufSchemaValidator) ValidateResource(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	var data models.ClickPipeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Source.IsNull() || data.Source.IsUnknown() {
		return
	}

	var sourceModel models.ClickPipeSourceModel
	resp.Diagnostics.Append(data.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}
	if sourceModel.Kinesis.IsNull() || sourceModel.Kinesis.IsUnknown() {
		return
	}

	var kinesisModel models.ClickPipeKinesisSourceModel
	resp.Diagnostics.Append(sourceModel.Kinesis.As(ctx, &kinesisModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	protobufSchemaPath := path.Root("source").AtName("kinesis").AtName("protobuf_schema")
	protobufSchemaKnown := !kinesisModel.ProtobufSchema.IsUnknown()
	protobufSchemaSet := protobufSchemaKnown && !kinesisModel.ProtobufSchema.IsNull()
	if protobufSchemaSet {
		encodedSchema := strings.TrimSpace(kinesisModel.ProtobufSchema.ValueString())
		if !isValidProtobufSchemaBase64(encodedSchema) {
			resp.Diagnostics.AddAttributeError(
				protobufSchemaPath,
				"Invalid Kinesis Protobuf schema",
				"protobuf_schema must contain valid base64 data and must not exceed 1 MiB.",
			)
		}
	}

	schemaRegistryPath := path.Root("source").AtName("kinesis").AtName("schema_registry")
	schemaRegistryKnown := !kinesisModel.SchemaRegistry.IsUnknown()
	schemaRegistrySet := schemaRegistryKnown && !kinesisModel.SchemaRegistry.IsNull()
	if protobufSchemaSet && schemaRegistrySet {
		resp.Diagnostics.AddAttributeError(
			schemaRegistryPath,
			"Invalid Kinesis schema configuration",
			"protobuf_schema cannot be combined with schema_registry.",
		)
	}

	if kinesisModel.Format.IsNull() || kinesisModel.Format.IsUnknown() {
		return
	}

	format := kinesisModel.Format.ValueString()
	if protobufSchemaSet && format != api.ClickPipeProtobufFormat {
		resp.Diagnostics.AddAttributeError(
			path.Root("source").AtName("kinesis").AtName("format"),
			"Invalid Kinesis Protobuf schema configuration",
			"protobuf_schema is supported only when format is Protobuf.",
		)
	}
	if schemaRegistrySet && format != api.ClickPipeAvroConfluentFormat && format != api.ClickPipeProtobufFormat {
		resp.Diagnostics.AddAttributeError(
			schemaRegistryPath,
			"Invalid Kinesis schema registry configuration",
			"schema_registry is supported only when format is AvroConfluent or Protobuf.",
		)
	}
	schemaRegistryMissing := schemaRegistryKnown && !schemaRegistrySet
	if format == api.ClickPipeAvroConfluentFormat && schemaRegistryMissing {
		resp.Diagnostics.AddAttributeError(
			schemaRegistryPath,
			"Missing Kinesis schema registry",
			"AvroConfluent format requires schema_registry.",
		)
	}
	protobufSchemaMissing := protobufSchemaKnown && !protobufSchemaSet
	if format == api.ClickPipeProtobufFormat && protobufSchemaMissing && schemaRegistryMissing {
		resp.Diagnostics.AddAttributeError(
			protobufSchemaPath,
			"Missing Kinesis Protobuf schema",
			"Protobuf format requires either protobuf_schema or schema_registry.",
		)
	}
}

// isValidProtobufSchemaBase64 reports whether a non-empty schema is canonical padded or unpadded base64 within the API limit.
func isValidProtobufSchemaBase64(encodedSchema string) bool {
	if encodedSchema == "" || len(encodedSchema) > maxClickPipeProtobufSchemaEncodedSize {
		return false
	}

	decodedSchema, err := base64.StdEncoding.DecodeString(encodedSchema)
	if err != nil {
		decodedSchema, err = base64.RawStdEncoding.DecodeString(encodedSchema)
		if err != nil {
			return false
		}
	}

	canonicalSchema := base64.StdEncoding.EncodeToString(decodedSchema)
	return encodedSchema == canonicalSchema || encodedSchema == strings.TrimRight(canonicalSchema, "=")
}

// pubsubSeekValidator enforces the cross-field rules between
// source.pubsub.seek_type and seek_timestamp. The server rejects mismatches
// with a 400; this surfaces the same error at plan time.
type pubsubSeekValidator struct{}

func (v pubsubSeekValidator) Description(_ context.Context) string {
	return "Validates that source.pubsub.seek_timestamp matches the chosen seek_type."
}

func (v pubsubSeekValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v pubsubSeekValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.ClickPipeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Source.IsNull() || data.Source.IsUnknown() {
		return
	}

	sourceModel := models.ClickPipeSourceModel{}
	resp.Diagnostics.Append(data.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	if sourceModel.PubSub.IsNull() || sourceModel.PubSub.IsUnknown() {
		return
	}

	pubsubModel := models.ClickPipePubSubSourceModel{}
	resp.Diagnostics.Append(sourceModel.PubSub.As(ctx, &pubsubModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Skip if seek_type is unknown — the framework will re-run validation once known.
	if pubsubModel.SeekType.IsUnknown() || pubsubModel.SeekType.IsNull() {
		return
	}

	seekType := pubsubModel.SeekType.ValueString()
	tsSet := !pubsubModel.SeekTimestamp.IsNull() && !pubsubModel.SeekTimestamp.IsUnknown()

	timestampPath := path.Root("source").AtName("pubsub").AtName("seek_timestamp")

	switch seekType {
	case api.ClickPipePubSubSeekTypeLatest, api.ClickPipePubSubSeekTypeEarliest:
		if tsSet {
			resp.Diagnostics.AddAttributeError(
				timestampPath,
				"Invalid Pub/Sub seek configuration",
				fmt.Sprintf("seek_timestamp must not be set when seek_type is %q.", seekType),
			)
		}
	case api.ClickPipePubSubSeekTypeTimestamp:
		if !tsSet {
			resp.Diagnostics.AddAttributeError(
				timestampPath,
				"Invalid Pub/Sub seek configuration",
				fmt.Sprintf("seek_timestamp is required when seek_type is %q.", seekType),
			)
		}
	}
}

// cdcClickPipeScalingValidator prevents a partial create where the ClickPipe
// POST succeeds but the follow-up scaling PATCH is rejected by the API.
type cdcClickPipeScalingValidator struct{}

func (v cdcClickPipeScalingValidator) Description(_ context.Context) string {
	return "Validates that ClickPipe scaling is not configured for CDC source types."
}

func (v cdcClickPipeScalingValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v cdcClickPipeScalingValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data models.ClickPipeResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Scaling.IsNull() || data.Scaling.IsUnknown() || data.Source.IsNull() || data.Source.IsUnknown() {
		return
	}

	sourceModel := models.ClickPipeSourceModel{}
	resp.Diagnostics.Append(data.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	if sourceModel.Postgres.IsNull() && sourceModel.MySQL.IsNull() && sourceModel.MongoDB.IsNull() {
		return
	}

	resp.Diagnostics.AddAttributeError(
		path.Root("scaling"),
		"Invalid CDC ClickPipe scaling configuration",
		"scaling cannot be configured on clickhouse_clickpipe for Postgres, MySQL, or MongoDB CDC sources. Configure CDC infrastructure sizing with clickhouse_clickpipe_cdc_infrastructure instead.",
	)
}

func (c *ClickPipeResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		kafkaTombstoneModeValidator{},
		kafkaProtobufSchemaValidator{},
		kafkaSchemaRegistryValidator{},
		kinesisProtobufSchemaValidator{},
		pubsubSeekValidator{},
		cdcClickPipeScalingValidator{},
	}
}
