package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
)

func TestRequiresReplaceIfSourceTypeChanges(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		stateRaw                tftypes.Value
		planRaw                 tftypes.Value
		stateValue              types.Object
		planValue               types.Object
		expectedRequiresReplace bool
	}{
		"null-to-null": {
			stateRaw:                tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			planRaw:                 tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			stateValue:              types.ObjectNull(nil),
			planValue:               types.ObjectNull(nil),
			expectedRequiresReplace: false,
		},
		"null-to-value": {
			stateRaw:   tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			planRaw:    tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			stateValue: types.ObjectNull(nil),
			planValue: types.ObjectValueMust(
				map[string]attr.Type{"test": types.StringType},
				map[string]attr.Value{"test": types.StringValue("value")},
			),
			expectedRequiresReplace: true,
		},
		"value-to-null": {
			stateRaw: tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			planRaw:  tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			stateValue: types.ObjectValueMust(
				map[string]attr.Type{"test": types.StringType},
				map[string]attr.Value{"test": types.StringValue("value")},
			),
			planValue:               types.ObjectNull(nil),
			expectedRequiresReplace: true,
		},
		"value-to-different-value": {
			stateRaw: tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			planRaw:  tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			stateValue: types.ObjectValueMust(
				map[string]attr.Type{"test": types.StringType},
				map[string]attr.Value{"test": types.StringValue("old")},
			),
			planValue: types.ObjectValueMust(
				map[string]attr.Type{"test": types.StringType},
				map[string]attr.Value{"test": types.StringValue("new")},
			),
			expectedRequiresReplace: false,
		},
		"value-to-same-value": {
			stateRaw: tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			planRaw:  tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			stateValue: types.ObjectValueMust(
				map[string]attr.Type{"test": types.StringType},
				map[string]attr.Value{"test": types.StringValue("same")},
			),
			planValue: types.ObjectValueMust(
				map[string]attr.Type{"test": types.StringType},
				map[string]attr.Value{"test": types.StringValue("same")},
			),
			expectedRequiresReplace: false,
		},
		"creating-resource": {
			stateRaw:                tftypes.NewValue(tftypes.Object{}, nil),
			planRaw:                 tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			stateValue:              types.ObjectNull(nil),
			planValue:               types.ObjectNull(nil),
			expectedRequiresReplace: false,
		},
		"destroying-resource": {
			stateRaw:                tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			planRaw:                 tftypes.NewValue(tftypes.Object{}, nil),
			stateValue:              types.ObjectNull(nil),
			planValue:               types.ObjectNull(nil),
			expectedRequiresReplace: false,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			modifier := requiresReplaceIfSourceTypeChanges{}
			req := planmodifier.ObjectRequest{
				State: tfsdk.State{
					Raw: testCase.stateRaw,
				},
				Plan: tfsdk.Plan{
					Raw: testCase.planRaw,
				},
				StateValue: testCase.stateValue,
				PlanValue:  testCase.planValue,
			}
			resp := &planmodifier.ObjectResponse{}

			modifier.PlanModifyObject(context.Background(), req, resp)

			if resp.RequiresReplace != testCase.expectedRequiresReplace {
				t.Errorf("expected RequiresReplace to be %v, got %v", testCase.expectedRequiresReplace, resp.RequiresReplace)
			}
		})
	}
}

func TestRequiresReplaceIfSourceTypeChanges_Description(t *testing.T) {
	modifier := requiresReplaceIfSourceTypeChanges{}

	description := modifier.Description(context.Background())
	if description == "" {
		t.Error("Description should not be empty")
	}

	markdownDescription := modifier.MarkdownDescription(context.Background())
	if markdownDescription == "" {
		t.Error("MarkdownDescription should not be empty")
	}
}

// TestVolatileComputedString is the regression test for issue #529: "Provider produced
// inconsistent result after apply: .state: was Paused, but now Snapshot". The original
// `UseStateForUnknown` plan modifier carried the prior state value forward, but the API
// can transition the pipe to a transient state (Snapshot during table_mappings update,
// Paused during stopped=true toggle) before settling, causing the framework's post-apply
// consistency check to fail. volatileComputedString marks the planned value Unknown for
// any real update, accepting any post-apply value, while preserving the prior value for
// refresh-only plans to avoid spurious diffs.
func TestVolatileComputedString(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		stateRaw          tftypes.Value
		planRaw           tftypes.Value
		stateValue        types.String
		initialPlanValue  types.String
		expectedPlanValue types.String
	}{
		"create-resource leaves PlanValue untouched (state is null)": {
			stateRaw:          tftypes.NewValue(tftypes.Object{}, nil),
			planRaw:           tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			stateValue:        types.StringNull(),
			initialPlanValue:  types.StringUnknown(),
			expectedPlanValue: types.StringUnknown(),
		},
		"destroy-resource leaves PlanValue untouched (plan is null)": {
			stateRaw:          tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{}),
			planRaw:           tftypes.NewValue(tftypes.Object{}, nil),
			stateValue:        types.StringValue("Running"),
			initialPlanValue:  types.StringValue("Running"),
			expectedPlanValue: types.StringValue("Running"),
		},
		"refresh-only copies StateValue into plan (no churn on no-op refresh)": {
			// state.Raw == plan.Raw → no real update; preserve the prior value.
			stateRaw: tftypes.NewValue(tftypes.Object{
				AttributeTypes: map[string]tftypes.Type{"test": tftypes.String},
			}, map[string]tftypes.Value{
				"test": tftypes.NewValue(tftypes.String, "same"),
			}),
			planRaw: tftypes.NewValue(tftypes.Object{
				AttributeTypes: map[string]tftypes.Type{"test": tftypes.String},
			}, map[string]tftypes.Value{
				"test": tftypes.NewValue(tftypes.String, "same"),
			}),
			stateValue:        types.StringValue("Running"),
			initialPlanValue:  types.StringUnknown(),
			expectedPlanValue: types.StringValue("Running"),
		},
		"real update marks PlanValue Unknown (the #529 fix)": {
			// state.Raw != plan.Raw → update in flight; mark Unknown so the framework
			// accepts whatever transient state the API surfaces (Snapshot, Paused, etc.).
			stateRaw: tftypes.NewValue(tftypes.Object{
				AttributeTypes: map[string]tftypes.Type{"test": tftypes.String},
			}, map[string]tftypes.Value{
				"test": tftypes.NewValue(tftypes.String, "old"),
			}),
			planRaw: tftypes.NewValue(tftypes.Object{
				AttributeTypes: map[string]tftypes.Type{"test": tftypes.String},
			}, map[string]tftypes.Value{
				"test": tftypes.NewValue(tftypes.String, "new"),
			}),
			stateValue:        types.StringValue("Paused"),
			initialPlanValue:  types.StringValue("Paused"),
			expectedPlanValue: types.StringUnknown(),
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			modifier := volatileComputedString{}
			req := planmodifier.StringRequest{
				State:      tfsdk.State{Raw: testCase.stateRaw},
				Plan:       tfsdk.Plan{Raw: testCase.planRaw},
				StateValue: testCase.stateValue,
				PlanValue:  testCase.initialPlanValue,
			}
			resp := &planmodifier.StringResponse{PlanValue: testCase.initialPlanValue}

			modifier.PlanModifyString(context.Background(), req, resp)

			if !resp.PlanValue.Equal(testCase.expectedPlanValue) {
				t.Errorf("expected PlanValue to be %v, got %v", testCase.expectedPlanValue, resp.PlanValue)
			}
		})
	}
}

func TestVolatileComputedString_Description(t *testing.T) {
	modifier := volatileComputedString{}

	if modifier.Description(context.Background()) == "" {
		t.Error("Description should not be empty")
	}
	if modifier.MarkdownDescription(context.Background()) == "" {
		t.Error("MarkdownDescription should not be empty")
	}
}

// TestRequiresReplaceIfBaseTypeChanges verifies that collapsing a legacy provider-flavored
// source type to its base type (e.g. rdspostgres → postgres) updates in place, while a real
// base-type change (mysql → mariadb) still forces replacement.
func TestRequiresReplaceIfBaseTypeChanges(t *testing.T) {
	t.Parallel()

	nonNull := tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})

	testCases := map[string]struct {
		collapse                func(string) string
		stateRaw                tftypes.Value
		planRaw                 tftypes.Value
		stateValue              types.String
		planValue               types.String
		expectedRequiresReplace bool
	}{
		"postgres flavor collapses to base - no replace": {
			collapse:                api.CollapsePostgresSourceType,
			stateRaw:                nonNull,
			planRaw:                 nonNull,
			stateValue:              types.StringValue("rdspostgres"),
			planValue:               types.StringValue("postgres"),
			expectedRequiresReplace: false,
		},
		"postgres same base - no replace": {
			collapse:                api.CollapsePostgresSourceType,
			stateRaw:                nonNull,
			planRaw:                 nonNull,
			stateValue:              types.StringValue("postgres"),
			planValue:               types.StringValue("postgres"),
			expectedRequiresReplace: false,
		},
		"mysql provider prefix collapses to base - no replace": {
			collapse:                api.CollapseMySQLSourceType,
			stateRaw:                nonNull,
			planRaw:                 nonNull,
			stateValue:              types.StringValue("rdsmysql"),
			planValue:               types.StringValue("mysql"),
			expectedRequiresReplace: false,
		},
		"mariadb prefix collapses to mariadb - no replace": {
			collapse:                api.CollapseMySQLSourceType,
			stateRaw:                nonNull,
			planRaw:                 nonNull,
			stateValue:              types.StringValue("rdsmariadb"),
			planValue:               types.StringValue("mariadb"),
			expectedRequiresReplace: false,
		},
		"mysql to mariadb changes engine - replace": {
			collapse:                api.CollapseMySQLSourceType,
			stateRaw:                nonNull,
			planRaw:                 nonNull,
			stateValue:              types.StringValue("mysql"),
			planValue:               types.StringValue("mariadb"),
			expectedRequiresReplace: true,
		},
		"creating-resource - no replace": {
			collapse:                api.CollapsePostgresSourceType,
			stateRaw:                tftypes.NewValue(tftypes.Object{}, nil),
			planRaw:                 nonNull,
			stateValue:              types.StringNull(),
			planValue:               types.StringValue("postgres"),
			expectedRequiresReplace: false,
		},
		"destroying-resource - no replace": {
			collapse:                api.CollapsePostgresSourceType,
			stateRaw:                nonNull,
			planRaw:                 tftypes.NewValue(tftypes.Object{}, nil),
			stateValue:              types.StringValue("postgres"),
			planValue:               types.StringNull(),
			expectedRequiresReplace: false,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			modifier := requiresReplaceIfBaseTypeChanges{collapse: testCase.collapse}
			req := planmodifier.StringRequest{
				State:      tfsdk.State{Raw: testCase.stateRaw},
				Plan:       tfsdk.Plan{Raw: testCase.planRaw},
				StateValue: testCase.stateValue,
				PlanValue:  testCase.planValue,
			}
			resp := &planmodifier.StringResponse{}

			modifier.PlanModifyString(context.Background(), req, resp)

			if resp.RequiresReplace != testCase.expectedRequiresReplace {
				t.Errorf("expected RequiresReplace to be %v, got %v", testCase.expectedRequiresReplace, resp.RequiresReplace)
			}
		})
	}
}

func TestCollapseSourceType(t *testing.T) {
	t.Parallel()

	if got := api.CollapsePostgresSourceType("aurorapostgres"); got != "postgres" {
		t.Errorf("CollapsePostgresSourceType(aurorapostgres) = %q, want postgres", got)
	}
	if got := api.CollapsePostgresSourceType("postgres"); got != "postgres" {
		t.Errorf("CollapsePostgresSourceType(postgres) = %q, want postgres", got)
	}
	if got := api.CollapseMySQLSourceType("auroramysql"); got != "mysql" {
		t.Errorf("CollapseMySQLSourceType(auroramysql) = %q, want mysql", got)
	}
	if got := api.CollapseMySQLSourceType("rdsmariadb"); got != "mariadb" {
		t.Errorf("CollapseMySQLSourceType(rdsmariadb) = %q, want mariadb", got)
	}
	// Unknown values pass through unchanged.
	if got := api.CollapsePostgresSourceType("something-else"); got != "something-else" {
		t.Errorf("CollapsePostgresSourceType(something-else) = %q, want something-else", got)
	}
}
