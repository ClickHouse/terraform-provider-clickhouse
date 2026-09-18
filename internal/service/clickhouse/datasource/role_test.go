package datasource

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func TestRoleDataSourcesExposePolicyGrants(t *testing.T) {
	var roleResp datasource.SchemaResponse
	(&roleDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &roleResp)
	assertPolicyGrantsSchema(t, roleResp.Schema.Attributes["policies"])

	var rolesResp datasource.SchemaResponse
	(&rolesDataSource{}).Schema(context.Background(), datasource.SchemaRequest{}, &rolesResp)
	roles := rolesResp.Schema.Attributes["roles"].(schema.ListNestedAttribute)
	assertPolicyGrantsSchema(t, roles.NestedObject.Attributes["policies"])
}

func assertPolicyGrantsSchema(t *testing.T, policiesAttribute schema.Attribute) {
	t.Helper()

	policies := policiesAttribute.(schema.ListNestedAttribute)
	tags := policies.NestedObject.Attributes["tags"].(schema.SingleNestedAttribute)
	grants, ok := tags.Attributes["grants"].(schema.ListAttribute)
	if !ok {
		t.Fatal("policy tags grants is not a list attribute")
	}
	if !grants.Computed {
		t.Fatal("policy tags grants must be computed")
	}
	if !reflect.DeepEqual(grants.ElementType, types.StringType) {
		t.Fatalf("policy tags grants element type does not match: got %v, want string", grants.ElementType)
	}
}

func TestAPIRoleToDataSourceModelPreservesGrantOrder(t *testing.T) {
	want := []string{
		"GRANT SELECT ON default.*",
		"REVOKE SELECT ON default.secret",
	}

	role, diags := apiRoleToDataSourceModel(api.RBACRole{
		Policies: []api.RBACPolicy{{
			AllowDeny: api.RBACAllowDenyAllow,
			Tags:      &api.RBACPolicyTags{Grants: want},
		}},
	})
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	policies := role.Attributes()["policies"].(types.List).Elements()
	tags := policies[0].(types.Object).Attributes()["tags"].(types.Object)
	grants := tags.Attributes()["grants"].(types.List)

	var got []string
	diags = grants.ElementsAs(context.Background(), &got, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading grants: %v", diags)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("grants order does not match: got %v, want %v", got, want)
	}
}
