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
				Description: "ClickHouse settings to configure, keyed by setting name. Values are strings; integer settings are sent as integers. Only the settings listed here are managed: removing one resets it to the platform default.",
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

	managed := map[string]string{}
	resp.Diagnostics.Append(state.Settings.ElementsAs(ctx, &managed, false)...)
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

	settings, diags := types.MapValueFrom(ctx, types.StringType, current)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("settings"), settings)...)
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

// clickhouseSettingsPayload sends settings the schema types as "integer" as
// JSON integers, and everything else as strings.
func clickhouseSettingsPayload(settings map[string]string, settingsSchema []api.ServiceClickhouseSettingSchemaEntry) map[string]any {
	integers := map[string]bool{}
	for _, entry := range settingsSchema {
		integers[entry.Name] = entry.Type == "integer"
	}

	payload := make(map[string]any, len(settings))
	for name, value := range settings {
		payload[name] = value
		if !integers[name] {
			continue
		}
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			payload[name] = n
		}
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
