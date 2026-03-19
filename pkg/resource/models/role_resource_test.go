package models

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
)

func TestAPIToRolePolicyModel(t *testing.T) {
	tests := []struct {
		name      string
		input     api.RBACPolicy
		wantModel RolePolicyModel
	}{
		{
			name: "scalar fields are mapped",
			input: api.RBACPolicy{
				ID:          "policy-id",
				RoleID:      "role-id",
				TenantID:    "tenant-id",
				AllowDeny:   api.RBACAllowDenyAllow,
				Permissions: []string{"control-plane:service:view"},
			},
			wantModel: RolePolicyModel{
				ID:          types.StringValue("policy-id"),
				RoleID:      types.StringValue("role-id"),
				TenantID:    types.StringValue("tenant-id"),
				Effect:      types.StringValue("ALLOW"),
				Permissions: strSetVal("control-plane:service:view"),
				Resources:   types.SetNull(types.StringType),
				Tags:        types.ObjectNull(RolePolicyTagsModel{}.ObjectType().AttrTypes),
			},
		},
		{
			name: "non-empty resources list is populated",
			input: api.RBACPolicy{
				AllowDeny:   api.RBACAllowDenyAllow,
				Permissions: []string{"perm"},
				Resources:   []string{"instance/*", "instance/abc-123"},
			},
			wantModel: RolePolicyModel{
				ID:          types.StringValue(""),
				RoleID:      types.StringValue(""),
				TenantID:    types.StringValue(""),
				Effect:      types.StringValue("ALLOW"),
				Permissions: strSetVal("perm"),
				Resources:   strSetVal("instance/*", "instance/abc-123"),
				Tags:        types.ObjectNull(RolePolicyTagsModel{}.ObjectType().AttrTypes),
			},
		},
		{
			name: "tags with role set",
			input: api.RBACPolicy{
				AllowDeny:   api.RBACAllowDenyAllow,
				Permissions: []string{"sql-console:database:access"},
				Tags:        &api.RBACPolicyTags{RoleV2: "sql-console-readonly"},
			},
			wantModel: RolePolicyModel{
				ID:          types.StringValue(""),
				RoleID:      types.StringValue(""),
				TenantID:    types.StringValue(""),
				Effect:      types.StringValue("ALLOW"),
				Permissions: strSetVal("sql-console:database:access"),
				Resources:   types.SetNull(types.StringType),
				Tags: RolePolicyTagsModel{
					RoleV2: types.StringValue("sql-console-readonly"),
				}.ObjectValue(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := APIToRolePolicyModel(tt.input)
			if diags.HasError() {
				t.Fatalf("%s unexpected error diagnostics: %v", tt.name, diags)
			}
			if !reflect.DeepEqual(got, tt.wantModel) {
				t.Errorf("%s model does not match:\ngot  = %v\nwant = %v", tt.name, got, tt.wantModel)
			}
		})
	}
}

// strSetVal builds a types.Set of strings for use in tests.
func strSetVal(strs ...string) types.Set {
	values := make([]attr.Value, len(strs))
	for i, s := range strs {
		values[i] = types.StringValue(s)
	}
	s, _ := types.SetValue(types.StringType, values)
	return s
}
