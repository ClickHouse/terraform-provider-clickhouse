package datasource

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func TestServiceProfilesToListValue_MapsItems(t *testing.T) {
	items := []api.ServiceProfile{
		{Profile: "v1-standard-byoc-4", CpuCores: 2, MemoryGi: 8},
		{Profile: "v1-standard-byoc-8", CpuCores: 4, MemoryGi: 16},
	}

	list, diags := serviceProfilesToListValue(items)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	elems := list.Elements()
	if len(elems) != 2 {
		t.Fatalf("len = %d; want 2", len(elems))
	}
	attrs := elems[0].(types.Object).Attributes()
	if got := attrs["profile"].(types.String).ValueString(); got != "v1-standard-byoc-4" {
		t.Errorf("elems[0].profile = %q; want v1-standard-byoc-4", got)
	}
	if got := attrs["cpu_cores"].(types.Float64).ValueFloat64(); got != 2 {
		t.Errorf("elems[0].cpu_cores = %v; want 2", got)
	}
	if got := attrs["memory_gi"].(types.Float64).ValueFloat64(); got != 8 {
		t.Errorf("elems[0].memory_gi = %v; want 8", got)
	}
}

func TestServiceProfilesToListValue_EmptyIsKnownNotNull(t *testing.T) {
	list, diags := serviceProfilesToListValue(nil)
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
