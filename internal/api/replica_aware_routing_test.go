package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGetReplicaAwareRouting(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/replicaAwareRouting"

	want := ReplicaAwareRouting{
		HTTPEnabled:   boolPtr(true),
		NativeEnabled: boolPtr(false),
	}

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ReplicaAwareRouting]{Result: want})
	})

	got, err := client.GetReplicaAwareRouting(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetReplicaAwareRouting: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetReplicaAwareRouting mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateReplicaAwareRouting_SendsBothFlags(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/replicaAwareRouting"

	update := ReplicaAwareRouting{
		HTTPEnabled:   boolPtr(true),
		NativeEnabled: boolPtr(false),
	}

	var rawBody map[string]json.RawMessage
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q; want PATCH", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &rawBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ReplicaAwareRouting]{Result: update})
	})

	got, err := client.UpdateReplicaAwareRouting(context.Background(), "svc-1", update)
	if err != nil {
		t.Fatalf("UpdateReplicaAwareRouting: %v", err)
	}
	if diff := cmp.Diff(&update, got); diff != "" {
		t.Errorf("UpdateReplicaAwareRouting mismatch (-want +got):\n%s", diff)
	}
	if _, ok := rawBody["httpEnabled"]; !ok {
		t.Errorf("request body missing httpEnabled; got keys %v", rawBody)
	}
	if _, ok := rawBody["nativeEnabled"]; !ok {
		t.Errorf("request body missing nativeEnabled; got keys %v", rawBody)
	}
}

func TestGetReplicaAwareRouting_Forbidden(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"feature not enabled"}`))
	})

	_, err := client.GetReplicaAwareRouting(context.Background(), "svc-1")
	if !IsForbidden(err) {
		t.Fatalf("GetReplicaAwareRouting error = %v, want forbidden", err)
	}
}
