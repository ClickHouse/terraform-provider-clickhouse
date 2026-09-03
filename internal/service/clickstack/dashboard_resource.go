package clickstack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	TileIDs        types.Map    `tfsdk:"tile_ids"`
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
			"Tile alerts are managed with `clickhouse_clickstack_alert` (`source = \"tile\"`), which " +
			"references this dashboard's `id` and a tile `id`. Tile ids are assigned by the server and " +
			"cannot be set in `dashboard_json` (an authored `id` is ignored on create and replaced on " +
			"update unless the server already has it). Use the computed `tile_ids` map to reference a " +
			"tile by name from `clickhouse_clickstack_alert`, and keep alerted tiles' names unique and " +
			"stable: Terraform carries each tile's id forward by name across updates, so a tile that keeps " +
			"its unique name keeps its id wherever it moves in the array, while a rename mints a new id " +
			"and drops the tile's alert. Position only decides ids among blank- or duplicate-named tiles. " +
			"Importing a dashboard does not import its tile alerts; " +
			"import each alert separately.",
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
			tileIDsAttr: schema.MapAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Server-assigned tile ids keyed by tile name, for tiles whose name is non-empty and " +
					"unique within the dashboard. Reference these from `clickhouse_clickstack_alert` " +
					"(`source = \"tile\"`): `tile_id = clickhouse_clickstack_dashboard.x.tile_ids[\"<tile name>\"]`. " +
					"Tile ids cannot be chosen in `dashboard_json`; the server assigns them and keeps them across " +
					"updates for tiles that keep their unique name. Position only decides which id a " +
					"blank- or duplicate-named tile gets, and only among those tiles: a named tile's id is " +
					"never taken by position. A tile that keeps its unique name keeps its id at plan time; a " +
					"new or renamed tile's id is known only after apply, and a name that disappears leaves the " +
					"map, so an alert still referencing it fails at plan time with an invalid index.",
				PlanModifiers: []planmodifier.Map{tileIDsPlanModifier{}},
			},
		},
	}
}

// tileIDsPlanModifier plans tile_ids one element at a time instead of pinning
// the whole map to prior state. Pinning the map (UseStateForUnknown) plans a
// value that apply then contradicts as soon as a tile is added, removed or
// renamed — Terraform rejects that as an inconsistent result. Planning per
// element keeps a surviving tile's id known, so a dependent alert's tile_id
// does not go unknown and force the alert to be replaced.
type tileIDsPlanModifier struct{}

// Description returns a plain-text description of the modifier.
func (m tileIDsPlanModifier) Description(_ context.Context) string {
	return "Plans each tile_ids element from the tile-id carry-forward: a tile keeping its unique name keeps its id, a new or renamed tile's id is known only after apply."
}

// MarkdownDescription returns a markdown description of the modifier.
func (m tileIDsPlanModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

// PlanModifyMap fills in the tile ids the update will carry forward and leaves
// the rest unknown. Any obstacle — create, destroy, an unknown body, a parse
// failure — leaves the framework's value alone. That value is the unknown map
// whenever the proposed plan differs from prior state, and unknown is always
// safe because apply overwrites it. On a no-op plan the framework instead hands
// over the prior map unchanged, which is equally safe: nothing applies.
//
// A plan with no authored change keeps the prior map verbatim, so an out-of-band
// UI edit stays undetected as the resource documents. Recomputing the map from
// the authored body would omit a UI-added tile, and the resulting diff would
// plan an update that PUTs the authored body and deletes that tile.
func (m tileIDsPlanModifier) PlanModifyMap(ctx context.Context, req planmodifier.MapRequest, resp *planmodifier.MapResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return // create (nothing to carry forward) or destroy
	}

	var planned, priorAuthored, prior types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(dashboardJSONAttr), &planned)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(dashboardJSONAttr), &priorAuthored)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(normalizedJSONAttr), &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if planned.IsNull() || planned.IsUnknown() || prior.IsNull() || prior.IsUnknown() {
		return
	}

	// Nothing authored changed: hand the prior map back. Setting it explicitly
	// matters — the framework may already have marked the attribute unknown
	// because the config text differed, and an unknown tile_ids would itself plan
	// an update.
	if known(priorAuthored) {
		plannedCanon, plannedErr := canonicalizeDashboardJSON(planned.ValueString())
		priorCanon, priorErr := canonicalizeDashboardJSON(priorAuthored.ValueString())
		if plannedErr == nil && priorErr == nil && plannedCanon == priorCanon {
			resp.PlanValue = req.StateValue
			return
		}
	}

	elems, err := plannedTileIDs([]byte(planned.ValueString()), []byte(prior.ValueString()))
	if err != nil {
		tflog.Warn(ctx, "could not plan tile_ids from the dashboard bodies; leaving them unknown: "+err.Error())
		return
	}

	value, diags := types.MapValue(types.StringType, elems)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
	resp.PlanValue = value
}

// plannedTileIDs predicts the tile_ids map an apply will produce, by running
// the same merge the update will run rather than re-deriving its rules: any id
// the merge keeps is one the server already has, so the server keeps it too,
// and any tile the merge leaves id-less is minted a fresh id on apply. Keys are
// the uniquely named tiles, the rule tileIDsByName applies to the response.
func plannedTileIDs(planned, prior []byte) (map[string]attr.Value, error) {
	merged, err := mergeTileIDs(planned, prior)
	if err != nil {
		return nil, err
	}
	tiles, err := parseDashboardTiles(merged)
	if err != nil {
		return nil, err
	}
	priorTiles, err := parseDashboardTiles(prior)
	if err != nil {
		return nil, err
	}

	// An id is only kept by the server if the server issued it. The merge drops
	// ids the prior body does not have, but it also returns the authored body
	// untouched when there is nothing to merge from, so check here too.
	priorIDs := map[string]bool{}
	for _, tile := range priorTiles {
		if tile.ID != "" {
			priorIDs[tile.ID] = true
		}
	}

	count := tileNameCount(tiles)
	elems := make(map[string]attr.Value, len(tiles))
	for _, tile := range tiles {
		if tile.Name == "" || count[tile.Name] > 1 {
			continue
		}
		if tile.ID != "" && priorIDs[tile.ID] {
			elems[tile.Name] = types.StringValue(tile.ID)
			continue
		}
		elems[tile.Name] = types.StringUnknown()
	}
	return elems, nil
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
		addNotConfiguredError(&resp.Diagnostics, "resource")
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

	diags := plan.applyDashboardBody(ctx, body)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		// applyDashboardBody fails only when the POST-success body cannot be read
		// (no usable id, or an unreadable tiles array), so nothing can be recovered
		// from it: the dashboard exists on the server but cannot be tracked in
		// state. Surface the raw body so the operator can find and delete the
		// now-unmanaged dashboard manually.
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

	resp.Diagnostics.Append(state.applyDashboardBody(ctx, body)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// On import, dashboard_json is null/unknown because no config value exists
	// yet. Populate it from the fetched body so the imported state is
	// re-appliable without an immediate diff. The dashboard id and filter ids
	// are dropped: the API rejects them in an authored body. Select entries are
	// cleaned up for the same reason — the API exports aggregation fields its own
	// write schema rejects.
	if state.DashboardJSON.IsNull() || state.DashboardJSON.IsUnknown() {
		authored := body
		if stripped, err := stripServerIDs(authored); err == nil {
			authored = stripped
		} else {
			tflog.Warn(ctx, "could not strip server-assigned ids from the imported dashboard body: "+err.Error())
		}
		if cleaned, err := dropInvalidSelectFields(authored); err == nil {
			authored = cleaned
		} else {
			tflog.Warn(ctx, "could not drop invalid select fields from the imported dashboard body: "+err.Error())
		}
		state.DashboardJSON = types.StringValue(string(authored))
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

	resp.Diagnostics.Append(plan.applyDashboardBody(ctx, updated)...)
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
	utils.BetaWarning("clickhouse_clickstack_dashboard", &resp.Diagnostics)
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

// applyDashboardBody records the server's returned dashboard body: it sets id,
// normalized_json and tile_ids but does NOT touch dashboard_json (the user's
// authored value is the source of truth for that attribute).
func (m *dashboardResourceModel) applyDashboardBody(ctx context.Context, body []byte) diag.Diagnostics {
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

	// Near-unreachable: the body already parsed above for its id, so its tiles
	// array parses too unless the API returns something structurally new. It is
	// still an error rather than a warning, because on update the plan has
	// already promised known tile_ids entries and an empty map would resurface as
	// Terraform's own "inconsistent result after apply" — the warning would never
	// be read.
	byName, err := tileIDsByName(body)
	if err != nil {
		diags.AddError("Invalid Dashboard Response", "could not read tile ids from the dashboard body: "+err.Error())
		return diags
	}
	tileIDs, d := types.MapValueFrom(ctx, types.StringType, byName)
	diags.Append(d...)
	m.TileIDs = tileIDs

	return diags
}

// dashboardTile is the part of a tile this resource reads: the name callers
// reference it by and its server-assigned id.
type dashboardTile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// parseDashboardTiles decodes just the tiles array of a dashboard body.
func parseDashboardTiles(body []byte) ([]dashboardTile, error) {
	var doc struct {
		Tiles []dashboardTile `json:"tiles"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("read tiles: %w", err)
	}
	return doc.Tiles, nil
}

// tileNameCount counts how many tiles share each name.
func tileNameCount(tiles []dashboardTile) map[string]int {
	count := map[string]int{}
	for _, tile := range tiles {
		count[tile.Name]++
	}
	return count
}

// tileIDsByName maps tile name to server-assigned tile id for every tile whose
// name is non-empty and unique within the dashboard body. Blank and duplicate
// names are left out: they cannot identify a single tile, so an alert could not
// reference them unambiguously.
func tileIDsByName(body []byte) (map[string]string, error) {
	tiles, err := parseDashboardTiles(body)
	if err != nil {
		return nil, err
	}
	count := tileNameCount(tiles)
	byName := map[string]string{}
	for _, tile := range tiles {
		if tile.Name == "" || tile.ID == "" || count[tile.Name] > 1 {
			continue
		}
		byName[tile.Name] = tile.ID
	}
	return byName, nil
}
