//go:build alpha

package resource

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
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

// NewPostgresServiceResource constructs the alpha-tagged Postgres resource.
// Registered in pkg/resource/register_debug.go.
func NewPostgresServiceResource() resource.Resource {
	return &PostgresServiceResource{}
}

// PostgresServiceResource manages a ClickHouse Cloud Managed Postgres
// instance via the api.Client interface. See the embedded description for
// scope and limitations.
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
				Description: "Instance size (VM SKU). See ClickHouse Cloud docs for the supported set. The server is the source of truth — invalid sizes are rejected with HTTP 400 at apply time. (Earlier alpha pinned the list to an 82-entry compile-time snapshot; that meant new AWS instance families needed a provider patch release before users could use them. Dropped in favor of the lower-friction 'server validates' pattern matching the region attribute.) Resizable in place.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"ha_type": schema.StringAttribute{
				Description: "High-availability mode. One of 'none' (single replica), 'async' (asynchronous replica), or 'sync' (synchronous replica). Mutable post-create; an HA flip triggers a transition. Omitting the attribute preserves the prior value (the server defaults to 'none' on Create); to actively downgrade, set 'ha_type = \"none\"' explicitly.",
				Optional:    true,
				Computed:    true,
				// No Default("none"): the server applies "none" by default on
				// Create when omitted. A schema-level Default would also fire
				// when a user DELETES the line on an existing resource, which
				// would silently downgrade HA from "async"/"sync" → "none" —
				// a real footgun caught in PR review. UseStateForUnknown
				// preserves the prior state when the user omits the line.
				// Explicit "none" still works and still triggers a downgrade.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(postgresHaTypes...),
				},
			},
			"tags": schema.SetNestedAttribute{
				Description: "Resource tags. Set of {key, value} objects where both fields are required (value of null or '' is rejected at plan time). Keys starting with 'chc_' are reserved by the server and rejected at plan time.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Set{
					// CRITICAL: without UseStateForUnknown, the framework
					// marks tags as Unknown in every plan (Optional+Computed
					// semantics), and Update would PATCH "tags": [] on every
					// apply that touches any other attribute — silent data
					// loss for any user with tags set. The Phase 2 e2e
					// resize test caught this live.
					setplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Description: "Tag key. Cannot start with 'chc_' (reserved).",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
								notReservedTagPrefixValidator{},
							},
						},
						"value": schema.StringAttribute{
							Description: "Tag value. Must be a non-empty alphanumeric/'.'/'-'/'_' string. The server rejects PATCHes containing tags whose value field is omitted, so we require it at the schema layer to keep CREATE and UPDATE behavior symmetric. (Empty strings are also rejected because the server normalizes them to no-value, which would cause perpetual plan/state drift.)",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
					},
				},
				Validators: []validator.Set{
					// SizeAtLeast(1) intentionally rejects explicit `tags = []`
					// in .tf. The round-trip server → state collapses empty
					// arrays to SetNull (because chc_-filtering produces an
					// empty filtered list), so an explicit empty set in
					// config would diff perpetually against null state.
					// Users who want no tags must omit the attribute
					// entirely; UseStateForUnknown then carries the prior
					// state forward without diff.
					setvalidator.SizeAtLeast(1),
					// SizeAtMost matches the server's MAX_TAGS_PER_RESOURCE
					// (50) at packages/cp-common/src/protocol/ResourcesTags.ts:9.
					// Earlier 64 was an over-permissive guess.
					setvalidator.SizeAtMost(50),
				},
			},

			// --- Computed ----------------------------------------------------
			"state": schema.StringAttribute{
				Description: "Server-reported state. Examples: 'creating', 'running', 'restarting', 'unavailable', 'deleting'. Forward-compatible: unknown values from the server are surfaced verbatim.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					// Without UseStateForUnknown, every plan would mark state
					// as (known after apply), forcing an Update on every apply
					// — and the no-op Update branch would write the Unknown
					// straight back to state, which the framework rejects as
					// "Provider produced inconsistent result after apply."
					// Drift is still detected on Read/refresh; USFU only
					// affects planning.
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
				Description: "True when this instance is a primary; false when it's a read replica. Phase 2 only ever provisions primaries (replicas land in Phase 5). syncPostgresState supplies the 'primary' fallback when the server response omits the field.",
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
				Description: "Full connection URI embedding the username and the server-generated password. Marked sensitive; secret-redaction also covers it in TF_LOG=DEBUG output.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},

			// --- Sensitive / write-only -------------------------------------
			"password": schema.StringAttribute{
				Description: "Server-generated superuser password. The GET endpoint never echoes it, so the resource captures it from the create response and pins state via UseStateForUnknown. User-supplied passwords land in Phase 4.",
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

// Create provisions a new Postgres instance.
//
// Sequence (per plan Phase 2):
//  1. Read plan, build PostgresCreate body.
//  2. POST /postgres — capture id + server-generated password.
//  3. Write partial state (id + password + explicit-null computed attrs) so a
//     subsequent failure leaves a recoverable Terraform state pointing at the
//     real server resource.
//  4. Wait for state=running.
//  5. Re-read to hydrate hostname/connection_string/created_at/state.
//  6. Write final state.
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

	// Step 3: mid-Create partial state write. Persists id + password so a
	// later step-4/5 failure leaves a state Terraform can reconcile against
	// the real server resource.
	partial := buildPartialCreateState(plan, pg, generatedPassword)
	resp.Diagnostics.Append(resp.State.Set(ctx, partial)...)
	if resp.Diagnostics.HasError() {
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

	// GetPostgres never echoes the password, so we only have the value the
	// server returned at create time. Phase 4 widens this once user-supplied
	// passwords land.
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

// Update applies in-place mutations for size, ha_type, and tags.
//
// Phase 2 does NOT mutate: id, name, cloud_provider, region, postgres_version
// (all RequiresReplace), or password (Phase 4 owns rotation).
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
		if err := r.client.WaitForPostgresStateTransitionAndReturn(ctx, state.ID.ValueString(), api.PostgresStateRunning, postgresDefaultUpdateTimeoutSeconds); err != nil {
			resp.Diagnostics.AddError(
				"Error waiting for Postgres service to settle after update",
				"Could not confirm Postgres service "+state.ID.ValueString()+" returned to 'running': "+err.Error(),
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

	// password is Computed with UseStateForUnknown — the framework already
	// carries the prior state value through to plan. syncPostgresState
	// intentionally does not touch model.Password, so the prior value
	// survives this final state write unchanged.
	resp.Diagnostics.Append(syncPostgresState(ctx, pg, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete is a thin wrapper: the Phase 1 DeletePostgres client method already
// owns the 404-idempotent / 409-retry machinery.
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

// isPostgresStateRunning is the state-checker passed to WaitForPostgresState.
// Forward-compatible: anything other than 'running' is treated as still
// transitioning, including server states the provider hasn't learned yet.
func isPostgresStateRunning(s string) bool { return s == api.PostgresStateRunning }

// buildPartialCreateState produces the intermediate model written to state
// between the CreatePostgres response and the post-wait re-read. It captures
// the two values that can't be recovered later (id + server-generated
// password) and explicitly nulls every other computed attribute so the
// plugin-framework state-write validator accepts the mid-Create write.
//
// Reason it exists as a separate helper: this is the most novel piece of
// Phase 2's lifecycle ordering. Inline in Create it was untestable without
// constructing synthetic tfsdk.Plan / tfsdk.State — extracting makes the
// regression target obvious.
func buildPartialCreateState(plan models.PostgresServiceResourceModel, pg *api.Postgres, generatedPassword string) models.PostgresServiceResourceModel {
	partial := plan
	partial.ID = types.StringValue(pg.Id)
	if generatedPassword != "" {
		partial.Password = types.StringValue(generatedPassword)
	} else {
		partial.Password = types.StringNull()
	}
	// Every other computed attribute must be explicitly null (not zero-value),
	// or the framework rejects the state write mid-Create.
	partial.State = types.StringNull()
	partial.CreatedAt = types.StringNull()
	partial.IsPrimary = types.BoolNull()
	partial.Hostname = types.StringNull()
	partial.Port = types.Int64Null()
	partial.Username = types.StringNull()
	partial.ConnectionString = types.StringNull()
	// HaType / PostgresVersion came in from the plan; the computed-side may
	// still be Unknown if the user didn't set them. Pin to the value the
	// server returned in the create response (typically "none" for HaType).
	if partial.HaType.IsUnknown() {
		if pg.HaType != "" {
			partial.HaType = types.StringValue(pg.HaType)
		} else {
			partial.HaType = types.StringValue("none")
		}
	}
	if partial.PostgresVersion.IsUnknown() {
		partial.PostgresVersion = types.StringValue(pg.PostgresVersion)
	}
	// Tags is Optional+Computed — if the user didn't set any, hold null until
	// the post-wait re-read populates it. If the user set tags, keep them.
	if partial.Tags.IsUnknown() {
		partial.Tags = types.SetNull(models.PostgresServiceTagObjectType())
	}
	return partial
}

// planToPostgresCreate maps a fully-resolved plan into the wire shape.
// Tags use a value-by-value walk so the cmp.Diff in tests sees a stable order.
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

// postgresUpdatePlan bundles the two artifacts buildPostgresUpdate produces
// so the call site doesn't have to remember positional bool semantics.
//
//   - Body == nil           → no diff; caller skips the API call entirely.
//   - Body != nil           → sparse PATCH body containing only the changed
//     fields (size, ha_type, tags).
//   - TransitionExpected    → the server processes the mutation as a state
//     transition (size, ha_type); caller should
//     follow up with WaitForPostgresStateTransitionAndReturn.
type postgresUpdatePlan struct {
	Body               *api.PostgresUpdate
	TransitionExpected bool
}

// buildPostgresUpdate diffs plan vs state and produces a sparse PATCH body,
// or returns Body=nil when nothing actually changed (true no-op).
//
// Tag handling follows plan line 158: *[]Tag. nil means "leave server-side
// tags alone"; pointer to empty slice means "clear all tags"; pointer to
// non-empty slice means "replace". Critical: callers must NEVER receive a
// PATCH body where Tags is the zero-value *[]Tag — that would marshal as
// the omitted field and silently fail to clear tags.
//
// Plan.Tags == Unknown is treated specially (see inline comment) to defend
// against a regression in the schema's UseStateForUnknown plan modifier.
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
	// Tags handling.
	//
	// CRITICAL server-side gotcha (caught in Phase 2 e2e v2 run, 2026-05-28):
	// the Postgres PATCH endpoint has PUT-like semantics for tags. If the
	// request body omits the `tags` field, the server CLEARS all tags
	// server-side. This is asymmetric with `size` / `ha_type`, which the
	// server preserves when omitted.
	//
	// The implication: whenever we PATCH any field (size or ha_type), we
	// MUST also include the current tags in the body, or they'll be silently
	// wiped. This is independent of whether the user changed tags or not.
	//
	// Plan-state combinations and our action:
	//   - Unknown plan tags + any state             -> include state.Tags
	//     (defense-in-depth: framework shouldn't resolve to Unknown after
	//     UseStateForUnknown, but if a regression slips through we still
	//     preserve server-side tags rather than clearing them).
	//   - Plan tags == state tags (no diff)         -> include state.Tags
	//     when ANYTHING ELSE changes, otherwise leave update.Tags nil.
	//   - Plan null, state populated (clear)        -> send tags: [].
	//     This branch counts as a tag change (sets `changed`).
	//   - Plan populated, state different           -> send the mapped slice.
	//   - Plan null + state null (no tags at all)   -> leave update.Tags nil.
	//
	// The implementation below funnels everything through a single helper
	// for clarity. Only set Tags when the caller will actually send a PATCH
	// (i.e., `changed` is true via size/ha_type or via a tag diff itself);
	// callers checking `Body == nil` for no-op should not see Tags forced in.
	tagsChanged, mappedFromPlan, d := diffTags(ctx, plan, state)
	diags.Append(d...)
	if diags.HasError() {
		return postgresUpdatePlan{}, diags
	}
	if tagsChanged {
		// Plan vs. state differs — adopt the plan's tag intent verbatim
		// (whether that's clear-all or replace).
		update.Tags = mappedFromPlan
		changed = true
	} else if changed {
		// Tags are unchanged but size or ha_type IS changing. Defend
		// against server-side PUT-like tag semantics by re-asserting the
		// current state tags in the PATCH body.
		preserved, d := planTagsToAPI(ctx, state.Tags)
		diags.Append(d...)
		if diags.HasError() {
			return postgresUpdatePlan{}, diags
		}
		if preserved == nil {
			// No tags in state — nothing to preserve. Leave update.Tags
			// nil; server has no tags to clear, so omitting is safe.
		} else {
			update.Tags = preserved
		}
	}

	if !changed {
		return postgresUpdatePlan{}, diags
	}
	return postgresUpdatePlan{Body: &update, TransitionExpected: transitionExpected}, diags
}

// diffTags compares the plan's tags attribute against the state's, returning:
//   - changed: true if the plan represents a different tag intent than state.
//   - body:    the *[]api.Tag to put in the PATCH body when the caller chooses
//     to send the diff. nil if plan.Tags is Unknown (treat as
//     "no diff" — defense-in-depth against missing UseStateForUnknown).
//
// Cases:
//   - Plan Unknown -> changed=false, body=nil. Caller should NOT touch tags
//     (covered by the UseStateForUnknown plan modifier in normal operation).
//   - Plan == state -> changed=false, body=nil.
//   - Plan null, state populated -> changed=true, body=&[]Tag{} (clear).
//   - Plan populated, plan != state -> changed=true, body=&mapped.
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

// planTagsToAPI extracts an *[]api.Tag from a Terraform set-of-objects
// attribute. Returns nil when the attribute is null/unknown (caller can
// distinguish "leave alone" from "explicit empty"); returns a pointer to
// the materialized slice otherwise.
func planTagsToAPI(ctx context.Context, tagsSet types.Set) (*[]api.Tag, diag.Diagnostics) {
	var diags diag.Diagnostics
	if tagsSet.IsNull() || tagsSet.IsUnknown() {
		return nil, diags
	}
	var tagModels []models.PostgresServiceTagModel
	diags.Append(tagsSet.ElementsAs(ctx, &tagModels, false)...)
	if diags.HasError() {
		return nil, diags
	}
	out := make([]api.Tag, 0, len(tagModels))
	for _, tm := range tagModels {
		t := api.Tag{Key: tm.Key.ValueString()}
		if !tm.Value.IsNull() && !tm.Value.IsUnknown() {
			t.Value = tm.Value.ValueString()
		}
		out = append(out, t)
	}
	return &out, diags
}

// syncPostgresState writes an api.Postgres response into the Terraform
// state model. Tags returned from the server are filtered to drop the
// chc_-prefixed ones (server-reserved); the user only ever sees their own.
func syncPostgresState(_ context.Context, pg *api.Postgres, state *models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(pg.Id)
	state.Name = types.StringValue(pg.Name)
	state.CloudProvider = types.StringValue(pg.Provider)
	state.Region = types.StringValue(pg.Region)

	// Empty-string handling for postgres_version / size:
	// the server should never return empty strings here for a valid instance.
	// If it ever does (mid-transition wire shape we don't currently know
	// about, or a future API change), we deliberately preserve the prior
	// state value rather than overwriting with an empty string — overwriting
	// would silently corrupt RequiresReplace-tracked state. The trade-off:
	// if the server permanently starts returning empty, the resource will
	// silently lie about its state. The expectation is that Phase 6
	// integration tests would catch this; the comment exists so a future
	// debugger knows where to look.
	if pg.PostgresVersion != "" {
		state.PostgresVersion = types.StringValue(pg.PostgresVersion)
	}
	if pg.Size != "" {
		state.Size = types.StringValue(pg.Size)
	}
	if pg.HaType != "" {
		state.HaType = types.StringValue(pg.HaType)
	} else {
		state.HaType = types.StringValue("none")
	}
	state.State = types.StringValue(pg.State)
	state.CreatedAt = types.StringValue(pg.CreatedAt)
	// IsPrimary fallback: Phase 2 only ever provisions primaries (Phase 5
	// adds read replicas). Defaulting nil to true is safe for Phase 2 but
	// will mismark a Phase 5 replica if the server starts returning
	// IsPrimary=nil for replicas. Revisit in Phase 5: prefer to error
	// loudly when the server omits a field the resource depends on, since
	// is_primary drives replica-specific UX.
	if pg.IsPrimary != nil {
		state.IsPrimary = types.BoolValue(*pg.IsPrimary)
	} else {
		state.IsPrimary = types.BoolValue(true)
	}
	if pg.Hostname != nil {
		state.Hostname = types.StringValue(*pg.Hostname)
	} else {
		state.Hostname = types.StringNull()
	}
	state.Port = types.Int64Value(postgresDefaultPort)
	if pg.Username != "" {
		state.Username = types.StringValue(pg.Username)
	} else {
		state.Username = types.StringNull()
	}
	if pg.ConnectionString != nil {
		state.ConnectionString = types.StringValue(*pg.ConnectionString)
	} else {
		state.ConnectionString = types.StringNull()
	}

	tagsValue, d := apiTagsToSetValue(pg.Tags)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.Tags = tagsValue

	return diags
}

// apiTagsToSetValue converts an api.Tag slice into the Terraform set of
// {key, value} objects. Drops any tag whose key starts with chc_ (server
// reserved) so it never surfaces as drift to the user.
func apiTagsToSetValue(apiTags []api.Tag) (types.Set, diag.Diagnostics) {
	var diags diag.Diagnostics
	objType, ok := models.PostgresServiceTagObjectType().(attr.TypeWithAttributeTypes)
	if !ok {
		// Unreachable unless models.PostgresServiceTagObjectType is changed
		// to return a non-Object type (e.g., a switch to types.MapType during
		// a future refactor). Surface the concrete type so the future
		// debugger doesn't need to grep for the assertion.
		diags.AddError(
			"Postgres tag schema definition is corrupt",
			fmt.Sprintf("models.PostgresServiceTagObjectType() returned %T, which does not implement attr.TypeWithAttributeTypes. The tag attribute requires an Object type. Report this to the provider developers.", models.PostgresServiceTagObjectType()),
		)
		return types.SetNull(models.PostgresServiceTagObjectType()), diags
	}
	attrTypes := objType.AttributeTypes()

	filtered := make([]attr.Value, 0, len(apiTags))
	for _, t := range apiTags {
		if strings.HasPrefix(t.Key, postgresReservedTagPrefix) {
			continue
		}
		var value attr.Value
		if t.Value == "" {
			value = types.StringNull()
		} else {
			value = types.StringValue(t.Value)
		}
		obj, d := types.ObjectValue(attrTypes, map[string]attr.Value{
			"key":   types.StringValue(t.Key),
			"value": value,
		})
		diags.Append(d...)
		if diags.HasError() {
			return types.SetNull(models.PostgresServiceTagObjectType()), diags
		}
		filtered = append(filtered, obj)
	}

	if len(filtered) == 0 {
		return types.SetNull(models.PostgresServiceTagObjectType()), diags
	}
	set, d := types.SetValue(models.PostgresServiceTagObjectType(), filtered)
	diags.Append(d...)
	return set, diags
}

// ---------------------------------------------------------------------------
// Validators
// ---------------------------------------------------------------------------

// notReservedTagPrefixValidator rejects tag keys that start with chc_.
// Implemented as a struct rather than the generic regex-based validators so
// the error message can name the specific reserved prefix.
type notReservedTagPrefixValidator struct{}

func (v notReservedTagPrefixValidator) Description(_ context.Context) string {
	return fmt.Sprintf("Tag key must not start with the reserved prefix %q", postgresReservedTagPrefix)
}

func (v notReservedTagPrefixValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v notReservedTagPrefixValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	key := req.ConfigValue.ValueString()
	if strings.HasPrefix(key, postgresReservedTagPrefix) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Reserved tag prefix",
			fmt.Sprintf("Tag key %q starts with the reserved prefix %q. The server rejects tags with this prefix.", key, postgresReservedTagPrefix),
		)
	}
}
