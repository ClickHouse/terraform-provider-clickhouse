package resource

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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

var (
	_ resource.Resource                = &ServiceClickHouseSettingsResource{}
	_ resource.ResourceWithConfigure   = &ServiceClickHouseSettingsResource{}
	_ resource.ResourceWithModifyPlan  = &ServiceClickHouseSettingsResource{}
	_ resource.ResourceWithImportState = &ServiceClickHouseSettingsResource{}
)

//go:embed descriptions/service_clickhouse_settings.md
var serviceClickHouseSettingsResourceDescription string

// clickHouseSettingsForbiddenDetail explains a 403 from the settings API. While
// the API is in beta it is enabled per organization, so a 403 can mean it is
// not enabled yet, not only that the key lacks permission.
const clickHouseSettingsForbiddenDetail = "The ClickHouse settings API is in beta and is enabled per organization, so either it is not enabled for your organization yet or the API key cannot view and manage the service (the permissions the API names for a service's other operations are `control-plane:service:view` and `control-plane:service:manage`). Contact ClickHouse support to enable it for your organization."

func NewServiceClickHouseSettingsResource() resource.Resource {
	return &ServiceClickHouseSettingsResource{}
}

type ServiceClickHouseSettingsResource struct {
	client api.Client
}

func (r *ServiceClickHouseSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_clickhouse_settings"
}

func (r *ServiceClickHouseSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: serviceClickHouseSettingsResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Resource identifier. Equal to service_id (one settings resource per service).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Cloud service ID whose ClickHouse settings this resource manages.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(uuidPattern, "must be a UUID"),
				},
			},
			"settings": schema.MapAttribute{
				Description: "ClickHouse settings to set on the service, keyed by setting name. Values are strings and are sent with the type the service's settings schema gives them. Only the settings named here are managed; removing one resets it to its platform default.",
				ElementType: types.StringType,
				Required:    true,
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.LengthAtLeast(1)),
					mapvalidator.NoNullValues(),
				},
			},
		},
	}
}

func (r *ServiceClickHouseSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan warns before an apply resets any setting: one removed from
// settings, every one on destroy, and every one on the old service when
// service_id changes. On the same service only keys known to be gone count: an
// unknown settings map is silent, and an unknown element counts as kept while
// any key removed beside it still warns.
func (r *ServiceClickHouseSettingsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() {
		return
	}

	var state models.ServiceClickHouseSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	managed := slices.Sorted(maps.Keys(state.Settings.Elements()))

	reset := managed
	if !req.Plan.Raw.IsNull() {
		var plan models.ServiceClickHouseSettingsResourceModel
		resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
		if resp.Diagnostics.HasError() {
			return
		}

		if plan.ServiceID.Equal(state.ServiceID) {
			if plan.Settings.IsUnknown() || plan.Settings.IsNull() {
				return
			}
			planned := plan.Settings.Elements()
			reset = slices.DeleteFunc(slices.Clone(managed), func(name string) bool {
				_, kept := planned[name]
				return kept
			})
		}
	}
	if len(reset) == 0 {
		return
	}

	detail := fmt.Sprintf("Applying this plan resets these ClickHouse settings on service %s to their platform defaults: %s.", state.ServiceID.ValueString(), strings.Join(reset, ", "))
	if slices.Contains(reset, "compatibility") {
		detail += "\n\nClickHouse Cloud sets compatibility when it creates a service. Resetting it changes which ClickHouse version's setting defaults the service's queries follow."
	}
	resp.Diagnostics.AddWarning("ClickHouse settings will be reset", detail)
}

func (r *ServiceClickHouseSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning("clickhouse_service_clickhouse_settings", &resp.Diagnostics)

	var plan models.ServiceClickHouseSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()
	settings := clickHouseSettingsMap(ctx, plan.Settings, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.writeClickHouseSettings(ctx, serviceID, settings, &resp.Diagnostics) {
		return
	}

	plan.ID = plan.ServiceID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if !resp.Diagnostics.HasError() {
		r.checkClickHouseSettings(ctx, serviceID, settings, &resp.Diagnostics)
	}
}

func (r *ServiceClickHouseSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ServiceClickHouseSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := state.ServiceID.ValueString()
	managed := clickHouseSettingsMap(ctx, state.Settings, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.ListClickHouseSettings(ctx, serviceID)
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addClickHouseSettingsErrorDiagnostic(&resp.Diagnostics, "Error reading ClickHouse settings", serviceID, err)
		return
	}

	settings, diags := types.MapValueFrom(ctx, types.StringType, managedClickHouseSettings(managed, current))
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state.ID = state.ServiceID
	state.Settings = settings
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceClickHouseSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning("clickhouse_service_clickhouse_settings", &resp.Diagnostics)

	var plan, state models.ServiceClickHouseSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()
	planned := clickHouseSettingsMap(ctx, plan.Settings, &resp.Diagnostics)
	prior := clickHouseSettingsMap(ctx, state.Settings, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	changed := map[string]string{}
	for name, value := range planned {
		if old, ok := prior[name]; !ok || old != value {
			changed[name] = value
		}
	}
	var removed []string
	for _, name := range slices.Sorted(maps.Keys(prior)) {
		if _, ok := planned[name]; !ok {
			removed = append(removed, name)
		}
	}

	if !r.writeClickHouseSettings(ctx, serviceID, changed, &resp.Diagnostics) {
		return
	}

	// Settings the reset loop did not get to are still set on the service, so
	// they stay in state for the next apply to retry.
	for _, name := range r.resetClickHouseSettings(ctx, serviceID, removed, &resp.Diagnostics) {
		planned[name] = prior[name]
	}
	settings, diags := types.MapValueFrom(ctx, types.StringType, planned)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}

	plan.ID = plan.ServiceID
	plan.Settings = settings
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if !resp.Diagnostics.HasError() {
		r.checkClickHouseSettings(ctx, serviceID, changed, &resp.Diagnostics)
	}
}

func (r *ServiceClickHouseSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ServiceClickHouseSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managed := clickHouseSettingsMap(ctx, state.Settings, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	remaining := r.resetClickHouseSettings(ctx, state.ServiceID.ValueString(), slices.Sorted(maps.Keys(managed)), &resp.Diagnostics)
	if len(remaining) == 0 {
		return
	}
	kept := make(map[string]string, len(remaining))
	for _, name := range remaining {
		kept[name] = managed[name]
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("settings"), kept)...)
}

func (r *ServiceClickHouseSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning("clickhouse_service_clickhouse_settings", &resp.Diagnostics)

	// Adopt every setting the service has explicitly set, so the first plan
	// after import shows exactly what Cloud has set.
	current, err := r.client.ListClickHouseSettings(ctx, req.ID)
	if err != nil {
		addClickHouseSettingsErrorDiagnostic(&resp.Diagnostics, "Error importing ClickHouse settings", req.ID, err)
		return
	}

	adopted := make(map[string]string, len(current))
	for _, setting := range current {
		adopted[setting.Name] = setting.Value.Text
	}
	settings, diags := types.MapValueFrom(ctx, types.StringType, adopted)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &models.ServiceClickHouseSettingsResourceModel{
		ID:        types.StringValue(req.ID),
		ServiceID: types.StringValue(req.ID),
		Settings:  settings,
	})...)
}

// writeClickHouseSettings sends settings in one PATCH, each value typed as the
// service's settings schema says, and surfaces the warnings the API returns.
func (r *ServiceClickHouseSettingsResource) writeClickHouseSettings(ctx context.Context, serviceID string, settings map[string]string, diags *diag.Diagnostics) bool {
	if len(settings) == 0 {
		return true
	}

	settingsSchema, err := r.client.GetClickHouseSettingsSchema(ctx, serviceID)
	if err != nil {
		addClickHouseSettingsErrorDiagnostic(diags, "Error reading ClickHouse settings schema", serviceID, err)
		return false
	}

	values := typedClickHouseSettings(settingsSchema, settings, diags)
	if diags.HasError() {
		return false
	}

	result, err := r.client.UpdateClickHouseSettings(ctx, serviceID, api.ClickHouseSettingsUpdate{Settings: values})
	if api.IsClickHouseSettingsResponseUnreadable(err) {
		// The API accepted the write, so the settings are managed from here
		// on; only the warnings the response carried are lost.
		diags.AddWarning(
			"Unable to read the ClickHouse settings response",
			fmt.Sprintf("The settings were written to service %s, but the response could not be read, so any warnings it carried were lost: %s", serviceID, err),
		)
		return true
	}
	if err != nil {
		addClickHouseSettingsErrorDiagnostic(diags, "Error updating ClickHouse settings", serviceID, err)
		return false
	}

	for _, warning := range result.Warnings {
		if warning.Name == "" {
			diags.AddWarning("ClickHouse Cloud returned a warning", warning.Message)
			continue
		}
		diags.AddAttributeWarning(
			path.Root("settings").AtMapKey(warning.Name),
			fmt.Sprintf("Warning for ClickHouse setting %s", warning.Name),
			warning.Message,
		)
	}
	return true
}

// resetClickHouseSettings resets each named setting to its platform default,
// in order. A 404 leaves nothing to reset, whether the setting is unknown or
// the service is gone, so the loop moves on; any other failure stops it. It
// returns the settings not reset.
func (r *ServiceClickHouseSettingsResource) resetClickHouseSettings(ctx context.Context, serviceID string, names []string, diags *diag.Diagnostics) []string {
	for i, name := range names {
		err := r.client.ResetClickHouseSetting(ctx, serviceID, name)
		if err == nil || api.IsNotFound(err) {
			continue
		}

		detail := fmt.Sprintf("Could not reset ClickHouse setting %s on service %s: %s", name, serviceID, clickHouseSettingsErrorDetail(err))
		if rest := names[i+1:]; len(rest) > 0 {
			detail += fmt.Sprintf("\n\nThe remaining settings were not reset: %s.", strings.Join(rest, ", "))
		}
		diags.AddError("Error resetting ClickHouse setting", detail)
		return names[i:]
	}
	return nil
}

// checkClickHouseSettings reads the settings back after a write and warns about
// any the service does not report at the written value. State keeps the planned
// values regardless, as Terraform requires, and the next refresh shows the
// difference.
func (r *ServiceClickHouseSettingsResource) checkClickHouseSettings(ctx context.Context, serviceID string, written map[string]string, diags *diag.Diagnostics) {
	if len(written) == 0 {
		return
	}

	current, err := r.client.ListClickHouseSettings(ctx, serviceID)
	if err != nil {
		diags.AddWarning(
			"Unable to read back ClickHouse settings",
			fmt.Sprintf("The settings were written to service %s, but reading them back failed: %s", serviceID, clickHouseSettingsErrorDetail(err)),
		)
		return
	}

	reported := managedClickHouseSettings(written, current)
	for _, name := range slices.Sorted(maps.Keys(written)) {
		if value, ok := reported[name]; !ok || value != written[name] {
			diags.AddAttributeWarning(
				path.Root("settings").AtMapKey(name),
				"ClickHouse setting not reported as written",
				fmt.Sprintf("After the update, service %s does not report ClickHouse setting %s as %q. The next plan will show the difference.", serviceID, name, written[name]),
			)
		}
	}
}

func clickHouseSettingsMap(ctx context.Context, m types.Map, diags *diag.Diagnostics) map[string]string {
	settings := map[string]string{}
	diags.Append(m.ElementsAs(ctx, &settings, false)...)
	return settings
}

// typedClickHouseSettings gives each value the JSON type the settings schema
// lists for it: an integer setting is sent as a number and anything else as a
// string. A setting the schema does not list is sent as a string, and the API
// decides whether to accept it.
func typedClickHouseSettings(settingsSchema []api.ClickHouseSettingSchema, settings map[string]string, diags *diag.Diagnostics) map[string]api.ClickHouseSettingValue {
	settingTypes := make(map[string]string, len(settingsSchema))
	for _, entry := range settingsSchema {
		settingTypes[entry.Name] = entry.Type
	}

	values := make(map[string]api.ClickHouseSettingValue, len(settings))
	for _, name := range slices.Sorted(maps.Keys(settings)) {
		value := settings[name]
		if !strings.EqualFold(settingTypes[name], "integer") {
			values[name] = api.ClickHouseSettingValue{Text: value}
			continue
		}

		n, ok := new(big.Int).SetString(value, 10)
		if !ok {
			diags.AddAttributeError(
				path.Root("settings").AtMapKey(name),
				"Invalid ClickHouse setting value",
				fmt.Sprintf("The settings schema gives %s the type integer, but %q is not an integer.", name, value),
			)
			continue
		}
		values[name] = api.ClickHouseSettingValue{Text: n.String(), IsNumber: true}
	}
	return values
}

// managedClickHouseSettings narrows the settings set on the service to the
// managed names. A managed setting the service no longer has is left out, so
// the next plan sets it again. Where the service holds the managed value the
// managed spelling is kept, so an integer 3 never differs from a configured "3".
func managedClickHouseSettings(managed map[string]string, current []api.ClickHouseSetting) map[string]string {
	settings := make(map[string]string, len(managed))
	for _, setting := range current {
		want, ok := managed[setting.Name]
		if !ok {
			continue
		}
		if clickHouseSettingValueMatches(want, setting.Value) {
			settings[setting.Name] = want
		} else {
			settings[setting.Name] = setting.Value.Text
		}
	}
	return settings
}

// clickHouseSettingValueMatches reports whether the API's value equals want. An
// integer the API returns as a number matches any decimal spelling of it.
func clickHouseSettingValueMatches(want string, got api.ClickHouseSettingValue) bool {
	if want == got.Text {
		return true
	}
	if !got.IsNumber {
		return false
	}
	wantInt, ok := new(big.Int).SetString(want, 10)
	if !ok {
		return false
	}
	gotInt, ok := new(big.Int).SetString(got.Text, 10)
	return ok && wantInt.Cmp(gotInt) == 0
}

// addClickHouseSettingsErrorDiagnostic specializes the diagnostic for a missing
// service and for a 403. Anything else falls through to the raw error string.
func addClickHouseSettingsErrorDiagnostic(diags *diag.Diagnostics, summary, serviceID string, err error) {
	switch {
	case api.IsNotFound(err):
		diags.AddError(
			"Service not found",
			fmt.Sprintf("Service %s does not exist or is not visible to the caller. Confirm the service ID is correct and the API key has access.", serviceID),
		)
	case api.IsForbidden(err):
		diags.AddError("ClickHouse settings API not available", clickHouseSettingsErrorDetail(err))
	default:
		diags.AddError(summary, err.Error())
	}
}

func clickHouseSettingsErrorDetail(err error) string {
	if api.IsForbidden(err) {
		return clickHouseSettingsForbiddenDetail + "\n\n" + err.Error()
	}
	return err.Error()
}
