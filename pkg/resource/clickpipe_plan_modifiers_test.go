//go:build alpha

package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestRequiresReplaceIfSourceTypeChanges(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		stateRaw          tftypes.Value
		planRaw           tftypes.Value
		stateValue        types.Object
		planValue         types.Object
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
		name, testCase := name, testCase
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
