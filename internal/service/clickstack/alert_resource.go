package clickstack

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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

// alertChannelAttrTypes mirrors alertChannelAttributes() for the framework-typed
// channel/channels model fields; the two must be kept in step.
var alertChannelAttrTypes = map[string]attr.Type{
	"type":       types.StringType,
	"webhook_id": types.StringType,
}

var alertChannelObjectType = types.ObjectType{AttrTypes: alertChannelAttrTypes}

// alertResourceModel maps the resource schema data. Server-managed transient
// fields (state, silenced, execution_errors) are intentionally not modeled: they
// are never sent, and the API's partial-update PUT preserves them (KTD8).
type alertResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Team          types.String `tfsdk:"team"`
	SavedSearchID types.String `tfsdk:"saved_search_id"`
	GroupBy       types.String `tfsdk:"group_by"`
	// Channel and Channels are framework types rather than a Go pointer/slice so
	// they can hold a wholly-unknown value. A config that takes either from a
	// module output or a data source leaves the whole attribute unknown until
	// Terraform resolves it, and reflecting that into *alertChannelModel or
	// []alertChannelModel fails Config.Get with a "this is always an error in
	// the provider" diagnostic (see asChannel/asChannels).
	Channel               types.Object  `tfsdk:"channel"`
	Channels              types.List    `tfsdk:"channels"`
	Threshold             types.Float64 `tfsdk:"threshold"`
	ThresholdType         types.String  `tfsdk:"threshold_type"`
	ThresholdMax          types.Float64 `tfsdk:"threshold_max"`
	Interval              types.String  `tfsdk:"interval"`
	NumConsecutiveWindows types.Int64   `tfsdk:"num_consecutive_windows"`
	ScheduleOffsetMinutes types.Int64   `tfsdk:"schedule_offset_minutes"`
	ScheduleStartAt       types.String  `tfsdk:"schedule_start_at"`
	Name                  types.String  `tfsdk:"name"`
	Message               types.String  `tfsdk:"message"`
	Note                  types.String  `tfsdk:"note"`
}

func (r *alertResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_alert"
}

func (r *alertResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ClickStack alert that evaluates a saved search on a schedule and " +
			"notifies one or more channels when a threshold is crossed.\n\n" +
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
				Optional: true,
				DeprecationMessage: "Use channels instead. channel notifies a single target; " +
					"channels takes a list and is the only way to notify more than one. See " +
					"[the docs](https://github.com/ClickHouse/terraform-provider-clickhouse?tab=readme-ov-file#breaking-changes-and-deprecations) " +
					"for migration steps.",
				Description: "Single notification channel for the alert. Deprecated: use `channels`. " +
					"Exactly one of `channel` or `channels` must be set. Importing an alert always " +
					"populates `channels`, so a config still on `channel` shows a diff after import.",
				Attributes: alertChannelAttributes(),
			},
			"channels": schema.ListNestedAttribute{
				Optional: true,
				Description: fmt.Sprintf(
					"Notification channels for the alert, in order. Between 1 and %d entries, no duplicates. "+
						"Exactly one of `channel` or `channels` must be set.", client.MaxAlertChannels),
				NestedObject: schema.NestedAttributeObject{Attributes: alertChannelAttributes()},
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

// alertChannelAttributes is the attribute set of a single channel, shared by the
// deprecated `channel` object and each entry of `channels`.
func alertChannelAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"type": schema.StringAttribute{
			Required:    true,
			Description: "Channel type. Currently only `webhook` is supported.",
		},
		"webhook_id": schema.StringAttribute{
			Optional:    true,
			Description: "ID of the webhook to notify. Required when `type` is `webhook`.",
		},
	}
}

// asChannel decodes the deprecated single `channel`. The returned pointer is nil
// when the attribute is unset. ok is false when the value is unknown as a whole,
// i.e. Terraform has not resolved it yet and no rule can inspect it.
func asChannel(ctx context.Context, o types.Object) (*alertChannelModel, bool, diag.Diagnostics) {
	if o.IsNull() {
		return nil, true, nil
	}
	if o.IsUnknown() {
		return nil, false, nil
	}
	var c alertChannelModel
	d := o.As(ctx, &c, basetypes.ObjectAsOptions{})
	return &c, true, d
}

// asChannels decodes `channels`. A null attribute yields a nil slice and an
// explicit `channels = []` yields a non-nil empty one, so callers can still tell
// "unset" from "set to empty". ok is false when the list is unknown as a whole;
// an unknown *element* field (a webhook_id referencing a webhook created in the
// same apply) decodes fine, since types.String holds unknown.
func asChannels(ctx context.Context, l types.List) ([]alertChannelModel, bool, diag.Diagnostics) {
	if l.IsNull() {
		return nil, true, nil
	}
	if l.IsUnknown() {
		return nil, false, nil
	}
	out := make([]alertChannelModel, 0, len(l.Elements()))
	d := l.ElementsAs(ctx, &out, false)
	return out, true, d
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
		addNotConfiguredError(&resp.Diagnostics, "resource")
		return
	}
	r.client = providerData.ClickStack
}

// ValidateConfig enforces the alert's cross-field rules at plan time. Every rule
// short-circuits when an operand is null or unknown, mirroring the guard in the
// dashboard resource's ValidateConfig.
func (r *alertResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.BetaWarning("clickhouse_clickstack_alert", &resp.Diagnostics)
	var cfg alertResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(cfg.validate(ctx)...)
}

// validate holds the alert's cross-field rules. It is a pure function of the
// model so it can be unit-tested directly. Every rule short-circuits when an
// operand is null or unknown.
func (m *alertResourceModel) validate(ctx context.Context) diag.Diagnostics {
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

	m.validateChannels(ctx, &diags)

	return diags
}

// validateChannels enforces the channel/channels selection rules: exactly one of
// the two, a list within the API's size limit, no duplicates, and a valid type
// with its required sub-field on every entry.
func (m *alertResourceModel) validateChannels(ctx context.Context, diags *diag.Diagnostics) {
	// Presence is decided on null alone, before any decoding: an attribute set to
	// a value Terraform has not resolved yet is still set, so the exactly-one-of
	// rule holds even when the value itself cannot be inspected.
	switch {
	case m.Channel.IsNull() && m.Channels.IsNull():
		diags.AddAttributeError(path.Root("channels"), "Notification channel required",
			"set channels, or the deprecated channel for a single target")
		return
	case !m.Channel.IsNull() && !m.Channels.IsNull():
		diags.AddAttributeError(path.Root("channels"), "Conflicting channel configuration",
			"set either channels or the deprecated channel, not both")
		return
	}

	single, singleOK, d := asChannel(ctx, m.Channel)
	diags.Append(d...)
	list, listOK, d := asChannels(ctx, m.Channels)
	diags.Append(d...)
	// Exactly one of the two is set by this point. Every remaining rule needs its
	// value, so an unresolved reference defers to the server and the post-apply
	// plan rather than guessing.
	if !singleOK || !listOK {
		return
	}

	if single != nil {
		validateAlertChannel(diags, path.Root("channel"), *single)
	}
	if list == nil {
		return
	}
	if len(list) == 0 || len(list) > client.MaxAlertChannels {
		diags.AddAttributeError(path.Root("channels"), "Invalid channels",
			fmt.Sprintf("channels must contain between 1 and %d entries, got %d", client.MaxAlertChannels, len(list)))
	}
	// The API rejects duplicates; catching them here names the offending index
	// instead of surfacing an opaque 400.
	seen := make(map[string]int, len(list))
	for i, c := range list {
		p := path.Root("channels").AtListIndex(i)
		validateAlertChannel(diags, p, c)
		// An unknown webhook_id — one referencing a webhook created in the same
		// apply — reads back as "", so every unresolved entry would key
		// identically and the second would be reported as a duplicate of the
		// first. Those can only be compared once Terraform resolves them; the
		// API's own duplicate check is the backstop. A null webhook_id stays in
		// the key on purpose, so a future channel type that has none still
		// dedupes correctly.
		if !known(c.Type) || c.WebhookID.IsUnknown() {
			continue
		}
		key := c.Type.ValueString() + "\x00" + c.WebhookID.ValueString()
		if first, dup := seen[key]; dup {
			diags.AddAttributeError(p, "Duplicate channel",
				fmt.Sprintf("channels[%d] duplicates channels[%d]: same type and webhook_id", i, first))
			continue
		}
		seen[key] = i
	}
}

// validateAlertChannel checks one channel block: a known type, and the sub-field
// that type requires.
func validateAlertChannel(diags *diag.Diagnostics, p path.Path, c alertChannelModel) {
	ct := c.Type
	if !known(ct) {
		return
	}
	if !slices.Contains(alertChannelTypes, ct.ValueString()) {
		diags.AddAttributeError(p.AtName("type"), "Invalid channel type",
			fmt.Sprintf("channel type must be one of %s, got %q", strings.Join(alertChannelTypes, ", "), ct.ValueString()))
		return
	}
	// An empty webhook_id is caught here rather than as an opaque API 400: the
	// client's webhookId is omitempty, so "" would serialize as absent.
	if ct.ValueString() == channelTypeWebhook &&
		(c.WebhookID.IsNull() || (known(c.WebhookID) && c.WebhookID.ValueString() == "")) {
		diags.AddAttributeError(p.AtName("webhook_id"), "webhook_id required",
			"webhook_id is required and must be non-empty when type is \"webhook\"")
	}
}

func (r *alertResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, d := plan.toClient(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	al, err := r.client.WithTeam(plan.Team.ValueString()).CreateAlert(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Alert", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyAlert(ctx, al)...)
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

	resp.Diagnostics.Append(state.applyAlert(ctx, al)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *alertResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan alertResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in, d := plan.toClient(ctx)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	al, err := r.client.WithTeam(plan.Team.ValueString()).UpdateAlert(ctx, plan.ID.ValueString(), in)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Updating Alert", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyAlert(ctx, al)...)
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

func (m *alertResourceModel) toClient(ctx context.Context) (client.Alert, diag.Diagnostics) {
	var diags diag.Diagnostics
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
	// Only one of the two is ever set (ValidateConfig rejects both). The client
	// mirrors `channel` from `channels[0]` before sending. Both values are known
	// by the time this runs — Terraform resolves the plan before apply — so the
	// unknown case asChannel/asChannels guard against cannot reach here.
	single, singleOK, d := asChannel(ctx, m.Channel)
	diags.Append(d...)
	list, listOK, d := asChannels(ctx, m.Channels)
	diags.Append(d...)
	if !singleOK || !listOK {
		// Unreachable: Terraform resolves the plan before apply. Reported rather
		// than ignored so a future caller that does reach it gets this instead of
		// a request with no channel and an opaque API 400.
		diags.AddError("Unresolved notification channel",
			"the alert's channel configuration was still unknown at apply time. This is a bug in the provider.")
		return al, diags
	}
	switch {
	case list != nil:
		al.Channels = make([]client.AlertChannel, 0, len(list))
		for _, c := range list {
			al.Channels = append(al.Channels, client.AlertChannel{
				Type:      c.Type.ValueString(),
				WebhookID: c.WebhookID.ValueString(),
			})
		}
	case single != nil:
		al.Channel = client.AlertChannel{
			Type:      single.Type.ValueString(),
			WebhookID: single.WebhookID.ValueString(),
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
	return al, diags
}

func (m *alertResourceModel) applyAlert(ctx context.Context, al *client.Alert) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(al.ID)
	m.SavedSearchID = types.StringValue(al.SavedSearchID)
	m.GroupBy = types.StringPointerValue(al.GroupBy)
	// Responses carry both `channel` and `channels`. Mirror back only the field
	// the config used, leaving the other null — writing both would show a
	// permanent diff against a config that sets one of them.
	chans := al.Channels
	if len(chans) == 0 {
		chans = []client.AlertChannel{al.Channel}
	}
	if !m.Channel.IsNull() {
		obj, d := types.ObjectValueFrom(ctx, alertChannelAttrTypes, alertChannelFromClient(chans[0]))
		diags.Append(d...)
		m.Channel, m.Channels = obj, types.ListNull(alertChannelObjectType)
	} else {
		models := make([]alertChannelModel, 0, len(chans))
		for _, c := range chans {
			models = append(models, alertChannelFromClient(c))
		}
		lst, d := types.ListValueFrom(ctx, alertChannelObjectType, models)
		diags.Append(d...)
		m.Channel, m.Channels = types.ObjectNull(alertChannelAttrTypes), lst
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
	return diags
}

func alertChannelFromClient(c client.AlertChannel) alertChannelModel {
	return alertChannelModel{Type: types.StringValue(c.Type), WebhookID: emptyToNull(c.WebhookID)}
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
