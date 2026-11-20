package resource

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

const serviceClickhouseSettingsResourceName = "clickhouse_service_clickhouse_settings"

var (
	_ resource.Resource                = &ServiceClickhouseSettingsResource{}
	_ resource.ResourceWithConfigure   = &ServiceClickhouseSettingsResource{}
	_ resource.ResourceWithImportState = &ServiceClickhouseSettingsResource{}
	_ resource.ResourceWithModifyPlan  = &ServiceClickhouseSettingsResource{}
)

//go:embed descriptions/service_clickhouse_settings.md
var serviceClickhouseSettingsResourceDescription string

func NewServiceClickhouseSettingsResource() resource.Resource {
	return &ServiceClickhouseSettingsResource{}
}

type ServiceClickhouseSettingsResource struct {
	client api.Client
}

func (r *ServiceClickhouseSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_clickhouse_settings"
}

func (r *ServiceClickhouseSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: serviceClickhouseSettingsResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Resource identifier. Equal to service_id (one settings resource per service).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Cloud service ID these settings apply to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"settings": schema.MapAttribute{
				Description: "ClickHouse settings keyed by name, with values as strings. Only these settings are managed: removing one resets it to the platform default.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Map{
					mapvalidator.SizeAtLeast(1),
				},
			},
		},
	}
}

func (r *ServiceClickhouseSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceClickhouseSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning(serviceClickhouseSettingsResourceName, &resp.Diagnostics)

	var plan models.ServiceClickhouseSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desired := map[string]string{}
	resp.Diagnostics.Append(plan.Settings.ElementsAs(ctx, &desired, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.applySettings(ctx, plan.ServiceID.ValueString(), desired, &resp.Diagnostics) {
		return
	}

	plan.ID = plan.ServiceID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceClickhouseSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ServiceClickhouseSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managed := map[string]string{}
	resp.Diagnostics.Append(state.Settings.ElementsAs(ctx, &managed, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.ListServiceClickhouseSettings(ctx, state.ServiceID.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading ClickHouse settings", err.Error())
		return
	}

	settings, diags := types.MapValueFrom(ctx, types.StringType, managedClickhouseSettings(managed, current))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ID = state.ServiceID
	state.Settings = settings
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceClickhouseSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning(serviceClickhouseSettingsResourceName, &resp.Diagnostics)

	var plan, state models.ServiceClickhouseSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	desired := map[string]string{}
	previous := map[string]string{}
	resp.Diagnostics.Append(plan.Settings.ElementsAs(ctx, &desired, false)...)
	resp.Diagnostics.Append(state.Settings.ElementsAs(ctx, &previous, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()

	changed := map[string]string{}
	for name, value := range desired {
		if prev, ok := previous[name]; !ok || prev != value {
			changed[name] = value
		}
	}
	if len(changed) > 0 && !r.applySettings(ctx, serviceID, changed, &resp.Diagnostics) {
		return
	}

	var removed []string
	for _, name := range sortedKeys(previous) {
		if _, ok := desired[name]; !ok {
			removed = append(removed, name)
		}
	}
	if !r.resetSettings(ctx, serviceID, removed, &resp.Diagnostics) {
		return
	}

	plan.ID = plan.ServiceID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceClickhouseSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ServiceClickhouseSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managed := map[string]string{}
	resp.Diagnostics.Append(state.Settings.ElementsAs(ctx, &managed, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.resetSettings(ctx, state.ServiceID.ValueString(), sortedKeys(managed), &resp.Diagnostics)
}

// ImportState adopts every setting currently configured on the service.
func (r *ServiceClickhouseSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning(serviceClickhouseSettingsResourceName, &resp.Diagnostics)

	current, err := r.client.ListServiceClickhouseSettings(ctx, req.ID)
	if err != nil {
		if api.IsNotFound(err) {
			resp.Diagnostics.AddError(
				"Service not found",
				fmt.Sprintf("Service %s does not exist or is not visible to the caller. Confirm the service ID is correct and the API key has access.", req.ID),
			)
			return
		}
		resp.Diagnostics.AddError("Error reading ClickHouse settings for import", err.Error())
		return
	}

	settings, diags := types.MapValueFrom(ctx, types.StringType, current)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("settings"), settings)...)
}

// ModifyPlan warns before apply when the plan changes or resets a setting
// whose schema warning says it restarts the service.
func (r *ServiceClickhouseSettingsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.client == nil {
		return
	}

	var plan, state *models.ServiceClickhouseSettingsResourceModel
	if !req.Plan.Raw.IsNull() {
		plan = &models.ServiceClickhouseSettingsResourceModel{}
		resp.Diagnostics.Append(req.Plan.Get(ctx, plan)...)
	}
	if !req.State.Raw.IsNull() {
		state = &models.ServiceClickhouseSettingsResourceModel{}
		resp.Diagnostics.Append(req.State.Get(ctx, state)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	touched, diags := touchedClickhouseSettings(ctx, plan, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Warnings follow service ID order so the plan output is stable.
	for _, serviceID := range sortedKeys(touched) {
		r.warnRestartingSettings(ctx, serviceID, touched[serviceID], &resp.Diagnostics)
	}
}

// plannedSettingsChange is what a plan does to one service's settings.
type plannedSettingsChange struct {
	// names lists the settings the plan writes or resets on this service.
	names []string
	// unknownValues holds the names whose new value is not known until apply.
	unknownValues map[string]bool
	// teardown means the settings are reset because the resource is destroyed or moves to another service.
	teardown bool
	// unknownSettings means the whole new settings map is not known until apply.
	unknownSettings bool
}

// touchedClickhouseSettings returns, per service ID, what the plan does to its
// settings. The schema lookup is skipped for a service ID that is not known
// yet, so no plan warning can be given for it.
func touchedClickhouseSettings(ctx context.Context, plan, state *models.ServiceClickhouseSettingsResourceModel) (map[string]*plannedSettingsChange, diag.Diagnostics) {
	var diags diag.Diagnostics
	touched := map[string]*plannedSettingsChange{}
	add := func(serviceID types.String, change plannedSettingsChange) {
		if serviceID.IsUnknown() || serviceID.IsNull() || (len(change.names) == 0 && !change.unknownSettings) {
			return
		}
		touched[serviceID.ValueString()] = &change
	}

	desired := map[string]types.String{}
	desiredKnown := false
	if plan != nil && !plan.Settings.IsUnknown() && !plan.Settings.IsNull() {
		diags.Append(plan.Settings.ElementsAs(ctx, &desired, false)...)
		desiredKnown = true
	}
	previous := map[string]string{}
	if state != nil {
		diags.Append(state.Settings.ElementsAs(ctx, &previous, false)...)
	}
	if diags.HasError() {
		return nil, diags
	}

	// changedFrom lists the settings the plan adds or changes compared with
	// before, and records which of them have no known value until apply.
	changedFrom := func(before map[string]string) plannedSettingsChange {
		change := plannedSettingsChange{unknownValues: map[string]bool{}}
		for _, name := range sortedKeys(desired) {
			value := desired[name]
			if value.IsUnknown() {
				change.unknownValues[name] = true
			}
			if prev, ok := before[name]; !ok || value.IsUnknown() || prev != value.ValueString() {
				change.names = append(change.names, name)
			}
		}
		return change
	}

	switch {
	case plan == nil && state == nil:
		// Nothing planned and nothing in state.
	case plan == nil:
		// Destroy resets every managed setting.
		add(state.ServiceID, plannedSettingsChange{names: sortedKeys(previous), teardown: true})
	case state == nil || !plan.ServiceID.Equal(state.ServiceID):
		// Create, or a replace that resets the old service and sets every value on the new one.
		if state != nil {
			add(state.ServiceID, plannedSettingsChange{names: sortedKeys(previous), teardown: true})
		}
		if !desiredKnown {
			add(plan.ServiceID, plannedSettingsChange{unknownSettings: true})
			break
		}
		add(plan.ServiceID, changedFrom(nil))
	case !desiredKnown:
		add(plan.ServiceID, plannedSettingsChange{unknownSettings: true})
	default:
		change := changedFrom(previous)
		for _, name := range sortedKeys(previous) {
			if _, ok := desired[name]; !ok {
				change.names = append(change.names, name)
			}
		}
		add(plan.ServiceID, change)
	}

	return touched, diags
}

// rolloutRestartMarker identifies a restart setting in the schema's free-text
// warning. It matches control-plane's ROLLOUT_RESTART_NOTE in
// CustomerSettingsAllowlist.ts; the API has no structured restart flag.
const rolloutRestartMarker = "rollout restart"

// settingsSchemaTimeout bounds the plan-time schema lookup, which only adds a warning.
const settingsSchemaTimeout = 30 * time.Second

// warnRestartingSettings reads the service's settings schema and adds one
// warning listing the touched settings that restart the service. It never adds
// an error: a failed lookup must not block the plan.
func (r *ServiceClickhouseSettingsResource) warnRestartingSettings(ctx context.Context, serviceID string, change *plannedSettingsChange, diags *diag.Diagnostics) {
	const uncheckedSummary = "Could not check whether this change restarts the ClickHouse service"
	if change.unknownSettings {
		diags.AddWarning(uncheckedSummary,
			fmt.Sprintf("The settings for service %s are not known until apply, so this plan cannot say whether applying it restarts the service.", serviceID))
		return
	}

	lookupCtx, cancel := context.WithTimeout(ctx, settingsSchemaTimeout)
	defer cancel()
	entries, err := r.client.GetServiceClickhouseSettingsSchema(lookupCtx, serviceID)
	if err != nil && ctx.Err() != nil {
		// The plan itself is being cancelled or has run out of time.
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		diags.AddWarning(uncheckedSummary,
			fmt.Sprintf("The ClickHouse settings schema for service %s did not load within %s, so this plan cannot say whether applying it restarts the service.", serviceID, settingsSchemaTimeout))
		return
	}
	if err != nil {
		diags.AddWarning(uncheckedSummary,
			fmt.Sprintf("Reading the ClickHouse settings schema for service %s failed, so this plan cannot say whether applying it restarts the service: %s", serviceID, err.Error()))
		return
	}

	warnings := make(map[string]string, len(entries))
	for _, e := range entries {
		warnings[e.Name] = e.Warning
	}

	names := slices.Sorted(slices.Values(change.names))
	var lines []string
	// definite is true when at least one listed setting has a known new value, so the restart is certain.
	definite := false
	for _, name := range names {
		warning := warnings[name]
		if !strings.Contains(strings.ToLower(warning), rolloutRestartMarker) {
			continue
		}
		if change.unknownValues[name] {
			lines = append(lines, fmt.Sprintf("%q (new value not known until apply; restarts only if it changes): %s", name, warning))
			continue
		}
		definite = true
		lines = append(lines, fmt.Sprintf("%q: %s", name, warning))
	}
	if len(lines) == 0 {
		return
	}

	summary := "This change restarts the ClickHouse service"
	intro := fmt.Sprintf("Applying this plan restarts service %s.", serviceID)
	switch {
	case change.teardown:
		intro = fmt.Sprintf("Applying this plan resets these settings on service %s, which restarts it. If this apply also deletes the service, the restart does not matter.", serviceID)
	case !definite:
		summary = "This change may restart the ClickHouse service"
		intro = fmt.Sprintf("Applying this plan may restart service %s.", serviceID)
	}
	diags.AddWarning(summary, intro+"\n\n"+strings.Join(lines, "\n"))
}

// applySettings PATCHes the settings and shows the server's warnings. It
// returns false when it added an error.
func (r *ServiceClickhouseSettingsResource) applySettings(ctx context.Context, serviceID string, settings map[string]string, diags *diag.Diagnostics) bool {
	result, err := r.client.UpdateServiceClickhouseSettings(ctx, serviceID, clickhouseSettingsPayload(settings))
	if err != nil {
		diags.AddError("Error updating ClickHouse settings", err.Error())
		return false
	}

	for _, w := range result.Warnings {
		diags.AddWarning(fmt.Sprintf("ClickHouse setting %q", w.Name), w.Message)
	}

	return true
}

// resetSettings reverts settings to their platform default. A 404 means the
// service is gone or the API no longer knows the setting; either way there is
// nothing to reset for that name.
func (r *ServiceClickhouseSettingsResource) resetSettings(ctx context.Context, serviceID string, names []string, diags *diag.Diagnostics) bool {
	for _, name := range names {
		err := r.client.DeleteServiceClickhouseSetting(ctx, serviceID, name)
		if api.IsNotFound(err) {
			continue
		}
		if err != nil {
			diags.AddError(fmt.Sprintf("Error resetting ClickHouse setting %q", name), err.Error())
			return false
		}
	}
	return true
}

// clickhouseSettingsPayload sends values that parse as integers as JSON
// integers, since the API takes each setting in its native type.
func clickhouseSettingsPayload(settings map[string]string) map[string]any {
	payload := make(map[string]any, len(settings))
	for name, value := range settings {
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			payload[name] = n
			continue
		}
		payload[name] = value
	}
	return payload
}

// managedClickhouseSettings narrows the server's settings to the keys this
// resource manages. A managed key the server no longer has is dropped, so the
// next plan proposes setting it again.
func managedClickhouseSettings(managed, current map[string]string) map[string]string {
	result := make(map[string]string, len(managed))
	for name := range managed {
		if value, ok := current[name]; ok {
			result[name] = value
		}
	}
	return result
}

func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
