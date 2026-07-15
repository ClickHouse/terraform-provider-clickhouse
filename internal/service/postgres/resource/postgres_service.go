package resource

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

var (
	_ resource.Resource                   = &PostgresServiceResource{}
	_ resource.ResourceWithConfigure      = &PostgresServiceResource{}
	_ resource.ResourceWithImportState    = &PostgresServiceResource{}
	_ resource.ResourceWithModifyPlan     = &PostgresServiceResource{}
	_ resource.ResourceWithValidateConfig = &PostgresServiceResource{}
)

//go:embed descriptions/postgres_service.md
var postgresServiceResourceDescription string

// NewPostgresServiceResource constructs the Postgres resource.
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

// ValidateConfig surfaces the alpha warning at plan time, matching the other
// alpha resources (clickhouse_service_upgrade_window, …).
//
// State-dependent rules are NOT enforced here: ValidateConfig is stateless, so
// it can't tell a create from an update or read prior state. That covers the
// create-time attribute rules (required for a standard create; inherited from
// the source for a replica / restore — enforcing them here would misfire when an
// existing instance drops its origin block) and the live-replica modification
// block (which needs is_primary from prior state). All of these live in
// ModifyPlan, which has prior state.
func (r *PostgresServiceResource) ValidateConfig(_ context.Context, _ resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_postgres_service", &resp.Diagnostics)
}

// configIsOrigin reports whether the config declares a read replica or restore.
// Returns false when the signal is unknown (interpolated) so callers defer.
func configIsOrigin(config models.PostgresServiceResourceModel) bool {
	if config.ReadReplicaOf.IsUnknown() || config.RestoreToPointInTime.IsUnknown() {
		return false
	}
	return !config.ReadReplicaOf.IsNull() || !config.RestoreToPointInTime.IsNull()
}

// originSourceChanged reports whether an update recreates the instance from a
// (different) source, so its inherited attributes must be re-derived from that
// source rather than left pinned to the old one. That is: changing
// restore_to_point_in_time (any change recreates), or re-pointing read_replica_of
// on a still-live replica (a promoted primary, is_primary=true, is adopted in
// place — not recreated). Unknown signals defer (handled once resolved).
func originSourceChanged(config, state models.PostgresServiceResourceModel) bool {
	if !config.RestoreToPointInTime.IsNull() && !config.RestoreToPointInTime.IsUnknown() &&
		!config.RestoreToPointInTime.Equal(state.RestoreToPointInTime) {
		return true
	}
	if !config.ReadReplicaOf.IsNull() && !config.ReadReplicaOf.IsUnknown() &&
		!config.ReadReplicaOf.Equal(state.ReadReplicaOf) && !state.IsPrimary.ValueBool() {
		return true
	}
	return false
}

// forbidEmptyConfigOnCreate rejects an explicit empty pg_config / pgbouncer_config
// on a create (or a source-change replace). The server's create endpoints
// validate these as undefinedOr(isPopulatedObject), so an empty {} is a 400 —
// omit the attribute to use the default (or inherit from the source for a read
// replica / restore), or set at least one parameter. (An empty map IS valid on
// a plain update, where it clears all parameters via POST /config.)
func forbidEmptyConfigOnCreate(config models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	check := func(name string, m types.Map) {
		if !m.IsNull() && !m.IsUnknown() && len(m.Elements()) == 0 {
			diags.AddAttributeError(
				path.Root(name),
				"Empty "+name+" not allowed on create",
				"`"+name+" = {}` is rejected by the server on create — omit it to use the default (or inherit from the source for a read replica / restore), or set at least one parameter. An empty map is only valid on an update, where it clears all parameters.",
			)
		}
	}
	check("pg_config", config.PgConfig)
	check("pgbouncer_config", config.PgBouncerConfig)
	return diags
}

// sourceAttributeConflicts validates the create-time attributes a read replica /
// restore takes from its source. cloud_provider / region / postgres_version
// (and, for a replica, size) are reproduced verbatim on the new instance, so a
// supplied value that differs from the source's is an error; omitting them
// inherits the source's value. size on a restore (the new instance comes up at
// the backup's size, not the source's current one) and ha_type (server-assigned
// for a new replica/restore, set in place afterward) are NOT taken from the
// source, so they must be omitted entirely — setting them would make a known
// planned value that the apply contradicts. The caller fetches src.
func sourceAttributeConflicts(config models.PostgresServiceResourceModel, src *api.Postgres, isReplica bool) diag.Diagnostics {
	var diags diag.Diagnostics
	mustMatch := func(name string, configVal types.String, srcVal string) {
		if configVal.IsNull() || configVal.IsUnknown() {
			return // omitted → inherited
		}
		if configVal.ValueString() != srcVal {
			diags.AddAttributeError(
				path.Root(name),
				"Attribute conflicts with the source instance",
				"`"+name+"` is \""+configVal.ValueString()+"\" but the source instance has \""+srcVal+"\". A read replica / restore inherits this from the source — set it to the source's value or omit it.",
			)
		}
	}
	mustOmit := func(name string, configVal types.String, reason string) {
		if configVal.IsNull() || configVal.IsUnknown() {
			return
		}
		diags.AddAttributeError(
			path.Root(name),
			"Attribute not allowed for a read replica or restore",
			"`"+name+"` cannot be set for a read replica or point-in-time restore — "+reason+"; omit it.",
		)
	}
	mustMatch("cloud_provider", config.CloudProvider, src.Provider)
	mustMatch("region", config.Region, src.Region)
	if src.PostgresVersion != "" {
		mustMatch("postgres_version", config.PostgresVersion, src.PostgresVersion)
	}
	if isReplica {
		mustMatch("size", config.Size, src.Size)
	} else {
		mustOmit("size", config.Size, "a restored instance comes up at the backup's size")
	}
	mustOmit("ha_type", config.HaType, "HA mode is server-assigned for a new replica/restore and changed in place afterward")
	return diags
}

// requireStandardCreateAttributes requires cloud_provider / region / size for a
// standard create (no read replica / restore). Called only from ModifyPlan's
// create branch: on update these come from prior state, so dropping an origin
// block from an existing instance's config must not trip it.
func requireStandardCreateAttributes(config models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if config.ReadReplicaOf.IsUnknown() || config.RestoreToPointInTime.IsUnknown() {
		return diags // defer: the origin signal is interpolated
	}
	if configIsOrigin(config) {
		return diags // replica / restore inherits these from the source
	}
	required := []struct {
		name string
		val  attr.Value
	}{
		{"cloud_provider", config.CloudProvider},
		{"region", config.Region},
		{"size", config.Size},
	}
	for _, a := range required {
		if a.val.IsNull() {
			diags.AddAttributeError(
				path.Root(a.name),
				"Missing required attribute",
				"`"+a.name+"` is required for a standard Postgres service (omit it only for a read replica or point-in-time restore).",
			)
		}
	}
	return diags
}

// replicaUpdateForbidden blocks the in-place edits the server rejects on a LIVE
// read replica. Ubicloud refuses any direct PATCH to a read replica with
// "Read replicas cannot be modified directly! Please modify the parent database
// instead." (a 400 before it even reads the body), and size / ha_type / tags all
// travel on that PATCH endpoint — so changing them on a live replica can never
// succeed and would otherwise surface only as an apply-time error plus a
// non-converging plan. pg_config / pgbouncer_config are NOT blocked: they use a
// separate POST /config endpoint that does accept per-replica values. The caller
// invokes this only for a live replica (read_replica_of set, is_primary false —
// a promoted replica is a standalone primary and is freely modifiable). Compares
// plan to state so it mirrors exactly what Update would PATCH.
func replicaUpdateForbidden(plan, state models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	forbid := func(name string, planVal, stateVal attr.Value) {
		// An unknown plan value (e.g. an interpolated size that may resolve to the
		// current value) can't be proven to be a change — defer to apply rather
		// than false-positive at plan. A known value that differs is the change we
		// block.
		if planVal.IsUnknown() || planVal.Equal(stateVal) {
			return
		}
		diags.AddAttributeError(
			path.Root(name),
			"Read replica cannot be modified directly",
			"`"+name+"` cannot be changed on a live read replica — the server rejects direct modifications. Change it on the parent (primary) instead. Removing read_replica_of turns this into a standalone primary, but destroys and recreates the instance (it is not an in-place detach).",
		)
	}
	forbid("size", plan.Size, state.Size)
	forbid("ha_type", plan.HaType, state.HaType)
	forbid("tags", plan.Tags, state.Tags)
	return diags
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
				Description: "Cloud provider hosting the instance. Currently only 'aws' is supported. Required for a standard create; omit for a read replica or point-in-time restore (inherited from the source).",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(postgresCloudProviders...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"region": schema.StringAttribute{
				Description: "Cloud region (e.g. 'us-east-1'). No client-side validation; the server rejects unsupported regions. Required for a standard create; omit for a read replica or point-in-time restore (inherited from the source).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"postgres_version": schema.StringAttribute{
				Description: "Major Postgres version (e.g. '18'). The server picks the patch release within that major. Changing the major triggers destroy-and-recreate. Omit for a read replica or point-in-time restore (inherited from the source).",
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
				Description: "Instance size (VM SKU). See https://clickhouse.com/docs/cloud/managed-postgres/scaling for the supported instance families. No client-side enum; the server rejects unsupported sizes with HTTP 400 at apply time. Resizable in place. Required for a standard create; omit for a read replica or point-in-time restore (inherited from the source).",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ha_type": schema.StringAttribute{
				Description: "High-availability mode. One of 'none' (single replica), 'async' (asynchronous replica), or 'sync' (synchronous replica). Mutable post-create; an HA flip triggers a transition. Omitting the attribute preserves the prior value (the server defaults to 'none' on Create); to actively downgrade, set 'ha_type = \"none\"' explicitly. Omit for a read replica or point-in-time restore (inherited from the source).",
				Optional:    true,
				Computed:    true,
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
					mapplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Map{
					mapvalidator.SizeAtMost(50), // server MAX_TAGS_PER_RESOURCE
					mapvalidator.KeysAre(stringvalidator.LengthAtLeast(1)),
					mapvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},

			// --- Runtime configuration ---------------------------------------
			"pg_config": schema.MapAttribute{
				Description: "Postgres server parameters (pgConfig) as a key-value map. Declared parameters are the desired state — every apply sends the full map via POST /config (full replacement), so removing a key from the map removes it server-side. Set `pg_config = {}` to clear all parameters; omit the attribute to preserve the prior state (read replicas inherit the primary's parameters, and the server may surface values the configuration never declared — so it is Optional+Computed like tags). Out-of-band changes are reverted on the next apply. Some parameters require a database restart; the provider surfaces the server's restart-required hint as a warning (restart out-of-band).",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					// Optional+Computed (like tags): a read replica inherits the
					// primary's config, and GET /config can return parameters the
					// user never declared — those must be allowed into state
					// without an inconsistent-result error. UseStateForUnknown pins
					// the prior value on omission; `pg_config = {}` clears all.
					mapplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.LengthAtLeast(1)),
					mapvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"pgbouncer_config": schema.MapAttribute{
				Description: "PgBouncer connection-pooler parameters (pgBouncerConfig) as a key-value map. Same Optional+Computed semantics as pg_config; set `pgbouncer_config = {}` to clear.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Map{
					mapvalidator.KeysAre(stringvalidator.LengthAtLeast(1)),
					mapvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},

			// --- Computed ----------------------------------------------------
			"state": schema.StringAttribute{
				Description: "Server-reported state. Examples: 'creating', 'running', 'restarting', 'unavailable', 'deleting'. Forward-compatible: unknown values from the server are surfaced verbatim.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
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
				Description: "Full connection URI embedding the username and the password. Marked sensitive; the secret-redaction layer also covers it in TF_LOG=DEBUG output. Plan stability is managed in ModifyPlan: pinned to the prior value on unrelated updates, marked unknown when a password rotation is planned (it embeds the password).",
				Computed:    true,
				Sensitive:   true,
			},

			// --- Sensitive ---------------------------------------------------
			"password": schema.StringAttribute{
				Description: "Superuser password. Optional: set it to manage the password in Terraform, or omit it and the server generates one. Computed and refreshed from each GET (the server echoes it), so it always reflects the live password and an out-of-band rotation is reconciled on the next refresh. Changing this value rotates the password (PATCH /password). Must be ≥12 chars with at least one lowercase, one uppercase, and one digit. Stored in (sensitive) state.",
				Optional:    true,
				Computed:    true,
				Sensitive:   true,
				Validators:  postgresPasswordValidators(),
				// No UseStateForUnknown plan modifier: it cannot distinguish
				// "unknown because unconfigured" (server-generated → pin to
				// state) from "unknown because the configured value is an
				// unresolved interpolation" (random_password.result → a
				// rotation that must NOT be suppressed). ModifyPlan makes that
				// config-aware distinction.
			},

			// --- Provenance / immutable --------------------------------------
			"read_replica_of": schema.StringAttribute{
				Description: "ID of the primary instance to replicate. When set, this instance is created as a read replica (streaming replication) of that primary. Immutable for a live replica: changing or removing it destroys and recreates the instance as a standalone primary. The one exception is an out-of-band promotion — if you promote the replica via the API/UI (is_primary becomes true), changing or removing read_replica_of then reconciles state in place without destroying the promoted primary. Mutually exclusive with restore_to_point_in_time and with password (a replica inherits the primary's superuser).",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						readReplicaOfRequiresReplace,
						"changing or removing read_replica_of replaces the instance unless it was promoted out-of-band",
						"changing or removing `read_replica_of` replaces the instance unless it was promoted out-of-band (`is_primary` is true)",
					),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(
						path.MatchRoot("restore_to_point_in_time"),
						path.MatchRoot("password"),
					),
				},
			},
			"restore_to_point_in_time": schema.SingleNestedAttribute{
				Description: "Create this instance by restoring another Postgres instance's backup to a point in time. The whole block is create-time only: changing source_id / restore_target (re-restore to a new point) OR removing it both destroy and recreate the instance. The restored instance's name is this resource's top-level `name` and it is independent of its source. cloud_provider / region / postgres_version are inherited from the source — omit them, or set them to match (a mismatch is a plan-time error); size and ha_type must be omitted (the restored instance comes up at the backup's size and a server-assigned HA mode). Mutually exclusive with read_replica_of.",
				Optional:    true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"source_id": schema.StringAttribute{
						Description: "ID of the source instance whose backup to restore from.",
						Required:    true,
					},
					"restore_target": schema.StringAttribute{
						Description: "RFC3339 timestamp to restore to (e.g. '2026-06-01T12:00:00Z'). The server restores to the closest available recovery point at or before this time.",
						Required:    true,
					},
				},
			},
		},
	}
}

func (r *PostgresServiceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new instance via one of three mutually-exclusive paths:
// standard, read replica (read_replica_of), or point-in-time restore.
func (r *PostgresServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.PostgresServiceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Three mutually-exclusive create paths (enforced by ConflictsWith). The
	// final password (generated, supplied, or inherited) is hydrated from the
	// post-create GET below, so the create response's value isn't needed here.
	var pg *api.Postgres
	switch {
	case !plan.ReadReplicaOf.IsNull() && !plan.ReadReplicaOf.IsUnknown():
		body, d := planToReadReplicaRequest(ctx, plan)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		p, err := r.client.CreatePostgresReadReplica(ctx, plan.ReadReplicaOf.ValueString(), body)
		if err != nil {
			resp.Diagnostics.AddError("Error creating Postgres read replica", "Could not create a read replica of "+plan.ReadReplicaOf.ValueString()+": "+err.Error())
			return
		}
		pg = p
	case !plan.RestoreToPointInTime.IsNull() && !plan.RestoreToPointInTime.IsUnknown():
		sourceID, body, d := planToRestoreRequest(ctx, plan)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		p, err := r.client.RestorePostgres(ctx, sourceID, body)
		if err != nil {
			resp.Diagnostics.AddError("Error restoring Postgres service", "Could not restore a Postgres service from source "+sourceID+": "+err.Error())
			return
		}
		pg = p
	default:
		createBody, d := planToPostgresCreate(ctx, plan)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		p, _, err := r.client.CreatePostgres(ctx, createBody)
		if err != nil {
			resp.Diagnostics.AddError("Error creating Postgres service", "Could not create Postgres service: "+err.Error())
			return
		}
		pg = p
	}

	// Restore and replica creates also transition through non-running states;
	// the running-state checker treats every non-running value as "still
	// transitioning", so the same wait covers all three paths.
	if err := r.client.WaitForPostgresState(ctx, pg.Id, isPostgresStateRunning, postgresDefaultCreateTimeoutSeconds); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for Postgres service to reach 'running'",
			"Could not finish provisioning Postgres service "+pg.Id+": "+err.Error(),
		)
		return
	}

	// If the user supplied a password, rotate to it now
	// via PATCH /password (CreatePostgres always has the server generate one).
	pwIntent := decidePasswordOnCreate(plan)
	if pwIntent.Set {
		value := pwIntent.Value
		if _, err := r.client.SetPostgresPassword(ctx, pg.Id, api.PostgresPassword{Password: value}); err != nil {
			resp.Diagnostics.AddError(
				"Error setting Postgres password",
				"Provisioned Postgres service "+pg.Id+" but could not set the supplied password: "+err.Error(),
			)
			return
		}
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

	model := plan
	model.ID = types.StringValue(final.Id)
	resp.Diagnostics.Append(syncPostgresState(ctx, final, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// postgres_version is Optional+Computed; on a replica/restore create the plan
	// value is unknown unless planInheritedAttributes pinned it, and syncPostgresState
	// only fills it when the server returns a non-empty value. Guarantee it
	// resolves to a known value so resp.State.Set can't fail on a lingering
	// unknown (also covers a standard create that omits postgres_version).
	if model.PostgresVersion.IsUnknown() {
		model.PostgresVersion = types.StringNull()
	}
	// password is Computed and hydrated from the GET above (the server echoes the
	// current password — generated, supplied, or inherited). Guarantee it resolves
	// to a known value so resp.State.Set can't fail if the server transiently omits it.
	if model.Password.IsUnknown() {
		model.Password = types.StringNull()
	}

	cfg, err := r.client.GetPostgresConfig(ctx, final.Id)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Postgres configuration after create",
			"Could not read pg_config / pgbouncer_config for Postgres service "+final.Id+": "+err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(syncPostgresConfig(ctx, cfg, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *PostgresServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	utils.AlphaWarning("clickhouse_postgres_service", &resp.Diagnostics)

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
	// Note on out-of-band promotion: the API exposes no parent id, so a promoted
	// replica is detected only by is_primary flipping true (synced above). We do
	// NOT rewrite read_replica_of here — config still declares it, so clearing it
	// in state would oscillate. Instead readReplicaOfRequiresReplace lets the
	// user remove read_replica_of from config without a destroy once is_primary
	// is true, adopting the promoted instance as a standalone primary.
	//
	// syncPostgresState hydrates password from the GET (the server echoes it),
	// so state always holds the live password. This lets import recover the
	// credential and reconciles out-of-band rotations.

	cfg, err := r.client.GetPostgresConfig(ctx, state.ID.ValueString())
	if err != nil {
		// Mirror the GetPostgres 404 handling above: if the instance vanished
		// between the two GETs, drop it from state so the next apply recreates
		// it rather than failing with a confusing config error.
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Postgres configuration",
			"Could not read pg_config / pgbouncer_config for Postgres service "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(syncPostgresConfig(ctx, cfg, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update applies in-place mutations: size / ha_type / tags (PATCH /postgres),
// pg_config / pgbouncer_config (POST /config), and password rotation
// (PATCH /password). name / cloud_provider / region / postgres_version and
// restore_to_point_in_time are RequiresReplace; read_replica_of is
// RequiresReplaceIf (replace for a live replica, adopted in place once promoted
// out-of-band) so Update also handles that in-place adoption.
func (r *PostgresServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state models.PostgresServiceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updatePlan, d := buildPostgresUpdate(ctx, plan, state)
	resp.Diagnostics.Append(d...)
	configUpdate, cd := buildConfigUpdate(ctx, plan, state)
	resp.Diagnostics.Append(cd...)
	if resp.Diagnostics.HasError() {
		return
	}
	rotateValue, rotate := decidePasswordRotationOnUpdate(plan, state)

	if updatePlan.Body == nil && !configUpdate.Changed && !rotate {
		// No-op: skips the GET hydration below, so resolve any unknown ModifyPlan
		// set for a rotation that didn't happen.
		if plan.Password.IsUnknown() {
			plan.Password = state.Password
		}
		if plan.ConnectionString.IsUnknown() {
			plan.ConnectionString = state.ConnectionString
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	}

	// Instance-level PATCH (size / ha_type / tags).
	if updatePlan.Body != nil {
		if _, err := r.client.UpdatePostgres(ctx, state.ID.ValueString(), *updatePlan.Body); err != nil {
			resp.Diagnostics.AddError(
				"Error updating Postgres service",
				"Could not update Postgres service "+state.ID.ValueString()+": "+err.Error(),
			)
			return
		}
		if updatePlan.TransitionExpected {
			// Field-aware wait: poll until the server reflects the PATCHed
			// values (size / ha_type / tags) AND is running, held for a settle
			// window. A state-only wait would race the Ubicloud queue.
			predicate := buildPostgresMatchPredicate(updatePlan.Body)
			if err := r.client.WaitForPostgresMatch(ctx, state.ID.ValueString(), predicate, postgresDefaultUpdateTimeoutSeconds); err != nil {
				resp.Diagnostics.AddError(
					"Error waiting for Postgres service to apply the requested update",
					"Could not confirm Postgres service "+state.ID.ValueString()+" reflects the PATCH values: "+err.Error(),
				)
				return
			}
		}
	}

	// Config replacement (pg_config / pgbouncer_config) via POST /config.
	// Message-driven, NOT state-polled: config changes do not auto-transition
	// instance state; the server's response `message` field is the
	// restart-required contract. Full replacement of BOTH maps from the plan.
	if configUpdate.Changed {
		cfgResp, err := r.client.ReplacePostgresConfig(ctx, state.ID.ValueString(), configUpdate.Body)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating Postgres configuration",
				"Could not replace pg_config / pgbouncer_config for Postgres service "+state.ID.ValueString()+": "+err.Error(),
			)
			return
		}
		if cfgResp.Message != "" {
			resp.Diagnostics.AddWarning(
				"Postgres configuration change requires a restart",
				cfgResp.Message+" Restart out-of-band via the ClickHouse Cloud UI or API; this resource does not expose restart.",
			)
		}
	}

	// Password rotation (PATCH /password): a change to the `password` value.
	// Never part of the instance PATCH body.
	if rotate {
		value := rotateValue
		if _, err := r.client.SetPostgresPassword(ctx, state.ID.ValueString(), api.PostgresPassword{Password: value}); err != nil {
			resp.Diagnostics.AddError(
				"Error rotating Postgres password",
				"Could not rotate the password for Postgres service "+state.ID.ValueString()+": "+err.Error(),
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
	// password is Computed and hydrated from the GET above. When a rotation was
	// planned, ModifyPlan marked it unknown; guarantee it resolves to a known
	// value so resp.State.Set can't fail if the server transiently omits it.
	if plan.Password.IsUnknown() {
		plan.Password = types.StringNull()
	}

	cfg, err := r.client.GetPostgresConfig(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Postgres configuration after update",
			"Could not refresh config for Postgres service "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}
	resp.Diagnostics.Append(syncPostgresConfig(ctx, cfg, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete is a thin wrapper around DeletePostgres, which owns the
// 404-idempotent / 409-retry machinery.
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

// ModifyPlan handles:
//
//   - On create: validate the create-time attributes (cloud_provider / region /
//     size required for a standard create) and, for a read replica / restore,
//     fetch the source to validate any supplied attributes against it and pin
//     the inherited values into the plan.
//   - On update: surface an out-of-band promotion (is_primary flipped while
//     read_replica_of is still declared) as an error.
//   - On update: keep the password-coupled attributes `password` and
//     `connection_string` plan-stable. Both are Computed and hydrated from the
//     server, so state always holds a known value; the only question is the
//     planned value. When a rotation is planned the new value isn't known yet
//     (an interpolated password) so mark them
//     unknown; otherwise pin the prior state value. `password` only needs
//     setting when the user didn't configure a literal (a configured value is
//     already the known plan value); this replaces the UseStateForUnknown plan
//     modifier, which can't tell "unknown because unconfigured" from "unknown
//     because interpolated".
//
// read_replica_of and restore_to_point_in_time are plain (non-Computed)
// attributes whose RequiresReplaceIf / RequiresReplace modifiers own their
// replace decisions; ModifyPlan does not touch them.
func (r *PostgresServiceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return // destroy (no plan)
	}

	var config, plan, state models.PostgresServiceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if req.State.Raw.IsNull() {
		// --- create ----------------------------------------------------------
		// cloud_provider / region / size are required for a standard create
		// (create-scoped: ModifyPlan has prior state, so a later resize /
		// origin-block edit isn't flagged).
		resp.Diagnostics.Append(requireStandardCreateAttributes(config)...)
		resp.Diagnostics.Append(forbidEmptyConfigOnCreate(config)...)
		if resp.Diagnostics.HasError() {
			return
		}
		// For a read replica / restore, fetch the source, validate any supplied
		// attributes against it (a conflict is an error), and pin the inherited
		// values so the plan shows real values that match the post-apply read.
		r.planInheritedAttributes(ctx, config, resp)
		return
	}

	// --- update --------------------------------------------------------------
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A replace that recreates the instance from a (different) source — changing
	// read_replica_of on a live replica, or changing restore_to_point_in_time —
	// re-derives the inherited attributes from the new source, exactly like a
	// create. Without this the geometry stays pinned to the OLD source (via
	// UseStateForUnknown) while the apply provisions from the new source, which
	// Terraform reports as an inconsistent result.
	if originSourceChanged(config, state) {
		resp.Diagnostics.Append(forbidEmptyConfigOnCreate(config)...)
		if resp.Diagnostics.HasError() {
			return
		}
		r.planInheritedAttributes(ctx, config, resp)
		return
	}

	// Out-of-band promotion: a replica promoted via the API/UI flips is_primary
	// true while read_replica_of is still declared. The API exposes no parent
	// id, so is_primary is the only signal. Surface it as an error directing the
	// user to remove read_replica_of — which readReplicaOfRequiresReplace then
	// adopts in place (no destroy) precisely because is_primary is true.
	if !config.ReadReplicaOf.IsNull() && state.IsPrimary.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("read_replica_of"),
			"Read replica has been promoted to a primary",
			"This instance's is_primary is true, so it was promoted to a standalone primary outside Terraform. Remove read_replica_of from the configuration to reconcile it as a primary — that change is applied in place and does not destroy the instance.",
		)
		return
	}

	// A live read replica (is_primary false — the promotion case returned above —
	// with read_replica_of still declared) cannot be modified directly: the server
	// 400s any size / ha_type / tags PATCH. Surface that at plan time instead of an
	// apply-time error and a plan that never converges. (read_replica_of changes
	// are handled earlier: a re-point re-derives via originSourceChanged, and a
	// removal replaces via readReplicaOfRequiresReplace.)
	if !config.ReadReplicaOf.IsNull() {
		resp.Diagnostics.Append(replicaUpdateForbidden(plan, state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// password and connection_string are Computed and hydrated from the server on
	// every read, so state always holds a known value. The only question is plan
	// stability: when a rotation is planned the new value isn't known yet (a
	// an interpolated password), so mark them
	// unknown; otherwise pin the prior state value (which equals the live one).
	// `password` only needs setting when the user didn't configure a literal —
	// a configured value is already the known plan value.
	rotation := passwordRotationPlanned(config, state)
	if config.Password.IsNull() {
		pw := state.Password
		if rotation {
			pw = types.StringUnknown()
		}
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("password"), pw)...)
	}
	connStr := state.ConnectionString
	if rotation {
		connStr = types.StringUnknown()
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("connection_string"), connStr)...)
}

// planInheritedAttributes handles a read-replica / point-in-time-restore create:
// it reads the source instance, validates any user-supplied attributes against
// it (sourceAttributeConflicts), and pins the inherited values into the plan so
// it shows real values instead of "(known after apply)".
//
// Pinned: cloud_provider and region (a replica is co-located with its primary;
// a restore stays in-region — both RequiresReplace), postgres_version (no
// cross-major replica/restore), and — for a replica only — size (a streaming
// replica matches its primary's SKU). size is NOT pinned for a restore: it can
// come up at the backup's size, which may differ from the source's current
// size, and pinning a KNOWN value the apply contradicts trips an
// inconsistent-result error. ha_type and (for restore) size are left for apply
// (sourceAttributeConflicts requires them omitted, so they stay unknown and
// syncPostgresState resolves them). Skipped when the source id is unknown
// (interpolated) or the client isn't configured — the Computed attributes then
// fall back to "(known after apply)".
func (r *PostgresServiceResource) planInheritedAttributes(ctx context.Context, config models.PostgresServiceResourceModel, resp *resource.ModifyPlanResponse) {
	var sourceID string
	var isReplica bool
	switch {
	case !config.ReadReplicaOf.IsNull() && !config.ReadReplicaOf.IsUnknown():
		sourceID = config.ReadReplicaOf.ValueString()
		isReplica = true
	case !config.RestoreToPointInTime.IsNull() && !config.RestoreToPointInTime.IsUnknown():
		var rm models.PostgresRestoreModel
		resp.Diagnostics.Append(config.RestoreToPointInTime.As(ctx, &rm, basetypes.ObjectAsOptions{})...)
		if resp.Diagnostics.HasError() {
			return
		}
		if rm.SourceID.IsNull() || rm.SourceID.IsUnknown() {
			return
		}
		sourceID = rm.SourceID.ValueString()
	default:
		return // standard create — attributes come from config
	}
	if r.client == nil || sourceID == "" {
		return
	}

	src, err := r.client.GetPostgres(ctx, sourceID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Cannot read the source Postgres instance",
			"Could not read source instance "+sourceID+" for a read replica / restore: "+err.Error(),
		)
		return
	}

	// Reject supplied attributes that conflict with (or aren't taken from) the source.
	resp.Diagnostics.Append(sourceAttributeConflicts(config, src, isReplica)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Pin or reset EVERY inherited attribute so none can survive as a stale
	// prior-state value (UseStateForUnknown) when this runs on a source-change
	// replace: pin the ones reproduced verbatim from the source; mark the
	// server-/backup-determined ones unknown so apply fills them.
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("cloud_provider"), types.StringValue(src.Provider))...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("region"), types.StringValue(src.Region))...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("postgres_version"), stringOrUnknown(src.PostgresVersion))...)
	if isReplica {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("size"), stringOrUnknown(src.Size))...)
	} else {
		// restore: the new instance comes up at the backup's size, not the source's.
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("size"), types.StringUnknown())...)
	}
	// ha_type is server-assigned for a new replica/restore.
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("ha_type"), types.StringUnknown())...)
}

// stringOrUnknown returns a known value, or Unknown for an empty string, so an
// omitted server field doesn't pin a stale prior-state value on a replace.
func stringOrUnknown(s string) types.String {
	if s == "" {
		return types.StringUnknown()
	}
	return types.StringValue(s)
}

// readReplicaOfRequiresReplace replaces the instance when read_replica_of is
// changed or removed — EXCEPT once it has been promoted out-of-band
// (is_primary=true), where the instance is already a standalone primary and the
// change is reconciled in place. is_primary comes from prior state (a refresh
// before the plan surfaces an out-of-band promotion); when it can't be read it
// defaults to false, so the safe "replace a live replica" path wins.
func readReplicaOfRequiresReplace(ctx context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return // create or destroy: nothing to replace
	}
	var isPrimary types.Bool
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("is_primary"), &isPrimary)...)
	resp.RequiresReplace = readReplicaOfShouldReplace(req.StateValue, req.PlanValue, isPrimary.ValueBool())
}

// readReplicaOfShouldReplace is the pure decision behind readReplicaOfRequiresReplace:
// changing or removing read_replica_of replaces a live replica, but once the
// instance has been promoted out-of-band (is_primary=true) it's already a
// standalone primary, so the change is reconciled in place. Unchanged → no
// replace. (is_primary defaults to false when unreadable, so the safe
// "replace a live replica" path wins.)
func readReplicaOfShouldReplace(stateVal, planVal types.String, isPrimary bool) bool {
	if stateVal.Equal(planVal) {
		return false // unchanged
	}
	return !isPrimary
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isPostgresStateRunning is the state-checker passed to WaitForPostgresState.
// Forward-compatible: anything other than 'running' is treated as still
// transitioning, including server states the provider hasn't learned yet.
func isPostgresStateRunning(s string) bool { return s == api.PostgresStateRunning }

// planToPostgresCreate maps a fully-resolved plan into the POST /postgres body.
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
	if tags != nil {
		body.Tags = *tags
	}

	pgConfig, d := planConfigToMap(ctx, plan.PgConfig)
	diags.Append(d...)
	pbConfig, d := planConfigToMap(ctx, plan.PgBouncerConfig)
	diags.Append(d...)
	if diags.HasError() {
		return api.PostgresCreate{}, diags
	}
	body.PgConfig = pgConfig
	body.PgBouncerConfig = pbConfig

	return body, diags
}

// planToReadReplicaRequest builds the POST /readReplica body. The replica's
// name is the resource's top-level name; tags / config carry over the same as
// a standard create. No password (a replica inherits the primary's superuser).
func planToReadReplicaRequest(ctx context.Context, plan models.PostgresServiceResourceModel) (api.PostgresReadReplicaRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	body := api.PostgresReadReplicaRequest{Name: plan.Name.ValueString()}

	tags, d := planTagsToAPI(ctx, plan.Tags)
	diags.Append(d...)
	if tags != nil {
		body.Tags = *tags
	}
	pgConfig, d := planConfigToMap(ctx, plan.PgConfig)
	diags.Append(d...)
	pbConfig, d := planConfigToMap(ctx, plan.PgBouncerConfig)
	diags.Append(d...)
	if diags.HasError() {
		return api.PostgresReadReplicaRequest{}, diags
	}
	body.PgConfig = pgConfig
	body.PgBouncerConfig = pbConfig
	return body, diags
}

// planToRestoreRequest builds the POST /restoredService body and returns the
// source instance ID. The restored instance's name is the top-level `name`.
func planToRestoreRequest(ctx context.Context, plan models.PostgresServiceResourceModel) (string, api.PostgresRestoreRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	var rm models.PostgresRestoreModel
	diags.Append(plan.RestoreToPointInTime.As(ctx, &rm, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return "", api.PostgresRestoreRequest{}, diags
	}

	body := api.PostgresRestoreRequest{
		Name:          plan.Name.ValueString(),
		RestoreTarget: rm.RestoreTarget.ValueString(),
	}
	tags, d := planTagsToAPI(ctx, plan.Tags)
	diags.Append(d...)
	if tags != nil {
		body.Tags = *tags
	}
	pgConfig, d := planConfigToMap(ctx, plan.PgConfig)
	diags.Append(d...)
	pbConfig, d := planConfigToMap(ctx, plan.PgBouncerConfig)
	diags.Append(d...)
	if diags.HasError() {
		return "", api.PostgresRestoreRequest{}, diags
	}
	body.PgConfig = pgConfig
	body.PgBouncerConfig = pbConfig
	return rm.SourceID.ValueString(), body, diags
}

// postgresUpdatePlan bundles the two artifacts buildPostgresUpdate produces.
//
//   - Body == nil           → no diff; caller skips the PATCH entirely.
//   - Body != nil           → sparse PATCH body (size, ha_type, tags).
//   - TransitionExpected    → server processes the mutation as a state
//     transition (size, ha_type); caller follows up with WaitForPostgresMatch
//     using buildPostgresMatchPredicate(Body).
type postgresUpdatePlan struct {
	Body               *api.PostgresUpdate
	TransitionExpected bool
}

// buildPostgresUpdate diffs plan vs state and produces a sparse PATCH body,
// or Body=nil when nothing changed. See diffTags for the tag contract; the
// server's PUT-like tag semantics mean we re-assert state tags on any PATCH.
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

	tagsChanged, mappedFromPlan, d := diffTags(ctx, plan, state)
	diags.Append(d...)
	if diags.HasError() {
		return postgresUpdatePlan{}, diags
	}
	if tagsChanged {
		update.Tags = mappedFromPlan
		changed = true
	} else if changed {
		// Tags unchanged but size/ha_type changing — re-assert state tags so
		// the server's PUT-like tag semantics don't wipe them.
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

// buildPostgresMatchPredicate returns a predicate for WaitForPostgresMatch that
// succeeds once the instance is running AND the server reflects the values we
// PATCHed (size / ha_type / tags). This is field-aware so the wait doesn't
// settle on a stale 'running' before Ubicloud commits the queued change.
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

// diffTags compares plan vs state tags. Returns (changed, body): nil body when
// plan is Unknown (defense against a missing UseStateForUnknown) or equal;
// &[]Tag{} to clear; &mapped to replace.
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

// planTagsToAPI extracts an *[]api.Tag from a Terraform map(string,string).
// Returns nil for null/unknown so the caller can distinguish "leave alone"
// from "explicit empty".
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
// Does not touch pg_config / pgbouncer_config; those are synced separately by
// syncPostgresConfig.
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

// ---------------------------------------------------------------------------
// Config helpers (pg_config / pgbouncer_config)
// ---------------------------------------------------------------------------

// planConfigToMap extracts an api.PgConfigMap from a pg_config / pgbouncer_config
// map attribute. Returns nil for null/unknown so the caller can express "no
// parameters" — ReplacePostgresConfig defaults a nil map to {} on the wire
// (full replacement / clear).
func planConfigToMap(ctx context.Context, configMap types.Map) (api.PgConfigMap, diag.Diagnostics) {
	var diags diag.Diagnostics
	if configMap.IsNull() || configMap.IsUnknown() {
		return nil, diags
	}
	raw := make(map[string]string, len(configMap.Elements()))
	diags.Append(configMap.ElementsAs(ctx, &raw, false)...)
	if diags.HasError() {
		return nil, diags
	}
	return api.PgConfigMap(raw), diags
}

// apiConfigToMapValue converts an api.PgConfigMap into a Terraform string map.
// An empty/nil map becomes a known empty map (not null), mirroring
// apiTagsToMapValue, so an Optional+Computed `pg_config = {}` round-trips.
func apiConfigToMapValue(config api.PgConfigMap) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics
	// Empty → a known empty map (not null), mirroring apiTagsToMapValue: the
	// attribute is Optional+Computed, so `pg_config = {}` (clear all params)
	// must round-trip to an empty map without an inconsistent-result error.
	elems := make(map[string]attr.Value, len(config))
	for k, v := range config {
		elems[k] = types.StringValue(v)
	}
	m, d := types.MapValue(types.StringType, elems)
	diags.Append(d...)
	return m, diags
}

// syncPostgresConfig writes a GET/POST /config response into the model's
// pg_config / pgbouncer_config attributes.
func syncPostgresConfig(_ context.Context, config *api.PostgresConfig, model *models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	pgMap, d := apiConfigToMapValue(config.PgConfig)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	pbMap, d := apiConfigToMapValue(config.PgBouncerConfig)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	model.PgConfig = pgMap
	model.PgBouncerConfig = pbMap
	return diags
}

// postgresConfigUpdate bundles what buildConfigUpdate produces.
type postgresConfigUpdate struct {
	Changed bool
	Body    api.PostgresConfig
}

// buildConfigUpdate diffs pg_config / pgbouncer_config between plan and state.
// When EITHER differs, Changed=true with Body carrying BOTH maps from the plan
// (full replacement). When neither differs, Changed=false.
func buildConfigUpdate(ctx context.Context, plan, state models.PostgresServiceResourceModel) (postgresConfigUpdate, diag.Diagnostics) {
	var diags diag.Diagnostics
	if plan.PgConfig.Equal(state.PgConfig) && plan.PgBouncerConfig.Equal(state.PgBouncerConfig) {
		return postgresConfigUpdate{Changed: false}, diags
	}
	pgConfig, d := planConfigToMap(ctx, plan.PgConfig)
	diags.Append(d...)
	pbConfig, d := planConfigToMap(ctx, plan.PgBouncerConfig)
	diags.Append(d...)
	if diags.HasError() {
		return postgresConfigUpdate{}, diags
	}
	return postgresConfigUpdate{
		Changed: true,
		Body:    api.PostgresConfig{PgConfig: pgConfig, PgBouncerConfig: pbConfig},
	}, diags
}

// ---------------------------------------------------------------------------
// Password helpers
// ---------------------------------------------------------------------------

// postgresPasswordValidators enforces the server's (Ubicloud) complexity
// rules: ≥12 chars, ≥1 lowercase, ≥1 uppercase, ≥1 digit. Validators fire only
// on config values, so the server-generated password is never checked.
func postgresPasswordValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtLeast(12),
		stringvalidator.RegexMatches(regexp.MustCompile(`[a-z]`), "must contain at least one lowercase letter"),
		stringvalidator.RegexMatches(regexp.MustCompile(`[A-Z]`), "must contain at least one uppercase letter"),
		stringvalidator.RegexMatches(regexp.MustCompile(`[0-9]`), "must contain at least one digit"),
	}
}

// passwordCreateIntent captures whether Create should rotate to a user-supplied
// password after provisioning (CreatePostgres always server-generates one). The
// resulting password — generated or supplied — is hydrated into state from the
// post-create GET either way, so there's no value to persist here.
type passwordCreateIntent struct {
	Set   bool   // call SetPostgresPassword?
	Value string // value to PATCH (when Set)
}

// decidePasswordOnCreate resolves the post-create password action: CreatePostgres
// always has the server generate a password, so we only PATCH a new one when the
// user supplied a literal `password`. The resulting password — generated or
// supplied — is hydrated into state from the post-create GET either way.
func decidePasswordOnCreate(plan models.PostgresServiceResourceModel) passwordCreateIntent {
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() {
		if pw := plan.Password.ValueString(); pw != "" {
			return passwordCreateIntent{Set: true, Value: pw}
		}
	}
	return passwordCreateIntent{Set: false}
}

// decidePasswordRotationOnUpdate returns the new password and whether to rotate:
// a change to the `password` value rotates to it via PATCH /password.
func decidePasswordRotationOnUpdate(plan, state models.PostgresServiceResourceModel) (value string, rotate bool) {
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() && !plan.Password.Equal(state.Password) {
		if pw := plan.Password.ValueString(); pw != "" {
			return pw, true
		}
	}
	return "", false
}

// passwordRotationPlanned reports whether a password rotation is planned, used
// by ModifyPlan to decide whether connection_string must be marked unknown. A
// rotation is planned when the configured password is an unresolved interpolation
// (unknown) or differs from the current state value.
func passwordRotationPlanned(config, state models.PostgresServiceResourceModel) bool {
	if config.Password.IsUnknown() {
		return true
	}
	return !config.Password.IsNull() && !config.Password.Equal(state.Password)
}
