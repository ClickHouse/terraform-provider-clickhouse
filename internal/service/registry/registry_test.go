package registry

import (
	"testing"
)

// TestServicePackages asserts group names are unique and every registered
// Terraform type name is globally unique and maps to exactly one group.
func TestServicePackages(t *testing.T) {
	t.Parallel()

	groups := map[string]string{} // group name -> owner
	for _, sp := range ServicePackages() {
		meta := sp.Meta()
		if meta.Name == "" || meta.Owner == "" || meta.HumanName == "" {
			t.Fatalf("group %q: metadata fields must all be set: %+v", meta.Name, meta)
		}
		if _, dup := groups[meta.Name]; dup {
			t.Fatalf("duplicate group name %q", meta.Name)
		}
		groups[meta.Name] = meta.Owner
	}

	resTypes := map[string]string{} // resource type name -> group
	dsTypes := map[string]string{}  // data source type name -> group
	for _, c := range Components() {
		types, label := resTypes, "resource"
		if c.Kind == KindDataSource {
			types, label = dsTypes, "data source"
		}
		if prev, dup := types[c.TypeName]; dup {
			t.Fatalf("%s type %q registered by both %q and %q", label, c.TypeName, prev, c.Group.Name)
		}
		types[c.TypeName] = c.Group.Name
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
		wantResources   = 24 // 14 clickhouse + 1 postgres + 9 clickstack
		wantDataSources = 12 // 7 clickhouse + 3 postgres + 2 clickstack
	)
	if len(resTypes) != wantResources {
		t.Errorf("registered resource count = %d, want %d (a factory was added or dropped?)", len(resTypes), wantResources)
	}
	if len(dsTypes) != wantDataSources {
		t.Errorf("registered data source count = %d, want %d (a factory was added or dropped?)", len(dsTypes), wantDataSources)
	}
}
