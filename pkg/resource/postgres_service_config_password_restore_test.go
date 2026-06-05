package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

// ---------------------------------------------------------------------------
// pg_config / pgbouncer_config — Map ⇄ api.PgConfigMap
// ---------------------------------------------------------------------------

func TestPlanConfigToMap(t *testing.T) {
	ctx := context.Background()

	t.Run("null/unknown returns nil", func(t *testing.T) {
		got, d := planConfigToMap(ctx, types.MapNull(types.StringType))
		if d.HasError() || got != nil {
			t.Errorf("null: got %#v diags=%v", got, d)
		}
		got, d = planConfigToMap(ctx, types.MapUnknown(types.StringType))
		if d.HasError() || got != nil {
			t.Errorf("unknown: got %#v diags=%v", got, d)
		}
	})

	t.Run("populated maps to PgConfigMap", func(t *testing.T) {
		got, d := planConfigToMap(ctx, mapTags("max_connections", "200", "work_mem", "8MB"))
		if d.HasError() {
			t.Fatalf("diags: %v", d)
		}
		if got["max_connections"] != "200" || got["work_mem"] != "8MB" {
			t.Errorf("unexpected: %#v", got)
		}
	})
}

func TestApiConfigToMapValue(t *testing.T) {
	t.Run("empty returns known empty map", func(t *testing.T) {
		got, d := apiConfigToMapValue(nil)
		if d.HasError() || got.IsNull() || len(got.Elements()) != 0 {
			t.Errorf("expected known empty map; got %v diags=%v", got, d)
		}
	})
	t.Run("populated round-trips", func(t *testing.T) {
		got, d := apiConfigToMapValue(api.PgConfigMap{"max_connections": "200"})
		if d.HasError() || got.IsNull() {
			t.Fatalf("got %v diags=%v", got, d)
		}
		if got.Elements()["max_connections"].(types.String).ValueString() != "200" {
			t.Errorf("value mismatch: %v", got)
		}
	})
}

func TestBuildConfigUpdate(t *testing.T) {
	ctx := context.Background()
	with := func(pg, pgb types.Map) models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{PgConfig: pg, PgBouncerConfig: pgb}
	}

	t.Run("no diff", func(t *testing.T) {
		got, d := buildConfigUpdate(ctx, with(mapTags("a", "1"), types.MapNull(types.StringType)), with(mapTags("a", "1"), types.MapNull(types.StringType)))
		if d.HasError() || got.Changed {
			t.Errorf("expected no change; got %#v diags=%v", got, d)
		}
	})
	t.Run("removes a key (full replacement)", func(t *testing.T) {
		plan := with(mapTags("max_connections", "200"), types.MapNull(types.StringType))
		state := with(mapTags("max_connections", "200", "work_mem", "4MB"), types.MapNull(types.StringType))
		got, d := buildConfigUpdate(ctx, plan, state)
		if d.HasError() || !got.Changed {
			t.Fatalf("expected change; got %#v diags=%v", got, d)
		}
		if _, present := got.Body.PgConfig["work_mem"]; present || len(got.Body.PgConfig) != 1 {
			t.Errorf("removed key leaked: %#v", got.Body.PgConfig)
		}
	})
	t.Run("clear both → Changed with nil PgConfig and PgBouncerConfig", func(t *testing.T) {
		plan := with(types.MapNull(types.StringType), types.MapNull(types.StringType))
		state := with(mapTags("max_connections", "200"), mapTags("pool_mode", "transaction"))
		got, d := buildConfigUpdate(ctx, plan, state)
		if d.HasError() || !got.Changed {
			t.Fatalf("expected change; got %#v diags=%v", got, d)
		}
		if got.Body.PgConfig != nil || got.Body.PgBouncerConfig != nil {
			t.Errorf("expected nil maps; got %#v / %#v", got.Body.PgConfig, got.Body.PgBouncerConfig)
		}
	})
	t.Run("partial: pg changes, pgbouncer carried from plan", func(t *testing.T) {
		plan := with(mapTags("max_connections", "300"), mapTags("pool_mode", "transaction"))
		state := with(mapTags("max_connections", "200"), mapTags("pool_mode", "transaction"))
		got, d := buildConfigUpdate(ctx, plan, state)
		if d.HasError() || !got.Changed {
			t.Fatalf("expected change; got %#v diags=%v", got, d)
		}
		if got.Body.PgConfig["max_connections"] != "300" || got.Body.PgBouncerConfig["pool_mode"] != "transaction" {
			t.Errorf("full-replacement body wrong: %#v", got.Body)
		}
	})
}

// ---------------------------------------------------------------------------
// password — validators + create/update rotation decisions
// ---------------------------------------------------------------------------

func TestPostgresPasswordValidators(t *testing.T) {
	ctx := context.Background()
	has := func(pw string) bool {
		req := validator.StringRequest{Path: path.Root("password"), ConfigValue: types.StringValue(pw)}
		for _, v := range postgresPasswordValidators() {
			resp := &validator.StringResponse{}
			v.ValidateString(ctx, req, resp)
			if resp.Diagnostics.HasError() {
				return true
			}
		}
		return false
	}
	cases := []struct {
		pw      string
		wantErr bool
	}{
		{"ValidPass123", false},
		{"Abcdefghij12", false},
		{"Aa1aaaa", true},
		{"PASSWORD1234", true},
		{"password1234", true},
		{"PasswordAbcd", true},
	}
	for _, c := range cases {
		if got := has(c.pw); got != c.wantErr {
			t.Errorf("password %q: hasError=%v want %v", c.pw, got, c.wantErr)
		}
	}
}

func TestDecidePasswordOnCreate(t *testing.T) {
	t.Run("neither → no rotation (server-generated stands)", func(t *testing.T) {
		got := decidePasswordOnCreate(
			models.PostgresServiceResourceModel{Password: types.StringNull()},
			models.PostgresServiceResourceModel{PasswordWO: types.StringNull()},
		)
		if got.Set {
			t.Errorf("got %#v", got)
		}
	})
	t.Run("regular password → set", func(t *testing.T) {
		got := decidePasswordOnCreate(
			models.PostgresServiceResourceModel{Password: types.StringValue("UserPass1234")},
			models.PostgresServiceResourceModel{PasswordWO: types.StringNull()},
		)
		if !got.Set || got.Value != "UserPass1234" {
			t.Errorf("got %#v", got)
		}
	})
	t.Run("write-only → set + precedence over regular", func(t *testing.T) {
		got := decidePasswordOnCreate(
			models.PostgresServiceResourceModel{Password: types.StringValue("UserPass1234")},
			models.PostgresServiceResourceModel{PasswordWO: types.StringValue("WriteOnly1234")},
		)
		if !got.Set || got.Value != "WriteOnly1234" {
			t.Errorf("write-only must win; got %#v", got)
		}
	})
}

func TestDecidePasswordRotationOnUpdate(t *testing.T) {
	t.Run("no change", func(t *testing.T) {
		if _, rot := decidePasswordRotationOnUpdate(
			models.PostgresServiceResourceModel{Password: types.StringValue("Same12345678"), PasswordWOVersion: types.Int64Null()},
			models.PostgresServiceResourceModel{Password: types.StringValue("Same12345678"), PasswordWOVersion: types.Int64Null()},
			models.PostgresServiceResourceModel{PasswordWO: types.StringNull()},
		); rot {
			t.Error("expected no rotation")
		}
	})
	t.Run("regular change", func(t *testing.T) {
		v, rot := decidePasswordRotationOnUpdate(
			models.PostgresServiceResourceModel{Password: types.StringValue("NewPass12345"), PasswordWOVersion: types.Int64Null()},
			models.PostgresServiceResourceModel{Password: types.StringValue("OldPass12345"), PasswordWOVersion: types.Int64Null()},
			models.PostgresServiceResourceModel{PasswordWO: types.StringNull()},
		)
		if !rot || v != "NewPass12345" {
			t.Errorf("got v=%q rot=%v", v, rot)
		}
	})
	t.Run("wo version bump", func(t *testing.T) {
		v, rot := decidePasswordRotationOnUpdate(
			models.PostgresServiceResourceModel{Password: types.StringNull(), PasswordWOVersion: types.Int64Value(2)},
			models.PostgresServiceResourceModel{Password: types.StringNull(), PasswordWOVersion: types.Int64Value(1)},
			models.PostgresServiceResourceModel{PasswordWO: types.StringValue("RotatedWO1234")},
		)
		if !rot || v != "RotatedWO1234" {
			t.Errorf("got v=%q rot=%v", v, rot)
		}
	})
	t.Run("wo version unchanged → no rotation even with config value", func(t *testing.T) {
		if _, rot := decidePasswordRotationOnUpdate(
			models.PostgresServiceResourceModel{Password: types.StringNull(), PasswordWOVersion: types.Int64Value(1)},
			models.PostgresServiceResourceModel{Password: types.StringNull(), PasswordWOVersion: types.Int64Value(1)},
			models.PostgresServiceResourceModel{PasswordWO: types.StringValue("PresentNoBump1")},
		); rot {
			t.Error("version is the sole trigger; must not rotate")
		}
	})
}

func TestPasswordRotationPlanned(t *testing.T) {
	cfg := func(pw types.String) models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{Password: pw}
	}
	st := func(pw types.String, ver types.Int64) models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{Password: pw, PasswordWOVersion: ver}
	}
	pl := func(ver types.Int64) models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{PasswordWOVersion: ver}
	}

	if !passwordRotationPlanned(cfg(types.StringUnknown()), pl(types.Int64Null()), st(types.StringValue("old"), types.Int64Null())) {
		t.Error("interpolated (unknown) config should rotate")
	}
	if !passwordRotationPlanned(cfg(types.StringValue("new")), pl(types.Int64Null()), st(types.StringValue("old"), types.Int64Null())) {
		t.Error("changed config should rotate")
	}
	if passwordRotationPlanned(cfg(types.StringValue("same")), pl(types.Int64Null()), st(types.StringValue("same"), types.Int64Null())) {
		t.Error("equal config should not rotate")
	}
	if !passwordRotationPlanned(cfg(types.StringNull()), pl(types.Int64Value(2)), st(types.StringNull(), types.Int64Value(1))) {
		t.Error("wo version bump should rotate")
	}
	if passwordRotationPlanned(cfg(types.StringNull()), pl(types.Int64Value(1)), st(types.StringNull(), types.Int64Value(1))) {
		t.Error("no change should not rotate")
	}
	// Removing password_wo + password_wo_version: plan version null, state set.
	// Must NOT be treated as a rotation (mirrors decidePasswordRotationOnUpdate),
	// else connection_string would be marked unknown with no actual rotation.
	if passwordRotationPlanned(cfg(types.StringNull()), pl(types.Int64Null()), st(types.StringNull(), types.Int64Value(1))) {
		t.Error("removing password_wo_version (plan null, state set) must not rotate")
	}
}

// ---------------------------------------------------------------------------
// create-time attribute validation: required for a standard create; for a
// replica/restore validated against the source (match/omit → ok, conflict →
// error; size-on-restore and ha_type must be omitted)
// ---------------------------------------------------------------------------

func TestRequireStandardCreateAttributes(t *testing.T) {
	restoreType := map[string]attr.Type{"source_id": types.StringType, "restore_target": types.StringType}
	nullRestore := types.ObjectNull(restoreType)
	setRestore := types.ObjectValueMust(restoreType, map[string]attr.Value{
		"source_id":      types.StringValue("src-1"),
		"restore_target": types.StringValue("2026-06-01T00:00:00Z"),
	})
	std := func() models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{
			CloudProvider:        types.StringValue("aws"),
			Region:               types.StringValue("us-east-1"),
			Size:                 types.StringValue("4x16"),
			PostgresVersion:      types.StringNull(),
			HaType:               types.StringNull(),
			ReadReplicaOf:        types.StringNull(),
			RestoreToPointInTime: nullRestore,
		}
	}
	cases := []struct {
		name    string
		mutate  func(*models.PostgresServiceResourceModel)
		wantErr int
	}{
		{"standard complete", func(m *models.PostgresServiceResourceModel) {}, 0},
		{"standard missing region", func(m *models.PostgresServiceResourceModel) { m.Region = types.StringNull() }, 1},
		{"standard missing all three", func(m *models.PostgresServiceResourceModel) {
			m.CloudProvider, m.Region, m.Size = types.StringNull(), types.StringNull(), types.StringNull()
		}, 3},
		{"replica: not required (inherited)", func(m *models.PostgresServiceResourceModel) {
			m.CloudProvider, m.Region, m.Size = types.StringNull(), types.StringNull(), types.StringNull()
			m.ReadReplicaOf = types.StringValue("primary-1")
		}, 0},
		{"restore: not required (inherited)", func(m *models.PostgresServiceResourceModel) {
			m.CloudProvider, m.Region, m.Size = types.StringNull(), types.StringNull(), types.StringNull()
			m.RestoreToPointInTime = setRestore
		}, 0},
		{"origin signal unknown → deferred", func(m *models.PostgresServiceResourceModel) {
			m.CloudProvider, m.Region, m.Size = types.StringNull(), types.StringNull(), types.StringNull()
			m.ReadReplicaOf = types.StringUnknown()
		}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := std()
			c.mutate(&m)
			if diags := requireStandardCreateAttributes(m); diags.ErrorsCount() != c.wantErr {
				t.Errorf("want %d errors; got %d: %v", c.wantErr, diags.ErrorsCount(), diags)
			}
		})
	}
}

func TestSourceAttributeConflicts(t *testing.T) {
	src := &api.Postgres{Provider: "aws", Region: "us-west-2", Size: "r6gd.large", PostgresVersion: "18"}
	full := func(cp, region, size, version, ha string) models.PostgresServiceResourceModel {
		s := func(v string) types.String {
			if v == "" {
				return types.StringNull()
			}
			return types.StringValue(v)
		}
		return models.PostgresServiceResourceModel{
			CloudProvider:   s(cp),
			Region:          s(region),
			Size:            s(size),
			PostgresVersion: s(version),
			HaType:          s(ha),
		}
	}
	cases := []struct {
		name      string
		config    models.PostgresServiceResourceModel
		isReplica bool
		wantErr   int
	}{
		{"all omitted → inherited", full("", "", "", "", ""), true, 0},
		{"replica matching → ok", full("aws", "us-west-2", "r6gd.large", "18", ""), true, 0},
		{"replica region collides", full("", "us-east-1", "", "", ""), true, 1},
		{"replica size collides", full("", "", "r6gd.xlarge", "", ""), true, 1},
		{"replica region+version collide", full("", "eu-west-1", "", "17", ""), true, 2},
		{"replica ha_type forbidden", full("", "", "", "", "async"), true, 1},
		{"restore region matches, size+version omitted → ok", full("", "us-west-2", "", "", ""), false, 0},
		{"restore size forbidden (backup-era size)", full("", "", "r6gd.xlarge", "", ""), false, 1},
		{"restore region collides", full("", "us-east-1", "", "", ""), false, 1},
		{"restore ha_type forbidden", full("", "", "", "", "sync"), false, 1},
		{"restore size+ha_type both forbidden", full("", "", "r6gd.large", "", "async"), false, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if diags := sourceAttributeConflicts(c.config, src, c.isReplica); diags.ErrorsCount() != c.wantErr {
				t.Errorf("want %d errors; got %d: %v", c.wantErr, diags.ErrorsCount(), diags)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// configIsOrigin — replica/restore detection (deferred when interpolated)
// ---------------------------------------------------------------------------

func TestConfigIsOrigin(t *testing.T) {
	restoreType := map[string]attr.Type{"source_id": types.StringType, "restore_target": types.StringType}
	nullRestore := types.ObjectNull(restoreType)
	setRestore := types.ObjectValueMust(restoreType, map[string]attr.Value{
		"source_id":      types.StringValue("src-1"),
		"restore_target": types.StringValue("2026-06-01T00:00:00Z"),
	})
	cases := []struct {
		name    string
		rro     types.String
		restore types.Object
		want    bool
	}{
		{"standard (both null)", types.StringNull(), nullRestore, false},
		{"replica", types.StringValue("primary-1"), nullRestore, true},
		{"restore", types.StringNull(), setRestore, true},
		{"read_replica_of unknown → deferred", types.StringUnknown(), nullRestore, false},
		{"restore unknown → deferred", types.StringNull(), types.ObjectUnknown(restoreType), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := models.PostgresServiceResourceModel{ReadReplicaOf: c.rro, RestoreToPointInTime: c.restore}
			if got := configIsOrigin(m); got != c.want {
				t.Errorf("configIsOrigin = %v, want %v", got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// originSourceChanged — replace that re-derives inherited attrs from a new source
// ---------------------------------------------------------------------------

func TestOriginSourceChanged(t *testing.T) {
	restoreType := map[string]attr.Type{"source_id": types.StringType, "restore_target": types.StringType}
	restore := func(src, target string) types.Object {
		if src == "" {
			return types.ObjectNull(restoreType)
		}
		return types.ObjectValueMust(restoreType, map[string]attr.Value{
			"source_id":      types.StringValue(src),
			"restore_target": types.StringValue(target),
		})
	}
	mk := func(rro string, isPrimary bool, rSrc, rTarget string) models.PostgresServiceResourceModel {
		ro := types.StringNull()
		if rro != "" {
			ro = types.StringValue(rro)
		}
		return models.PostgresServiceResourceModel{
			ReadReplicaOf:        ro,
			IsPrimary:            types.BoolValue(isPrimary),
			RestoreToPointInTime: restore(rSrc, rTarget),
		}
	}
	cases := []struct {
		name   string
		config models.PostgresServiceResourceModel
		state  models.PostgresServiceResourceModel
		want   bool
	}{
		{"replica unchanged", mk("p1", false, "", ""), mk("p1", false, "", ""), false},
		{"replica re-pointed (live) → re-derive", mk("p2", false, "", ""), mk("p1", false, "", ""), true},
		{"replica re-pointed but promoted → no (adopted in place)", mk("p2", true, "", ""), mk("p1", true, "", ""), false},
		{"replica removed → no (becoming standalone)", mk("", false, "", ""), mk("p1", false, "", ""), false},
		{"restore unchanged", mk("", true, "s1", "2026-06-01T00:00:00Z"), mk("", true, "s1", "2026-06-01T00:00:00Z"), false},
		{"restore source changed → re-derive", mk("", true, "s2", "2026-06-01T00:00:00Z"), mk("", true, "s1", "2026-06-01T00:00:00Z"), true},
		{"restore target changed → re-derive", mk("", true, "s1", "2026-07-01T00:00:00Z"), mk("", true, "s1", "2026-06-01T00:00:00Z"), true},
		{"standard (no origin)", mk("", true, "", ""), mk("", true, "", ""), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := originSourceChanged(c.config, c.state); got != c.want {
				t.Errorf("originSourceChanged = %v, want %v", got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// readReplicaOfShouldReplace — promotion-aware replace decision
// ---------------------------------------------------------------------------

func TestReadReplicaOfShouldReplace(t *testing.T) {
	primary := types.StringValue("primary-1")
	other := types.StringValue("primary-2")
	none := types.StringNull()
	cases := []struct {
		name      string
		state     types.String
		plan      types.String
		isPrimary bool
		want      bool
	}{
		{"unchanged live replica", primary, primary, false, false},
		{"unchanged promoted", primary, primary, true, false},
		{"repoint live replica → replace", primary, other, false, true},
		{"remove from live replica → replace", primary, none, false, true},
		{"repoint promoted → in place", primary, other, true, false},
		{"remove from promoted → in place", primary, none, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := readReplicaOfShouldReplace(c.state, c.plan, c.isPrimary); got != c.want {
				t.Errorf("readReplicaOfShouldReplace(%v,%v,%v) = %v, want %v", c.state, c.plan, c.isPrimary, got, c.want)
			}
		})
	}
}

// A live read replica rejects direct size / ha_type / tags edits (the server
// 400s the PATCH); pg_config changes are allowed (separate endpoint).
func TestReplicaUpdateForbidden(t *testing.T) {
	large, xlarge := types.StringValue("r6gd.large"), types.StringValue("r6gd.xlarge")
	none, async := types.StringValue("none"), types.StringValue("async")
	tagsA, tagsB := mapTags("env", "prod"), mapTags("env", "dev")
	cfgA, cfgB := mapTags("max_connections", "200"), mapTags("max_connections", "250")
	m := func(size, ha types.String, tags, pg types.Map) models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{Size: size, HaType: ha, Tags: tags, PgConfig: pg}
	}
	cases := []struct {
		name        string
		plan, state models.PostgresServiceResourceModel
		wantErr     int
	}{
		{"no change", m(large, none, tagsA, cfgA), m(large, none, tagsA, cfgA), 0},
		{"size changed", m(xlarge, none, tagsA, cfgA), m(large, none, tagsA, cfgA), 1},
		{"ha_type changed", m(large, async, tagsA, cfgA), m(large, none, tagsA, cfgA), 1},
		{"tags changed", m(large, none, tagsB, cfgA), m(large, none, tagsA, cfgA), 1},
		{"size+ha_type changed", m(xlarge, async, tagsA, cfgA), m(large, none, tagsA, cfgA), 2},
		{"all three changed", m(xlarge, async, tagsB, cfgA), m(large, none, tagsA, cfgA), 3},
		{"pg_config only changed → allowed", m(large, none, tagsA, cfgB), m(large, none, tagsA, cfgA), 0},
		// Unknown (interpolated) plan values can't be proven changed → defer to
		// apply, don't false-positive at plan.
		{"unknown size → deferred", m(types.StringUnknown(), none, tagsA, cfgA), m(large, none, tagsA, cfgA), 0},
		{"unknown tags → deferred", m(large, none, types.MapUnknown(types.StringType), cfgA), m(large, none, tagsA, cfgA), 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if diags := replicaUpdateForbidden(c.plan, c.state); diags.ErrorsCount() != c.wantErr {
				t.Errorf("want %d errors; got %d: %v", c.wantErr, diags.ErrorsCount(), diags)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// restore / read replica request builders
// ---------------------------------------------------------------------------

func TestPlanToReadReplicaRequest(t *testing.T) {
	ctx := context.Background()
	plan := models.PostgresServiceResourceModel{
		Name:            types.StringValue("replica-1"),
		Tags:            mapTags("team", "billing"),
		PgConfig:        mapTags("max_connections", "200"),
		PgBouncerConfig: types.MapNull(types.StringType),
	}
	body, d := planToReadReplicaRequest(ctx, plan)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if body.Name != "replica-1" || body.PgConfig["max_connections"] != "200" || len(body.Tags) != 1 {
		t.Errorf("unexpected body: %#v", body)
	}
}

// An explicit empty pg_config / pgbouncer_config is rejected by the server on
// create (undefinedOr(isPopulatedObject)), so the provider blocks it at plan.
func TestForbidEmptyConfigOnCreate(t *testing.T) {
	empty := types.MapValueMust(types.StringType, map[string]attr.Value{})
	pop := mapTags("max_connections", "200")
	null := types.MapNull(types.StringType)
	cases := []struct {
		name    string
		pg      types.Map
		pgb     types.Map
		wantErr int
	}{
		{"both omitted (null)", null, null, 0},
		{"both populated", pop, pop, 0},
		{"pg_config empty", empty, null, 1},
		{"pgbouncer_config empty", null, empty, 1},
		{"both empty", empty, empty, 2},
		{"pg populated, pgbouncer empty", pop, empty, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := models.PostgresServiceResourceModel{PgConfig: c.pg, PgBouncerConfig: c.pgb}
			if diags := forbidEmptyConfigOnCreate(m); diags.ErrorsCount() != c.wantErr {
				t.Errorf("want %d errors; got %d: %v", c.wantErr, diags.ErrorsCount(), diags)
			}
		})
	}
}

func TestPlanToRestoreRequest(t *testing.T) {
	ctx := context.Background()
	restoreObj := types.ObjectValueMust(
		map[string]attr.Type{"source_id": types.StringType, "restore_target": types.StringType},
		map[string]attr.Value{
			"source_id":      types.StringValue("src-123"),
			"restore_target": types.StringValue("2026-06-01T00:00:00Z"),
		},
	)
	plan := models.PostgresServiceResourceModel{
		Name:                 types.StringValue("restored-1"),
		Tags:                 types.MapNull(types.StringType),
		PgConfig:             types.MapNull(types.StringType),
		PgBouncerConfig:      types.MapNull(types.StringType),
		RestoreToPointInTime: restoreObj,
	}
	sourceID, body, d := planToRestoreRequest(ctx, plan)
	if d.HasError() {
		t.Fatalf("diags: %v", d)
	}
	if sourceID != "src-123" || body.Name != "restored-1" || body.RestoreTarget != "2026-06-01T00:00:00Z" {
		t.Errorf("unexpected: sourceID=%q body=%#v", sourceID, body)
	}
}
