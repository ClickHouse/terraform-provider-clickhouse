//go:build alpha

package resource

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

// boolPtrPG / strPtrPG — local readability sugar for *bool / *string
// fixtures. Suffixed to avoid duplicate declarations with clickpipe_test.go.
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
				Username:         "default",
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
	v := stringvalidator.LengthAtLeast(1)
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
// buildPartialCreateState — mid-Create intermediate state shape
//
// Between CreatePostgres' 200 and the post-wait re-read, the resource
// writes a state with just id + password so a wait-step failure leaves
// Terraform able to reconcile against the real server resource. The
// function is small but behavioral: if the framework rejects the mid-
// Create write (e.g., a computed attribute is Unknown), the user ends
// up with an orphaned server-side instance and no Terraform reference.
// ---------------------------------------------------------------------------

func TestBuildPartialCreateState(t *testing.T) {
	// Minimal plan resembling what req.Plan.Get would return for a fresh
	// resource: required attrs set, computed attrs Unknown (the framework's
	// default before any state exists).
	planFresh := func() models.PostgresServiceResourceModel {
		return models.PostgresServiceResourceModel{
			Name:             types.StringValue("primary-1"),
			CloudProvider:    types.StringValue("aws"),
			Region:           types.StringValue("us-east-1"),
			Size:             types.StringValue("c6gd.large"),
			PostgresVersion:  types.StringUnknown(), // Optional+Computed, not set in .tf
			HaType:           types.StringUnknown(),
			Tags:             types.SetUnknown(models.PostgresServiceTagObjectType()),
			ID:               types.StringUnknown(),
			Password:         types.StringUnknown(),
			State:            types.StringUnknown(),
			CreatedAt:        types.StringUnknown(),
			IsPrimary:        types.BoolUnknown(),
			Hostname:         types.StringUnknown(),
			Port:             types.Int64Unknown(),
			Username:         types.StringUnknown(),
			ConnectionString: types.StringUnknown(),
		}
	}

	pg := &api.Postgres{
		Id:              "pg-mid-create",
		Name:            "primary-1",
		Provider:        "aws",
		Region:          "us-east-1",
		Size:            "c6gd.large",
		PostgresVersion: "18",
		State:           api.PostgresStateCreating,
	}

	t.Run("with server-generated password", func(t *testing.T) {
		password := "ServerGen123XYZ"
		partial := buildPartialCreateState(planFresh(), pg, password)

		// Must carry id + password; otherwise the recovery contract breaks.
		if partial.ID.ValueString() != "pg-mid-create" {
			t.Errorf("id missing or wrong; got %v", partial.ID)
		}
		if partial.Password.ValueString() != password {
			t.Errorf("password not persisted; got %v", partial.Password)
		}

		// Every other Computed attr must be explicitly Null, never Unknown —
		// the framework rejects mid-Create state writes containing Unknown
		// computed values. The whole point of this helper.
		mustBeNull := []struct {
			name string
			v    attr.Value
		}{
			{"State", partial.State},
			{"CreatedAt", partial.CreatedAt},
			{"IsPrimary", partial.IsPrimary},
			{"Hostname", partial.Hostname},
			{"Port", partial.Port},
			{"Username", partial.Username},
			{"ConnectionString", partial.ConnectionString},
			{"Tags", partial.Tags},
		}
		for _, attr := range mustBeNull {
			if attr.v.IsUnknown() {
				t.Errorf("%s must not be Unknown mid-Create; got %v", attr.name, attr.v)
			}
			if !attr.v.IsNull() {
				t.Errorf("%s must be explicit Null mid-Create; got %v", attr.name, attr.v)
			}
		}

		// HaType / PostgresVersion: came in as Unknown from a fresh plan;
		// helper must pin them to concrete values so the state write
		// validator accepts them.
		if partial.HaType.IsUnknown() || partial.HaType.ValueString() != "none" {
			t.Errorf("HaType must default to 'none'; got %v", partial.HaType)
		}
		if partial.PostgresVersion.IsUnknown() || partial.PostgresVersion.ValueString() != "18" {
			t.Errorf("PostgresVersion must be pinned from server response; got %v", partial.PostgresVersion)
		}
	})

	t.Run("with no server-generated password", func(t *testing.T) {
		partial := buildPartialCreateState(planFresh(), pg, "")
		if !partial.Password.IsNull() {
			t.Errorf("Password must be explicit Null when no generated password; got %v", partial.Password)
		}
	})

	t.Run("preserves user-set HaType from plan", func(t *testing.T) {
		plan := planFresh()
		plan.HaType = types.StringValue("async")
		partial := buildPartialCreateState(plan, pg, "")
		if partial.HaType.ValueString() != "async" {
			t.Errorf("user-set HaType must survive; got %v", partial.HaType)
		}
	})

	t.Run("preserves user-set PostgresVersion from plan", func(t *testing.T) {
		plan := planFresh()
		plan.PostgresVersion = types.StringValue("17")
		partial := buildPartialCreateState(plan, pg, "")
		if partial.PostgresVersion.ValueString() != "17" {
			t.Errorf("user-set PostgresVersion must survive; got %v", partial.PostgresVersion)
		}
	})

	t.Run("preserves user-set tags from plan", func(t *testing.T) {
		plan := planFresh()
		tagObj, _ := types.ObjectValue(
			tagAttrTypes(),
			map[string]attr.Value{"key": types.StringValue("team"), "value": types.StringValue("billing")},
		)
		set, _ := types.SetValue(models.PostgresServiceTagObjectType(), []attr.Value{tagObj})
		plan.Tags = set
		partial := buildPartialCreateState(plan, pg, "")
		if partial.Tags.IsNull() || partial.Tags.IsUnknown() {
			t.Errorf("user-set tags must survive mid-Create; got %v", partial.Tags)
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
		plan := baseModel("c6gd.xlarge", "none", emptyTags())
		state := baseModel("c6gd.large", "none", emptyTags())
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
		plan := baseModel("c6gd.large", "async", emptyTags())
		state := baseModel("c6gd.large", "none", emptyTags())
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
		plan := baseModel("c6gd.large", "none", tagSet("team", "billing"))
		state := baseModel("c6gd.large", "none", emptyTags())
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

	t.Run("tags cleared: plan has null tags, state had tags", func(t *testing.T) {
		// Removing all tags must send "tags": [] (empty array), not omit
		// the field entirely. Validates that the pointer-to-slice in
		// PostgresUpdate.Tags is being used correctly.
		plan := baseModel("c6gd.large", "none", emptyTags())
		state := baseModel("c6gd.large", "none", tagSet("team", "billing"))
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
		// Review feedback: the existing matrix never exercised changing a
		// value on an existing key. This is the exact path that would have
		// caught the (now-fixed) "tag value omitted" issue if the value
		// transitioned to null.
		plan := baseModel("c6gd.large", "none", tagSet("team", "engineering"))
		state := baseModel("c6gd.large", "none", tagSet("team", "billing"))
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
		// Wire-shape assertion: value must be present on the wire (not
		// omitted via the api.Tag.Value omitempty tag).
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
		//
		// Combined defence: whenever Unknown plan tags meet non-empty
		// state tags AND we're patching something else, force the state's
		// tags into the PATCH body so the server can't drop them.
		plan := baseModel("c6gd.xlarge", "none", types.SetUnknown(models.PostgresServiceTagObjectType()))
		state := baseModel("c6gd.large", "none", tagSet("team", "billing"))
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
		// Server PATCH endpoint clears tags when the request body omits
		// them. When the user changes size and state has tags, we MUST
		// include those tags in the PATCH body even though the user
		// didn't ask us to.
		plan := baseModel("c6gd.xlarge", "none", tagSet("team", "billing"))
		state := baseModel("c6gd.large", "none", tagSet("team", "billing"))
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
		// Inverse case: no tags in state means no risk of server-clear.
		// Body.Tags should stay nil so we don't send an empty array
		// unnecessarily.
		plan := baseModel("c6gd.xlarge", "none", emptyTags())
		state := baseModel("c6gd.large", "none", emptyTags())
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
		plan := baseModel("c6gd.xlarge", "none", tagSet("env", "prod"))
		state := baseModel("c6gd.large", "none", emptyTags())
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
// fields the resource syncs. Uses Equal() on each types.* field so types.Set
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
