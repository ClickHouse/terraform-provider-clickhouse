package models

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// PostgresServiceResourceModel is the Terraform plan/state model for the
// clickhouse_postgres_service resource.
type PostgresServiceResourceModel struct {
	// Identity / immutable.
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	CloudProvider   types.String `tfsdk:"cloud_provider"`
	Region          types.String `tfsdk:"region"`
	PostgresVersion types.String `tfsdk:"postgres_version"`

	// Mutable.
	Size   types.String `tfsdk:"size"`
	HaType types.String `tfsdk:"ha_type"`
	Tags   types.Map    `tfsdk:"tags"`

	// Runtime configuration. Terraform-owned, full-replacement: whatever is
	// declared is the desired state; omitting a key removes it server-side;
	// omitting the attribute clears all parameters. Modeled as string maps to
	// match the tags convention (the server's pgConfig is a flat key/value
	// object, so a map is the natural shape).
	PgConfig        types.Map `tfsdk:"pg_config"`
	PgBouncerConfig types.Map `tfsdk:"pgbouncer_config"`

	// Computed.
	State            types.String `tfsdk:"state"`
	CreatedAt        types.String `tfsdk:"created_at"`
	IsPrimary        types.Bool   `tfsdk:"is_primary"`
	Hostname         types.String `tfsdk:"hostname"`
	Port             types.Int64  `tfsdk:"port"`
	Username         types.String `tfsdk:"username"`
	ConnectionString types.String `tfsdk:"connection_string"`

	// Sensitive / write-only.
	// Password is Optional+Computed: user-supplied, or server-generated when
	// omitted; always hydrated from the GET so it holds the live password.
	// PasswordWO is the write-only input attribute (never stored in state — but
	// the password it sets is still visible via Password / connection_string);
	// its rotation is triggered by bumping PasswordWOVersion. Password and
	// PasswordWO are mutually exclusive.
	Password          types.String `tfsdk:"password"`
	PasswordWO        types.String `tfsdk:"password_wo"`
	PasswordWOVersion types.Int64  `tfsdk:"password_wo_version"`

	// Provenance — create-time only, mutually exclusive. ReadReplicaOf holds the
	// parent primary's ID for a read replica; changing/removing it replaces the
	// instance, EXCEPT once it has been promoted out-of-band (is_primary true),
	// where it's adopted in place (RequiresReplaceIf). RestoreToPointInTime
	// records a point-in-time restore; any change to it, or removing it, replaces
	// the instance (RequiresReplace).
	ReadReplicaOf        types.String `tfsdk:"read_replica_of"`
	RestoreToPointInTime types.Object `tfsdk:"restore_to_point_in_time"`
}

// PostgresRestoreModel is the nested restore_to_point_in_time object. The new
// instance's name comes from the resource's top-level `name`; this block only
// carries the restore source and target.
type PostgresRestoreModel struct {
	SourceID      types.String `tfsdk:"source_id"`
	RestoreTarget types.String `tfsdk:"restore_target"`
}
