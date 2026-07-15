package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// A horizontal create must pass the mode, the replica band, and the fixed (min == max) per-replica memory
// through on POST /services, and must not fabricate any vertical-only fields (num_replicas or the deprecated
// totals — FixMemoryBounds should be a no-op when the band is present).
func TestCreateService_horizontalBand(t *testing.T) {
	var gotBody map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"result":{"service":{"id":"svc-1","name":"svc"},"password":"p"}}`))
	})

	_, _, err := client.CreateService(context.Background(), Service{
		Name:               "svc",
		Provider:           "aws",
		Region:             "us-east-1",
		Tier:               TierPPv2,
		AutoscalingMode:    "horizontal",
		MinReplicas:        intPtr(2),
		MaxReplicas:        intPtr(6),
		MinReplicaMemoryGb: intPtr(16),
		MaxReplicaMemoryGb: intPtr(16),
	})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	if gotBody["autoscalingMode"] != "horizontal" ||
		gotBody["minReplicas"] != float64(2) || gotBody["maxReplicas"] != float64(6) ||
		gotBody["minReplicaMemoryGb"] != float64(16) || gotBody["maxReplicaMemoryGb"] != float64(16) {
		t.Errorf("POST body missing the horizontal mode/band/memory: %v", gotBody)
	}
	// Horizontal must not carry a fixed replica count or the deprecated totals — FixMemoryBounds is a no-op here.
	for _, k := range []string{"numReplicas", "minTotalMemoryGb", "maxTotalMemoryGb"} {
		if _, present := gotBody[k]; present {
			t.Errorf("POST body unexpectedly carried vertical-only field %q for a horizontal create: %v", k, gotBody)
		}
	}
}

func TestListServices_HappyPath_NoFilter(t *testing.T) {
	want := []Service{
		{Id: "svc-1", Name: "one", Provider: "aws", Region: "us-east-1", State: "running", ClickHouseVersion: "24.5", CreatedAt: "t1"},
		{Id: "svc-2", Name: "two", Provider: "gcp", Region: "us-central1", State: "idle", ClickHouseVersion: "24.5", CreatedAt: "t2"},
	}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/services" {
			t.Errorf("path = %q; want /organizations/org-1/services", r.URL.Path)
		}
		if got := r.URL.Query()["filter"]; len(got) != 0 {
			t.Errorf("filter = %v; want none", got)
		}
		assertBasicAuth(t, r)
		_ = json.NewEncoder(w).Encode(ResponseWithResult[[]Service]{Result: want})
	})

	got, err := client.ListServices(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListServices mismatch (-want +got):\n%s", diff)
	}
}

func TestListServices_SendsTagFilters(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query()["filter"]
		sort.Strings(got)
		want := []string{"tag:Env=prod", "tag:Team=data"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("filter params mismatch (-want +got):\n%s", diff)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[[]Service]{Result: []Service{}})
	})

	_, err := client.ListServices(context.Background(), []string{"tag:Env=prod", "tag:Team=data"})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
}

func TestListServices_APIError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	})
	if _, err := client.ListServices(context.Background(), nil); err == nil {
		t.Fatal("expected error; got nil")
	}
}

// GetServiceBase must fetch only the core service object with a single request —
// no private-endpoint-config, backup, or query-endpoint enrichment calls (unlike
// GetService).
func TestGetServiceBase_SingleRequestNoEnrichment(t *testing.T) {
	var calls int32
	want := Service{Id: "svc-1", Name: "svc", Provider: "aws", Region: "us-east-1", State: "running"}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/services/svc-1" {
			t.Errorf("path = %q; want /organizations/org-1/services/svc-1", r.URL.Path)
		}
		assertBasicAuth(t, r)
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: want})
	})

	got, err := client.GetServiceBase(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetServiceBase: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetServiceBase mismatch (-want +got):\n%s", diff)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("GetServiceBase made %d HTTP calls; want exactly 1 (no enrichment)", n)
	}
}
