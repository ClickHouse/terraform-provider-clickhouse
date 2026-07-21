package clickstack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	_ resource.Resource                   = (*dashboardResource)(nil)
	_ resource.ResourceWithConfigure      = (*dashboardResource)(nil)
	_ resource.ResourceWithImportState    = (*dashboardResource)(nil)
	_ resource.ResourceWithValidateConfig = (*dashboardResource)(nil)
)

// NewDashboardResource is a helper to register the resource with the provider.
func NewDashboardResource() resource.Resource {
	return &dashboardResource{}
}

// dashboardResource manages a ClickStack dashboard via its JSON body.
type dashboardResource struct {
	client *client.Client
}

// dashboardResourceModel maps the resource schema data.
type dashboardResourceModel struct {
	ID             types.String `tfsdk:"id"`
	Team           types.String `tfsdk:"team"`
	DashboardJSON  types.String `tfsdk:"dashboard_json"`
	NormalizedJSON types.String `tfsdk:"normalized_json"`
}

func (r *dashboardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_dashboard"
}

func (r *dashboardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ClickStack dashboard from a JSON document (the v2 API " +
			"dashboard body: name, tiles, tags, filters, savedQuery, containers). The JSON is " +
			"validated at plan time against the ClickStack API when the validate endpoint is " +
			"available. Export an existing dashboard with `GET /api/v2/dashboards/{id}` or " +
			"`terraform import`. PromQL tiles are not supported by the API and cannot be managed here. " +
			"The `dashboard_json` configuration is the sole source of truth: this resource does not " +
			"detect changes made to the dashboard outside Terraform (e.g. edits in the UI). Such " +
			"out-of-band changes are not reported as drift on `terraform plan`; they persist until the " +
			"`dashboard_json` value itself changes, at which point the entire dashboard is replaced and " +
			"any manual edits are overwritten. Manage a dashboard either entirely in Terraform or " +
			"entirely in the UI, not both.\n\n" +
			"Tile alerts (alerts bound to a dashboard tile) are not managed by this resource. On " +
			"update, Terraform carries each tile's server-assigned ID forward — matched by tile " +
			"`name` — so a UI-created tile alert survives an apply. Tiles with duplicate or blank " +
			"names, or renamed between applies, fall back to positional matching and may lose their " +
			"alert; pin an explicit `id` on such tiles if you manage tile alerts in the UI.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:      true,
				Description:   "Identifier of the dashboard.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			teamAttr: schema.StringAttribute{
				Optional:      true,
				Description:   "Team ID to manage this dashboard under (`x-hdx-team`). Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			dashboardJSONAttr: schema.StringAttribute{
				Required:    true,
				Description: "The dashboard body as a JSON string, in the v2 API format. Use `jsonencode(...)` or `file(...)`.",
				PlanModifiers: []planmodifier.String{
					dashboardJSONPlanModifier{},
				},
			},
			normalizedJSONAttr: schema.StringAttribute{
				Computed:    true,
				Description: "Server-canonical dashboard body returned by the API (defaults applied, server-assigned tile IDs).",
			},
		},
	}
}

func (r *dashboardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *dashboardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dashboardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, err := r.client.WithTeam(plan.Team.ValueString()).
		CreateDashboard(ctx, json.RawMessage(plan.DashboardJSON.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Dashboard", err.Error())
		return
	}

	if diags := plan.applyDashboardBody(body); diags.HasError() {
		// applyDashboardBody fails only when the POST-success body carries no
		// usable id, so there is no id to recover: the dashboard exists on the
		// server but cannot be tracked in state. Surface the raw body so the
		// operator can find and delete the now-unmanaged dashboard manually.
		resp.Diagnostics.Append(diags...)
		resp.Diagnostics.AddError("Orphaned Dashboard",
			"A dashboard was created but could not be recorded in Terraform state and is now unmanaged. "+
				"Delete it manually if it is not wanted. Server response: "+string(body))
		return
	}

	tflog.Trace(ctx, "created dashboard resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dashboardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dashboardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, err := r.client.WithTeam(state.Team.ValueString()).GetDashboard(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Dashboard", err.Error())
		return
	}

	resp.Diagnostics.Append(state.applyDashboardBody(body)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// On import, dashboard_json is null/unknown because no config value exists
	// yet. Populate it from the fetched body so the imported state is
	// re-appliable without an immediate diff.
	if state.DashboardJSON.IsNull() || state.DashboardJSON.IsUnknown() {
		state.DashboardJSON = types.StringValue(string(body))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dashboardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dashboardResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// The prior state holds the server-canonical body with its assigned tile IDs.
	var state dashboardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Prepare tile and filter IDs for the update. Tiles carry their
	// server-assigned id forward so UI-created tile alerts survive (the server
	// otherwise mints fresh ids and drops the alerts). Filters must carry an id
	// on update too — the Cloud API requires one on every filter (it rejects it
	// on create), so mergeFilterIDs carries existing ids forward and mints
	// placeholders for new ones. Each step is best effort: if it fails, that
	// step's ids are left as authored and only that transformation is skipped.
	body := json.RawMessage(plan.DashboardJSON.ValueString())
	if !state.NormalizedJSON.IsNull() && !state.NormalizedJSON.IsUnknown() {
		prior := json.RawMessage(state.NormalizedJSON.ValueString())
		if merged, err := mergeTileIDs(body, prior); err == nil {
			body = merged
		} else {
			tflog.Warn(ctx, "could not merge server tile IDs into dashboard update; sending authored tiles as-is: "+err.Error())
		}
		if merged, err := mergeFilterIDs(body, prior); err == nil {
			body = merged
		} else {
			tflog.Warn(ctx, "could not prepare filter IDs for dashboard update; sending authored filters as-is: "+err.Error())
		}
	}

	updated, err := r.client.WithTeam(plan.Team.ValueString()).
		UpdateDashboard(ctx, plan.ID.ValueString(), body)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			// Deleted out-of-band between plan and apply: drop it from state so the next
			// plan recreates it, rather than hard-erroring.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Updating Dashboard", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applyDashboardBody(updated)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Trace(ctx, "updated dashboard resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dashboardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dashboardResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.WithTeam(state.Team.ValueString()).DeleteDashboard(ctx, state.ID.ValueString()); err != nil {
		// A dashboard already deleted out-of-band is not an error.
		if errors.Is(err, client.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting Dashboard", err.Error())
	}
}

func (r *dashboardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Accept either "<id>" (default team) or "<team>/<id>" so dashboards in a
	// non-default team can be imported. The team is required by the API to
	// resolve the team-scoped dashboard ID during the import Read.
	team, id, err := parseDashboardImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", err.Error())
		return
	}
	if team != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team"), team)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// parseDashboardImportID splits an import ID of the form "<id>" or "<team>/<id>".
// Both parts must be non-empty; a returned empty team means the default team.
func parseDashboardImportID(raw string) (team, id string, err error) {
	if team, id, ok := strings.Cut(raw, "/"); ok {
		if team == "" || id == "" {
			return "", "", fmt.Errorf("expected \"<id>\" or \"<team>/<id>\" with both parts non-empty, got %q", raw)
		}
		return team, id, nil
	}
	if raw == "" {
		return "", "", fmt.Errorf("import ID must not be empty")
	}
	return "", raw, nil
}

// parseDashboardJSON checks that s is a JSON object (the dashboard body shape).
func parseDashboardJSON(s string) error {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return fmt.Errorf("dashboard_json must be a JSON object: %w", err)
	}
	if obj == nil {
		return fmt.Errorf("dashboard_json must be a JSON object, got null")
	}
	return nil
}

func (r *dashboardResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_clickstack_dashboard", &resp.Diagnostics)
	var cfg dashboardResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.DashboardJSON.IsNull() || cfg.DashboardJSON.IsUnknown() {
		return
	}
	body := cfg.DashboardJSON.ValueString()
	if err := parseDashboardJSON(body); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root(dashboardJSONAttr), "Invalid dashboard_json", err.Error())
		return
	}
	// r.client is nil during early validation (Configure runs later); only call
	// the API when the client is available.
	if r.client == nil {
		return
	}
	res, err := r.client.WithTeam(cfg.Team.ValueString()).ValidateDashboard(ctx, json.RawMessage(body))
	if err != nil {
		if errors.Is(err, client.ErrValidateUnsupported) {
			resp.Diagnostics.AddAttributeWarning(path.Root(dashboardJSONAttr),
				"Dashboard validation skipped",
				"The ClickStack API does not expose /api/v2/dashboards/validate; the dashboard will be validated on apply.")
			return
		}
		// Distinct from ErrValidateUnsupported (endpoint absent): the endpoint is
		// present but broken (5xx, transport failure, malformed response). Still a
		// warning so a transient outage does not block plan, but log the underlying
		// error so a persistent misconfiguration is diagnosable rather than looking
		// like graceful degradation.
		tflog.Warn(ctx, "dashboard validation endpoint returned an error; deferring validation to apply: "+err.Error())
		resp.Diagnostics.AddAttributeWarning(path.Root(dashboardJSONAttr),
			"Dashboard validation unavailable", "Could not validate dashboard_json: "+err.Error())
		return
	}
	if !res.Valid {
		for _, e := range res.Errors {
			detail := e.Message
			if e.Path != "" {
				detail = e.Path + ": " + e.Message
			}
			resp.Diagnostics.AddAttributeError(path.Root(dashboardJSONAttr), "Invalid dashboard configuration", detail)
		}
		if len(res.Errors) == 0 {
			resp.Diagnostics.AddAttributeError(path.Root(dashboardJSONAttr), "Invalid dashboard configuration",
				"the API reported the dashboard as invalid but returned no error details")
		}
	}
}

// applyDashboardBody records the server's returned dashboard body: it sets id
// and normalized_json but does NOT touch dashboard_json (the user's authored
// value is the source of truth for that attribute).
func (m *dashboardResourceModel) applyDashboardBody(body []byte) diag.Diagnostics {
	var diags diag.Diagnostics
	id, err := client.DashboardID(body)
	if err != nil {
		diags.AddError("Invalid Dashboard Response", err.Error())
		return diags
	}
	if id == "" {
		diags.AddError("Invalid Dashboard Response", "the API returned a dashboard body with no id; this is a provider or API bug")
		return diags
	}
	m.ID = types.StringValue(id)
	m.NormalizedJSON = types.StringValue(string(body))
	return diags
}
