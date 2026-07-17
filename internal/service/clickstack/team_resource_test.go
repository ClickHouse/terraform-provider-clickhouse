package clickstack

import (
	"context"
	"testing"

	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestTeamResource_Schema(t *testing.T) {
	t.Parallel()

	r := NewTeamResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "team", "default_user_role_id"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected resource schema to contain attribute %q", attr)
		}
	}
}

func TestRoleDataSource_Schema(t *testing.T) {
	t.Parallel()

	d := NewRoleDataSource()
	resp := &fwdatasource.SchemaResponse{}
	d.Schema(context.Background(), fwdatasource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "team", "name", "description", "is_predefined"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected data source schema to contain attribute %q", attr)
		}
	}
}
