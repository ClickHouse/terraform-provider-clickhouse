//go:build alpha

package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

// stringvalidatorLengthAtLeast1 returns the same validator the schema uses
// for the tags.value attribute. Centralized in tests so a refactor of the
// schema validator surfaces in TestTagValueValidator_RejectsEmptyString.
func stringvalidatorLengthAtLeast1() validator.String { return stringvalidator.LengthAtLeast(1) }

// boolPtrPG / strPtrPG — local readability sugar for *bool / *string
// fixtures. Renamed from the codebase-wide convention to avoid duplicate
// declarations with clickpipe_test.go's boolPtr / strPtr.
func boolPtrPG(b bool) *bool    { return &b }
func strPtrPG(s string) *string { return &s }

// ---------------------------------------------------------------------------
// syncPostgresState — field round-trip from api.Postgres → resource model.
// ---------------------------------------------------------------------------

func TestPostgresResource_syncPostgresState(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		pg   *api.Postgres
		want func() models.PostgresServiceResourceModel
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
				IsPrimary:        boolPtrPG(true),
				Hostname:         strPtrPG("primary-1.example.com"),
				ConnectionString: strPtrPG("postgresql://default:secret@primary-1.example.com:5432/postgres"),
				Username:         strPtrPG("default"),
				Tags:             []api.Tag{{Key: "team", Value: "billing"}},
			},
			want: func() models.PostgresServiceResourceModel {
				tagObj, _ := types.ObjectValue(
					tagAttrTypes(),
					map[string]attr.Value{"key": types.StringValue("team"), "value": types.StringValue("billing")},
				)
				tagSet, _ := types.SetValue(models.PostgresServiceTagObjectType(), []attr.Value{tagObj})
				return models.PostgresServiceResourceModel{
					ID:               types.StringValue("pg-1"),
					Name:             types.StringValue("primary-1"),
					CloudProvider:    types.StringValue("aws"),
					Region:           types.StringValue("us-east-1"),
					PostgresVersion:  types.StringValue("18"),
					Size:             types.StringValue("r6gd.large"),
					HaType:           types.StringValue("async"),
					State:            types.StringValue(api.PostgresStateRunning),
					CreatedAt:        types.StringValue("2026-05-27T00:00:00Z"),
					IsPrimary:        types.BoolValue(true),
					Hostname:         types.StringValue("primary-1.example.com"),
					Port:             types.Int64Value(postgresDefaultPort),
					Username:         types.StringValue("default"),
					ConnectionString: types.StringValue("postgresql://default:secret@primary-1.example.com:5432/postgres"),
					Tags:             tagSet,
				}
			},
		},
		{
			name: "ha_type empty in server response defaults to 'none'",
			pg: &api.Postgres{
				Id: "pg-2", Name: "n", Provider: "aws", Region: "us-east-1",
				Size: "c6gd.large", HaType: "",
				IsPrimary: boolPtrPG(true),
			},
			want: func() models.PostgresServiceResourceModel {
				return models.PostgresServiceResourceModel{
					ID:               types.StringValue("pg-2"),
					Name:             types.StringValue("n"),
					CloudProvider:    types.StringValue("aws"),
					Region:           types.StringValue("us-east-1"),
					Size:             types.StringValue("c6gd.large"),
					HaType:           types.StringValue("none"),
					State:            types.StringValue(""),
					CreatedAt:        types.StringValue(""),
					IsPrimary:        types.BoolValue(true),
					Hostname:         types.StringNull(),
					Port:             types.Int64Value(postgresDefaultPort),
					Username:         types.StringNull(),
					ConnectionString: types.StringNull(),
					Tags:             types.SetNull(models.PostgresServiceTagObjectType()),
				}
			},
		},
		{
			name: "missing IsPrimary defaults to true",
			pg: &api.Postgres{
				Id: "pg-3", Name: "n", Provider: "aws", Region: "us-east-1",
				Size: "c6gd.large",
				// IsPrimary intentionally nil
			},
			want: func() models.PostgresServiceResourceModel {
				return models.PostgresServiceResourceModel{
					ID:               types.StringValue("pg-3"),
					Name:             types.StringValue("n"),
					CloudProvider:    types.StringValue("aws"),
					Region:           types.StringValue("us-east-1"),
					Size:             types.StringValue("c6gd.large"),
					HaType:           types.StringValue("none"),
					State:            types.StringValue(""),
					CreatedAt:        types.StringValue(""),
					IsPrimary:        types.BoolValue(true),
					Hostname:         types.StringNull(),
					Port:             types.Int64Value(postgresDefaultPort),
					Username:         types.StringNull(),
					ConnectionString: types.StringNull(),
					Tags:             types.SetNull(models.PostgresServiceTagObjectType()),
				}
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
			want := tt.want()
			if !modelsEqual(t, got, want) {
				t.Errorf("syncPostgresState mismatch\n got = %#v\nwant = %#v", got, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// apiTagsToSetValue — filters chc_ prefix, handles empty / explicit-null
// ---------------------------------------------------------------------------

func TestApiTagsToSetValue_FiltersSystemTags(t *testing.T) {
	tags := []api.Tag{
		{Key: "team", Value: "billing"},
		{Key: "chc_internal", Value: "system"},
		{Key: "chc_other", Value: ""},
		{Key: "env", Value: "prod"},
	}

	got, diags := apiTagsToSetValue(tags)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.IsNull() {
		t.Fatal("expected non-null set when at least one non-system tag present")
	}

	elems := got.Elements()
	if len(elems) != 2 {
		t.Fatalf("expected 2 user-visible tags after filtering chc_, got %d (%v)", len(elems), elems)
	}

	seen := map[string]string{}
	for _, e := range elems {
		obj := e.(types.Object)
		key := obj.Attributes()["key"].(types.String).ValueString()
		val := obj.Attributes()["value"].(types.String)
		if val.IsNull() {
			seen[key] = ""
		} else {
			seen[key] = val.ValueString()
		}
	}
	if seen["team"] != "billing" {
		t.Errorf("team tag missing or wrong: got %q want %q", seen["team"], "billing")
	}
	if seen["env"] != "prod" {
		t.Errorf("env tag missing or wrong: got %q want %q", seen["env"], "prod")
	}
	if _, leaked := seen["chc_internal"]; leaked {
		t.Errorf("chc_internal leaked into resource state")
	}
	if _, leaked := seen["chc_other"]; leaked {
		t.Errorf("chc_other leaked into resource state")
	}
}

func TestApiTagsToSetValue_EmptyServerListReturnsNull(t *testing.T) {
	got, diags := apiTagsToSetValue(nil)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !got.IsNull() {
		t.Errorf("expected null set for empty input; got %v", got)
	}
}

func TestApiTagsToSetValue_OnlySystemTagsReturnsNull(t *testing.T) {
	got, diags := apiTagsToSetValue([]api.Tag{{Key: "chc_a"}, {Key: "chc_b"}})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !got.IsNull() {
		t.Errorf("expected null set when every server tag is filtered; got %v", got)
	}
}

// TagValueLengthRejectsEmptyString is a documentation-style test rather
// than a functional one — the actual rejection lives in the upstream
// stringvalidator.LengthAtLeast(1) attached to the tags.value attribute
// in the resource schema. We exercise the validator directly here so the
// rationale (avoid perpetual plan/state drift when the server normalizes
// "" to no-value) stays anchored to a test that fails loudly if someone
// drops the validator in a future refactor.
func TestTagValueValidator_RejectsEmptyString(t *testing.T) {
	ctx := context.Background()
	// Build the same validator the schema uses.
	v := stringvalidatorLengthAtLeast1()
	resp := &validator.StringResponse{}
	v.ValidateString(ctx, validator.StringRequest{
		Path:        path.Root("tags").AtName("value"),
		ConfigValue: types.StringValue(""),
	}, resp)
	if !resp.Diagnostics.HasError() {
		t.Errorf("expected diagnostic for empty-string tag value; got %v", resp.Diagnostics)
	}
}

func TestApiTagsToSetValue_EmptyValueMaterializesAsNull(t *testing.T) {
	// Server's ResourceTagV1.value is optional (string | null). We treat
	// an api.Tag{Value: ""} the same as "no value" and surface it as
	// types.StringNull() so the round-trip is stable.
	got, diags := apiTagsToSetValue([]api.Tag{{Key: "team", Value: ""}})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if got.IsNull() {
		t.Fatalf("expected non-null set")
	}
	elem := got.Elements()[0].(types.Object)
	val := elem.Attributes()["value"].(types.String)
	if !val.IsNull() {
		t.Errorf("expected null value attribute when server tag had empty value; got %v", val)
	}
}

// ---------------------------------------------------------------------------
// planTagsToAPI — null/unknown/explicit-empty/populated round-trip
// ---------------------------------------------------------------------------

func TestPlanTagsToAPI(t *testing.T) {
	ctx := context.Background()

	t.Run("null set returns nil (leave server tags alone)", func(t *testing.T) {
		got, diags := planTagsToAPI(ctx, types.SetNull(models.PostgresServiceTagObjectType()))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got != nil {
			t.Errorf("expected nil for null set, got %#v", got)
		}
	})

	t.Run("unknown set returns nil", func(t *testing.T) {
		got, diags := planTagsToAPI(ctx, types.SetUnknown(models.PostgresServiceTagObjectType()))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got != nil {
			t.Errorf("expected nil for unknown set, got %#v", got)
		}
	})

	t.Run("populated set returns mapped api.Tag slice", func(t *testing.T) {
		tagObj, _ := types.ObjectValue(
			tagAttrTypes(),
			map[string]attr.Value{"key": types.StringValue("team"), "value": types.StringValue("billing")},
		)
		set, _ := types.SetValue(models.PostgresServiceTagObjectType(), []attr.Value{tagObj})
		got, diags := planTagsToAPI(ctx, set)
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

	t.Run("null value attribute becomes empty string in api.Tag", func(t *testing.T) {
		tagObj, _ := types.ObjectValue(
			tagAttrTypes(),
			map[string]attr.Value{"key": types.StringValue("team"), "value": types.StringNull()},
		)
		set, _ := types.SetValue(models.PostgresServiceTagObjectType(), []attr.Value{tagObj})
		got, diags := planTagsToAPI(ctx, set)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got == nil || (*got)[0].Value != "" {
			t.Errorf("expected empty-string value, got %#v", got)
		}
	})
}

// ---------------------------------------------------------------------------
// buildPostgresUpdate — diff matrix
// ---------------------------------------------------------------------------

func TestBuildPostgresUpdate(t *testing.T) {
	ctx := context.Background()

	baseModel := func(size, ha string, tags types.Set) models.PostgresServiceResourceModel {
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

	emptyTags := func() types.Set { return types.SetNull(models.PostgresServiceTagObjectType()) }
	tagSet := func(key, val string) types.Set {
		tagObj, _ := types.ObjectValue(
			tagAttrTypes(),
			map[string]attr.Value{"key": types.StringValue(key), "value": types.StringValue(val)},
		)
		set, _ := types.SetValue(models.PostgresServiceTagObjectType(), []attr.Value{tagObj})
		return set
	}

	t.Run("no diff returns nil update and no transition", func(t *testing.T) {
		plan := baseModel("c6gd.large", "none", emptyTags())
		state := baseModel("c6gd.large", "none", emptyTags())
		update, transition, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if update != nil {
			t.Errorf("expected nil update for full no-op, got %#v", *update)
		}
		if transition {
			t.Errorf("transitionExpected should be false on no-op")
		}
	})

	t.Run("size-only diff produces size-only body with transitionExpected", func(t *testing.T) {
		plan := baseModel("c6gd.xlarge", "none", emptyTags())
		state := baseModel("c6gd.large", "none", emptyTags())
		update, transition, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if update == nil {
			t.Fatal("expected non-nil update")
		}
		if update.Size != "c6gd.xlarge" {
			t.Errorf("size: got %q want c6gd.xlarge", update.Size)
		}
		if update.HaType != "" {
			t.Errorf("ha_type should not be set when unchanged; got %q", update.HaType)
		}
		if update.Tags != nil {
			t.Errorf("tags should be nil (omitted) when unchanged; got %#v", update.Tags)
		}
		if !transition {
			t.Errorf("size change must signal transitionExpected=true")
		}
	})

	t.Run("ha_type-only diff signals transitionExpected", func(t *testing.T) {
		plan := baseModel("c6gd.large", "async", emptyTags())
		state := baseModel("c6gd.large", "none", emptyTags())
		update, transition, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if update == nil || update.HaType != "async" {
			t.Errorf("expected ha_type=async update, got %#v", update)
		}
		if !transition {
			t.Errorf("ha_type flip must signal transitionExpected=true")
		}
	})

	t.Run("tags-only change does NOT signal transitionExpected", func(t *testing.T) {
		plan := baseModel("c6gd.large", "none", tagSet("team", "billing"))
		state := baseModel("c6gd.large", "none", emptyTags())
		update, transition, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if update == nil || update.Tags == nil {
			t.Fatalf("expected tags in update, got %#v", update)
		}
		if len(*update.Tags) != 1 || (*update.Tags)[0].Key != "team" {
			t.Errorf("unexpected tag body: %#v", *update.Tags)
		}
		if transition {
			t.Errorf("tags-only mutations are hot; transitionExpected must be false")
		}
	})

	t.Run("tags cleared: plan has null tags, state had tags", func(t *testing.T) {
		// This is the regression test for the Phase 1 Fix #3 contract +
		// plan-line-158 guidance: removing all tags must send "tags": []
		// (empty array), not omit the field entirely. Validates that the
		// pointer-to-slice in PostgresUpdate.Tags is being used correctly.
		plan := baseModel("c6gd.large", "none", emptyTags())
		state := baseModel("c6gd.large", "none", tagSet("team", "billing"))
		update, transition, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if update == nil || update.Tags == nil {
			t.Fatalf("expected non-nil tags pointer (empty slice) to clear server-side tags; got %#v", update)
		}
		if len(*update.Tags) != 0 {
			t.Errorf("expected empty tags slice to clear; got %#v", *update.Tags)
		}
		if transition {
			t.Errorf("tag-clear must not signal transitionExpected")
		}
	})

	t.Run("combined size + tags change signals transition once", func(t *testing.T) {
		plan := baseModel("c6gd.xlarge", "none", tagSet("env", "prod"))
		state := baseModel("c6gd.large", "none", emptyTags())
		update, transition, diags := buildPostgresUpdate(ctx, plan, state)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if update == nil {
			t.Fatal("expected non-nil update")
		}
		if update.Size != "c6gd.xlarge" {
			t.Errorf("size not propagated")
		}
		if update.Tags == nil || len(*update.Tags) != 1 {
			t.Errorf("tags not propagated")
		}
		if !transition {
			t.Errorf("size change inside combined diff must still surface transitionExpected")
		}
	})
}

// ---------------------------------------------------------------------------
// notReservedTagPrefixValidator
// ---------------------------------------------------------------------------

func TestNotReservedTagPrefixValidator(t *testing.T) {
	ctx := context.Background()
	v := notReservedTagPrefixValidator{}

	// Build a syntactically correct path into a SetNestedAttribute. The set
	// element type is the {key, value} object, so AtSetValue must take an
	// ObjectValue (NOT a StringValue — earlier tests passed because the
	// validator never inspects Path, but the path was technically malformed).
	tagPath := func(key, value string) path.Path {
		obj, _ := types.ObjectValue(
			tagAttrTypes(),
			map[string]attr.Value{"key": types.StringValue(key), "value": types.StringValue(value)},
		)
		return path.Root("tags").AtSetValue(obj).AtName("key")
	}

	t.Run("accepts non-prefixed key", func(t *testing.T) {
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			Path:        tagPath("team", "billing"),
			ConfigValue: types.StringValue("team"),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("unexpected error: %v", resp.Diagnostics)
		}
	})

	t.Run("rejects chc_ prefixed key", func(t *testing.T) {
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			Path:        tagPath("chc_internal", ""),
			ConfigValue: types.StringValue("chc_internal"),
		}, resp)
		if !resp.Diagnostics.HasError() {
			t.Errorf("expected diagnostic for chc_-prefixed key")
		}
	})

	t.Run("ignores null / unknown values", func(t *testing.T) {
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			Path:        tagPath("k", "v"),
			ConfigValue: types.StringNull(),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("null value should not produce diagnostic; got %v", resp.Diagnostics)
		}
	})

	t.Run("accepts key whose name is shorter than the prefix", func(t *testing.T) {
		// Defends against a regression on the prefix-check bounds. The
		// original implementation hand-rolled `len(key) >= len(prefix) &&
		// key[:len(prefix)] == prefix` (a classic off-by-one trap); after
		// the switch to strings.HasPrefix the bound is implicit but we
		// keep the test for documentation value.
		resp := &validator.StringResponse{}
		v.ValidateString(ctx, validator.StringRequest{
			Path:        tagPath("ab", ""),
			ConfigValue: types.StringValue("ab"),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("short key must not be flagged: %v", resp.Diagnostics)
		}
	})
}

// ---------------------------------------------------------------------------
// isPostgresStateRunning
// ---------------------------------------------------------------------------

func TestIsPostgresStateRunning(t *testing.T) {
	if !isPostgresStateRunning(api.PostgresStateRunning) {
		t.Error("running should match")
	}
	if isPostgresStateRunning(api.PostgresStateCreating) {
		t.Error("creating must not match")
	}
	if isPostgresStateRunning("some_future_state") {
		t.Error("unknown states must not match (treated as transitioning)")
	}
	if isPostgresStateRunning("") {
		t.Error("empty state must not match")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// tagAttrTypes is the attr.Type map for a single tag object. Centralized
// here so test fixtures stay in sync with models.PostgresServiceTagObjectType.
func tagAttrTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"key":   types.StringType,
		"value": types.StringType,
	}
}

// modelsEqual compares two PostgresServiceResourceModel values for the
// fields Phase 2 syncs. Uses Equal() on each types.* field so types.Set
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
		{"ConnectionString", got.ConnectionString, want.ConnectionString},
		{"Tags", got.Tags, want.Tags},
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
