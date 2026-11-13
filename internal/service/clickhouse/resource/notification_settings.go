package resource

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"maps"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &NotificationSettingsResource{}
	_ resource.ResourceWithConfigure   = &NotificationSettingsResource{}
	_ resource.ResourceWithImportState = &NotificationSettingsResource{}
)

//go:embed descriptions/notification_settings.md
var notificationSettingsResourceDescription string

const notificationSettingsResourceName = "clickhouse_notification_settings"

func NewNotificationSettingsResource() resource.Resource {
	return &NotificationSettingsResource{}
}

type NotificationSettingsResource struct {
	client api.Client
}

type NotificationSettingsModel struct {
	ID       types.String `tfsdk:"id"`
	UserID   types.String `tfsdk:"user_id"`
	Channels types.Set    `tfsdk:"channels"`
	Rules    types.Set    `tfsdk:"rules"`
}

type NotificationSettingsRuleModel struct {
	Effect        types.String `tfsdk:"effect"`
	Notifications types.Set    `tfsdk:"notifications"`
	Resources     types.Set    `tfsdk:"resources"`
}

var notificationSettingsRuleAttrTypes = map[string]attr.Type{
	"effect":        types.StringType,
	"notifications": types.SetType{ElemType: types.StringType},
	"resources":     types.SetType{ElemType: types.StringType},
}

func notificationSettingsRuleObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: notificationSettingsRuleAttrTypes}
}

func (r *NotificationSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_settings"
}

func (r *NotificationSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: notificationSettingsResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Same as user_id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.StringAttribute{
				Description: "ID of the organization member whose settings these are. Use the clickhouse_user data source to look one up by email.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"channels": schema.SetAttribute{
				Description: "Delivery channels every rule in this resource applies to, such as 'ui' or 'email'. Slack is configured for the whole organization and is never a member setting.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
			},
			"rules": schema.SetNestedAttribute{
				Description: "Rules to apply. Each one applies its effect to every combination of the notification types and resources it names, on every channel in channels.",
				Required:    true,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"effect": schema.StringAttribute{
							Description: "What the rule does to the settings it names. 'enabled' and 'disabled' store an explicit choice; 'inherit' removes the setting, so the notification follows the platform default for the member's role again, whoever set it.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf(api.NotificationRuleEffects...),
							},
						},
						"notifications": schema.SetAttribute{
							Description: "Notification types the rule applies to. Use the clickhouse_notification_types data source to discover them; new types are added over time.",
							Required:    true,
							ElementType: types.StringType,
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
							},
						},
						"resources": schema.SetAttribute{
							Description: "IDs of the resources the rule applies to, of the kind each notification type reports as its resource_type. There are no wildcards: name every ID, and a rule covers no resource created afterwards. Notification types about the organization itself, such as billing, take the organization ID.",
							Required:    true,
							ElementType: types.StringType,
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
							},
						},
					},
				},
			},
		},
	}
}

func (r *NotificationSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError("ClickHouse Cloud API not configured",
			"This resource requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).")
		return
	}
	r.client = providerData.API
}

func (r *NotificationSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning(notificationSettingsResourceName, &resp.Diagnostics)

	var plan NotificationSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rules, channels, d := readNotificationSettingsModel(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.applyNotificationSettings(ctx, applyNotificationSettingsArgs{
		UserID:          plan.UserID.ValueString(),
		Desired:         rules,
		DesiredChannels: channels,
	}, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = types.StringValue(plan.UserID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *NotificationSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state NotificationSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// An imported resource declares nothing yet, so there is nothing to check it
	// against; the first plan after the import applies what the configuration says.
	if state.Rules.IsNull() || state.Channels.IsNull() {
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	}

	rules, channels, d := readNotificationSettingsModel(ctx, state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	settings, err := r.client.ListMemberNotificationSettings(ctx, state.UserID.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading notification settings", "Could not read notification settings: "+err.Error())
		return
	}

	// Only the declared rules are reconciled: a rule the stored settings no longer
	// agree with is recorded at the effect they report, so the next plan re-applies
	// that rule and nothing else. Settings this resource never declared are left
	// alone.
	stored := storedNotificationSettings(settings)
	elements := state.Rules.Elements()
	reconciled := make([]attr.Value, 0, len(rules))
	for i, rule := range rules {
		if notificationRuleHolds(rule, channels, stored) {
			// Read back the stored element rather than rebuilding it, so a rule that
			// still holds keeps the exact value Terraform planned.
			reconciled = append(reconciled, elements[i])
			continue
		}
		// A drifted rule stays in state rather than being dropped, because state is
		// also what says which settings this resource wrote: a rule that vanished
		// here would leave its settings behind when the configuration stops
		// declaring it.
		element, d := notificationRuleWithEffect(elements[i], driftedNotificationEffect(rule, channels, stored))
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		reconciled = append(reconciled, element)
	}

	reconciledSet, d := types.SetValue(notificationSettingsRuleObjectType(), reconciled)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Rules = reconciledSet

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *NotificationSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning(notificationSettingsResourceName, &resp.Diagnostics)

	var plan NotificationSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state NotificationSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desired, desiredChannels, d := readNotificationSettingsModel(ctx, plan)
	resp.Diagnostics.Append(d...)
	prior, priorChannels, d := readNotificationSettingsModel(ctx, state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.applyNotificationSettings(ctx, applyNotificationSettingsArgs{
		UserID:          plan.UserID.ValueString(),
		Desired:         desired,
		DesiredChannels: desiredChannels,
		Prior:           prior,
		PriorChannels:   priorChannels,
	}, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ID = types.StringValue(plan.UserID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *NotificationSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state NotificationSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Rules.IsNull() || state.Channels.IsNull() {
		return
	}

	prior, priorChannels, d := readNotificationSettingsModel(ctx, state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Declaring nothing hands every setting this resource held back to the
	// platform default, rather than leaving the member opted out of what the
	// configuration merely stopped mentioning.
	r.applyNotificationSettings(ctx, applyNotificationSettingsArgs{
		UserID:              state.UserID.ValueString(),
		Prior:               prior,
		PriorChannels:       priorChannels,
		MissingMemberIsDone: true,
	}, &resp.Diagnostics)
}

func (r *NotificationSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning(notificationSettingsResourceName, &resp.Diagnostics)

	// The import ID is the member's ID. Which rules the configuration declares is
	// not something the stored settings can answer, so they arrive null and the
	// first apply after the import writes them; the write is idempotent, so it
	// changes nothing that already matches.
	resp.Diagnostics.Append(resp.State.Set(ctx, NotificationSettingsModel{
		ID:       types.StringValue(req.ID),
		UserID:   types.StringValue(req.ID),
		Channels: types.SetNull(types.StringType),
		Rules:    types.SetNull(notificationSettingsRuleObjectType()),
	})...)
}

// applyNotificationSettingsArgs describes one move from what the prior state
// declared to what the configuration declares, on each one's channels.
type applyNotificationSettingsArgs struct {
	UserID string
	// Desired and DesiredChannels are empty on a destroy, which declares nothing.
	Desired         []notificationRule
	DesiredChannels []string
	// Prior and PriorChannels are empty on a create, which has nothing to abandon.
	Prior         []notificationRule
	PriorChannels []string
	// MissingMemberIsDone treats a member who is gone as the settings already
	// being gone, which a destroy wants and a write does not.
	MissingMemberIsDone bool
}

// applyNotificationSettings sends the one request that carries out the move,
// reporting a request the API would reject as a configuration error rather than
// splitting it and giving up atomicity.
func (r *NotificationSettingsResource) applyNotificationSettings(ctx context.Context, args applyNotificationSettingsArgs, diags *diag.Diagnostics) {
	// A rule carrying no channel is read by the API as every channel a member can
	// hold, which would widen the write well past what the configuration names.
	if len(args.Desired) > 0 && len(args.DesiredChannels) == 0 {
		diags.AddError(
			"Notification rules with no channels",
			"The rules name no delivery channel, so the API would apply them to every channel. This is a bug in the provider.",
		)
		return
	}

	rules, err := notificationSettingsRequest(args.Desired, args.Prior, args.DesiredChannels, args.PriorChannels)
	if err != nil {
		summary := "Invalid notification settings request"
		var requestErr *notificationSettingsError
		if errors.As(err, &requestErr) {
			summary = requestErr.summary
		}
		diags.AddError(summary, err.Error())
		return
	}
	if len(rules) == 0 {
		return
	}

	if _, err := r.client.UpdateMemberNotificationSettings(ctx, args.UserID, rules); err != nil {
		if args.MissingMemberIsDone && api.IsNotFound(err) {
			return
		}
		diags.AddError("Error updating notification settings", "Could not update notification settings: "+err.Error())
	}
}

// notificationRuleWithEffect returns a rule element carrying a different effect,
// keeping the notifications and resources exactly as Terraform holds them.
func notificationRuleWithEffect(element attr.Value, effect string) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics

	object, ok := element.(types.Object)
	if !ok {
		diags.AddError(
			"Unexpected notification rule in state",
			fmt.Sprintf("A notification rule is held as %T rather than an object. This is a bug in the provider.", element),
		)
		return element, diags
	}

	attributes := maps.Clone(object.Attributes())
	attributes["effect"] = types.StringValue(effect)
	return types.ObjectValue(notificationSettingsRuleAttrTypes, attributes)
}

// readNotificationSettingsModel converts the Terraform model into the plain
// rules and channels the decision works on.
func readNotificationSettingsModel(ctx context.Context, model NotificationSettingsModel) ([]notificationRule, []string, diag.Diagnostics) {
	var diags diag.Diagnostics

	var channels []string
	if !model.Channels.IsNull() && !model.Channels.IsUnknown() {
		diags.Append(model.Channels.ElementsAs(ctx, &channels, false)...)
	}

	var ruleModels []NotificationSettingsRuleModel
	if !model.Rules.IsNull() && !model.Rules.IsUnknown() {
		diags.Append(model.Rules.ElementsAs(ctx, &ruleModels, false)...)
	}
	if diags.HasError() {
		return nil, nil, diags
	}

	rules := make([]notificationRule, 0, len(ruleModels))
	for _, ruleModel := range ruleModels {
		var notifications, resources []string
		diags.Append(ruleModel.Notifications.ElementsAs(ctx, &notifications, false)...)
		diags.Append(ruleModel.Resources.ElementsAs(ctx, &resources, false)...)
		if diags.HasError() {
			return nil, nil, diags
		}
		rules = append(rules, notificationRule{
			Effect:        ruleModel.Effect.ValueString(),
			Notifications: notifications,
			Resources:     resources,
		})
	}

	return rules, channels, diags
}
