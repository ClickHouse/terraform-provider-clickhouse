package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestOrganizationResource_Schema(t *testing.T) {
	r := &OrganizationResource{}
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}

	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Error("Expected schema attributes to be set")
	}

	if _, ok := resp.Schema.Attributes["id"]; !ok {
		t.Error("Expected 'id' attribute in schema")
	}

	if _, ok := resp.Schema.Attributes["core_dumps_enabled"]; !ok {
		t.Error("Expected 'core_dumps_enabled' attribute in schema")
	}
}
