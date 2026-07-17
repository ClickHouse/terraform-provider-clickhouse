package resource

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
)

// mapTags is a convenience builder for the tags map fixture used across
// every tag-related test. Returns an empty map when called with no args
// (matching the contract apiTagsToMapValue produces for empty server input).
func mapTags(kv ...string) types.Map {
	if len(kv)%2 != 0 {
		panic("mapTags requires an even number of args (key, value, ...)")
	}
	elems := make(map[string]attr.Value, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		elems[kv[i]] = types.StringValue(kv[i+1])
	}
	m, diags := types.MapValue(types.StringType, elems)
	if diags.HasError() {
		panic(diags)
	}
	return m
}

// ---------------------------------------------------------------------------
// syncPostgresState — field round-trip from api.Postgres → resource model.
// ---------------------------------------------------------------------------

func TestPostgresResource_syncPostgresState(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		pg   *api.Postgres
		want models.PostgresServiceResourceModel
	}{
		{
			name: "primary with all fields populated",
			pg: &api.Postgres{
				Id:               "pg-1",
				Name:             "primary-1",
				Provider:         "aws",
				Region:           "us-east-1",
				PostgresVersion:  "18",
				Size:             "r6gd.large",
				HaType:           "async",
				State:            api.PostgresStateRunning,
				CreatedAt:        "2026-05-27T00:00:00Z",
				IsPrimary:        true,
				Hostname:         "primary-1.example.com",
				ConnectionString: "postgresql://default:secret@primary-1.example.com:5432/postgres",
				Username:         "default",
				Tags:             []api.Tag{{Key: "team", Value: "billing"}},
			},
			want: models.PostgresServiceResourceModel{
				ID:              types.StringValue("pg-1"),
				Name:            types.StringValue("primary-1"),
				CloudProvider:   types.StringValue("aws"),
				Region:          types.StringValue("us-east-1"),
				PostgresVersion: types.StringValue("18"),
				Size:            types.StringValue("r6gd.large"),
				HaType:          types.StringValue("async"),
				State:           types.StringValue(api.PostgresStateRunning),
				CreatedAt:       types.StringValue("2026-05-27T00:00:00Z"),
				IsPrimary:       types.BoolValue(true),
				Hostname:        types.StringValue("primary-1.example.com"),
				Port:            types.Int64Value(postgresDefaultPort),
				Username:        types.StringValue("default"),
				Tags:            mapTags("team", "billing"),
			},
		},
		{
			name: "ha_type empty in server response defaults to 'none'",
			pg: &api.Postgres{
				Id: "pg-2", Name: "n", Provider: "aws", Region: "us-east-1",
				Size: "c6gd.large", HaType: "",
				State: api.PostgresStateRunning, CreatedAt: "2026-05-27T00:00:00Z",
				IsPrimary: true,
			},
			want: models.PostgresServiceResourceModel{
				ID:            types.StringValue("pg-2"),
				Name:          types.StringValue("n"),
				CloudProvider: types.StringValue("aws"),
				Region:        types.StringValue("us-east-1"),
				Size:          types.StringValue("c6gd.large"),
				HaType:        types.StringValue("none"),
				State:         types.StringValue(api.PostgresStateRunning),
				CreatedAt:     types.StringValue("2026-05-27T00:00:00Z"),
				IsPrimary:     types.BoolValue(true),
				Hostname:      types.StringNull(),
				Port:          types.Int64Value(postgresDefaultPort),
				Username:      types.StringNull(),
				Tags:          mapTags(),
			},
		},
		{
			name: "is_primary=false (replica) propagates as false",
			pg: &api.Postgres{
				Id: "pg-3", Name: "n", Provider: "aws", Region: "us-east-1",
				Size:  "c6gd.large",
				State: api.PostgresStateRunning, CreatedAt: "2026-05-27T00:00:00Z",
				IsPrimary: false,
			},
			want: models.PostgresServiceResourceModel{
				ID:            types.StringValue("pg-3"),
				Name:          types.StringValue("n"),
				CloudProvider: types.StringValue("aws"),
				Region:        types.StringValue("us-east-1"),
				Size:          types.StringValue("c6gd.large"),
				HaType:        types.StringValue("none"),
				State:         types.StringValue(api.PostgresStateRunning),
				CreatedAt:     types.StringValue("2026-05-27T00:00:00Z"),
				IsPrimary:     types.BoolValue(false),
				Hostname:      types.StringNull(),
				Port:          types.Int64Value(postgresDefaultPort),
				Username:      types.StringNull(),
				Tags:          mapTags(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got models.PostgresServiceResourceModel
			diags := syncPostgresState(ctx, tt.pg, &got)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if !modelsEqual(t, got, tt.want) {
				t.Errorf("syncPostgresState mismatch\n got = %#v\nwant = %#v", got, tt.want)
			}
		})
	}
}

func TestPostgresResource_syncPostgresState_password(t *testing.T) {
	ctx := context.Background()

	t.Run("server echo is ignored: password stays config-owned", func(t *testing.T) {
		// Config-owned contract: the credential-redaction flag means GET is not
		// guaranteed to return the password, so the resource must never source
		// it from the server — even when a (pre-flag) server does echo one.
		const prior = "config-owned-secret"
		pre := models.PostgresServiceResourceModel{Password: types.StringValue(prior)}
		pg := &api.Postgres{
			Id: "pg-x", Name: "n", Provider: "aws", Region: "us-east-1",
			Size: "c6gd.large", State: api.PostgresStateRunning, IsPrimary: true,
			Password: "server-echoed-secret",
		}
		if diags := syncPostgresState(ctx, pg, &pre); diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if pre.Password.ValueString() != prior {
			t.Errorf("Password overwritten from server response: got %q want %q", pre.Password.ValueString(), prior)
		}
	})

	t.Run("no configured password: stays null regardless of server response", func(t *testing.T) {
		var pre models.PostgresServiceResourceModel
		pg := &api.Postgres{
			Id: "pg-x", Name: "n", Provider: "aws", Region: "us-east-1",
			Size: "c6gd.large", State: api.PostgresStateRunning, IsPrimary: true,
			Password: "server-echoed-secret",
		}
		if diags := syncPostgresState(ctx, pg, &pre); diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if !pre.Password.IsNull() {
			t.Errorf("Password should remain null when not configured: got %q", pre.Password.ValueString())
		}
	})
}

// ---------------------------------------------------------------------------
// Schema — credential attribute shape
// ---------------------------------------------------------------------------

func TestPostgresSchema_passwordAttributes(t *testing.T) {
	r := &PostgresServiceResource{}
	resp := resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)

	pw, ok := resp.Schema.Attributes["password"].(schema.StringAttribute)
	if !ok || !pw.Optional || pw.Computed || !pw.Sensitive {
		t.Errorf("password must be Optional+Sensitive and NOT Computed: %+v", pw)
	}
	wo, ok := resp.Schema.Attributes["password_wo"].(schema.StringAttribute)
	if !ok || !wo.Optional || !wo.Sensitive || !wo.WriteOnly {
		t.Errorf("password_wo must be Optional+Sensitive+WriteOnly: %+v", wo)
	}
	if _, ok := resp.Schema.Attributes["password_wo_version"].(schema.Int64Attribute); !ok {
		t.Errorf("password_wo_version must be an Int64Attribute")
	}
	// The v0 state upgrader is keyed on the schema version; a revert to 0
	// would silently skip upgrades and fail decoding v0 states.
	if resp.Schema.Version != 1 {
		t.Errorf("schema Version must be 1: got %d", resp.Schema.Version)
	}
	if _, exists := resp.Schema.Attributes["connection_string"]; exists {
		t.Errorf("connection_string must not exist in schema v1")
	}
}

// gateModel builds a fully-populated current-schema model for ModifyPlan
// tests; every field must be a valid framework value for tfsdk encoding.
func gateModel(primary bool) models.PostgresServiceResourceModel {
	return models.PostgresServiceResourceModel{
		ID:                types.StringValue("pg-1"),
		Name:              types.StringValue("n"),
		CloudProvider:     types.StringValue("aws"),
		Region:            types.StringValue("us-east-1"),
		PostgresVersion:   types.StringValue("18"),
		Size:              types.StringValue("m6gd.large"),
		HaType:            types.StringValue("none"),
		Tags:              mapTags(),
		PgConfig:          mapTags(),
		PgBouncerConfig:   mapTags(),
		State:             types.StringValue("running"),
		CreatedAt:         types.StringValue("2026-05-27T00:00:00Z"),
		IsPrimary:         types.BoolValue(primary),
		Hostname:          types.StringValue("h.example.com"),
		Port:              types.Int64Value(5432),
		Username:          types.StringValue("postgres"),
		Password:          types.StringNull(),
		PasswordWO:        types.StringNull(),
		PasswordWOVersion: types.Int64Null(),
		ReadReplicaOf:     types.StringNull(),
		RestoreToPointInTime: types.ObjectNull(map[string]attr.Type{
			"source_id":      types.StringType,
			"restore_target": types.StringType,
		}),
	}
}

// TestPostgresModifyPlan_updateCredentialGate exercises the update-branch
// gate itself (not just the requireDeclaredCredential helper): primaries
// must declare a credential; a live replica adopted by import is exempt but
// draws a warning when one is declared.
func TestPostgresModifyPlan_updateCredentialGate(t *testing.T) {
	ctx := context.Background()
	r := &PostgresServiceResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResp.Diagnostics)
	}
	sch := schemaResp.Schema

	mk := func(m models.PostgresServiceResourceModel) tfsdk.State {
		s := tfsdk.State{Schema: sch}
		if diags := s.Set(ctx, m); diags.HasError() {
			t.Fatalf("encoding model: %v", diags)
		}
		return s
	}
	run := func(state, plan, config models.PostgresServiceResourceModel) *resource.ModifyPlanResponse {
		req := resource.ModifyPlanRequest{
			State:  mk(state),
			Plan:   tfsdk.Plan{Schema: sch, Raw: mk(plan).Raw},
			Config: tfsdk.Config{Schema: sch, Raw: mk(config).Raw},
		}
		resp := &resource.ModifyPlanResponse{Plan: req.Plan}
		r.ModifyPlan(ctx, req, resp)
		return resp
	}

	t.Run("primary without credential: plan error", func(t *testing.T) {
		resp := run(gateModel(true), gateModel(true), gateModel(true))
		if resp.Diagnostics.ErrorsCount() != 1 {
			t.Fatalf("want 1 error, got %d: %v", resp.Diagnostics.ErrorsCount(), resp.Diagnostics)
		}
		if !strings.Contains(resp.Diagnostics.Errors()[0].Summary(), "Missing credential") {
			t.Errorf("unexpected error: %v", resp.Diagnostics.Errors()[0])
		}
	})

	t.Run("primary with password: clean", func(t *testing.T) {
		withPw := gateModel(true)
		withPw.Password = types.StringValue("ValidPass1234x")
		resp := run(withPw, withPw, withPw)
		if resp.Diagnostics.ErrorsCount() != 0 || resp.Diagnostics.WarningsCount() != 0 {
			t.Errorf("want clean plan, got %v", resp.Diagnostics)
		}
	})

	t.Run("imported replica, bare config: exempt and clean", func(t *testing.T) {
		resp := run(gateModel(false), gateModel(false), gateModel(false))
		if resp.Diagnostics.ErrorsCount() != 0 || resp.Diagnostics.WarningsCount() != 0 {
			t.Errorf("want clean plan, got %v", resp.Diagnostics)
		}
	})

	t.Run("imported replica with declared password: warning, no error", func(t *testing.T) {
		state := gateModel(false)
		cfg := gateModel(false)
		cfg.Password = types.StringValue("ValidPass1234x")
		resp := run(state, cfg, cfg)
		if resp.Diagnostics.ErrorsCount() != 0 {
			t.Fatalf("want no errors, got %v", resp.Diagnostics)
		}
		if resp.Diagnostics.WarningsCount() != 1 ||
			!strings.Contains(resp.Diagnostics.Warnings()[0].Summary(), "Read replica cannot take a credential") {
			t.Errorf("want the replica-credential warning, got %v", resp.Diagnostics)
		}
	})
}

// ---------------------------------------------------------------------------
// apiTagsToMapValue — drops empty-value tags
// ---------------------------------------------------------------------------

func TestApiTagsToMapValue_EmptyServerListReturnsEmptyMap(t *testing.T) {
	// Empty server list maps to empty map (not null) so that config
	// `tags = {}` round-trips cleanly. Returning null would diff forever
	// against an explicit empty map in config.
	got, diags := apiTagsToMapValue(nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.IsNull() {
		t.Errorf("expected empty (non-null) map for empty input; got %v", got)
	}
	if len(got.Elements()) != 0 {
		t.Errorf("expected zero elements; got %d", len(got.Elements()))
	}
}

func TestApiTagsToMapValue_DropsEmptyValueTags(t *testing.T) {
	// Schema requires non-empty values. A server-side empty value would
	// diff forever against any user-supplied non-empty value, so the read
	// path drops them entirely.
	got, diags := apiTagsToMapValue([]api.Tag{
		{Key: "team", Value: "billing"},
		{Key: "blank", Value: ""},
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.IsNull() {
		t.Fatalf("expected non-null map")
	}
	if _, present := got.Elements()["blank"]; present {
		t.Errorf("empty-value tag must not be retained; got %v", got)
	}
}

// TestTagValueValidator_RejectsEmptyString exercises the upstream
// stringvalidator.LengthAtLeast(1) the schema attaches to map values.
// Locks in the contract so a future refactor that drops the validator
// fails this test.
func TestTagValueValidator_RejectsEmptyString(t *testing.T) {
	ctx := context.Background()
	v := stringvalidator.LengthAtLeast(1)
	resp := &validator.StringResponse{}
	v.ValidateString(ctx, validator.StringRequest{
		Path:        path.Root("tags").AtMapKey("team"),
		ConfigValue: types.StringValue(""),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Errorf("expected diagnostic for empty-string tag value; got %v", resp.Diagnostics)
	}
}

// ---------------------------------------------------------------------------
// planTagsToAPI — null/unknown/populated round-trip
// ---------------------------------------------------------------------------

func TestPlanTagsToAPI(t *testing.T) {
	ctx := context.Background()

	t.Run("null map returns nil (leave server tags alone)", func(t *testing.T) {
		got, diags := planTagsToAPI(ctx, types.MapNull(types.StringType))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got != nil {
			t.Errorf("expected nil for null map, got %#v", got)
		}
	})

	t.Run("unknown map returns nil", func(t *testing.T) {
		got, diags := planTagsToAPI(ctx, types.MapUnknown(types.StringType))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got != nil {
			t.Errorf("expected nil for unknown map, got %#v", got)
		}
	})

	t.Run("populated map returns mapped api.Tag slice", func(t *testing.T) {
		got, diags := planTagsToAPI(ctx, mapTags("team", "billing"))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got == nil || len(*got) != 1 {
			t.Fatalf("expected 1 tag, got %v", got)
		}
		if (*got)[0].Key != "team" || (*got)[0].Value != "billing" {
			t.Errorf("unexpected mapped tag: %#v", (*got)[0])
		}
	})
}

// ---------------------------------------------------------------------------
// buildPostgresUpdate — diff matrix
// ---------------------------------------------------------------------------

func TestBuildPostgresUpdate(t *testing.T) {
	ctx := context.Background()

	baseModel := func(size, ha string, tags types.Map) models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{
			ID:            types.StringValue("pg-1"),
			Name:          types.StringValue("primary-1"),
			CloudProvider: types.StringValue("aws"),
			Region:        types.StringValue("us-east-1"),
			Size:          types.StringValue(size),
			HaType:        types.StringValue(ha),
			Tags:          tags,
		}
	}

	t.Run("no diff returns nil update and no transition", func(t *testing.T) {
		plan := baseModel("c6gd.large", "none", mapTags())
		state := baseModel("c6gd.large", "none", mapTags())
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body != nil {
			t.Errorf("expected nil Body for full no-op, got %#v", *result.Body)
		}
		if result.TransitionExpected {
			t.Errorf("TransitionExpected should be false on no-op")
		}
	})

	t.Run("size-only diff produces size-only body with TransitionExpected", func(t *testing.T) {
		plan := baseModel("c6gd.xlarge", "none", mapTags())
		state := baseModel("c6gd.large", "none", mapTags())
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body == nil {
			t.Fatal("expected non-nil Body")
		}
		if result.Body.Size != "c6gd.xlarge" {
			t.Errorf("size: got %q want c6gd.xlarge", result.Body.Size)
		}
		if result.Body.HaType != "" {
			t.Errorf("ha_type should not be set when unchanged; got %q", result.Body.HaType)
		}
		if result.Body.Tags != nil {
			t.Errorf("tags should be nil (omitted) when unchanged; got %#v", result.Body.Tags)
		}
		if !result.TransitionExpected {
			t.Errorf("size change must signal TransitionExpected=true")
		}
	})

	t.Run("ha_type-only diff signals TransitionExpected", func(t *testing.T) {
		plan := baseModel("c6gd.large", "async", mapTags())
		state := baseModel("c6gd.large", "none", mapTags())
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body == nil || result.Body.HaType != "async" {
			t.Errorf("expected ha_type=async body, got %#v", result.Body)
		}
		if !result.TransitionExpected {
			t.Errorf("ha_type flip must signal TransitionExpected=true")
		}
	})

	t.Run("tags-only change does NOT signal TransitionExpected", func(t *testing.T) {
		plan := baseModel("c6gd.large", "none", mapTags("team", "billing"))
		state := baseModel("c6gd.large", "none", mapTags())
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body == nil || result.Body.Tags == nil {
			t.Fatalf("expected tags in body, got %#v", result.Body)
		}
		if len(*result.Body.Tags) != 1 || (*result.Body.Tags)[0].Key != "team" {
			t.Errorf("unexpected tag body: %#v", *result.Body.Tags)
		}
		if result.TransitionExpected {
			t.Errorf("tags-only mutations are hot; TransitionExpected must be false")
		}
	})

	t.Run("tags cleared: plan is empty map, state had tags", func(t *testing.T) {
		// User config `tags = {}` clears all tags. Must send "tags": []
		// (empty array), not omit the field entirely. Validates that the
		// pointer-to-slice in PostgresUpdate.Tags is being used correctly.
		plan := baseModel("c6gd.large", "none", mapTags())
		state := baseModel("c6gd.large", "none", mapTags("team", "billing"))
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body == nil || result.Body.Tags == nil {
			t.Fatalf("expected non-nil tags pointer (empty slice) to clear server-side tags; got %#v", result.Body)
		}
		if len(*result.Body.Tags) != 0 {
			t.Errorf("expected empty tags slice to clear; got %#v", *result.Body.Tags)
		}
		if result.TransitionExpected {
			t.Errorf("tag-clear must not signal TransitionExpected")
		}
		// Confirm JSON wire shape — server must see "tags": []
		// (NOT field omitted), otherwise the *[]Tag pointer-to-slice
		// intent is lost in marshalling.
		marshaled, err := json.Marshal(*result.Body)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		if !strings.Contains(string(marshaled), `"tags":[]`) {
			t.Errorf("expected JSON body to contain \"tags\":[] to clear server-side; got %s", string(marshaled))
		}
	})

	t.Run("tag value mutation: same key, different value", func(t *testing.T) {
		plan := baseModel("c6gd.large", "none", mapTags("team", "engineering"))
		state := baseModel("c6gd.large", "none", mapTags("team", "billing"))
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body == nil || result.Body.Tags == nil {
			t.Fatalf("expected non-nil tags pointer in PATCH body; got %#v", result.Body)
		}
		if len(*result.Body.Tags) != 1 {
			t.Fatalf("expected 1 tag in PATCH body; got %d", len(*result.Body.Tags))
		}
		t0 := (*result.Body.Tags)[0]
		if t0.Key != "team" || t0.Value != "engineering" {
			t.Errorf("expected key=team value=engineering, got key=%q value=%q", t0.Key, t0.Value)
		}
		if result.TransitionExpected {
			t.Errorf("tag value mutation must NOT signal TransitionExpected")
		}
		// Wire-shape assertion: value must be present on the wire.
		marshaled, err := json.Marshal(*result.Body)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		if !strings.Contains(string(marshaled), `"value":"engineering"`) {
			t.Errorf("expected value in JSON body; got %s", string(marshaled))
		}
	})

	t.Run("plan.Tags == Unknown: re-asserts state.Tags so server doesn't clear them", func(t *testing.T) {
		// Regression test for two distinct silent-data-loss paths:
		//   1) Without UseStateForUnknown on the tags attribute, the
		//      framework marks tags as Unknown and buildPostgresUpdate
		//      would treat nil-from-Unknown as "clear all tags".
		//   2) The server's PATCH endpoint has PUT-like semantics for the
		//      tags field, so a body of just {"size":...} (no tags) also
		//      clears server-side tags.
		plan := baseModel("c6gd.xlarge", "none", types.MapUnknown(types.StringType))
		state := baseModel("c6gd.large", "none", mapTags("team", "billing"))
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body == nil {
			t.Fatal("expected size diff to still produce a Body")
		}
		if result.Body.Tags == nil {
			t.Errorf("expected state.Tags to be re-asserted in PATCH body to defend against server-side clear; got nil")
		} else if len(*result.Body.Tags) != 1 || (*result.Body.Tags)[0].Key != "team" {
			t.Errorf("expected state.Tags ([team=billing]) preserved in PATCH; got %#v", *result.Body.Tags)
		}
		if result.Body.Size != "c6gd.xlarge" {
			t.Errorf("size change must still propagate; got %q", result.Body.Size)
		}
	})

	t.Run("size-only diff with non-empty state tags: server-clear defense", func(t *testing.T) {
		plan := baseModel("c6gd.xlarge", "none", mapTags("team", "billing"))
		state := baseModel("c6gd.large", "none", mapTags("team", "billing"))
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body == nil {
			t.Fatal("expected size diff to produce a Body")
		}
		if result.Body.Size != "c6gd.xlarge" {
			t.Errorf("size not propagated")
		}
		if result.Body.Tags == nil {
			t.Errorf("expected tags re-asserted in PATCH body to defend against server-side clear; got nil")
		} else if len(*result.Body.Tags) != 1 || (*result.Body.Tags)[0].Key != "team" {
			t.Errorf("expected unchanged tags preserved; got %#v", *result.Body.Tags)
		}
	})

	t.Run("size-only diff with no state tags: tags stays nil (nothing to defend)", func(t *testing.T) {
		plan := baseModel("c6gd.xlarge", "none", mapTags())
		state := baseModel("c6gd.large", "none", mapTags())
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body == nil || result.Body.Size != "c6gd.xlarge" {
			t.Fatalf("size diff missing: %#v", result.Body)
		}
		if result.Body.Tags != nil {
			t.Errorf("no state tags → tags should remain nil; got %#v", result.Body.Tags)
		}
	})

	t.Run("combined size + tags change signals transition once", func(t *testing.T) {
		plan := baseModel("c6gd.xlarge", "none", mapTags("env", "prod"))
		state := baseModel("c6gd.large", "none", mapTags())
		result, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if result.Body == nil {
			t.Fatal("expected non-nil Body")
		}
		if result.Body.Size != "c6gd.xlarge" {
			t.Errorf("size not propagated")
		}
		if result.Body.Tags == nil || len(*result.Body.Tags) != 1 {
			t.Errorf("tags not propagated")
		}
		if !result.TransitionExpected {
			t.Errorf("size change inside combined diff must still surface TransitionExpected")
		}
	})
}

// ---------------------------------------------------------------------------
// buildPostgresMatchPredicate
// ---------------------------------------------------------------------------

func TestBuildPostgresMatchPredicate(t *testing.T) {
	t.Run("size-only PATCH: predicate matches when size and state are correct", func(t *testing.T) {
		body := &api.PostgresUpdate{Size: "r6gd.xlarge"}
		predicate := buildPostgresMatchPredicate(body)
		if !predicate(&api.Postgres{State: api.PostgresStateRunning, Size: "r6gd.xlarge"}) {
			t.Error("should match when size+state align")
		}
		if predicate(&api.Postgres{State: api.PostgresStateRunning, Size: "r6gd.large"}) {
			t.Error("must NOT match while size is still pre-PATCH value (queued-work race case)")
		}
		if predicate(&api.Postgres{State: api.PostgresStateRestarting, Size: "r6gd.xlarge"}) {
			t.Error("must NOT match while state is non-terminal")
		}
	})

	t.Run("ha_type-only PATCH: only ha_type is gated", func(t *testing.T) {
		body := &api.PostgresUpdate{HaType: "async"}
		predicate := buildPostgresMatchPredicate(body)
		if !predicate(&api.Postgres{State: api.PostgresStateRunning, HaType: "async", Size: "r6gd.large"}) {
			t.Error("should match regardless of size when only ha_type was PATCHed")
		}
		if predicate(&api.Postgres{State: api.PostgresStateRunning, HaType: "none"}) {
			t.Error("must NOT match while ha_type is still pre-PATCH value")
		}
	})

	t.Run("tags-only PATCH: exact tag set must be present", func(t *testing.T) {
		body := &api.PostgresUpdate{Tags: &[]api.Tag{{Key: "team", Value: "billing"}, {Key: "env", Value: "dev"}}}
		predicate := buildPostgresMatchPredicate(body)
		if !predicate(&api.Postgres{State: api.PostgresStateRunning, Tags: []api.Tag{{Key: "team", Value: "billing"}, {Key: "env", Value: "dev"}}}) {
			t.Error("should match when tags are equal (order-insensitive)")
		}
		if !predicate(&api.Postgres{State: api.PostgresStateRunning, Tags: []api.Tag{{Key: "env", Value: "dev"}, {Key: "team", Value: "billing"}}}) {
			t.Error("should match when tags are equal but in different order")
		}
		if predicate(&api.Postgres{State: api.PostgresStateRunning, Tags: []api.Tag{{Key: "team", Value: "billing"}}}) {
			t.Error("must NOT match when a tag is missing")
		}
		if predicate(&api.Postgres{State: api.PostgresStateRunning, Tags: []api.Tag{{Key: "team", Value: "ops"}, {Key: "env", Value: "dev"}}}) {
			t.Error("must NOT match when a tag value differs")
		}
	})

	t.Run("clear-all tags: matches only when server reports zero tags", func(t *testing.T) {
		empty := []api.Tag{}
		body := &api.PostgresUpdate{Tags: &empty}
		predicate := buildPostgresMatchPredicate(body)
		if !predicate(&api.Postgres{State: api.PostgresStateRunning}) {
			t.Error("should match when server has no tags")
		}
		if predicate(&api.Postgres{State: api.PostgresStateRunning, Tags: []api.Tag{{Key: "team", Value: "billing"}}}) {
			t.Error("must NOT match while server still reports the cleared tags")
		}
	})

	t.Run("combined PATCH: all fields must match simultaneously", func(t *testing.T) {
		body := &api.PostgresUpdate{
			Size:   "r6gd.xlarge",
			HaType: "async",
			Tags:   &[]api.Tag{{Key: "team", Value: "billing"}},
		}
		predicate := buildPostgresMatchPredicate(body)
		if !predicate(&api.Postgres{State: api.PostgresStateRunning, Size: "r6gd.xlarge", HaType: "async", Tags: []api.Tag{{Key: "team", Value: "billing"}}}) {
			t.Error("should match when every PATCHed field reflects the request")
		}
		// Each partial commit must keep the predicate false (the race-trigger case).
		if predicate(&api.Postgres{State: api.PostgresStateRunning, Size: "r6gd.xlarge", HaType: "none", Tags: []api.Tag{{Key: "team", Value: "billing"}}}) {
			t.Error("must NOT match while ha_type still pending")
		}
		if predicate(&api.Postgres{State: api.PostgresStateRunning, Size: "r6gd.large", HaType: "async", Tags: []api.Tag{{Key: "team", Value: "billing"}}}) {
			t.Error("must NOT match while size still pending")
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// modelsEqual compares two PostgresServiceResourceModel values for the
// fields the resource syncs. Uses Equal() on each types.* field so types.Map
// element ordering doesn't make the comparison flaky.
func modelsEqual(t *testing.T, got, want models.PostgresServiceResourceModel) bool {
	t.Helper()
	pairs := []struct {
		name string
		a, b attr.Value
	}{
		{"ID", got.ID, want.ID},
		{"Name", got.Name, want.Name},
		{"CloudProvider", got.CloudProvider, want.CloudProvider},
		{"Region", got.Region, want.Region},
		{"PostgresVersion", got.PostgresVersion, want.PostgresVersion},
		{"Size", got.Size, want.Size},
		{"HaType", got.HaType, want.HaType},
		{"State", got.State, want.State},
		{"CreatedAt", got.CreatedAt, want.CreatedAt},
		{"IsPrimary", got.IsPrimary, want.IsPrimary},
		{"Hostname", got.Hostname, want.Hostname},
		{"Port", got.Port, want.Port},
		{"Username", got.Username, want.Username},
		{"Tags", got.Tags, want.Tags},
		{"Password", got.Password, want.Password},
	}
	ok := true
	for _, p := range pairs {
		if !p.a.Equal(p.b) {
			t.Errorf("  %s: got=%v want=%v", p.name, p.a, p.b)
			ok = false
		}
	}
	return ok
}
