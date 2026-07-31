package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
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

func TestRequiresReplaceIfSchemaRegistryChanges(t *testing.T) {
	t.Parallel()

	credsType := models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes
	srType := models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes

	creds := func(username, password types.String, woVersion types.Int64) types.Object {
		return types.ObjectValueMust(credsType, map[string]attr.Value{
			"username":            username,
			"password":            password,
			"password_wo":         types.StringNull(),
			"password_wo_version": woVersion,
		})
	}
	sr := func(url, auth string, credentials types.Object) types.Object {
		return types.ObjectValueMust(srType, map[string]attr.Value{
			"url":            types.StringValue(url),
			"authentication": types.StringValue(auth),
			"credentials":    credentials,
		})
	}

	knownCreds := creds(types.StringValue("sr-user"), types.StringValue("sr-pass"), types.Int64Null())
	updateRaw := tftypes.NewValue(tftypes.Object{}, map[string]tftypes.Value{})

	testCases := map[string]struct {
		stateRaw                tftypes.Value
		planRaw                 tftypes.Value
		stateValue              types.Object
		planValue               types.Object
		expectedRequiresReplace bool
		expectedWarning         bool
	}{
		"null-to-null": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              types.ObjectNull(srType),
			planValue:               types.ObjectNull(srType),
			expectedRequiresReplace: false,
		},
		"adding-registry": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              types.ObjectNull(srType),
			planValue:               sr("https://sr.example", "PLAIN", knownCreds),
			expectedRequiresReplace: true,
		},
		"removing-registry": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              sr("https://sr.example", "PLAIN", knownCreds),
			planValue:               types.ObjectNull(srType),
			expectedRequiresReplace: true,
		},
		"identical": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              sr("https://sr.example", "PLAIN", knownCreds),
			planValue:               sr("https://sr.example", "PLAIN", knownCreds),
			expectedRequiresReplace: false,
		},
		"url-changed": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              sr("https://sr.example", "PLAIN", knownCreds),
			planValue:               sr("https://other.example", "PLAIN", knownCreds),
			expectedRequiresReplace: true,
		},
		"credentials-changed": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              sr("https://sr.example", "PLAIN", knownCreds),
			planValue:               sr("https://sr.example", "PLAIN", creds(types.StringValue("sr-user"), types.StringValue("rotated"), types.Int64Null())),
			expectedRequiresReplace: true,
		},
		"wo-version-bump": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              sr("https://sr.example", "PLAIN", creds(types.StringValue("sr-user"), types.StringNull(), types.Int64Value(1))),
			planValue:               sr("https://sr.example", "PLAIN", creds(types.StringValue("sr-user"), types.StringNull(), types.Int64Value(2))),
			expectedRequiresReplace: true,
		},
		"import-reconciliation-null-state-credentials": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              sr("https://sr.example", "PLAIN", types.ObjectNull(credsType)),
			planValue:               sr("https://sr.example", "PLAIN", knownCreds),
			expectedRequiresReplace: false,
		},
		"plan-credentials-unknown": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              sr("https://sr.example", "PLAIN", knownCreds),
			planValue:               sr("https://sr.example", "PLAIN", types.ObjectUnknown(credsType)),
			expectedRequiresReplace: false,
			expectedWarning:         true,
		},
		"plan-registry-unknown": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              sr("https://sr.example", "PLAIN", knownCreds),
			planValue:               types.ObjectUnknown(srType),
			expectedRequiresReplace: false,
			expectedWarning:         true,
		},
		"plan-url-unknown": {
			stateRaw:   updateRaw,
			planRaw:    updateRaw,
			stateValue: sr("https://sr.example", "PLAIN", knownCreds),
			planValue: types.ObjectValueMust(srType, map[string]attr.Value{
				"url":            types.StringUnknown(),
				"authentication": types.StringValue("PLAIN"),
				"credentials":    knownCreds,
			}),
			expectedRequiresReplace: false,
			expectedWarning:         true,
		},
		"plan-password-attribute-unknown": {
			stateRaw:                updateRaw,
			planRaw:                 updateRaw,
			stateValue:              sr("https://sr.example", "PLAIN", knownCreds),
			planValue:               sr("https://sr.example", "PLAIN", creds(types.StringValue("sr-user"), types.StringUnknown(), types.Int64Null())),
			expectedRequiresReplace: true,
		},
		"creating-resource": {
			stateRaw:                tftypes.NewValue(tftypes.Object{}, nil),
			planRaw:                 updateRaw,
			stateValue:              types.ObjectNull(srType),
			planValue:               sr("https://sr.example", "PLAIN", knownCreds),
			expectedRequiresReplace: false,
		},
		"destroying-resource": {
			stateRaw:                updateRaw,
			planRaw:                 tftypes.NewValue(tftypes.Object{}, nil),
			stateValue:              sr("https://sr.example", "PLAIN", knownCreds),
			planValue:               types.ObjectNull(srType),
			expectedRequiresReplace: false,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			modifier := requiresReplaceIfSchemaRegistryChanges{}
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
			if hasWarning := resp.Diagnostics.WarningsCount() > 0; hasWarning != testCase.expectedWarning {
				t.Errorf("expected warning presence to be %v, got diagnostics: %v", testCase.expectedWarning, resp.Diagnostics)
			}
		})
	}
}
