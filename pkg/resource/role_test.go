//go:build alpha

package resource

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRoleResource_syncRoleState(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		state       models.RoleResourceModel
		response    *api.RBACRole
		responseErr error
		wantErr     bool
		wantState   models.RoleResourceModel
	}{
		{
			name:  "maps scalar fields from API response",
			state: models.RoleResourceModel{ID: types.StringValue("role-1")},
			response: &api.RBACRole{
				ID:        "role-1",
				TenantID:  "tenant-1",
				OwnerID:   "owner-1",
				Name:      "my-role",
				Type:      api.RBACRoleTypeCustom,
				Actors:    []string{},
				Policies:  []api.RBACPolicy{},
				CreatedAt: "2024-01-01T00:00:00Z",
				UpdatedAt: "2024-01-02T00:00:00Z",
			},
			wantState: models.RoleResourceModel{
				ID:        types.StringValue("role-1"),
				TenantID:  types.StringValue("tenant-1"),
				OwnerID:   types.StringValue("owner-1"),
				Name:      types.StringValue("my-role"),
				Type:      types.StringValue(api.RBACRoleTypeCustom),
				CreatedAt: types.StringValue("2024-01-01T00:00:00Z"),
				UpdatedAt: types.StringValue("2024-01-02T00:00:00Z"),
				Policies:  types.ListNull(models.RolePolicyModel{}.ObjectType()),
			},
		},
		{
			name:  "maps policies from API response",
			state: models.RoleResourceModel{ID: types.StringValue("role-1")},
			response: &api.RBACRole{
				ID: "role-1",
				Policies: []api.RBACPolicy{
					{ID: "pol-1", AllowDeny: api.RBACAllowDenyAllow, Permissions: []string{"control-plane:service:view"}},
					{ID: "pol-2", AllowDeny: api.RBACAllowDenyDeny, Permissions: []string{"control-plane:organization:manage-billing"}},
				},
			},
			wantState: models.RoleResourceModel{
				ID:        types.StringValue("role-1"),
				TenantID:  types.StringValue(""),
				OwnerID:   types.StringValue(""),
				Name:      types.StringValue(""),
				Type:      types.StringValue(""),
				CreatedAt: types.StringValue(""),
				UpdatedAt: types.StringValue(""),
				Policies: newTestPolicyList(t,
					models.RolePolicyModel{
						ID:          types.StringValue("pol-1"),
						RoleID:      types.StringValue(""),
						TenantID:    types.StringValue(""),
						Effect:      types.StringValue("ALLOW"),
						Permissions: strSetValue("control-plane:service:view"),
						Resources:   types.SetNull(types.StringType),
						Tags:        types.ObjectNull(models.RolePolicyTagsModel{}.ObjectType().AttrTypes),
					},
					models.RolePolicyModel{
						ID:          types.StringValue("pol-2"),
						RoleID:      types.StringValue(""),
						TenantID:    types.StringValue(""),
						Effect:      types.StringValue("DENY"),
						Permissions: strSetValue("control-plane:organization:manage-billing"),
						Resources:   types.SetNull(types.StringType),
						Tags:        types.ObjectNull(models.RolePolicyTagsModel{}.ObjectType().AttrTypes),
					},
				),
			},
		},
		{
			name:        "propagates API error",
			state:       models.RoleResourceModel{ID: types.StringValue("role-1")},
			responseErr: fmt.Errorf("status: 500, body: internal error"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)

			apiClientMock := api.NewClientMock(mc).
				GetRoleMock.
				Expect(ctx, tt.state.ID.ValueString()).
				Return(tt.response, tt.responseErr)

			r := &RoleResource{client: apiClientMock}

			_, err := r.syncRoleState(ctx, &tt.state)

			if (err != nil) != tt.wantErr {
				t.Errorf("%s error does not match:\ngot  = %v\nwant error = %v", tt.name, err, tt.wantErr)
			}

			if !tt.wantErr && !reflect.DeepEqual(tt.state, tt.wantState) {
				t.Errorf("%s state does not match:\ngot  = %v\nwant = %v", tt.name, tt.state, tt.wantState)
			}
		})
	}
}

func newTestPolicyModel(t *testing.T, effect string, perms []string) models.RolePolicyModel {
	t.Helper()
	permValues := make([]attr.Value, len(perms))
	for i, p := range perms {
		permValues[i] = types.StringValue(p)
	}
	permsSet, diags := types.SetValue(types.StringType, permValues)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics building permissions set: %v", diags)
	}

	return models.RolePolicyModel{
		ID:          types.StringValue(""),
		RoleID:      types.StringValue(""),
		TenantID:    types.StringValue(""),
		Effect:      types.StringValue(effect),
		Permissions: permsSet,
		Resources:   types.SetNull(types.StringType),
		Tags:        types.ObjectNull(models.RolePolicyTagsModel{}.ObjectType().AttrTypes),
	}
}

func newTestPolicyModelWithResources(t *testing.T, effect string, perms []string, resources []string) models.RolePolicyModel {
	t.Helper()
	pm := newTestPolicyModel(t, effect, perms)
	resValues := make([]attr.Value, len(resources))
	for i, r := range resources {
		resValues[i] = types.StringValue(r)
	}
	resSet, diags := types.SetValue(types.StringType, resValues)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics building resources set: %v", diags)
	}
	pm.Resources = resSet
	return pm
}

func newTestPolicyModelWithTags(t *testing.T, effect string, perms []string, resources []string, roleV2 string) models.RolePolicyModel {
	t.Helper()
	pm := newTestPolicyModelWithResources(t, effect, perms, resources)
	tagsModel := models.RolePolicyTagsModel{
		RoleV2: types.StringValue(roleV2),
	}
	pm.Tags = tagsModel.ObjectValue()
	return pm
}

func newTestPolicyList(t *testing.T, policies ...models.RolePolicyModel) types.List {
	t.Helper()
	values := make([]attr.Value, len(policies))
	for i, p := range policies {
		values[i] = p.ObjectValue()
	}
	list, diags := types.ListValue(models.RolePolicyModel{}.ObjectType(), values)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics building policy list: %v", diags)
	}
	return list
}

func TestPlanPoliciesToAPICreate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		input      types.List
		wantResult []api.RBACPolicyCreateRequest
		wantErr    bool
	}{
		{
			name:       "null list returns empty slice",
			input:      types.ListNull(models.RolePolicyModel{}.ObjectType()),
			wantResult: []api.RBACPolicyCreateRequest{},
		},
		{
			name:  "ALLOW policy without resources or tags",
			input: newTestPolicyList(t, newTestPolicyModel(t, "ALLOW", []string{"control-plane:service:view"})),
			wantResult: []api.RBACPolicyCreateRequest{
				{AllowDeny: api.RBACAllowDenyAllow, Permissions: []string{"control-plane:service:view"}},
			},
		},
		{
			name:  "DENY policy with resources",
			input: newTestPolicyList(t, newTestPolicyModelWithResources(t, "DENY", []string{"perm"}, []string{"instance/*"})),
			wantResult: []api.RBACPolicyCreateRequest{
				{AllowDeny: api.RBACAllowDenyDeny, Permissions: []string{"perm"}, Resources: []string{"instance/*"}},
			},
		},
		{
			name: "policy with tags: role and resources",
			input: newTestPolicyList(t, newTestPolicyModelWithTags(t,
				"ALLOW",
				[]string{"sql-console:database:access"},
				[]string{"instance/*"},
				"sql-console-readonly",
			)),
			wantResult: []api.RBACPolicyCreateRequest{
				{
					AllowDeny:   api.RBACAllowDenyAllow,
					Permissions: []string{"sql-console:database:access"},
					Resources:   []string{"instance/*"},
					Tags:        &api.RBACPolicyTags{RoleV2: "sql-console-readonly"},
				},
			},
		},
		{
			name: "multiple policies are all included",
			input: newTestPolicyList(t,
				newTestPolicyModel(t, "ALLOW", []string{"perm-a"}),
				newTestPolicyModel(t, "DENY", []string{"perm-b"}),
			),
			wantResult: []api.RBACPolicyCreateRequest{
				{AllowDeny: api.RBACAllowDenyAllow, Permissions: []string{"perm-a"}},
				{AllowDeny: api.RBACAllowDenyDeny, Permissions: []string{"perm-b"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := planPoliciesToAPICreate(ctx, tt.input)

			if tt.wantErr {
				if !diags.HasError() {
					t.Errorf("%s expected error diagnostics but got none", tt.name)
				}
				return
			}

			if diags.HasError() {
				t.Errorf("%s unexpected error diagnostics: %v", tt.name, diags)
			}

			if !reflect.DeepEqual(got, tt.wantResult) {
				t.Errorf("%s result does not match:\ngot  = %v\nwant = %v", tt.name, got, tt.wantResult)
			}
		})
	}
}
