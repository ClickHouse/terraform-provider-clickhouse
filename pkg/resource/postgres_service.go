//go:build alpha

package resource

import (
	"context"
	_ "embed"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

var (
	_ resource.Resource                = &PostgresServiceResource{}
	_ resource.ResourceWithConfigure   = &PostgresServiceResource{}
	_ resource.ResourceWithImportState = &PostgresServiceResource{}
)

//go:embed descriptions/postgres_service.md
var postgresServiceResourceDescription string

func NewPostgresServiceResource() resource.Resource {
	return &PostgresServiceResource{}
}

type PostgresServiceResource struct {
	client api.Client
}

func (r *PostgresServiceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_service"
}

func (r *PostgresServiceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: postgresServiceResourceDescription,
		Attributes: map[string]schema.Attribute{
			// --- Identity / immutable ----------------------------------------
			"id": schema.StringAttribute{
				Description: "Unique identifier for the Postgres service. Assigned by ClickHouse Cloud.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Human-readable name. Immutable post-create; changes force destroy-and-recreate. Differs from clickhouse_service, which allows in-place rename.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthBetween(postgresInstanceNameMin, postgresInstanceNameMax),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider hosting the instance. Currently only 'aws' is supported.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(postgresCloudProviders...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				Description: "Cloud region (e.g. 'us-east-1'). No client-side validation; the server rejects unsupported regions.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"postgres_version": schema.StringAttribute{
				Description: "Major Postgres version (e.g. '18'). The server picks the patch release within that major. Changing the major triggers destroy-and-recreate.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(postgresVersions...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},

			// --- Mutable -----------------------------------------------------
			"size": schema.StringAttribute{
				Description: "Instance size (VM SKU). See ClickHouse Cloud docs for the supported set. No client-side enum; the server rejects unsupported sizes with HTTP 400 at apply time. Resizable in place.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"ha_type": schema.StringAttribute{
				Description: "High-availability mode. One of 'none' (single replica), 'async' (asynchronous replica), or 'sync' (synchronous replica). Mutable post-create; an HA flip triggers a transition. Omitting the attribute preserves the prior value (the server defaults to 'none' on Create); to actively downgrade, set 'ha_type = \"none\"' explicitly.",
				Optional:    true,
				Computed:    true,
				// No Default("none"): would silently downgrade HA when the
				// user later deletes the line on an existing resource.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(postgresHaTypes...),
				},
			},
			"tags": schema.MapAttribute{
				Description: "Resource tags as a key-value map. Values must be non-empty (server's PATCH returns 400 on omitted value). Set `tags = {}` to clear all tags; omit the attribute to preserve the prior value.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					// Without USFU, Update would PATCH "tags": [] on every
					// apply that touches any other attribute → silent loss.
					mapplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Map{
					mapvalidator.SizeAtMost(50), // server MAX_TAGS_PER_RESOURCE
					mapvalidator.KeysAre(stringvalidator.LengthAtLeast(1)),
					mapvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},

			// --- Computed ----------------------------------------------------
			"state": schema.StringAttribute{
				Description: "Server-reported state. Examples: 'creating', 'running', 'restarting', 'unavailable', 'deleting'. Forward-compatible: unknown values from the server are surfaced verbatim.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					// Without USFU, planner marks state as known-after-apply
					// on no-op applies, framework rejects the round-trip.
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Description: "RFC3339 timestamp when the service was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"is_primary": schema.BoolAttribute{
				Description: "True when this instance is a writeable primary cluster; false only for separately-provisioned read replicas. HA standby servers (`ha_type = async`/`sync`) live inside the same primary instance and don't affect this value.",
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Description: "Network hostname for client connections.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"port": schema.Int64Attribute{
				Description: "TCP port for client connections. Hardcoded to 5432 today; will become server-supplied once the API exposes a per-instance port field.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"username": schema.StringAttribute{
				Description: "Default superuser name.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"connection_string": schema.StringAttribute{
				Description: "Full connection URI embedding the username and the server-generated password. Marked sensitive; the secret-redaction layer also covers it in TF_LOG=DEBUG output.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			// --- Sensitive / write-only -------------------------------------
			"password": schema.StringAttribute{
				Description: "Server-generated superuser password. Captured from the create response and refreshed from each GET (the server echoes it).",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *PostgresServiceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			"Expected api.Client, got something else. Please report this issue to the provider developers.",
		)
		return
	}
	r.client = client
}

func (r *PostgresServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.PostgresServiceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createBody, d := planToPostgresCreate(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	pg, generatedPassword, err := r.client.CreatePostgres(ctx, createBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Postgres service",
			"Could not create Postgres service: "+err.Error(),
		)
		return
	}

	if err := r.client.WaitForPostgresState(ctx, pg.Id, isPostgresStateRunning, postgresDefaultCreateTimeoutSeconds); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for Postgres service to reach 'running'",
			"Could not finish provisioning Postgres service "+pg.Id+": "+err.Error(),
		)
		return
	}

	// Re-read to pick up hostname / connection_string / created_at / final state.
	final, err := r.client.GetPostgres(ctx, pg.Id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Postgres service after create",
			"Could not read Postgres service "+pg.Id+" after waiting for 'running': "+err.Error(),
		)
		return
	}

	// Capture the Create-response password as a fallback; syncPostgresState
	// will overwrite if the post-Create GET also echoes it.
	model := plan
	model.ID = types.StringValue(final.Id)
	if generatedPassword != "" {
		model.Password = types.StringValue(generatedPassword)
	} else {
		model.Password = types.StringNull()
	}
	resp.Diagnostics.Append(syncPostgresState(ctx, final, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *PostgresServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.PostgresServiceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pg, err := r.client.GetPostgres(ctx, state.ID.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Postgres service",
			"Could not read Postgres service "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(syncPostgresState(ctx, pg, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies in-place mutations for size, ha_type, and tags. Everything
// else is RequiresReplace; password isn't mutable here.
func (r *PostgresServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state models.PostgresServiceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updatePlan, d := buildPostgresUpdate(ctx, plan, state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	if updatePlan.Body == nil {
		// No diff — write plan back so Computed-from-Optional attrs propagate.
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	}

	if _, err := r.client.UpdatePostgres(ctx, state.ID.ValueString(), *updatePlan.Body); err != nil {
		resp.Diagnostics.AddError(
			"Error updating Postgres service",
			"Could not update Postgres service "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	if updatePlan.TransitionExpected {
		predicate := buildPostgresMatchPredicate(updatePlan.Body)
		if err := r.client.WaitForPostgresMatch(ctx, state.ID.ValueString(), predicate, postgresDefaultUpdateTimeoutSeconds); err != nil {
			resp.Diagnostics.AddError(
				"Error waiting for Postgres service to apply the requested update",
				"Could not confirm Postgres service "+state.ID.ValueString()+" reflects the PATCH values: "+err.Error(),
			)
			return
		}
	}

	pg, err := r.client.GetPostgres(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Postgres service after update",
			"Could not refresh Postgres service "+state.ID.ValueString()+" after PATCH: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(syncPostgresState(ctx, pg, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete wraps DeletePostgres (which owns 404-idempotent / 409-retry behavior).
func (r *PostgresServiceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.PostgresServiceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeletePostgres(ctx, state.ID.ValueString()); err != nil {
		if api.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting Postgres service",
			"Could not delete Postgres service "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}
}

func (r *PostgresServiceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isPostgresStateRunning treats any non-running value as still-transitioning
// (forward-compatible with new server states).
func isPostgresStateRunning(s string) bool { return s == api.PostgresStateRunning }

func planToPostgresCreate(ctx context.Context, plan models.PostgresServiceResourceModel) (api.PostgresCreate, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := api.PostgresCreate{
		Name:     plan.Name.ValueString(),
		Provider: plan.CloudProvider.ValueString(),
		Region:   plan.Region.ValueString(),
		Size:     plan.Size.ValueString(),
	}
	if !plan.PostgresVersion.IsNull() && !plan.PostgresVersion.IsUnknown() {
		body.PostgresVersion = plan.PostgresVersion.ValueString()
	}
	if !plan.HaType.IsNull() && !plan.HaType.IsUnknown() {
		body.HaType = plan.HaType.ValueString()
	}

	tags, d := planTagsToAPI(ctx, plan.Tags)
	diags.Append(d...)
	if diags.HasError() {
		return api.PostgresCreate{}, diags
	}
	if tags != nil {
		body.Tags = *tags
	}

	return body, diags
}

// postgresUpdatePlan: Body nil = no diff; TransitionExpected = caller must
// follow PATCH with WaitForPostgresMatch.
type postgresUpdatePlan struct {
	Body               *api.PostgresUpdate
	TransitionExpected bool
}

// buildPostgresUpdate diffs plan vs state. Tags use the *[]Tag contract:
// nil = leave alone; &[]{} = clear; &[]{...} = replace.
func buildPostgresUpdate(ctx context.Context, plan, state models.PostgresServiceResourceModel) (postgresUpdatePlan, diag.Diagnostics) {
	var diags diag.Diagnostics
	update := api.PostgresUpdate{}
	changed := false
	transitionExpected := false

	if !plan.Size.Equal(state.Size) {
		update.Size = plan.Size.ValueString()
		changed = true
		transitionExpected = true
	}
	if !plan.HaType.Equal(state.HaType) {
		update.HaType = plan.HaType.ValueString()
		changed = true
		transitionExpected = true
	}
	// PATCH has PUT-like semantics for tags: omitting `tags` from the body
	// clears them server-side. So whenever size/ha_type change, re-assert
	// the current tags or they'll be wiped.
	tagsChanged, mappedFromPlan, d := diffTags(ctx, plan, state)
	diags.Append(d...)
	if diags.HasError() {
		return postgresUpdatePlan{}, diags
	}
	if tagsChanged {
		update.Tags = mappedFromPlan
		changed = true
	} else if changed {
		preserved, d := planTagsToAPI(ctx, state.Tags)
		diags.Append(d...)
		if diags.HasError() {
			return postgresUpdatePlan{}, diags
		}
		// Only re-assert when there's something to defend; sending `"tags": []`
		// on a server that has no tags is wasted bytes.
		if preserved != nil && len(*preserved) > 0 {
			update.Tags = preserved
		}
	}

	if !changed {
		return postgresUpdatePlan{}, diags
	}
	return postgresUpdatePlan{Body: &update, TransitionExpected: transitionExpected}, diags
}

// buildPostgresMatchPredicate returns a predicate that succeeds when
// state==running AND every PATCHed field reflects the requested value.
// State-only checks would race Ubicloud's async queue.
func buildPostgresMatchPredicate(body *api.PostgresUpdate) func(*api.Postgres) bool {
	expectSize := body.Size
	expectHaType := body.HaType
	var expectTags map[string]string
	tagsRequested := body.Tags != nil
	if tagsRequested {
		expectTags = make(map[string]string, len(*body.Tags))
		for _, t := range *body.Tags {
			expectTags[t.Key] = t.Value
		}
	}
	return func(pg *api.Postgres) bool {
		if pg.State != api.PostgresStateRunning {
			return false
		}
		if expectSize != "" && pg.Size != expectSize {
			return false
		}
		if expectHaType != "" && pg.HaType != expectHaType {
			return false
		}
		if tagsRequested {
			if len(pg.Tags) != len(expectTags) {
				return false
			}
			for _, t := range pg.Tags {
				if want, ok := expectTags[t.Key]; !ok || want != t.Value {
					return false
				}
			}
		}
		return true
	}
}

// diffTags returns (changed, body, diags). body is nil for "leave alone"
// (Unknown or equal); &[]Tag{} for clear-all; &mapped for replace.
func diffTags(ctx context.Context, plan, state models.PostgresServiceResourceModel) (bool, *[]api.Tag, diag.Diagnostics) {
	var diags diag.Diagnostics
	if plan.Tags.IsUnknown() {
		return false, nil, diags
	}
	if plan.Tags.Equal(state.Tags) {
		return false, nil, diags
	}
	mapped, d := planTagsToAPI(ctx, plan.Tags)
	diags.Append(d...)
	if diags.HasError() {
		return false, nil, diags
	}
	if mapped == nil {
		empty := []api.Tag{}
		return true, &empty, diags
	}
	return true, mapped, diags
}

// planTagsToAPI returns nil for null/unknown maps (so callers can tell
// "leave alone" from "explicit empty"); pointer to slice otherwise.
func planTagsToAPI(ctx context.Context, tagsMap types.Map) (*[]api.Tag, diag.Diagnostics) {
	var diags diag.Diagnostics
	if tagsMap.IsNull() || tagsMap.IsUnknown() {
		return nil, diags
	}
	raw := make(map[string]string, len(tagsMap.Elements()))
	diags.Append(tagsMap.ElementsAs(ctx, &raw, false)...)
	if diags.HasError() {
		return nil, diags
	}
	out := make([]api.Tag, 0, len(raw))
	for k, v := range raw {
		out = append(out, api.Tag{Key: k, Value: v})
	}
	return &out, diags
}

// syncPostgresState writes a GetPostgres response into the resource model.
// Builds into a local copy and assigns only on success — a fallible step
// (apiTagsToMapValue) can return diagnostics without leaving *state half-mutated.
func syncPostgresState(_ context.Context, pg *api.Postgres, state *models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	out := *state

	out.ID = types.StringValue(pg.Id)
	out.Name = types.StringValue(pg.Name)
	out.CloudProvider = types.StringValue(pg.Provider)
	out.Region = types.StringValue(pg.Region)

	out.Size = types.StringValue(pg.Size)
	out.State = types.StringValue(pg.State)
	out.CreatedAt = types.StringValue(pg.CreatedAt)
	// postgresVersion / haType / hostname / username / connectionString /
	// password are schema-optional on PostgresInstanceV1 — preserve prior
	// values when the server omits them rather than writing empty strings.
	if pg.PostgresVersion != "" {
		out.PostgresVersion = types.StringValue(pg.PostgresVersion)
	}
	if pg.HaType != "" {
		out.HaType = types.StringValue(pg.HaType)
	} else {
		out.HaType = types.StringValue("none")
	}
	out.IsPrimary = types.BoolValue(pg.IsPrimary)
	if pg.Hostname != "" {
		out.Hostname = types.StringValue(pg.Hostname)
	} else {
		out.Hostname = types.StringNull()
	}
	out.Port = types.Int64Value(postgresDefaultPort)
	if pg.Username != "" {
		out.Username = types.StringValue(pg.Username)
	} else {
		out.Username = types.StringNull()
	}
	if pg.ConnectionString != "" {
		out.ConnectionString = types.StringValue(pg.ConnectionString)
	} else {
		out.ConnectionString = types.StringNull()
	}
	// Password is returned on GET. Hydrate it when present so terraform
	// import recovers the credential and Read reconciles out-of-band
	// rotations. Skip when empty so the Create-time capture survives.
	if pg.Password != "" {
		out.Password = types.StringValue(pg.Password)
	}

	tagsValue, d := apiTagsToMapValue(pg.Tags)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	out.Tags = tagsValue

	*state = out
	return diags
}

// apiTagsToMapValue maps []api.Tag → types.Map. Drops empty-value tags
// (schema requires non-empty values). Empty server input maps to an empty
// map (not null) so config `tags = {}` round-trips cleanly.
func apiTagsToMapValue(apiTags []api.Tag) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics
	filtered := make(map[string]attr.Value, len(apiTags))
	for _, t := range apiTags {
		if t.Value == "" {
			continue
		}
		filtered[t.Key] = types.StringValue(t.Value)
	}
	m, d := types.MapValue(types.StringType, filtered)
	diags.Append(d...)
	return m, diags
}
