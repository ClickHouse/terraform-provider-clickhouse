package clickstack

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                   = (*alertResource)(nil)
	_ resource.ResourceWithConfigure      = (*alertResource)(nil)
	_ resource.ResourceWithImportState    = (*alertResource)(nil)
	_ resource.ResourceWithValidateConfig = (*alertResource)(nil)
)

// Threshold type values referenced in more than one place.
const (
	thresholdTypeAbove   = "above"
	thresholdTypeBetween = "between"
)

// alertThresholdTypes is the set of accepted threshold comparison types.
var alertThresholdTypes = []string{
	thresholdTypeAbove, "below", "above_exclusive", "below_or_equal",
	"equal", "not_equal", thresholdTypeBetween, "not_between",
}

// alertRangeThresholdTypes are the threshold types that require threshold_max.
var alertRangeThresholdTypes = []string{thresholdTypeBetween, "not_between"}

// alertIntervalMinutes maps each accepted interval to its length in minutes.
var alertIntervalMinutes = map[string]int64{
	"1m": 1, "5m": 5, "15m": 15, "30m": 30,
	"1h": 60, "6h": 360, "12h": 720, "1d": 1440,
}

// channelTypeWebhook is the only channel type supported today.
const channelTypeWebhook = "webhook"

// alertChannelTypes is the set of accepted channel types. Only webhook exists
// today; more are expected, at which point each adds its own required sub-field.
var alertChannelTypes = []string{channelTypeWebhook}

func isRangeThresholdType(t string) bool { return slices.Contains(alertRangeThresholdTypes, t) }

// NewAlertResource is a helper to register the resource with the provider.
func NewAlertResource() resource.Resource {
	return &alertResource{}
}

// alertResource manages a ClickStack alert (saved-search source only).
type alertResource struct {
	client *client.Client
}

// alertChannelModel maps the nested channel block.
type alertChannelModel struct {
	Type      types.String `tfsdk:"type"`
	WebhookID types.String `tfsdk:"webhook_id"`
}

// alertResourceModel maps the resource schema data. Server-managed transient
// fields (state, silenced, execution_errors) are intentionally not modeled: they
// are never sent, and the API's partial-update PUT preserves them (KTD8).
type alertResourceModel struct {
	ID                    types.String       `tfsdk:"id"`
	Team                  types.String       `tfsdk:"team"`
	SavedSearchID         types.String       `tfsdk:"saved_search_id"`
	GroupBy               types.String       `tfsdk:"group_by"`
	Channel               *alertChannelModel `tfsdk:"channel"`
	Threshold             types.Float64      `tfsdk:"threshold"`
	ThresholdType         types.String       `tfsdk:"threshold_type"`
	ThresholdMax          types.Float64      `tfsdk:"threshold_max"`
	Interval              types.String       `tfsdk:"interval"`
	NumConsecutiveWindows types.Int64        `tfsdk:"num_consecutive_windows"`
	ScheduleOffsetMinutes types.Int64        `tfsdk:"schedule_offset_minutes"`
	ScheduleStartAt       types.String       `tfsdk:"schedule_start_at"`
	Name                  types.String       `tfsdk:"name"`
	Message               types.String       `tfsdk:"message"`
	Note                  types.String       `tfsdk:"note"`
}

func (r *alertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_alert"
}

func (r *alertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ClickStack alert that evaluates a saved search on a schedule and " +
			"notifies through a channel when a threshold is crossed.\n\n" +
			"Alerts are threshold-based (there is no anomaly mode). Configuration is validated at " +
			"plan time; those rules mirror the ClickStack server contract on a best-effort basis, so " +
			"a server-side rule change may make the plan-time checks slightly stale until a new " +
			"provider release.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:      true,
				Description:   "Identifier of the alert.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID to manage this alert under (`x-hdx-team`). " +
					"Changing this forces the alert to be replaced.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"saved_search_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the saved search this alert evaluates.",
			},
			"group_by": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Optional expression to evaluate the alert per group. Sticky once set: the " +
					"API keeps the previous value when the field is omitted and cannot clear it, so " +
					"removing it from config is a no-op (recreate the alert to fully reset it).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"channel": schema.SingleNestedAttribute{
				Required:    true,
				Description: "Notification channel for the alert.",
				Attributes: map[string]schema.Attribute{
					"type": schema.StringAttribute{
						Required:    true,
						Description: "Channel type. Currently only `webhook` is supported.",
					},
					"webhook_id": schema.StringAttribute{
						Optional:    true,
						Description: "ID of the webhook to notify. Required when `type` is `webhook`.",
					},
				},
			},
			"threshold": schema.Float64Attribute{
				Required:    true,
				Description: "Threshold value the alert compares against. For range types (`between`/`not_between`) this is the lower bound.",
			},
			"threshold_type": schema.StringAttribute{
				Required: true,
				Description: "Comparison type: one of `above`, `below`, `above_exclusive`, " +
					"`below_or_equal`, `equal`, `not_equal`, `between`, `not_between`.",
			},
			"threshold_max": schema.Float64Attribute{
				Optional:    true,
				Description: "Upper bound, required for `between`/`not_between` and ignored otherwise. Must be >= `threshold`.",
			},
			"interval": schema.StringAttribute{
				Required:    true,
				Description: "Evaluation window: one of `1m`, `5m`, `15m`, `30m`, `1h`, `6h`, `12h`, `1d`.",
			},
			"num_consecutive_windows": schema.Int64Attribute{
				Optional:    true,
				Description: "Fire only after the condition holds for this many consecutive windows (>= 1).",
			},
			"schedule_offset_minutes": schema.Int64Attribute{
				Optional: true,
				Description: "Offset window boundaries by this many minutes (0-1439, and less than the " +
					"interval). Mutually exclusive with `schedule_start_at`; setting one clears the " +
					"other.",
			},
			"schedule_start_at": schema.StringAttribute{
				Optional: true,
				Description: "Absolute UTC anchor (RFC3339) for window alignment. Mutually exclusive with " +
					"a non-zero `schedule_offset_minutes`; setting one clears the other.",
				PlanModifiers: []planmodifier.String{rfc3339EqualPlanModifier{}},
			},
			nameAttr: schema.StringAttribute{
				Optional:    true,
				Description: "Optional alert name (1-512 characters).",
			},
			"message": schema.StringAttribute{
				Optional:    true,
				Description: "Optional notification message template (1-4096 characters).",
			},
			"note": schema.StringAttribute{
				Optional:    true,
				Description: "Optional markdown note (1-4096 characters).",
			},
		},
	}
}

func (r *alertResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("expected *service.ProviderData, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	if providerData.ClickStack == nil {
		resp.Diagnostics.AddError("ClickStack not configured",
			"This resource requires ClickStack credentials. For self-hosted ClickStack, set clickstack_endpoint and "+
				"clickstack_api_key on the provider (or the CLICKSTACK_ENDPOINT / CLICKSTACK_API_KEY environment variables). "+
				"For ClickStack on ClickHouse Cloud, set clickstack_service_id (or CLICKSTACK_SERVICE_ID) together with "+
				"the ClickHouse Cloud credentials (organization_id, token_key, token_secret).")
		return
	}
	r.client = providerData.ClickStack
}

// ValidateConfig enforces the alert's cross-field rules at plan time. Every rule
// short-circuits when an operand is null or unknown, mirroring the guard in the
// dashboard resource's ValidateConfig.
func (r *alertResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_clickstack_alert", &resp.Diagnostics)
	var cfg alertResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(cfg.validate()...)
}

// validate holds the alert's cross-field rules. It is a pure function of the
// model so it can be unit-tested directly. Every rule short-circuits when an
// operand is null or unknown.
func (m *alertResourceModel) validate() diag.Diagnostics {
	var diags diag.Diagnostics

	tt := m.ThresholdType
	if known(tt) && !slices.Contains(alertThresholdTypes, tt.ValueString()) {
		diags.AddAttributeError(path.Root("threshold_type"), "Invalid threshold_type",
			fmt.Sprintf("threshold_type must be one of %s, got %q", strings.Join(alertThresholdTypes, ", "), tt.ValueString()))
	}

	iv := m.Interval
	if known(iv) {
		if _, ok := alertIntervalMinutes[iv.ValueString()]; !ok {
			diags.AddAttributeError(path.Root("interval"), "Invalid interval",
				fmt.Sprintf("interval must be one of 1m, 5m, 15m, 30m, 1h, 6h, 12h, 1d, got %q", iv.ValueString()))
		}
	}

	// threshold_max presence/ordering for range types.
	if known(tt) && isRangeThresholdType(tt.ValueString()) {
		if m.ThresholdMax.IsNull() {
			diags.AddAttributeError(path.Root("threshold_max"), "threshold_max required",
				fmt.Sprintf("threshold_max is required when threshold_type is %q", tt.ValueString()))
		} else if known(m.ThresholdMax) && known(m.Threshold) &&
			m.ThresholdMax.ValueFloat64() < m.Threshold.ValueFloat64() {
			diags.AddAttributeError(path.Root("threshold_max"), "threshold_max too small",
				"threshold_max must be greater than or equal to threshold")
		}
	}

	// Scheduling modes are mutually exclusive.
	offsetSet := known(m.ScheduleOffsetMinutes) && m.ScheduleOffsetMinutes.ValueInt64() > 0
	if known(m.ScheduleStartAt) && offsetSet {
		diags.AddAttributeError(path.Root("schedule_offset_minutes"), "Conflicting scheduling",
			"set either schedule_start_at or a non-zero schedule_offset_minutes, not both")
	}

	// Offset must be within [0,1439] and smaller than the interval.
	if known(m.ScheduleOffsetMinutes) {
		off := m.ScheduleOffsetMinutes.ValueInt64()
		if off < 0 || off > 1439 {
			diags.AddAttributeError(path.Root("schedule_offset_minutes"), "Invalid offset",
				"schedule_offset_minutes must be between 0 and 1439")
		}
		if known(iv) {
			if mins, ok := alertIntervalMinutes[iv.ValueString()]; ok && off >= mins {
				diags.AddAttributeError(path.Root("schedule_offset_minutes"), "Offset too large",
					"schedule_offset_minutes must be smaller than the interval")
			}
		}
	}

	if known(m.NumConsecutiveWindows) && m.NumConsecutiveWindows.ValueInt64() < 1 {
		diags.AddAttributeError(path.Root("num_consecutive_windows"), "Invalid value",
			"num_consecutive_windows must be at least 1")
	}

	validateLen(&diags, path.Root("name"), m.Name, 512)
	validateLen(&diags, path.Root("message"), m.Message, 4096)
	validateLen(&diags, path.Root("note"), m.Note, 4096)

	// Channel: type must be known, and webhook channels require a webhook_id.
	if m.Channel != nil {
		ct := m.Channel.Type
		if known(ct) && !slices.Contains(alertChannelTypes, ct.ValueString()) {
			diags.AddAttributeError(path.Root("channel").AtName("type"), "Invalid channel type",
				fmt.Sprintf("channel type must be one of %s, got %q", strings.Join(alertChannelTypes, ", "), ct.ValueString()))
		}
		// An empty webhook_id is caught here rather than as an opaque API 400: the
		// client's webhookId is omitempty, so "" would serialize as absent.
		if known(ct) && ct.ValueString() == channelTypeWebhook &&
			(m.Channel.WebhookID.IsNull() || (known(m.Channel.WebhookID) && m.Channel.WebhookID.ValueString() == "")) {
			diags.AddAttributeError(path.Root("channel").AtName("webhook_id"), "webhook_id required",
				"channel.webhook_id is required and must be non-empty when channel.type is \"webhook\"")
		}
	}

	return diags
}

func (r *alertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	al, err := r.client.WithTeam(plan.Team.ValueString()).CreateAlert(ctx, plan.toClient())
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Alert", err.Error())
		return
	}

	plan.applyAlert(al)
	tflog.Trace(ctx, "created alert resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *alertResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	al, err := r.client.WithTeam(state.Team.ValueString()).GetAlert(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Alert", err.Error())
		return
	}

	state.applyAlert(al)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *alertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	al, err := r.client.WithTeam(plan.Team.ValueString()).UpdateAlert(ctx, plan.ID.ValueString(), plan.toClient())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Updating Alert", err.Error())
		return
	}

	plan.applyAlert(al)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *alertResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.WithTeam(state.Team.ValueString()).DeleteAlert(ctx, state.ID.ValueString()); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting Alert", err.Error())
	}
}

func (r *alertResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if team, id, ok := strings.Cut(req.ID, "/"); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team"), team)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// --- conversion helpers ---

func (m *alertResourceModel) toClient() client.Alert {
	al := client.Alert{
		Interval:        m.Interval.ValueString(),
		Threshold:       m.Threshold.ValueFloat64(),
		ThresholdType:   m.ThresholdType.ValueString(),
		SavedSearchID:   m.SavedSearchID.ValueString(),
		GroupBy:         optStringPtr(m.GroupBy),
		Name:            optStringPtr(m.Name),
		Message:         optStringPtr(m.Message),
		Note:            optStringPtr(m.Note),
		ScheduleStartAt: optStringPtr(m.ScheduleStartAt),
	}
	if m.Channel != nil {
		al.Channel = client.AlertChannel{
			Type:      m.Channel.Type.ValueString(),
			WebhookID: m.Channel.WebhookID.ValueString(),
		}
	}
	// threshold_max is only meaningful for range types; ignore it otherwise.
	if isRangeThresholdType(m.ThresholdType.ValueString()) && known(m.ThresholdMax) {
		v := m.ThresholdMax.ValueFloat64()
		al.ThresholdMax = &v
	}
	if known(m.NumConsecutiveWindows) {
		v := int(m.NumConsecutiveWindows.ValueInt64())
		al.NumConsecutiveWindows = &v
	}
	// Scheduling modes are mutually exclusive. schedule_start_at is always sent
	// (nil -> JSON null clears it, and the server then forces the offset to 0).
	// Only send an explicit offset when schedule_start_at is NOT set, so:
	//   - switching modes never emits both fields (the API rejects that), and
	//   - dropping a field propagates as a clear instead of resending a stale
	//     value read back from a sticky plan.
	if !known(m.ScheduleStartAt) && known(m.ScheduleOffsetMinutes) {
		v := int(m.ScheduleOffsetMinutes.ValueInt64())
		al.ScheduleOffsetMinutes = &v
	}
	return al
}

func (m *alertResourceModel) applyAlert(al *client.Alert) {
	m.ID = types.StringValue(al.ID)
	m.SavedSearchID = types.StringValue(al.SavedSearchID)
	m.GroupBy = types.StringPointerValue(al.GroupBy)
	m.Channel = &alertChannelModel{
		Type:      types.StringValue(al.Channel.Type),
		WebhookID: emptyToNull(al.Channel.WebhookID),
	}
	m.Threshold = types.Float64Value(al.Threshold)
	m.ThresholdType = types.StringValue(al.ThresholdType)
	// For range types reflect the server's threshold_max; for other types the
	// server ignores it, so leave the configured value in place to avoid a
	// spurious diff (KTD3).
	if isRangeThresholdType(al.ThresholdType) {
		if al.ThresholdMax != nil {
			m.ThresholdMax = types.Float64Value(*al.ThresholdMax)
		} else {
			m.ThresholdMax = types.Float64Null()
		}
	}
	m.Interval = types.StringValue(al.Interval)
	if al.NumConsecutiveWindows != nil {
		m.NumConsecutiveWindows = types.Int64Value(int64(*al.NumConsecutiveWindows))
	} else {
		m.NumConsecutiveWindows = types.Int64Null()
	}
	// The server forces the offset to 0 when scheduling by start-at (or when no
	// scheduling is set). Treat a *server-forced* 0 as "unset" (null) so it does
	// not show a spurious diff against a null config — but preserve a 0 the config
	// set explicitly, so an explicit `schedule_offset_minutes = 0` round-trips
	// instead of producing an "inconsistent result after apply". Preserving the
	// explicit 0 in the nil case too means correctness does not depend on whether
	// the server echoes a zero offset as literal 0 or omits it.
	configZero := known(m.ScheduleOffsetMinutes) && m.ScheduleOffsetMinutes.ValueInt64() == 0
	switch {
	case al.ScheduleOffsetMinutes != nil && *al.ScheduleOffsetMinutes != 0:
		m.ScheduleOffsetMinutes = types.Int64Value(int64(*al.ScheduleOffsetMinutes))
	case configZero:
		m.ScheduleOffsetMinutes = types.Int64Value(0) // explicit config 0, preserved across a nil or zero server echo
	default:
		m.ScheduleOffsetMinutes = types.Int64Null() // server-forced 0 (or no scheduling)
	}
	// Keep the authored timestamp when it denotes the same instant the server
	// returned, so a server canonicalization (e.g. adding milliseconds) does not
	// diverge from the known planned value and raise "inconsistent result after
	// apply".
	keepAuthoredStartAt := known(m.ScheduleStartAt) && al.ScheduleStartAt != nil &&
		rfc3339Equal(m.ScheduleStartAt.ValueString(), *al.ScheduleStartAt)
	if !keepAuthoredStartAt {
		m.ScheduleStartAt = types.StringPointerValue(al.ScheduleStartAt)
	}
	m.Name = types.StringPointerValue(al.Name)
	m.Message = types.StringPointerValue(al.Message)
	m.Note = types.StringPointerValue(al.Note)
}

// nullUnknown is satisfied by every basetypes value (types.String, types.Int64,
// types.Float64, ...).
type nullUnknown interface {
	IsNull() bool
	IsUnknown() bool
}

// known reports whether a value is neither null nor unknown, i.e. safe to read.
func known(v nullUnknown) bool { return !v.IsNull() && !v.IsUnknown() }

// validateLen adds an error when a set string value is empty or exceeds max.
func validateLen(diags *diag.Diagnostics, p path.Path, v types.String, max int) {
	if !known(v) {
		return
	}
	n := len(v.ValueString())
	if n < 1 || n > max {
		diags.AddAttributeError(p, "Invalid length",
			fmt.Sprintf("value must be between 1 and %d characters", max))
	}
}
