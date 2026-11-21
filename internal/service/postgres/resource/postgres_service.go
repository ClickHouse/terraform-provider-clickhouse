package resource

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	internalplanmodifier "github.com/ClickHouse/terraform-provider-clickhouse/internal/planmodifier"
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
	_ resource.ResourceWithUpgradeState   = &PostgresServiceResource{}
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

// State-dependent rules are NOT enforced here: ValidateConfig is stateless, so
// it can't tell a create from an update or read prior state. That covers the
// create-time attribute rules (required for a standard create; inherited from
// the source for a replica / restore — enforcing them here would misfire when an
// existing instance drops its origin block) and the live-replica modification
// block (which needs is_primary from prior state). All of these live in
// ModifyPlan, which has prior state.
func (r *PostgresServiceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cloudProvider types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("cloud_provider"), &cloudProvider)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cloudProvider.ValueString() == "gcp" {
		resp.Diagnostics.AddAttributeWarning(
			path.Root("cloud_provider"),
			"Postgres on GCP is in private preview",
			"Postgres on GCP is in private preview. Your organization must have access to GCP and the selected Postgres region. Contact ClickHouse support to request access.",
		)
	}
	var haType types.String
	var standbyCount types.Int64
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("ha_type"), &haType)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("standby_count"), &standbyCount)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(validateStandbyCount(haType, standbyCount)...)
	}
	var ipAccess types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("ip_access"), &ipAccess)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(validatePostgresIPAccess(ipAccess)...)
	}
}

// validateStandbyCount rejects a standby_count that ha_type does not accept.
// Unknown (interpolated) values and unlisted ha_type values are left to their
// own validators and the server.
func validateStandbyCount(haType types.String, standbyCount types.Int64) diag.Diagnostics {
	var diags diag.Diagnostics
	if haType.IsNull() || haType.IsUnknown() || standbyCount.IsNull() || standbyCount.IsUnknown() {
		return diags
	}
	allowed, ok := PostgresStandbyCounts[haType.ValueString()]
	if !ok || slices.Contains(allowed, standbyCount.ValueInt64()) {
		return diags
	}
	supported := make([]string, len(allowed))
	for i, n := range slices.Sorted(slices.Values(allowed)) {
		supported[i] = strconv.FormatInt(n, 10)
	}
	diags.AddAttributeError(
		path.Root("standby_count"),
		"Unsupported standby_count for ha_type",
		fmt.Sprintf("`standby_count` %d is not supported for `ha_type` %q. Supported values: %s.", standbyCount.ValueInt64(), haType.ValueString(), strings.Join(supported, ", ")),
	)
	return diags
}

// defaultStandbyCount returns the standby count the server assigns to haType
// when none is requested, or unknown for an unknown or unlisted haType.
func defaultStandbyCount(haType types.String) types.Int64 {
	if counts, ok := PostgresStandbyCounts[haType.ValueString()]; ok && !haType.IsNull() && !haType.IsUnknown() {
		return types.Int64Value(counts[0])
	}
	return types.Int64Unknown()
}

// planStandbyCount resolves standby_count when the config omits it: the prior
// value while ha_type is unchanged, otherwise the default for the planned
// ha_type, matching the server's PATCH behavior. state is nil on create. A
// state written before standby_count existed has no prior value, so the count
// stays unknown and the post-apply read fills it.
func planStandbyCount(config, plan models.PostgresServiceResourceModel, state *models.PostgresServiceResourceModel) types.Int64 {
	if !config.StandbyCount.IsNull() {
		return plan.StandbyCount
	}
	if state != nil && plan.HaType.Equal(state.HaType) {
		if state.StandbyCount.IsNull() {
			return types.Int64Unknown()
		}
		return state.StandbyCount
	}
	return defaultStandbyCount(plan.HaType)
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
// the backup's size, not the source's current one) and ha_type / standby_count
// (server-assigned for a new replica/restore, set in place afterward) are NOT taken from the
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
	mustOmit := func(name string, configVal attr.Value, reason string) {
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
	mustOmit("standby_count", config.StandbyCount, "HA mode is server-assigned for a new replica/restore and changed in place afterward")
	return diags
}

// requireDeclaredCredential enforces the ClickHouse-style credential rule at
// plan time: a standard service must declare password or password_wo; a read
// replica or point-in-time restore inherits its credential from the origin.
// This is deliberately NOT an AtLeastOneOf schema validator — those also run
// for destroy plans (a credential-less config could never be destroyed) and
// know nothing of prior state. Callers scope it: ModifyPlan's create branch
// applies it to every new instance; the update branch only to primaries, so a
// replica adopted by import — is_primary=false with no read_replica_of in
// config, because the API exposes no parent id — needs no credential. Unknown
// values defer to apply (an interpolated credential or origin may resolve).
func requireDeclaredCredential(config models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if config.Password.IsUnknown() || config.PasswordWO.IsUnknown() ||
		config.ReadReplicaOf.IsUnknown() || config.RestoreToPointInTime.IsUnknown() {
		return diags
	}
	if configIsOrigin(config) || !config.Password.IsNull() || !config.PasswordWO.IsNull() {
		return diags
	}
	diags.AddAttributeError(
		path.Root("password"),
		"Missing credential",
		"A standard Postgres service requires `password` or `password_wo` (a read replica or point-in-time restore inherits its credential from the source instead).",
	)
	return diags
}

// warnCredentialOnReplica warns when a live replica adopted by import (no
// read_replica_of in config — the API exposes no parent id, so the schema
// ConflictsWith cannot fire) declares a credential: the server rejects a
// direct password change on a replica, so an in-place rotation apply can
// never converge. A WARNING, not an error, because the same config can be a
// legitimate replacement into a standalone primary (e.g. a name change) —
// that replacement's create-side plan re-runs with null prior state and the
// credential is then required. Unknown values defer to apply.
func warnCredentialOnReplica(config models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if config.Password.IsUnknown() || config.PasswordWO.IsUnknown() ||
		config.ReadReplicaOf.IsUnknown() || config.RestoreToPointInTime.IsUnknown() {
		return diags
	}
	if configIsOrigin(config) || (config.Password.IsNull() && config.PasswordWO.IsNull()) {
		return diags
	}
	diags.AddAttributeWarning(
		path.Root("password"),
		"Read replica cannot take a credential",
		"This instance is a live read replica (is_primary is false): it inherits the primary's superuser and the server rejects a direct password change, so an in-place apply of `password` / `password_wo` will fail. Remove the credential — unless this plan replaces the instance (destroy and recreate as a standalone primary), where it is required.",
	)
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
// instead." (a 400 before it even reads the body), and size / ha_type / standby_count / tags all
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
	forbid("standby_count", plan.StandbyCount, state.StandbyCount)
	forbid("tags", plan.Tags, state.Tags)
	forbid("deletion_protection", plan.DeletionProtection, state.DeletionProtection)
	return diags
}

// forbidDeletionProtectionOnReplicaCreate rejects deletion_protection = true on
// a new read replica: the setting is applied with a PATCH after create, and the
// server rejects any direct PATCH to a read replica. Unknown values defer.
func forbidDeletionProtectionOnReplicaCreate(config models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if config.ReadReplicaOf.IsNull() || config.ReadReplicaOf.IsUnknown() {
		return diags
	}
	if config.DeletionProtection.IsUnknown() || !config.DeletionProtection.ValueBool() {
		return diags
	}
	diags.AddAttributeError(
		path.Root("deletion_protection"),
		"Deletion protection not available for a read replica",
		"`deletion_protection = true` cannot be set on a read replica: the provider applies it with an update after create, and the server rejects direct modifications to a read replica. Omit it or set it to false.",
	)
	return diags
}

func (r *PostgresServiceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// v1: removed connection_string (the API no longer returns it once
		// credential redaction is enabled); added password_wo/password_wo_version.
		Version:             1,
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
				Description: "Cloud provider hosting the instance. Supported values are 'aws' and 'gcp'. Postgres on GCP is in private preview; contact ClickHouse support to enable access for your organization and region. Required for a standard create; omit for a read replica or point-in-time restore (inherited from the source).",
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
				Description: "Cloud region (e.g. 'us-east-1' for AWS or 'us-west1' for GCP). No client-side validation; the server rejects unsupported regions. Required for a standard create; omit for a read replica or point-in-time restore (inherited from the source).",
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
			"standby_count": schema.Int64Attribute{
				Description: "Optional number of standbys. For 1 Standby Synchronous (Preview), which uses one standby and a WAL listener, set `ha_type = \"sync\"` and `standby_count = 1`. See [High availability](https://clickhouse.com/docs/cloud/managed-postgres/high-availability) for the options.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.OneOf(0, 1, 2),
					int64validator.AlsoRequires(path.MatchRoot("ha_type")),
				},
			},
			"ip_access": postgresIPAccessSchema(),
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

			"deletion_protection": schema.BoolAttribute{
				Description: "Prevent the Postgres service from being deleted. While `true`, `terraform destroy` and any change that replaces the service fail. To delete the service, first set this to `false` in its own apply. Removing the attribute from configuration leaves the current setting unchanged. The API does not accept this setting on create, so the provider turns it on with an update once the new service is running. Only available for organizations with deletion protection enabled. Cannot be set on a live read replica (the server rejects direct modifications to replicas).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					internalplanmodifier.UseStateWhenConfigNull(),
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
			// --- Sensitive ---------------------------------------------------
			"password": schema.StringAttribute{
				Description: "Superuser password. Config-owned: the API does not return the password, so Terraform manages exactly the value declared here and never reads it back. One of `password` or `password_wo` is required for a standard service; forbidden for a read replica (it inherits the primary's superuser); optional for a point-in-time restore (omit to keep the source's password, which Terraform then does not track). Changing this value rotates the password (PATCH /password). Must be ≥12 chars with at least one lowercase, one uppercase, and one digit. Stored in (sensitive) state — prefer `password_wo` to keep it out of state. `terraform import` cannot recover the live password — the configured value is rotated in on the first apply after import.",
				Optional:    true,
				Sensitive:   true,
				// The "one of password/password_wo is required for a standard
				// service" rule is enforced in ModifyPlan (requireDeclaredCredential),
				// NOT as an AtLeastOneOf schema validator: schema validators also
				// run for destroy plans (a credential-less config could never be
				// destroyed) and for imported replicas, whose config legitimately
				// has neither a credential nor read_replica_of.
				Validators: append(
					postgresPasswordValidators(),
					stringvalidator.ConflictsWith(path.MatchRoot("password_wo")),
				),
			},
			"password_wo": schema.StringAttribute{
				Description: "Superuser password, write-only: applied to the service but never persisted to Terraform state (requires Terraform >= 1.11). Preferred over `password`. Requires `password_wo_version`; increment the version to rotate to the current `password_wo` value. Same complexity rules as `password`. Forbidden for a read replica.",
				Optional:    true,
				Sensitive:   true,
				WriteOnly:   true,
				// Credential requirement enforced in ModifyPlan — see password.
				Validators: append(
					postgresPasswordValidators(),
					stringvalidator.AlsoRequires(path.MatchRoot("password_wo_version")),
				),
			},
			"password_wo_version": schema.Int64Attribute{
				Description: "Version number for `password_wo`. Increment to trigger a password rotation using the current `password_wo` value.",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AlsoRequires(path.MatchRoot("password_wo")),
				},
			},

			// --- Provenance / immutable --------------------------------------
			"read_replica_of": schema.StringAttribute{
				Description: "ID of the primary instance to replicate. When set, this instance is created as a read replica (streaming replication) of that primary. Immutable for a live replica: changing or removing it destroys and recreates the instance as a standalone primary. The one exception is an out-of-band promotion — if you promote the replica via the API/UI (is_primary becomes true), changing or removing read_replica_of then reconciles state in place without destroying the promoted primary. Mutually exclusive with restore_to_point_in_time and with password/password_wo (a replica inherits the primary's superuser). After an out-of-band promotion, removing read_replica_of requires declaring password or password_wo, which rotates the promoted primary's superuser password.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						readReplicaOfRequiresReplace,
						"changing or removing read_replica_of replaces the instance unless it was promoted out-of-band",
						"changing or removing `read_replica_of` replaces the instance unless it was promoted out-of-band (`is_primary` is true)",
					),
				},
				Validators: []validator.String{
					// Non-empty: a known "" would count as a declared origin,
					// waiving the plan-time credential/geometry requirements and
					// sending the create to a malformed API path.
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ConflictsWith(
						path.MatchRoot("restore_to_point_in_time"),
						path.MatchRoot("password"),
						path.MatchRoot("password_wo"),
						path.MatchRoot("password_wo_version"),
					),
				},
			},
			"restore_to_point_in_time": schema.SingleNestedAttribute{
				Description: "Create this instance by restoring another Postgres instance's backup to a point in time. The whole block is create-time only: changing source_id / restore_target (re-restore to a new point) OR removing it both destroy and recreate the instance. The restored instance's name is this resource's top-level `name` and it is independent of its source. cloud_provider / region / postgres_version are inherited from the source — omit them, or set them to match (a mismatch is a plan-time error); size, ha_type and standby_count must be omitted (the restored instance comes up at the backup's size and a server-assigned HA mode). Mutually exclusive with read_replica_of.",
				Optional:    true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"source_id": schema.StringAttribute{
						Description: "ID of the source instance whose backup to restore from.",
						Required:    true,
						Validators: []validator.String{
							stringvalidator.LengthAtLeast(1),
						},
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
	utils.BetaWarning("clickhouse_postgres_service", &resp.Diagnostics)

	var plan, config models.PostgresServiceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Three mutually-exclusive create paths (enforced by ConflictsWith).
	// CreatePostgres always has the server generate an initial password; when
	// the config declares one (password, or write-only password_wo — never
	// present in the plan) it is rotated in below. The password is
	// config-owned — never read back from the API.
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

	// Keep the allocated service tracked if a later configuration step fails.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(pg.Id))...)
	if resp.Diagnostics.HasError() {
		return
	}
	if configIsOrigin(plan) && !plan.IpAccessList.IsNull() && !plan.IpAccessList.IsUnknown() {
		if err := r.updatePostgresIPAccess(ctx, pg.Id, plan.IpAccessList); err != nil {
			resp.Diagnostics.AddError("Error configuring Postgres IP access", "Created Postgres service "+pg.Id+" but could not apply its access list: "+err.Error())
			return
		}
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

	// Rotate to the declared credential (server always generates an initial one).
	pwIntent := decidePasswordOnCreate(plan, config)
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

	// Deletion protection is not accepted on create, so turn it on now. Done
	// last so a failure in an earlier step never leaves a protected service
	// that Terraform has to clean up.
	var sentProtection, reportedProtection *bool
	if !plan.DeletionProtection.IsNull() && !plan.DeletionProtection.IsUnknown() && plan.DeletionProtection.ValueBool() {
		sentProtection = plan.DeletionProtection.ValueBoolPointer()
		body := api.PostgresUpdate{DeletionProtection: sentProtection}
		// The server treats tags as PUT-like on PATCH, so re-assert them.
		tags, d := planTagsToAPI(ctx, plan.Tags)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		if tags != nil && len(*tags) > 0 {
			body.Tags = tags
		}
		updated, err := r.client.UpdatePostgres(ctx, pg.Id, body)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error enabling Postgres deletion protection",
				"Provisioned Postgres service "+pg.Id+" but could not turn on deletion protection: "+err.Error(),
			)
			return
		}
		if updated != nil {
			reportedProtection = updated.DeletionProtection
		}
	}

	// Re-read to pick up hostname / created_at / final state.
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
	if reportedProtection == nil {
		reportedProtection = final.DeletionProtection
	}
	if detail := settlePostgresProtection(plan.DeletionProtection, sentProtection, reportedProtection, &model); detail != "" {
		resp.Diagnostics.AddError("Postgres Deletion Protection Not Applied", detail+" Terraform marks the new service as tainted, so run terraform untaint on it before applying again, or the next apply replaces it.")
	}
	// postgres_version is Optional+Computed; on a replica/restore create the plan
	// value is unknown unless planInheritedAttributes pinned it, and syncPostgresState
	// only fills it when the server returns a non-empty value. Guarantee it
	// resolves to a known value so resp.State.Set can't fail on a lingering
	// unknown (also covers a standard create that omits postgres_version).
	if model.PostgresVersion.IsUnknown() {
		model.PostgresVersion = types.StringNull()
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
	// password is config-owned and never read from the server (credential
	// redaction means GET does not return it): state keeps the declared value,
	// import cannot recover the live credential, and an out-of-band rotation
	// is not detected — the next Terraform-driven rotation re-asserts config.

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

// Update applies in-place mutations: size / ha_type / standby_count / tags / ip_access (PATCH /postgres),
// pg_config / pgbouncer_config (POST /config), and password rotation
// (PATCH /password). name / cloud_provider / region / postgres_version and
// restore_to_point_in_time are RequiresReplace; read_replica_of is
// RequiresReplaceIf (replace for a live replica, adopted in place once promoted
// out-of-band) so Update also handles that in-place adoption.
func (r *PostgresServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning("clickhouse_postgres_service", &resp.Diagnostics)

	var plan, state, config models.PostgresServiceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
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
	rotateValue, rotate := decidePasswordRotationOnUpdate(plan, state, config)
	ipAccessChanged := !plan.IpAccessList.IsNull() && !plan.IpAccessList.IsUnknown() && !plan.IpAccessList.Equal(state.IpAccessList)

	// An unknown standby_count (state written before the attribute existed)
	// needs the read below to resolve it.
	if updatePlan.Body == nil && !configUpdate.Changed && !rotate && !ipAccessChanged && !plan.StandbyCount.IsUnknown() {
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	}

	// Instance-level PATCH (size / ha_type / standby_count / tags / deletion_protection).
	plannedProtection := plan.DeletionProtection
	var sentProtection, reportedProtection *bool
	if updatePlan.Body != nil {
		updated, err := r.client.UpdatePostgres(ctx, state.ID.ValueString(), *updatePlan.Body)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating Postgres service",
				"Could not update Postgres service "+state.ID.ValueString()+": "+err.Error(),
			)
			return
		}
		sentProtection = updatePlan.Body.DeletionProtection
		if updated != nil {
			reportedProtection = updated.DeletionProtection
		}
		if updatePlan.TransitionExpected {
			// Field-aware wait: poll until the server reflects the PATCHed
			// values (size / ha_type / standby_count / tags) AND is running, held for a settle
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

	// Password rotation (PATCH /password): a password_wo_version bump (rotating
	// to the config-read, write-only password_wo) or, otherwise, a change to the
	// `password` value. Never part of the instance PATCH body.
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

	if ipAccessChanged {
		if err := r.updatePostgresIPAccess(ctx, state.ID.ValueString(), plan.IpAccessList); err != nil {
			resp.Diagnostics.AddError("Error updating Postgres IP access", "Could not update IP access for Postgres service "+state.ID.ValueString()+": "+err.Error())
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
	// The GET is the most recent value; fall back to the PATCH response.
	if pg.DeletionProtection != nil {
		reportedProtection = pg.DeletionProtection
	}
	if detail := settlePostgresProtection(plannedProtection, sentProtection, reportedProtection, &plan); detail != "" {
		resp.Diagnostics.AddError("Postgres Deletion Protection Not Applied", detail)
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
		if errors.Is(err, api.ErrPostgresDeletionProtected) {
			resp.Diagnostics.AddError(
				"Postgres Service Is Protected From Deletion",
				"Deletion protection is enabled for this Postgres service. Set deletion_protection = false and run terraform apply, then delete the service. "+
					"If this delete is part of a change that replaces the service (any attribute that forces replacement, or -replace), "+
					"turn protection off in its own apply, without that change, first. "+
					"If the resource is tainted, run terraform untaint first. "+
					"If the resource is no longer in your configuration, add it back with deletion_protection = false and apply, or turn protection off in the ClickHouse Cloud console. "+
					"If Terraform already created a replacement (create_before_destroy), turn protection off for Postgres service "+state.ID.ValueString()+" in the ClickHouse Cloud console, then apply again.",
			)
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
	utils.BetaWarning("clickhouse_postgres_service", &resp.Diagnostics)

	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// ModifyPlan handles:
//
//   - On create: validate the create-time attributes (cloud_provider / region /
//     size and a declared credential — password/password_wo — required for a
//     standard create) and, for a read replica / restore, fetch the source to
//     validate any supplied attributes against it and pin the inherited values
//     into the plan. Replacements re-run this branch: Terraform Core re-plans
//     the create side of a replace with a NULL prior state, so these checks
//     also cover detach/ForceNew/-replace plans.
//   - On update: require a declared credential for primaries (a live replica
//     adopted by import is exempt — it can take no credential — but declaring
//     one on it draws a plan-time warning, since the apply would fail).
//   - On update: surface an out-of-band promotion (is_primary flipped while
//     read_replica_of is still declared) as an error.
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
		resp.Diagnostics.Append(requireDeclaredCredential(config)...)
		resp.Diagnostics.Append(forbidEmptyConfigOnCreate(config)...)
		resp.Diagnostics.Append(forbidDeletionProtectionOnReplicaCreate(config)...)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("standby_count"), planStandbyCount(config, plan, nil))...)
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

	plan.StandbyCount = planStandbyCount(config, plan, &state)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("standby_count"), plan.StandbyCount)...)

	// Primaries must declare a credential (or an origin). A live replica —
	// known is_primary=false, including one adopted by import where
	// read_replica_of is absent because the API exposes no parent id — is
	// exempt (it can take no credential), but declaring one on it draws a
	// warning: the in-place rotation apply would fail. Replacement plans
	// (detach, rename, ForceNew) are covered separately: Terraform Core
	// re-plans the create side of a replace with a null prior state,
	// re-running the create branch's unconditional requirement.
	if state.IsPrimary.ValueBool() {
		resp.Diagnostics.Append(requireDeclaredCredential(config)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else if !state.IsPrimary.IsNull() {
		resp.Diagnostics.Append(warnCredentialOnReplica(config)...)
	}

	// A replace that recreates the instance from a (different) source — changing
	// read_replica_of on a live replica, or changing restore_to_point_in_time —
	// re-derives the inherited attributes from the new source, exactly like a
	// create. Without this the geometry stays pinned to the OLD source (via
	// UseStateForUnknown) while the apply provisions from the new source, which
	// Terraform reports as an inconsistent result.
	if originSourceChanged(config, state) {
		resp.Diagnostics.Append(forbidEmptyConfigOnCreate(config)...)
		resp.Diagnostics.Append(forbidDeletionProtectionOnReplicaCreate(config)...)
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
	// 400s any size / ha_type / standby_count / tags PATCH. Surface that at plan time instead of an
	// apply-time error and a plan that never converges. (read_replica_of changes
	// are handled earlier: a re-point re-derives via originSourceChanged, and a
	// removal replaces via readReplicaOfRequiresReplace.)
	if !config.ReadReplicaOf.IsNull() {
		resp.Diagnostics.Append(replicaUpdateForbidden(plan, state)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}
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
// inconsistent-result error. ha_type, standby_count and (for restore) size are left for apply
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
	// ha_type and standby_count are server-assigned for a new replica/restore.
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("ha_type"), types.StringUnknown())...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("standby_count"), types.Int64Unknown())...)
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
	ipAccess, ipDiags := planPostgresIPAccess(ctx, plan.IpAccessList)
	diags.Append(ipDiags...)
	body.IpAccessList = ipAccess
	if !plan.PostgresVersion.IsNull() && !plan.PostgresVersion.IsUnknown() {
		body.PostgresVersion = plan.PostgresVersion.ValueString()
	}
	if !plan.HaType.IsNull() && !plan.HaType.IsUnknown() {
		body.HaType = plan.HaType.ValueString()
	}
	body.StandbyCount = standbyCountToAPI(plan.StandbyCount)

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
//   - Body != nil           → sparse PATCH body (size, ha_type, standby_count, tags).
//   - TransitionExpected    → server processes the mutation as a state
//     transition (size, ha_type, standby_count); caller follows up with WaitForPostgresMatch
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
	// The pair travels together: the server validates standbyCount against
	// haType, and haType alone would keep or reset the count.
	if !plan.HaType.Equal(state.HaType) || (!plan.StandbyCount.IsUnknown() && !plan.StandbyCount.Equal(state.StandbyCount)) {
		update.HaType = plan.HaType.ValueString()
		update.StandbyCount = standbyCountToAPI(plan.StandbyCount)
		changed = true
		transitionExpected = true
	}

	// Not a transition: the flag flips without changing the instance.
	if !plan.DeletionProtection.IsNull() && !plan.DeletionProtection.IsUnknown() && !plan.DeletionProtection.Equal(state.DeletionProtection) {
		update.DeletionProtection = plan.DeletionProtection.ValueBoolPointer()
		changed = true
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
		// Tags unchanged but size/ha_type/standby_count/deletion_protection changing — re-assert state tags so
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

// standbyCountToAPI returns the wire value for a known standby_count, or nil
// (field omitted) when it is null or unknown.
func standbyCountToAPI(v types.Int64) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := int(v.ValueInt64())
	return &n
}

// buildPostgresMatchPredicate returns a predicate for WaitForPostgresMatch that
// succeeds once the instance is running AND the server reflects the values we
// PATCHed (size / ha_type / standby_count / tags). This is field-aware so the wait doesn't
// settle on a stale 'running' before Ubicloud commits the queued change.
func buildPostgresMatchPredicate(body *api.PostgresUpdate) func(*api.Postgres) bool {
	expectSize := body.Size
	expectHaType := body.HaType
	expectStandbyCount := body.StandbyCount
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
		if expectStandbyCount != nil && (pg.StandbyCount == nil || *pg.StandbyCount != *expectStandbyCount) {
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
func syncPostgresState(ctx context.Context, pg *api.Postgres, state *models.PostgresServiceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	out := *state

	out.ID = types.StringValue(pg.Id)
	out.Name = types.StringValue(pg.Name)
	out.CloudProvider = types.StringValue(pg.Provider)
	out.Region = types.StringValue(pg.Region)

	out.Size = types.StringValue(pg.Size)
	out.State = types.StringValue(pg.State)
	out.CreatedAt = types.StringValue(pg.CreatedAt)
	// postgresVersion / haType / hostname / username are
	// schema-optional on PostgresInstanceV1 — preserve prior values when the
	// server omits them rather than writing empty strings. password is
	// config-owned and deliberately never read from the server: with
	// credential redaction enabled, GET does not return it.
	if pg.PostgresVersion != "" {
		out.PostgresVersion = types.StringValue(pg.PostgresVersion)
	}
	if pg.HaType != "" {
		out.HaType = types.StringValue(pg.HaType)
	} else {
		out.HaType = types.StringValue("none")
	}
	// standbyCount is schema-optional too; fall back to the ha_type default.
	if pg.StandbyCount != nil {
		out.StandbyCount = types.Int64Value(int64(*pg.StandbyCount))
	} else {
		out.StandbyCount = defaultStandbyCount(out.HaType)
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
	tagsValue, d := apiTagsToMapValue(pg.Tags)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	out.Tags = tagsValue
	ipAccess, ipDiags := syncPostgresIPAccess(ctx, pg.IpAccessList, state.IpAccessList)
	diags.Append(ipDiags...)
	if diags.HasError() {
		return diags
	}
	out.IpAccessList = ipAccess
	out.DeletionProtection = postgresDeletionProtectionFromAPI(state.DeletionProtection, pg.DeletionProtection)

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

// postgresDeletionProtectionFromAPI reads an omitted field as off (the API
// reports a protected service) and never turns an unset value into false, so
// organizations without the feature see no drift.
func postgresDeletionProtectionFromAPI(current types.Bool, apiValue *bool) types.Bool {
	switch {
	case apiValue != nil && *apiValue:
		return types.BoolValue(true)
	case current.IsNull() || current.IsUnknown():
		return types.BoolNull()
	default:
		return types.BoolValue(false)
	}
}

// settlePostgresProtection sets the deletion_protection value to record after
// an apply, and returns an error detail when the API did not confirm the sent
// value (instead of Terraform's generic "inconsistent result after apply").
// sent is nil when no value was sent.
func settlePostgresProtection(planned types.Bool, sent, reported *bool, state *models.PostgresServiceResourceModel) string {
	switch {
	case sent == nil:
		if !planned.IsUnknown() {
			state.DeletionProtection = planned
		}
		return ""
	case reported == nil && *sent:
		state.DeletionProtection = types.BoolNull()
		return "ClickHouse Cloud did not confirm deletion_protection = true, so the Postgres service may not be protected. Check that deletion protection is available for your organization, then apply again."
	case reported != nil && *reported != *sent:
		state.DeletionProtection = types.BoolValue(*reported)
		return fmt.Sprintf("ClickHouse Cloud returned deletion_protection = %t after the provider sent %t. Apply again, or check the setting in the ClickHouse Cloud console.", *reported, *sent)
	default:
		state.DeletionProtection = types.BoolValue(*sent)
		return ""
	}
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

// passwordCreateIntent captures whether Create should rotate to a declared
// credential after provisioning (CreatePostgres always has the server generate
// one). The password is config-owned — never read back from the API — so the
// declared value is what lands in state (or nothing, when using password_wo).
type passwordCreateIntent struct {
	Set   bool   // call SetPostgresPassword?
	Value string // value to PATCH (when Set)
}

// decidePasswordOnCreate resolves the post-create password action: CreatePostgres
// always has the server generate a password, so rotate to the declared credential
// when there is one. password_wo is write-only — readable only from config — and
// preferred over password, mirroring clickhouse_service.
func decidePasswordOnCreate(plan, config models.PostgresServiceResourceModel) passwordCreateIntent {
	if pw := config.PasswordWO.ValueString(); pw != "" {
		return passwordCreateIntent{Set: true, Value: pw}
	}
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() {
		if pw := plan.Password.ValueString(); pw != "" {
			return passwordCreateIntent{Set: true, Value: pw}
		}
	}
	return passwordCreateIntent{Set: false}
}

// decidePasswordRotationOnUpdate returns the new password and whether to rotate:
// a password_wo_version change rotates to the (write-only, config-read)
// password_wo value; otherwise a change to `password` rotates to it. Removing
// password from config unmanages it (no rotation).
func decidePasswordRotationOnUpdate(plan, state, config models.PostgresServiceResourceModel) (value string, rotate bool) {
	if !plan.PasswordWOVersion.Equal(state.PasswordWOVersion) {
		if pw := config.PasswordWO.ValueString(); pw != "" {
			return pw, true
		}
	}
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() && !plan.Password.Equal(state.Password) {
		if pw := plan.Password.ValueString(); pw != "" {
			return pw, true
		}
	}
	return "", false
}
