package resource

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
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
	_ resource.Resource                = &ServiceUpgradeWindowResource{}
	_ resource.ResourceWithConfigure   = &ServiceUpgradeWindowResource{}
	_ resource.ResourceWithImportState = &ServiceUpgradeWindowResource{}
)

//go:embed descriptions/service_upgrade_window.md
var serviceUpgradeWindowResourceDescription string

func NewServiceUpgradeWindowResource() resource.Resource {
	return &ServiceUpgradeWindowResource{}
}

type ServiceUpgradeWindowResource struct {
	client api.Client
}

func (r *ServiceUpgradeWindowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_upgrade_window"
}

func (r *ServiceUpgradeWindowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: serviceUpgradeWindowResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Resource identifier. Equal to service_id (one window per service).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Cloud service ID this upgrade window applies to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"weekday": schema.Int64Attribute{
				Description: "Day of the week the upgrade window starts. 0 = Sunday, 1 = Monday, …, 6 = Saturday.",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.Between(0, 6),
				},
			},
			"start_hour_utc": schema.Int64Attribute{
				Description: "UTC hour when the upgrade window starts. Must be one of 0, 6, 12, or 18.",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.OneOf(api.UpgradeWindowAllowedStartHoursUtc...),
				},
			},
			"duration": schema.Int64Attribute{
				Description: "Length of the upgrade window in hours. Server-controlled; currently always 6.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ServiceUpgradeWindowResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceUpgradeWindowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.ServiceUpgradeWindowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()

	// Refuse to clobber an existing window. The user should import it.
	existing, err := r.client.GetUpgradeWindow(ctx, serviceID)
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error checking for existing upgrade window", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Upgrade window already exists for this service",
			fmt.Sprintf("Service %s already has an upgrade window. Import it into Terraform with: terraform import clickhouse_service_upgrade_window.<name> %s", serviceID, serviceID),
		)
		return
	}

	window, err := r.client.UpdateUpgradeWindow(ctx, serviceID, api.UpgradeWindowUpdate{
		Weekday:      int(plan.Weekday.ValueInt64()),
		StartHourUtc: int(plan.StartHourUtc.ValueInt64()),
	})
	if err != nil {
		addUpgradeWindowWriteErrorDiagnostic(&resp.Diagnostics, "creating", serviceID, err)
		return
	}

	plan.ID = plan.ServiceID
	applyUpgradeWindowToState(window, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceUpgradeWindowResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_service_upgrade_window", &resp.Diagnostics)
}

func (r *ServiceUpgradeWindowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	utils.AlphaWarning("clickhouse_service_upgrade_window", &resp.Diagnostics)
	var state models.ServiceUpgradeWindowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := state.ServiceID.ValueString()

	window, err := r.client.GetUpgradeWindow(ctx, serviceID)
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading upgrade window", err.Error())
		return
	}

	state.ID = state.ServiceID
	applyUpgradeWindowToState(window, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceUpgradeWindowResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.ServiceUpgradeWindowResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	window, err := r.client.UpdateUpgradeWindow(ctx, plan.ServiceID.ValueString(), api.UpgradeWindowUpdate{
		Weekday:      int(plan.Weekday.ValueInt64()),
		StartHourUtc: int(plan.StartHourUtc.ValueInt64()),
	})
	if err != nil {
		addUpgradeWindowWriteErrorDiagnostic(&resp.Diagnostics, "updating", plan.ServiceID.ValueString(), err)
		return
	}

	plan.ID = plan.ServiceID
	applyUpgradeWindowToState(window, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceUpgradeWindowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ServiceUpgradeWindowResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteUpgradeWindow(ctx, state.ServiceID.ValueString())
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting upgrade window", err.Error())
	}
}

func (r *ServiceUpgradeWindowResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Validate the service exists and is a primary before writing state.
	// GET /upgradeWindow on a secondary returns the inherited primary's window
	// (so import would succeed at the upgrade-window layer), but every
	// subsequent PUT/DELETE would 400 on "secondary service" — wedging the
	// resource. Reject up front via GetService, which exposes isPrimary.
	service, err := r.client.GetService(ctx, req.ID)
	if err != nil {
		if api.IsNotFound(err) {
			resp.Diagnostics.AddError(
				"Service not found",
				fmt.Sprintf("Service %s does not exist or is not visible to the caller. Confirm the service ID is correct and the API key has access.", req.ID),
			)
			return
		}
		resp.Diagnostics.AddError("Error verifying service for import", err.Error())
		return
	}
	if service.IsPrimary != nil && !*service.IsPrimary {
		resp.Diagnostics.AddError(
			"Cannot import upgrade window on a secondary service",
			fmt.Sprintf("Service %s is a secondary service. Upgrade windows can only be managed on the primary service; secondary services inherit the primary's window.", req.ID),
		)
		return
	}

	// `id` and `service_id` are equal — write both so Read finds the service.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), req.ID)...)
}

// addUpgradeWindowWriteErrorDiagnostic specializes the diagnostic for the
// documented client errors of `PUT /upgradeWindow` (404 service-missing,
// 400 secondary-service, 403 entitlement). Anything else falls through to
// the raw error string, which is the same shape every other resource uses.
// `verb` is "creating" or "updating" so the operation reads naturally in
// the diagnostic summary.
func addUpgradeWindowWriteErrorDiagnostic(diags *diag.Diagnostics, verb, serviceID string, err error) {
	switch {
	case api.IsNotFound(err):
		diags.AddError(
			"Service not found",
			fmt.Sprintf("Service %s does not exist or is not visible to the caller. Confirm clickhouse_service.<name>.id is correct and the API key has access.", serviceID),
		)
	case api.IsBadRequestWith(err, "secondary service"):
		diags.AddError(
			"Upgrade windows can only be set on primary services",
			fmt.Sprintf("Service %s is a secondary service and inherits its upgrade window from the primary. Configure the upgrade window on the primary service instead.", serviceID),
		)
	case api.IsForbidden(err):
		diags.AddError(
			"Setting an upgrade window requires the scheduled upgrades entitlement",
			"The organization does not have the `canUseScheduledUpgrades` feature enabled, or the API key lacks `control-plane:service:manage`. Contact ClickHouse support to enable the entitlement.",
		)
	default:
		diags.AddError(fmt.Sprintf("Error %s upgrade window", verb), err.Error())
	}
}

// applyUpgradeWindowToState maps an API UpgradeWindow response into the
// Terraform state model.
func applyUpgradeWindowToState(window *api.UpgradeWindow, state *models.ServiceUpgradeWindowResourceModel) {
	state.Weekday = types.Int64Value(int64(window.Weekday))
	state.StartHourUtc = types.Int64Value(int64(window.StartHourUtc))
	state.Duration = types.Int64Value(int64(window.Duration))
}
