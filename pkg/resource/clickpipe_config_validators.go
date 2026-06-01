package resource

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

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

func (c *ClickPipeResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		pubsubSeekValidator{},
	}
}
