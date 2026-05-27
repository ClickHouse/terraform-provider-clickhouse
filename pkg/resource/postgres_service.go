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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
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
				Description: "Instance size (VM SKU). See ClickHouse Cloud docs for the supported set; the validator snapshot matches VM_SPECS at provider build time. Resizable in place.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(postgresSizes...),
				},
			},
			"ha_type": schema.StringAttribute{
				Description: "High-availability mode. One of 'none' (single replica), 'async' (asynchronous replica), or 'sync' (synchronous replica). Mutable post-create; an HA flip triggers a transition.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("none"),
				Validators: []validator.String{
					stringvalidator.OneOf(postgresHaTypes...),
				},
			},
			"tags": schema.SetNestedAttribute{
				Description: "Resource tags. Set of {key, value} objects; value is optional. Keys starting with 'chc_' are reserved by the server and rejected at plan time.",
				Optional:    true,
				Computed:    true,
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
							Description: "Tag value. Omit or set to null when no value is needed. Explicit empty strings are rejected at plan time because the server normalizes them to no-value, which would cause perpetual drift between plan and state.",
							Optional:    true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
					},
				},
				Validators: []validator.Set{
					setvalidator.SizeAtMost(64),
				},
			},

			// --- Computed ----------------------------------------------------
			"state": schema.StringAttribute{
				Description: "Server-reported state. Examples: 'creating', 'running', 'restarting', 'unavailable', 'deleting'. Forward-compatible: unknown values from the server are surfaced verbatim.",
				Computed:    true,
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
	// the real server resource. Every other computed attribute must be
	// explicitly null (not zero-value), or the framework rejects the write.
	partial := plan
	partial.ID = types.StringValue(pg.Id)
	if generatedPassword != nil {
		partial.Password = types.StringValue(*generatedPassword)
	} else {
		partial.Password = types.StringNull()
	}
	partial.State = types.StringNull()
	partial.CreatedAt = types.StringNull()
	partial.IsPrimary = types.BoolNull()
	partial.Hostname = types.StringNull()
	partial.Port = types.Int64Null()
	partial.Username = types.StringNull()
	partial.ConnectionString = types.StringNull()
	// HaType / PostgresVersion came in from the plan, may have been Unknown.
	// Initialize the computed-only side of those Optional+Computed attrs.
	if partial.HaType.IsUnknown() {
		partial.HaType = types.StringValue("none")
	}
	if partial.PostgresVersion.IsUnknown() {
		partial.PostgresVersion = types.StringValue(pg.PostgresVersion)
	}
	// Tags is Optional+Computed — if the user didn't set any, hold null until
	// the post-wait re-read populates it. If the user set tags, keep them.
	if partial.Tags.IsUnknown() {
		partial.Tags = types.SetNull(models.PostgresServiceTagObjectType())
	}
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
	if generatedPassword != nil {
		model.Password = types.StringValue(*generatedPassword)
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

	update, transitionExpected, d := buildPostgresUpdate(ctx, plan, state)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	if update == nil {
		// No diff — write plan back so Computed-from-Optional attrs propagate.
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	}

	if _, err := r.client.UpdatePostgres(ctx, state.ID.ValueString(), *update); err != nil {
		resp.Diagnostics.AddError(
			"Error updating Postgres service",
			"Could not update Postgres service "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	if transitionExpected {
		if err := r.client.WaitForPostgresLeaveAndReturn(ctx, state.ID.ValueString(), api.PostgresStateRunning, postgresDefaultUpdateTimeoutSeconds); err != nil {
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
// owns the 404-idempotent / 409-retry / dependent-replica fail-fast machinery.
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

// buildPostgresUpdate diffs plan vs state and produces a sparse PATCH body,
// or returns nil when nothing actually changed (true no-op).
//
// transitionExpected is true when the diff includes a field whose mutation
// the server processes via a state transition (currently size, ha_type),
// signaling the caller should run WaitForPostgresLeaveAndReturn.
//
// Tag handling follows plan line 158: *[]Tag. nil means "leave server-side
// tags alone"; pointer to empty slice means "clear all tags"; pointer to
// non-empty slice means "replace". Critical: callers must NEVER receive a
// PATCH body where Tags is the zero-value *[]Tag — that would marshal as
// the omitted field and silently fail to clear tags.
func buildPostgresUpdate(ctx context.Context, plan, state models.PostgresServiceResourceModel) (*api.PostgresUpdate, bool, diag.Diagnostics) {
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
	if !plan.Tags.Equal(state.Tags) {
		mapped, d := planTagsToAPI(ctx, plan.Tags)
		diags.Append(d...)
		if diags.HasError() {
			return nil, false, diags
		}
		if mapped == nil {
			// User removed the tags block from .tf -> clear all tags
			// (send []) rather than omit the field.
			empty := []api.Tag{}
			update.Tags = &empty
		} else {
			update.Tags = mapped
		}
		changed = true
	}

	if !changed {
		return nil, false, diags
	}
	return &update, transitionExpected, diags
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
	if pg.Username != nil {
		state.Username = types.StringValue(*pg.Username)
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
		diags.AddError("internal error", "tag object type does not implement TypeWithAttributeTypes")
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
