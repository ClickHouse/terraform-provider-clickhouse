package registry

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

const providerTypeName = "clickhouse"

// TestServicePackages asserts group names are unique and every registered
// Terraform type name is globally unique and maps to exactly one group.
func TestServicePackages(t *testing.T) {
	t.Parallel()
	groups := map[string]string{}   // group name -> owner
	resTypes := map[string]string{} // resource type name -> group
	dsTypes := map[string]string{}  // data source type name -> group

	for _, sp := range ServicePackages() {
		meta := sp.Meta()
		if meta.Name == "" || meta.Owner == "" || meta.HumanName == "" {
			t.Fatalf("group %q: metadata fields must all be set: %+v", meta.Name, meta)
		}
		if _, dup := groups[meta.Name]; dup {
			t.Fatalf("duplicate group name %q", meta.Name)
		}
		groups[meta.Name] = meta.Owner

		for _, f := range sp.Resources() {
			var mr resource.MetadataResponse
			f().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: providerTypeName}, &mr)
			if prev, dup := resTypes[mr.TypeName]; dup {
				t.Fatalf("resource type %q registered by both %q and %q", mr.TypeName, prev, meta.Name)
			}
			resTypes[mr.TypeName] = meta.Name
		}
		for _, f := range sp.DataSources() {
			var mr datasource.MetadataResponse
			f().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: providerTypeName}, &mr)
			if prev, dup := dsTypes[mr.TypeName]; dup {
				t.Fatalf("data source type %q registered by both %q and %q", mr.TypeName, prev, meta.Name)
			}
			dsTypes[mr.TypeName] = meta.Name
		}
	}
	if len(resTypes) == 0 || len(dsTypes) == 0 {
		t.Fatal("registry returned no resources or no data sources")
	}

	// Golden-count guard: the registered surface must stay exactly what the
	// pre-restructure provider exposed. This catches a factory accidentally
	// dropped from a group's list (which the uniqueness check above would miss).
	// Bump these numbers deliberately when a group gains or loses a
	// resource/data source.
	const (
		wantResources   = 14
		wantDataSources = 8
	)
	if len(resTypes) != wantResources {
		t.Errorf("registered resource count = %d, want %d (a factory was added or dropped?)", len(resTypes), wantResources)
	}
	if len(dsTypes) != wantDataSources {
		t.Errorf("registered data source count = %d, want %d (a factory was added or dropped?)", len(dsTypes), wantDataSources)
	}
}
