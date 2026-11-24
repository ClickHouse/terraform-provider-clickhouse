package resource

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

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

	var removed []string
	for _, name := range sortedKeys(previous) {
		if _, ok := desired[name]; !ok {
			removed = append(removed, name)
		}
	}

	if !r.writeSettings(ctx, settingsWrite{serviceID: serviceID, set: changed, reset: removed}, &resp.Diagnostics) {
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

	r.writeSettings(ctx, settingsWrite{
		serviceID:        state.ServiceID.ValueString(),
		reset:            sortedKeys(managed),
		serviceMayBeGone: true,
	}, &resp.Diagnostics)
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

// applySettings PATCHes the settings and shows the server's warnings. It
// returns false when it added an error.
func (r *ServiceClickhouseSettingsResource) applySettings(ctx context.Context, serviceID string, settings map[string]string, diags *diag.Diagnostics) bool {
	result, err := r.client.UpdateServiceClickhouseSettings(ctx, serviceID, clickhouseSettingsPayload(settings), nil)
	if err != nil {
		diags.AddError("Error updating ClickHouse settings", err.Error())
		return false
	}

	addClickhouseSettingWarnings(result.Warnings, diags)
	return true
}

// Substring of the API's 400 "request body property can't be validated: resetSettings", sent by an API that
// predates resetSettings; it rejects the whole PATCH, so nothing in it was applied.
const resetSettingsUnsupported = "property can't be validated: resetSettings"

// Substring of the API's 400 when a setting name isn't configurable; the API then rejects the whole PATCH.
const unknownSetting = "Unknown setting"

type settingsWrite struct {
	serviceID string
	set       map[string]string
	reset     []string
	// True on destroy, where a 404 means the service is already gone.
	serviceMayBeGone bool
}

// writeSettings sends changes and resets in one PATCH, so the service restarts
// at most once. It falls back to a settings PATCH and one DELETE per reset when
// the API doesn't support resetSettings, or rejects an unknown setting after the
// schema read failed. It returns false when it added an error.
func (r *ServiceClickhouseSettingsResource) writeSettings(ctx context.Context, w settingsWrite, diags *diag.Diagnostics) bool {
	reset := w.reset
	resetNamesUnfiltered := false
	if len(reset) > 0 {
		// The API rejects the whole PATCH for a name it no longer accepts, so drop retired names first.
		entries, err := r.client.GetServiceClickhouseSettingsSchema(ctx, w.serviceID)
		switch {
		case err == nil:
			reset = knownClickhouseSettings(reset, entries)
			if len(reset) < len(w.reset) {
				tflog.Debug(ctx, "skipping ClickHouse settings the service no longer offers", map[string]any{"requested": w.reset, "sent": reset})
			}
		case api.IsNotFound(err) && w.serviceMayBeGone:
			return true
		case api.IsNotFound(err):
			diags.AddError("Error reading ClickHouse settings schema", err.Error())
			return false
		default:
			tflog.Warn(ctx, "could not read the ClickHouse settings schema; sending every reset name", map[string]any{"error": err.Error()})
			resetNamesUnfiltered = true
		}
	}

	if len(w.set) == 0 && len(reset) == 0 {
		return true
	}

	result, err := r.client.UpdateServiceClickhouseSettings(ctx, w.serviceID, clickhouseSettingsPayload(w.set), reset)
	switch {
	case err == nil:
		addClickhouseSettingWarnings(result.Warnings, diags)
		return true
	case api.IsNotFound(err) && w.serviceMayBeGone:
		return true
	// When the schema read failed, a retired name may still be in the PATCH; per-name DELETE treats its 404 as already reset.
	// An unknown name in settings also matches, and then fails again in applySettings.
	case api.IsBadRequestWith(err, resetSettingsUnsupported),
		resetNamesUnfiltered && api.IsBadRequestWith(err, unknownSetting):
		tflog.Warn(ctx, "API rejected the batched reset; resetting ClickHouse settings one request at a time", map[string]any{"error": err.Error(), "names": reset})
		if len(w.set) > 0 && !r.applySettings(ctx, w.serviceID, w.set, diags) {
			return false
		}
		return r.resetSettingsOneByOne(ctx, w.serviceID, reset, diags)
	default:
		diags.AddError("Error updating ClickHouse settings", err.Error())
		return false
	}
}

func addClickhouseSettingWarnings(warnings []api.ServiceClickhouseSettingWarning, diags *diag.Diagnostics) {
	for _, w := range warnings {
		diags.AddWarning(fmt.Sprintf("ClickHouse setting %q", w.Name), w.Message)
	}
}

// knownClickhouseSettings keeps the names the schema still lists, in order.
func knownClickhouseSettings(names []string, entries []api.ServiceClickhouseSettingSchemaEntry) []string {
	known := make(map[string]bool, len(entries))
	for _, entry := range entries {
		known[entry.Name] = true
	}

	var result []string
	for _, name := range names {
		if known[name] {
			result = append(result, name)
		}
	}
	return result
}

// resetSettingsOneByOne reverts settings with one DELETE each, for an API
// without resetSettings. A 404 means the service is gone or the API no longer
// knows the setting; either way there is nothing to reset for that name.
func (r *ServiceClickhouseSettingsResource) resetSettingsOneByOne(ctx context.Context, serviceID string, names []string, diags *diag.Diagnostics) bool {
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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
