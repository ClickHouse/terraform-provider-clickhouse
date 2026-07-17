package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
)

// v0StateFixture returns a fully-populated v0 state: every field must be a
// valid (non-zero) framework value so tfsdk.State.Set can encode it.
func v0StateFixture() postgresServiceResourceModelV0 {
	return postgresServiceResourceModelV0{
		ID:               types.StringValue("pg-1"),
		Name:             types.StringValue("primary-1"),
		CloudProvider:    types.StringValue("aws"),
		Region:           types.StringValue("us-east-1"),
		PostgresVersion:  types.StringValue("18"),
		Size:             types.StringValue("r6gd.large"),
		HaType:           types.StringValue("async"),
		Tags:             mapTags("team", "billing"),
		PgConfig:         mapTags(),
		PgBouncerConfig:  mapTags(),
		State:            types.StringValue("running"),
		CreatedAt:        types.StringValue("2026-05-27T00:00:00Z"),
		IsPrimary:        types.BoolValue(true),
		Hostname:         types.StringValue("primary-1.example.com"),
		Port:             types.Int64Value(5432),
		Username:         types.StringValue("default"),
		ConnectionString: types.StringValue("postgresql://default:secret@primary-1.example.com:5432/postgres"),
		Password:         types.StringValue("KeepThisSecret1"),
		ReadReplicaOf:    types.StringNull(),
		RestoreToPointInTime: types.ObjectNull(map[string]attr.Type{
			"source_id":      types.StringType,
			"restore_target": types.StringType,
		}),
	}
}

func TestUpgradePostgresServiceStateV0(t *testing.T) {
	t.Run("primary: password carried, connection_string dropped, password_wo null", func(t *testing.T) {
		old := v0StateFixture()
		got := upgradePostgresServiceStateV0(old)

		if !got.Password.Equal(old.Password) {
			t.Errorf("password not carried over: got %v", got.Password)
		}
		if !got.PasswordWO.IsNull() || !got.PasswordWOVersion.IsNull() {
			t.Errorf("password_wo fields must upgrade to null: got %v / %v", got.PasswordWO, got.PasswordWOVersion)
		}
		if !got.ID.Equal(old.ID) || !got.Name.Equal(old.Name) || !got.Size.Equal(old.Size) ||
			!got.HaType.Equal(old.HaType) || !got.Hostname.Equal(old.Hostname) ||
			!got.Username.Equal(old.Username) || !got.Tags.Equal(old.Tags) ||
			!got.RestoreToPointInTime.Equal(old.RestoreToPointInTime) {
			t.Errorf("carried fields mismatch:\n got = %#v\n old = %#v", got, old)
		}
	})

	t.Run("replica: server-hydrated password dropped", func(t *testing.T) {
		// A replica's config can never declare password (ConflictsWith), so a
		// v0 value is always the server-echoed inherited credential.
		old := v0StateFixture()
		old.ReadReplicaOf = types.StringValue("pg-parent")
		old.IsPrimary = types.BoolValue(false)
		got := upgradePostgresServiceStateV0(old)
		if !got.Password.IsNull() {
			t.Errorf("replica password must upgrade to null, got %v", got.Password)
		}
	})

	t.Run("imported replica: password dropped on is_primary=false alone", func(t *testing.T) {
		// An imported v0 replica has read_replica_of=null (import stores only
		// the ID and GET exposes no parent), but is_primary is known false —
		// its password is still the server-echoed inherited credential.
		old := v0StateFixture()
		old.ReadReplicaOf = types.StringNull()
		old.IsPrimary = types.BoolValue(false)
		got := upgradePostgresServiceStateV0(old)
		if !got.Password.IsNull() {
			t.Errorf("imported replica password must upgrade to null, got %v", got.Password)
		}
	})

	t.Run("restore: password carried (is_primary=true, restore block set)", func(t *testing.T) {
		// A v0 restore is a primary; its password may be config-declared or
		// server-echoed (indistinguishable in state), so it carries verbatim —
		// the documented one-time `password -> null` diff covers the echoed case.
		old := v0StateFixture()
		old.RestoreToPointInTime = types.ObjectValueMust(
			map[string]attr.Type{"source_id": types.StringType, "restore_target": types.StringType},
			map[string]attr.Value{
				"source_id":      types.StringValue("src-1"),
				"restore_target": types.StringValue("2026-06-01T00:00:00Z"),
			},
		)
		got := upgradePostgresServiceStateV0(old)
		if !got.Password.Equal(old.Password) {
			t.Errorf("restore password must carry over: got %v", got.Password)
		}
		if !got.RestoreToPointInTime.Equal(old.RestoreToPointInTime) {
			t.Errorf("restore block must carry over: got %v", got.RestoreToPointInTime)
		}
	})

	t.Run("null is_primary: password kept (treat as primary)", func(t *testing.T) {
		// Defensive: no v0 code path writes a null is_primary, but if one
		// existed, dropping a primary's declared password would force a
		// needless rotation — keep it.
		old := v0StateFixture()
		old.IsPrimary = types.BoolNull()
		got := upgradePostgresServiceStateV0(old)
		if !got.Password.Equal(old.Password) {
			t.Errorf("password must be kept when is_primary is null, got %v", got.Password)
		}
	})
}

func TestPostgresServiceStateUpgraderV0_EndToEnd(t *testing.T) {
	ctx := context.Background()

	prior := tfsdk.State{Schema: postgresServiceResourceSchemaV0}
	if diags := prior.Set(ctx, v0StateFixture()); diags.HasError() {
		t.Fatalf("seeding v0 state: %v", diags)
	}

	r := &PostgresServiceResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("current schema: %v", schemaResp.Diagnostics)
	}

	resp := resource.UpgradeStateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.UpgradeState(ctx)[0].StateUpgrader(ctx, resource.UpgradeStateRequest{State: &prior}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("upgrade diagnostics: %v", resp.Diagnostics)
	}

	var got models.PostgresServiceResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("decoding upgraded state: %v", diags)
	}
	if got.Password.ValueString() != "KeepThisSecret1" {
		t.Errorf("password not carried through upgrade: got %v", got.Password)
	}
	if !got.PasswordWO.IsNull() || !got.PasswordWOVersion.IsNull() {
		t.Errorf("password_wo fields must be null after upgrade: got %v / %v", got.PasswordWO, got.PasswordWOVersion)
	}
	if got.Hostname.ValueString() != "primary-1.example.com" || got.Port.ValueInt64() != 5432 {
		t.Errorf("connectivity fields not carried: %v / %v", got.Hostname, got.Port)
	}
}
