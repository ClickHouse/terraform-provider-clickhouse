package resource

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
)

// postgresServiceResourceModelV0 is the schema-version-0 state shape: it still
// carries connection_string (removed in v1 — the server no longer returns it
// once credential redaction is enabled) and predates password_wo.
type postgresServiceResourceModelV0 struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	CloudProvider        types.String `tfsdk:"cloud_provider"`
	Region               types.String `tfsdk:"region"`
	PostgresVersion      types.String `tfsdk:"postgres_version"`
	Size                 types.String `tfsdk:"size"`
	HaType               types.String `tfsdk:"ha_type"`
	Tags                 types.Map    `tfsdk:"tags"`
	PgConfig             types.Map    `tfsdk:"pg_config"`
	PgBouncerConfig      types.Map    `tfsdk:"pgbouncer_config"`
	State                types.String `tfsdk:"state"`
	CreatedAt            types.String `tfsdk:"created_at"`
	IsPrimary            types.Bool   `tfsdk:"is_primary"`
	Hostname             types.String `tfsdk:"hostname"`
	Port                 types.Int64  `tfsdk:"port"`
	Username             types.String `tfsdk:"username"`
	ConnectionString     types.String `tfsdk:"connection_string"`
	Password             types.String `tfsdk:"password"`
	ReadReplicaOf        types.String `tfsdk:"read_replica_of"`
	RestoreToPointInTime types.Object `tfsdk:"restore_to_point_in_time"`
}

// postgresServiceResourceSchemaV0 declares only the type shape of schema version 0 —
// enough for the framework to decode prior state; validators and plan
// modifiers are irrelevant during upgrade.
var postgresServiceResourceSchemaV0 = schema.Schema{
	Attributes: map[string]schema.Attribute{
		"id":               schema.StringAttribute{Computed: true},
		"name":             schema.StringAttribute{Required: true},
		"cloud_provider":   schema.StringAttribute{Optional: true, Computed: true},
		"region":           schema.StringAttribute{Optional: true, Computed: true},
		"postgres_version": schema.StringAttribute{Optional: true, Computed: true},
		"size":             schema.StringAttribute{Optional: true, Computed: true},
		"ha_type":          schema.StringAttribute{Optional: true, Computed: true},
		"tags":             schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType},
		"pg_config":        schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType},
		"pgbouncer_config": schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType},
		"state":            schema.StringAttribute{Computed: true},
		"created_at":       schema.StringAttribute{Computed: true},
		"is_primary":       schema.BoolAttribute{Computed: true},
		"hostname":         schema.StringAttribute{Computed: true},
		"port":             schema.Int64Attribute{Computed: true},
		"username":         schema.StringAttribute{Computed: true},
		"connection_string": schema.StringAttribute{
			Computed:  true,
			Sensitive: true,
		},
		"password":        schema.StringAttribute{Optional: true, Computed: true, Sensitive: true},
		"read_replica_of": schema.StringAttribute{Optional: true},
		"restore_to_point_in_time": schema.SingleNestedAttribute{
			Optional: true,
			Attributes: map[string]schema.Attribute{
				"source_id":      schema.StringAttribute{Required: true},
				"restore_target": schema.StringAttribute{Required: true},
			},
		},
	},
}

// upgradePostgresServiceStateV0 maps a v0 state to the current model: drop
// connection_string (no longer part of the resource) and initialize the
// password_wo attributes to null. password carries over verbatim — it remains
// whatever the configuration last declared (or the pre-redaction GET echoed) —
// EXCEPT for a read replica: its config can never declare password
// (ConflictsWith), so a v0 value is always a server-echoed inherited
// credential; drop it rather than surface a spurious password-removal diff.
// A replica is recognized by read_replica_of OR known is_primary=false — an
// imported v0 replica has no read_replica_of in state (import stores only the
// ID and GET exposes no parent id). A null is_primary (written by no v0 code
// path) keeps the password: mis-dropping a primary's declared credential
// would force a needless rotation, the worse failure.
func upgradePostgresServiceStateV0(old postgresServiceResourceModelV0) models.PostgresServiceResourceModel {
	password := old.Password
	if !old.ReadReplicaOf.IsNull() || (!old.IsPrimary.IsNull() && !old.IsPrimary.ValueBool()) {
		password = types.StringNull()
	}
	return models.PostgresServiceResourceModel{
		ID:                   old.ID,
		Name:                 old.Name,
		CloudProvider:        old.CloudProvider,
		Region:               old.Region,
		PostgresVersion:      old.PostgresVersion,
		Size:                 old.Size,
		HaType:               old.HaType,
		Tags:                 old.Tags,
		PgConfig:             old.PgConfig,
		PgBouncerConfig:      old.PgBouncerConfig,
		State:                old.State,
		CreatedAt:            old.CreatedAt,
		IsPrimary:            old.IsPrimary,
		Hostname:             old.Hostname,
		Port:                 old.Port,
		Username:             old.Username,
		Password:             password,
		PasswordWO:           types.StringNull(),
		PasswordWOVersion:    types.Int64Null(),
		ReadReplicaOf:        old.ReadReplicaOf,
		RestoreToPointInTime: old.RestoreToPointInTime,
	}
}

func (r *PostgresServiceResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// v1 removed connection_string and added password_wo/password_wo_version.
		0: {
			PriorSchema: &postgresServiceResourceSchemaV0,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var old postgresServiceResourceModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &old)...)
				if resp.Diagnostics.HasError() {
					return
				}
				resp.Diagnostics.Append(resp.State.Set(ctx, upgradePostgresServiceStateV0(old))...)
			},
		},
	}
}
