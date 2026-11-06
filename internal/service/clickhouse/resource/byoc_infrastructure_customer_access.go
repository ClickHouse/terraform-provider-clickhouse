package resource

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

// Mirrors the API's 48 hour cap so the plan fails before the apply does.
const byocCustomerAccessMaxDurationSeconds = 48 * 60 * 60

type byocCustomerAccessModel struct {
	Enabled         types.Bool   `tfsdk:"enabled"`
	DurationSeconds types.Int64  `tfsdk:"duration_seconds"`
	StartTimestamp  types.String `tfsdk:"start_timestamp"`
}

func byocCustomerAccessAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"enabled":          types.BoolType,
		"duration_seconds": types.Int64Type,
		"start_timestamp":  types.StringType,
	}
}

func byocCustomerAuthorizedAccessSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "Controls when ClickHouse personnel may access the infrastructure. Omit the block to leave the setting unmanaged (the ClickHouse Cloud default allows access). Removing the block leaves the last applied setting in place.",
		Optional:    true,
		Attributes: map[string]schema.Attribute{
			"enabled": schema.BoolAttribute{
				Description: "Whether ClickHouse personnel may access the infrastructure. 'false' denies access regardless of the other fields.",
				Required:    true,
			},
			"duration_seconds": schema.Int64Attribute{
				Description: fmt.Sprintf("Length of the access window in seconds, counted from 'start_timestamp' (max %d, 48 hours). Defaults to 0, which allows access without a deadline while 'enabled' is true.", byocCustomerAccessMaxDurationSeconds),
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				Validators: []validator.Int64{
					int64validator.Between(0, byocCustomerAccessMaxDurationSeconds),
				},
			},
			"start_timestamp": schema.StringAttribute{
				Description: "Start of the access window as an RFC3339 timestamp (e.g. '2026-06-01T12:00:00Z'). Required when 'duration_seconds' is greater than 0.",
				Optional:    true,
				Validators: []validator.String{
					rfc3339Validator{},
				},
			},
		},
	}
}

// The API's RFC3339 check allows at most three fractional-second digits.
var subMillisecondFraction = regexp.MustCompile(`T\d{2}:\d{2}:\d{2}\.\d{4,}`)

type rfc3339Validator struct{}

func (rfc3339Validator) Description(context.Context) string {
	return "value must be an RFC3339 timestamp"
}

func (v rfc3339Validator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (rfc3339Validator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueString()
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid RFC3339 timestamp",
			fmt.Sprintf("%q is not an RFC3339 timestamp such as '2026-06-01T12:00:00Z': %s", value, err.Error()))
		return
	}
	if subMillisecondFraction.MatchString(value) {
		resp.Diagnostics.AddAttributeError(req.Path, "Timestamp too precise",
			fmt.Sprintf("%q has more than three fractional-second digits; ClickHouse Cloud accepts at most millisecond precision, such as '2026-06-01T12:00:00.123Z'.", value))
	}
}

// byocValidateCustomerAccess re-checks the window invariant the API enforces
// so a misconfiguration fails at plan time. Unknown values pass.
func byocValidateCustomerAccess(ctx context.Context, obj types.Object, diags *diag.Diagnostics) {
	if !isKnown(obj) {
		return
	}
	var model byocCustomerAccessModel
	diags.Append(obj.As(ctx, &model, byocObjectAsOptions())...)
	if diags.HasError() || !isKnown(model.Enabled) || !isKnown(model.DurationSeconds) || model.StartTimestamp.IsUnknown() {
		return
	}
	if model.Enabled.ValueBool() && model.DurationSeconds.ValueInt64() > 0 && model.StartTimestamp.IsNull() {
		diags.AddAttributeError(
			path.Root("customer_authorized_access").AtName("start_timestamp"),
			"start_timestamp is required for a time-limited window",
			"Set customer_authorized_access.start_timestamp when duration_seconds is greater than 0, or set duration_seconds = 0 to allow access without a deadline.",
		)
	}
}

func byocCustomerAccessRequestFromObject(ctx context.Context, obj types.Object) (api.ByocInfrastructureCustomerAuthorizedAccessUpdateRequest, diag.Diagnostics) {
	var model byocCustomerAccessModel
	diags := obj.As(ctx, &model, byocObjectAsOptions())
	request := api.ByocInfrastructureCustomerAuthorizedAccessUpdateRequest{
		Enabled:         model.Enabled.ValueBool(),
		DurationSeconds: model.DurationSeconds.ValueInt64(),
	}
	if isKnown(model.StartTimestamp) {
		request.StartTimestamp = model.StartTimestamp.ValueStringPointer()
	}
	return request, diags
}

// byocCustomerAccessObjectFromAPI maps the API read-back onto the nested
// object. The API normalizes timestamps (UTC, millisecond precision), so a
// configured start that denotes the same instant is kept verbatim to avoid a
// perpetual diff.
func byocCustomerAccessObjectFromAPI(ctx context.Context, access *api.ByocInfrastructureCustomerAuthorizedAccess, prior types.Object) (types.Object, diag.Diagnostics) {
	start := types.StringNull()
	if access.StartTimestamp != nil {
		start = types.StringValue(*access.StartTimestamp)
		if isKnown(prior) {
			var priorModel byocCustomerAccessModel
			if d := prior.As(ctx, &priorModel, byocObjectAsOptions()); !d.HasError() && isKnown(priorModel.StartTimestamp) && sameInstant(priorModel.StartTimestamp.ValueString(), *access.StartTimestamp) {
				start = priorModel.StartTimestamp
			}
		}
	}
	return types.ObjectValue(byocCustomerAccessAttrTypes(), map[string]attr.Value{
		"enabled":          types.BoolValue(access.Enabled),
		"duration_seconds": types.Int64Value(access.DurationSeconds),
		"start_timestamp":  start,
	})
}

func byocObjectAsOptions() basetypes.ObjectAsOptions {
	return basetypes.ObjectAsOptions{}
}

func sameInstant(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339, a)
	tb, errB := time.Parse(time.RFC3339, b)
	return errA == nil && errB == nil && ta.Equal(tb)
}
