package resource

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
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

const serviceClickhouseSettingsResourceName = "clickhouse_service_clickhouse_settings"

var (
	_ resource.Resource                     = &ServiceClickhouseSettingsResource{}
	_ resource.ResourceWithConfigure        = &ServiceClickhouseSettingsResource{}
	_ resource.ResourceWithImportState      = &ServiceClickhouseSettingsResource{}
	_ resource.ResourceWithConfigValidators = &ServiceClickhouseSettingsResource{}
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
				Description: "ClickHouse settings to configure, keyed by setting name. Values are strings; integer settings are sent as integers. Only the settings listed here are managed: removing one resets it to the platform default. Settings that have a dedicated attribute on this resource cannot be set here.",
				Optional:    true,
				ElementType: types.StringType,
				Validators: []validator.Map{
					mapvalidator.SizeAtLeast(1),
					mapvalidator.KeysAre(stringvalidator.NoneOf(typedClickhouseSettingNames()...)),
				},
			},
			"merges_enabled": schema.BoolAttribute{
				Description: "Whether background merges and mutations are assigned on this service. Set to false to stop them, for example on a compute group that should only serve reads. Maps to the `" + settingDisableMergesAndMutations + "` setting. Changing it triggers a rolling restart of the service.",
				Optional:    true,
			},
		},
	}
}

func (r *ServiceClickhouseSettingsResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	paths := []path.Expression{path.MatchRoot("settings")}
	for _, t := range typedClickhouseSettings {
		paths = append(paths, path.MatchRoot(t.attribute))
	}
	return []resource.ConfigValidator{
		resourcevalidator.AtLeastOneOf(paths...),
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

	desired, diags := desiredClickhouseSettings(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()
	if !r.applySettings(ctx, serviceID, desired, &resp.Diagnostics) {
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

	current, err := r.client.ListServiceClickhouseSettings(ctx, state.ServiceID.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading ClickHouse settings", err.Error())
		return
	}

	if !state.Settings.IsNull() {
		managed := map[string]string{}
		resp.Diagnostics.Append(state.Settings.ElementsAs(ctx, &managed, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		settings, diags := types.MapValueFrom(ctx, types.StringType, managedClickhouseSettings(managed, current))
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Settings = settings
	}

	for _, t := range typedClickhouseSettings {
		if _, managed := t.toValue(&state); managed {
			value, ok := current[t.setting]
			t.fromValue(&state, value, ok)
		}
	}

	state.ID = state.ServiceID
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

	desired, diags := desiredClickhouseSettings(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	previous, diags := desiredClickhouseSettings(ctx, &state)
	resp.Diagnostics.Append(diags...)
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

	for _, name := range sortedKeys(previous) {
		if _, ok := desired[name]; ok {
			continue
		}
		if err := r.client.DeleteServiceClickhouseSetting(ctx, serviceID, name); err != nil {
			resp.Diagnostics.AddError(fmt.Sprintf("Error resetting ClickHouse setting %q", name), err.Error())
			return
		}
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

	managed, diags := desiredClickhouseSettings(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := state.ServiceID.ValueString()
	for _, name := range sortedKeys(managed) {
		err := r.client.DeleteServiceClickhouseSetting(ctx, serviceID, name)
		if err != nil {
			if api.IsNotFound(err) {
				return
			}
			resp.Diagnostics.AddError(fmt.Sprintf("Error resetting ClickHouse setting %q", name), err.Error())
			return
		}
	}
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

	state := models.ServiceClickhouseSettingsResourceModel{
		ID:        types.StringValue(req.ID),
		ServiceID: types.StringValue(req.ID),
		Settings:  types.MapNull(types.StringType),
	}

	untyped := map[string]string{}
	for name, value := range current {
		untyped[name] = value
	}
	for _, t := range typedClickhouseSettings {
		value, ok := current[t.setting]
		t.fromValue(&state, value, ok)
		delete(untyped, t.setting)
	}

	if len(untyped) > 0 {
		settings, diags := types.MapValueFrom(ctx, types.StringType, untyped)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.Settings = settings
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// applySettings PATCHes the given settings and surfaces the server's
// disruption warnings. It returns false when an error diagnostic was added.
func (r *ServiceClickhouseSettingsResource) applySettings(ctx context.Context, serviceID string, settings map[string]string, diags *diag.Diagnostics) bool {
	settingsSchema, err := r.client.GetServiceClickhouseSettingsSchema(ctx, serviceID)
	if err != nil {
		if api.IsNotFound(err) {
			diags.AddError(
				"Service not found",
				fmt.Sprintf("Service %s does not exist or is not visible to the caller. Confirm clickhouse_service.<name>.id is correct and the API key has access.", serviceID),
			)
			return false
		}
		diags.AddError("Error reading ClickHouse settings schema", err.Error())
		return false
	}

	result, err := r.client.UpdateServiceClickhouseSettings(ctx, serviceID, clickhouseSettingsPayload(settings, settingsSchema))
	if err != nil {
		diags.AddError("Error updating ClickHouse settings", err.Error())
		return false
	}

	for _, w := range result.Warnings {
		diags.AddWarning(fmt.Sprintf("ClickHouse setting %q", w.Name), w.Message)
	}

	return true
}

// clickhouseSettingsPayload converts Terraform's string values to the native
// JSON type the API expects: settings the schema types as "string" stay
// strings, anything else that parses as an integer is sent as one.
func clickhouseSettingsPayload(settings map[string]string, settingsSchema []api.ServiceClickhouseSettingSchemaEntry) map[string]any {
	settingTypes := make(map[string]string, len(settingsSchema))
	for _, entry := range settingsSchema {
		settingTypes[entry.Name] = entry.Type
	}

	payload := make(map[string]any, len(settings))
	for name, value := range settings {
		if settingTypes[name] == "string" {
			payload[name] = value
			continue
		}
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			payload[name] = n
			continue
		}
		payload[name] = value
	}

	return payload
}

const settingDisableMergesAndMutations = "shared_merge_tree_disable_merges_and_mutations_assignment"

// typedClickhouseSetting maps a dedicated resource attribute to the
// ClickHouse setting it controls.
type typedClickhouseSetting struct {
	attribute string
	setting   string
	// toValue returns the setting value for the model, and false when the
	// attribute is null and the setting is not managed.
	toValue func(m *models.ServiceClickhouseSettingsResourceModel) (string, bool)
	// fromValue sets the attribute from the service's value, or to null when
	// the setting is not configured on the service.
	fromValue func(m *models.ServiceClickhouseSettingsResourceModel, value string, ok bool)
}

var typedClickhouseSettings = []typedClickhouseSetting{
	{
		attribute: "merges_enabled",
		setting:   settingDisableMergesAndMutations,
		toValue: func(m *models.ServiceClickhouseSettingsResourceModel) (string, bool) {
			if m.MergesEnabled.IsNull() || m.MergesEnabled.IsUnknown() {
				return "", false
			}
			if m.MergesEnabled.ValueBool() {
				return "0", true
			}
			return "1", true
		},
		fromValue: func(m *models.ServiceClickhouseSettingsResourceModel, value string, ok bool) {
			if !ok {
				m.MergesEnabled = types.BoolNull()
				return
			}
			disabled, err := strconv.ParseBool(value)
			m.MergesEnabled = types.BoolValue(err != nil || !disabled)
		},
	},
}

func typedClickhouseSettingNames() []string {
	names := make([]string, 0, len(typedClickhouseSettings))
	for _, t := range typedClickhouseSettings {
		names = append(names, t.setting)
	}
	return names
}

// desiredClickhouseSettings merges the settings map with the dedicated
// attributes into the full set of settings the model manages.
func desiredClickhouseSettings(ctx context.Context, m *models.ServiceClickhouseSettingsResourceModel) (map[string]string, diag.Diagnostics) {
	settings := map[string]string{}
	var diags diag.Diagnostics
	if !m.Settings.IsNull() && !m.Settings.IsUnknown() {
		diags.Append(m.Settings.ElementsAs(ctx, &settings, false)...)
	}
	for _, t := range typedClickhouseSettings {
		if value, ok := t.toValue(m); ok {
			settings[t.setting] = value
		}
	}
	return settings, diags
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
