package resource

import (
	"context"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestOrganizationResource_Metadata(t *testing.T) {
	r := &OrganizationResource{}
	req := resource.MetadataRequest{
		ProviderTypeName: "clickhouse",
	}
	resp := &resource.MetadataResponse{}

	r.Metadata(context.Background(), req, resp)

	expectedTypeName := "clickhouse_organization"
	if resp.TypeName != expectedTypeName {
		t.Errorf("Expected type name %s, got %s", expectedTypeName, resp.TypeName)
	}
}

func TestOrganizationResource_Configure(t *testing.T) {
	mc := minimock.NewController(t)
	mockClient := api.NewClientMock(mc)

	r := &OrganizationResource{}
	req := resource.ConfigureRequest{
		ProviderData: mockClient,
	}
	resp := &resource.ConfigureResponse{}

	r.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("Configure() unexpected error: %v", resp.Diagnostics)
	}

	if r.client == nil {
		t.Error("Expected client to be set, but it was nil")
	}
}

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
