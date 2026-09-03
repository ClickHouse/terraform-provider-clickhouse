package resource

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

const maxClickPipeProtobufSchemaEncodedSize = 1 << 20 // 1 MiB in bytes

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
		kafkaProtobufSchemaValidator{},
		pubsubSeekValidator{},
		cdcClickPipeScalingValidator{},
	}
}
