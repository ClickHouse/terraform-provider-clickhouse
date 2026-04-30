package resource

import (
	"context"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
)

func TestClickPipeReversePrivateEndpointResource_Schema(t *testing.T) {
	r := &ClickPipeReversePrivateEndpointResource{}
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}

	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("Expected schema attributes to be set")
	}

	for _, name := range []string{"id", "service_id", "type", "description", "gcp_service_attachment", "msk_cluster_arn"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("Expected %q attribute in schema", name)
		}
	}

	attr, ok := resp.Schema.Attributes["gcp_service_attachment"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("Expected gcp_service_attachment to be a StringAttribute, got %T", resp.Schema.Attributes["gcp_service_attachment"])
	}
	if !attr.Optional {
		t.Error("Expected gcp_service_attachment to be Optional")
	}
	if attr.Required {
		t.Error("Expected gcp_service_attachment to not be Required")
	}
	if attr.Computed {
		t.Error("Expected gcp_service_attachment to not be Computed")
	}
	if len(attr.Validators) != 1 {
		t.Errorf("Expected gcp_service_attachment to have 1 validator, got %d", len(attr.Validators))
	}

	if !slices.Contains(api.ReversePrivateEndpointTypes, api.ReversePrivateEndpointTypeGcpPscServiceAttachment) {
		t.Error("Expected ReversePrivateEndpointTypes to contain GCP_PSC_SERVICE_ATTACHMENT")
	}
}
