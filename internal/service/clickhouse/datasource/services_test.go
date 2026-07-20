package datasource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func TestServicesToListValue_MapsItems(t *testing.T) {
	ctx := context.Background()
	items := []api.Service{
		{Id: "svc-1", Name: "a", Provider: "aws", Region: "us-east-1"},
		{Id: "svc-2", Name: "b", Provider: "aws", Region: "eu-west-1"},
	}

	list, diags := servicesToListValue(ctx, items)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	elems := list.Elements()
	if len(elems) != 2 {
		t.Fatalf("len = %d; want 2", len(elems))
	}
	if got := elems[0].(types.Object).Attributes()["id"].(types.String).ValueString(); got != "svc-1" {
		t.Errorf("elems[0].id = %q; want svc-1", got)
	}
	if got := elems[1].(types.Object).Attributes()["id"].(types.String).ValueString(); got != "svc-2" {
		t.Errorf("elems[1].id = %q; want svc-2", got)
	}
}

func TestServicesToListValue_EmptyIsKnownNotNull(t *testing.T) {
	ctx := context.Background()

	list, diags := servicesToListValue(ctx, nil)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if list.IsNull() {
		t.Errorf("list should not be null for empty input")
	}
	if len(list.Elements()) != 0 {
		t.Errorf("len = %d; want 0", len(list.Elements()))
	}
}
