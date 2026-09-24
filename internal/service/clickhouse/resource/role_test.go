package resource

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func TestRoleResourceTagsSchema(t *testing.T) {
	ctx := context.Background()
	var schemaResp frameworkresource.SchemaResponse
	(&RoleResource{}).Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)

	policies := schemaResp.Schema.Attributes["policies"].(schema.ListNestedAttribute)
	tags := policies.NestedObject.Attributes["tags"].(schema.SingleNestedAttribute)
	role := tags.Attributes["role"].(schema.StringAttribute)
	grants := tags.Attributes["grants"].(schema.ListAttribute)

	tests := []struct {
		name      string
		role      any
		grants    any
		wantError bool
	}{
		{name: "role only", role: "sql-console-readonly", grants: nil},
		{name: "grants only", role: nil, grants: []tftypes.Value{tftypes.NewValue(tftypes.String, "GRANT SELECT ON default.*")}},
		{name: "role and grants", role: "sql-console-readonly", grants: []tftypes.Value{tftypes.NewValue(tftypes.String, "GRANT SELECT ON default.*")}, wantError: true},
		{name: "neither role nor grants", role: nil, grants: nil, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tfsdk.Config{
				Schema: schema.Schema{Attributes: tags.Attributes},
				Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{
					"role":   tftypes.String,
					"grants": tftypes.List{ElementType: tftypes.String},
				}}, map[string]tftypes.Value{
					"role":   tftypes.NewValue(tftypes.String, tt.role),
					"grants": tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, tt.grants),
				}),
			}

			var configValue types.String
			if tt.role == nil {
				configValue = types.StringNull()
			} else {
				configValue = types.StringValue(tt.role.(string))
			}

			var resp validator.StringResponse
			for _, v := range role.Validators {
				v.ValidateString(ctx, validator.StringRequest{
					Path:           path.Root("role"),
					PathExpression: path.MatchRoot("role"),
					Config:         config,
					ConfigValue:    configValue,
				}, &resp)
			}

			if resp.Diagnostics.HasError() != tt.wantError {
				t.Fatalf("error diagnostics do not match: got %v, want error %t", resp.Diagnostics, tt.wantError)
			}
		})
	}

	t.Run("grants cannot be empty", func(t *testing.T) {
		var resp validator.ListResponse
		for _, v := range grants.Validators {
			v.ValidateList(ctx, validator.ListRequest{
				Path:        path.Root("grants"),
				ConfigValue: types.ListValueMust(types.StringType, []attr.Value{}),
			}, &resp)
		}

		if !resp.Diagnostics.HasError() {
			t.Fatal("expected an error for an empty grants list")
		}
	})
}

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

// TestApplyRoleToState_BackendInjectedPermissions covers the permission-split
// side effect: granting control-plane:service:manage makes the backend
// auto-grant control-plane:service:delete, so the API returns more permissions
// than the user declared. applyRoleToState must reconcile against the
// permissions the caller already has (the plan on Create/Update, prior state on
// Read) instead of mirroring the API, otherwise Terraform reports "Provider
// produced inconsistent result after apply".
func TestApplyRoleToState_BackendInjectedPermissions(t *testing.T) {
	declared := []string{
		"control-plane:service:manage",
		"control-plane:service:manage-backups",
		"control-plane:service:view",
		"control-plane:service:view-backups",
	}
	// The backend echoes the declared permissions plus the injected one.
	withInjected := append(append([]string{}, declared...), "control-plane:service:delete")

	// desiredPolicies builds the plan / prior-state policy list carrying the
	// permissions the user actually declared.
	desiredPolicies := func(perms ...string) types.List {
		return newTestPolicyList(t, newTestPolicyModelWithResources(t, "ALLOW", perms, []string{"instance/*"}))
	}

	tests := []struct {
		name           string
		targetPolicies types.List
		apiPerms       []string
		wantPerms      types.Set
	}{
		{
			// Declared 4, backend returns 5: state keeps the 4 so plan == applied.
			name:           "drops backend-injected permission",
			targetPolicies: desiredPolicies(declared...),
			apiPerms:       withInjected,
			wantPerms:      strSetValue(declared...),
		},
		{
			// A declared permission actually removed on the backend must surface.
			name:           "read surfaces genuine permission removal",
			targetPolicies: desiredPolicies(declared...),
			apiPerms:       []string{"control-plane:service:manage", "control-plane:service:view"},
			wantPerms:      strSetValue("control-plane:service:manage", "control-plane:service:view"),
		},
		{
			// No prior state to reconcile against, so keep whatever the API returns.
			name:           "import with no prior policies keeps API permissions",
			targetPolicies: types.ListNull(models.RolePolicyModel{}.ObjectType()),
			apiPerms:       withInjected,
			wantPerms:      strSetValue(withInjected...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := models.RoleResourceModel{
				ID:       types.StringValue("role-1"),
				Policies: tt.targetPolicies,
			}

			role := &api.RBACRole{
				ID:        "role-1",
				TenantID:  "tenant-1",
				OwnerID:   "owner-1",
				Name:      "tf-test-restricted-operator",
				Type:      api.RBACRoleTypeCustom,
				CreatedAt: "2024-01-01T00:00:00Z",
				UpdatedAt: "2024-01-02T00:00:00Z",
				Policies: []api.RBACPolicy{
					{
						ID:          "pol-1",
						RoleID:      "role-1",
						TenantID:    "tenant-1",
						AllowDeny:   api.RBACAllowDenyAllow,
						Permissions: tt.apiPerms,
						Resources:   []string{"instance/*"},
					},
				},
			}

			diags := applyRoleToState(context.Background(), role, &target)
			if diags.HasError() {
				t.Fatalf("%s unexpected diagnostics: %v", tt.name, diags)
			}

			wantState := models.RoleResourceModel{
				ID:        types.StringValue("role-1"),
				TenantID:  types.StringValue("tenant-1"),
				OwnerID:   types.StringValue("owner-1"),
				Name:      types.StringValue("tf-test-restricted-operator"),
				Type:      types.StringValue(api.RBACRoleTypeCustom),
				CreatedAt: types.StringValue("2024-01-01T00:00:00Z"),
				UpdatedAt: types.StringValue("2024-01-02T00:00:00Z"),
				Policies: newTestPolicyList(t, models.RolePolicyModel{
					ID:          types.StringValue("pol-1"),
					RoleID:      types.StringValue("role-1"),
					TenantID:    types.StringValue("tenant-1"),
					Effect:      types.StringValue("ALLOW"),
					Permissions: tt.wantPerms,
					Resources:   strSetValue("instance/*"),
					Tags:        types.ObjectNull(models.RolePolicyTagsModel{}.ObjectType().AttrTypes),
				}),
			}

			if !reflect.DeepEqual(target, wantState) {
				t.Errorf("%s state does not match:\ngot  = %v\nwant = %v", tt.name, target, wantState)
			}
		})
	}
}

func TestApplyRoleToState_CustomGrantsAreAuthoritative(t *testing.T) {
	target := models.RoleResourceModel{
		Policies: newTestPolicyList(t, newTestPolicyModelWithGrants(t,
			"ALLOW",
			[]string{"sql-console:database:access"},
			[]string{"instance/service-id"},
			[]string{
				"GRANT SELECT ON default.*",
				"REVOKE SELECT ON default.secret",
			},
		)),
	}
	want := []string{
		"REVOKE SELECT ON default.secret",
		"GRANT SELECT ON default.*",
	}
	role := &api.RBACRole{Policies: []api.RBACPolicy{{
		AllowDeny:   api.RBACAllowDenyAllow,
		Permissions: []string{"sql-console:database:access"},
		Resources:   []string{"instance/service-id"},
		Tags:        &api.RBACPolicyTags{Grants: want},
	}}}

	diags := applyRoleToState(context.Background(), role, &target)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	policies := target.Policies.Elements()
	tags := policies[0].(types.Object).Attributes()["tags"].(types.Object)
	grants := tags.Attributes()["grants"].(types.List)
	var got []string
	diags = grants.ElementsAs(context.Background(), &got, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading grants: %v", diags)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("grants do not match API order: got %v, want %v", got, want)
	}
}

// The backend reorders policies and reassigns their IDs on PATCH; state must
// keep the prior order or the ordered List diffs positionally and shows churn.
func TestApplyRoleToState_PreservesPolicyOrderAcrossBackendReorder(t *testing.T) {
	orgPolicy := func(id string) models.RolePolicyModel {
		pm := newTestPolicyModelWithResources(t, "ALLOW",
			[]string{"control-plane:organization:view"},
			[]string{"organization/1dff95ae-7280-4e33-bd5d-677e5f9456f9"})
		pm.ID = types.StringValue(id)
		pm.RoleID = types.StringValue("role-1")
		pm.TenantID = types.StringValue("tenant-1")
		return pm
	}
	servicePolicy := func(id string) models.RolePolicyModel {
		pm := newTestPolicyModelWithResources(t, "ALLOW",
			[]string{"control-plane:service:view", "control-plane:service:view-backups"},
			[]string{"instance/1", "instance/2"})
		pm.ID = types.StringValue(id)
		pm.RoleID = types.StringValue("role-1")
		pm.TenantID = types.StringValue("tenant-1")
		return pm
	}

	target := models.RoleResourceModel{
		ID:       types.StringValue("role-1"),
		Policies: newTestPolicyList(t, orgPolicy("pol-1"), servicePolicy("pol-2")),
	}

	// The backend hands the same two policies back with fresh IDs, swapped.
	role := &api.RBACRole{
		ID:       "role-1",
		TenantID: "tenant-1",
		OwnerID:  "owner-1",
		Type:     api.RBACRoleTypeCustom,
		Policies: []api.RBACPolicy{
			{
				ID: "pol-4", RoleID: "role-1", TenantID: "tenant-1", AllowDeny: api.RBACAllowDenyAllow,
				Permissions: []string{"control-plane:service:view", "control-plane:service:view-backups"},
				Resources:   []string{"instance/1", "instance/2"},
			},
			{
				ID: "pol-3", RoleID: "role-1", TenantID: "tenant-1", AllowDeny: api.RBACAllowDenyAllow,
				Permissions: []string{"control-plane:organization:view"},
				Resources:   []string{"organization/1dff95ae-7280-4e33-bd5d-677e5f9456f9"},
			},
		},
	}

	diags := applyRoleToState(context.Background(), role, &target)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	want := newTestPolicyList(t, orgPolicy("pol-3"), servicePolicy("pol-4"))
	if !reflect.DeepEqual(target.Policies, want) {
		t.Errorf("policy order was not preserved across a backend reorder:\ngot  = %v\nwant = %v", target.Policies, want)
	}
}

func TestCorrelatePolicies(t *testing.T) {
	ctx := context.Background()

	apiPolicy := func(id, effect string, perms, resources []string) api.RBACPolicy {
		return api.RBACPolicy{ID: id, AllowDeny: api.RBACAllowDeny(effect), Permissions: perms, Resources: resources}
	}
	orgView := []string{"control-plane:organization:view"}
	svcView := []string{"control-plane:service:view", "control-plane:service:view-backups"}
	sqlAccess := []string{"sql-console:database:access"}
	org := []string{"organization/org-1"}
	inst := []string{"instance/1"}

	nullPerms := types.SetNull(types.StringType)

	tests := []struct {
		name string
		base types.List
		api  []api.RBACPolicy
		// wantPerms[i] is the declared set paired with ordered[i]; null when unmatched.
		wantIDs   []string
		wantPerms []types.Set
	}{
		{
			name:      "nil base keeps API order with null permissions",
			base:      types.ListNull(models.RolePolicyModel{}.ObjectType()),
			api:       []api.RBACPolicy{apiPolicy("a", "ALLOW", svcView, inst), apiPolicy("b", "ALLOW", orgView, org)},
			wantIDs:   []string{"a", "b"},
			wantPerms: []types.Set{nullPerms, nullPerms},
		},
		{
			name: "reorder with distinct keys restores base order",
			base: newTestPolicyList(t,
				newTestPolicyModelWithResources(t, "ALLOW", orgView, org),
				newTestPolicyModelWithResources(t, "ALLOW", svcView, inst),
			),
			api:       []api.RBACPolicy{apiPolicy("svc", "ALLOW", svcView, inst), apiPolicy("org", "ALLOW", orgView, org)},
			wantIDs:   []string{"org", "svc"},
			wantPerms: []types.Set{strSetValue(orgView...), strSetValue(svcView...)},
		},
		{
			// Mirrors examples/resources/clickhouse_role: two ALLOW policies on one instance.
			name: "reorder with colliding keys is disambiguated by permission overlap",
			base: newTestPolicyList(t,
				newTestPolicyModelWithResources(t, "ALLOW", svcView, inst),
				newTestPolicyModelWithResources(t, "ALLOW", sqlAccess, inst),
			),
			api: []api.RBACPolicy{
				apiPolicy("sql", "ALLOW", sqlAccess, inst),
				// Backend also injected an extra permission on the control-plane policy.
				apiPolicy("svc", "ALLOW", append(slices.Clone(svcView), "control-plane:service:delete"), inst),
			},
			wantIDs:   []string{"svc", "sql"},
			wantPerms: []types.Set{strSetValue(svcView...), strSetValue(sqlAccess...)},
		},
		{
			name: "base policy missing from API is dropped",
			base: newTestPolicyList(t,
				newTestPolicyModelWithResources(t, "ALLOW", orgView, org),
				newTestPolicyModelWithResources(t, "ALLOW", svcView, inst),
			),
			api:       []api.RBACPolicy{apiPolicy("svc", "ALLOW", svcView, inst)},
			wantIDs:   []string{"svc"},
			wantPerms: []types.Set{strSetValue(svcView...)},
		},
		{
			name: "API policy not in base is appended with null permissions",
			base: newTestPolicyList(t,
				newTestPolicyModelWithResources(t, "ALLOW", svcView, inst),
			),
			api:       []api.RBACPolicy{apiPolicy("new", "DENY", orgView, org), apiPolicy("svc", "ALLOW", svcView, inst)},
			wantIDs:   []string{"svc", "new"},
			wantPerms: []types.Set{strSetValue(svcView...), nullPerms},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			base := basePolicies(ctx, tt.base, &diags)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			ordered, perms := correlatePolicies(base, tt.api)

			gotIDs := make([]string, len(ordered))
			for i, p := range ordered {
				gotIDs[i] = p.ID
			}
			if !reflect.DeepEqual(gotIDs, tt.wantIDs) {
				t.Fatalf("order does not match: got %v, want %v", gotIDs, tt.wantIDs)
			}
			if !reflect.DeepEqual(perms, tt.wantPerms) {
				t.Errorf("paired permissions do not match:\ngot  = %v\nwant = %v", perms, tt.wantPerms)
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
		Grants: types.ListNull(types.StringType),
	}
	pm.Tags = tagsModel.ObjectValue()
	return pm
}

func newTestPolicyModelWithGrants(t *testing.T, effect string, perms []string, resources, grants []string) models.RolePolicyModel {
	t.Helper()
	pm := newTestPolicyModelWithResources(t, effect, perms, resources)
	grantValues := make([]attr.Value, len(grants))
	for i, grant := range grants {
		grantValues[i] = types.StringValue(grant)
	}
	grantsList, diags := types.ListValue(types.StringType, grantValues)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics building grants list: %v", diags)
	}
	pm.Tags = models.RolePolicyTagsModel{
		RoleV2: types.StringNull(),
		Grants: grantsList,
	}.ObjectValue()
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
			name: "policy with custom grants preserves order",
			input: newTestPolicyList(t, newTestPolicyModelWithGrants(t,
				"ALLOW",
				[]string{"sql-console:database:access"},
				[]string{"instance/service-id"},
				[]string{
					"GRANT SELECT ON default.*",
					"REVOKE SELECT ON default.secret",
				},
			)),
			wantResult: []api.RBACPolicyCreateRequest{
				{
					AllowDeny:   api.RBACAllowDenyAllow,
					Permissions: []string{"sql-console:database:access"},
					Resources:   []string{"instance/service-id"},
					Tags: &api.RBACPolicyTags{Grants: []string{
						"GRANT SELECT ON default.*",
						"REVOKE SELECT ON default.secret",
					}},
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
