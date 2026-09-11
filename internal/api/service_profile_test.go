package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestListServiceProfiles_HappyPath(t *testing.T) {
	want := []ServiceProfile{
		{Profile: "v1-standard-byoc-4", CpuCores: 2, MemoryGi: 8},
		{Profile: "v1-standard-byoc-8", CpuCores: 4, MemoryGi: 16},
	}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/serviceProfiles" {
			t.Errorf("path = %q; want /organizations/org-1/serviceProfiles", r.URL.Path)
		}
		if got := r.URL.Query().Get("region_id"); got != "us-east-1" {
			t.Errorf("region_id = %q; want us-east-1", got)
		}
		if got := r.URL.Query().Get("byoc_id"); got != "byoc-1" {
			t.Errorf("byoc_id = %q; want byoc-1", got)
		}
		assertBasicAuth(t, r)
		_ = json.NewEncoder(w).Encode(ResponseWithResult[[]ServiceProfile]{Result: want})
	})

	got, err := client.ListServiceProfiles(context.Background(), "us-east-1", "byoc-1")
	if err != nil {
		t.Fatalf("ListServiceProfiles: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListServiceProfiles mismatch (-want +got):\n%s", diff)
	}
}

func TestListServiceProfiles_OmitsEmptyByocId(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.URL.Query()["byoc_id"]; present {
			t.Errorf("byoc_id should not be sent when empty; query = %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[[]ServiceProfile]{Result: []ServiceProfile{}})
	})

	got, err := client.ListServiceProfiles(context.Background(), "us-east-1", "")
	if err != nil {
		t.Fatalf("ListServiceProfiles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d; want 0", len(got))
	}
}
