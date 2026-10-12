package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGetSnapshotConfiguration(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/snapshotConfiguration"

	want := SnapshotConfiguration{
		Enabled:   boolPtr(true),
		Gap:       int32Ptr(30),
		TimeFrame: int32Ptr(1440),
	}

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[SnapshotConfiguration]{Result: want})
	})

	got, err := client.GetSnapshotConfiguration(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetSnapshotConfiguration: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetSnapshotConfiguration mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateSnapshotConfiguration(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/snapshotConfiguration"

	update := SnapshotConfiguration{
		Enabled:   boolPtr(true),
		Gap:       int32Ptr(60),
		TimeFrame: int32Ptr(2880),
	}

	var capturedBody SnapshotConfiguration
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q; want PATCH", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[SnapshotConfiguration]{Result: update})
	})

	got, err := client.UpdateSnapshotConfiguration(context.Background(), "svc-1", update)
	if err != nil {
		t.Fatalf("UpdateSnapshotConfiguration: %v", err)
	}
	if diff := cmp.Diff(&update, got); diff != "" {
		t.Errorf("UpdateSnapshotConfiguration mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(update, capturedBody); diff != "" {
		t.Errorf("request body mismatch (-want +got):\n%s", diff)
	}
}

// A partial update (only enabled set) must not send gap/timeFrame at all — the API treats an omitted field as
// "leave unchanged", whereas a zero value would be rejected as an unsupported cadence.
func TestUpdateSnapshotConfiguration_PartialOmitsCadence(t *testing.T) {
	update := SnapshotConfiguration{Enabled: boolPtr(false)}

	var rawBody map[string]json.RawMessage
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &rawBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[SnapshotConfiguration]{Result: update})
	})

	if _, err := client.UpdateSnapshotConfiguration(context.Background(), "svc-1", update); err != nil {
		t.Fatalf("UpdateSnapshotConfiguration: %v", err)
	}
	if _, ok := rawBody["enabled"]; !ok {
		t.Errorf("request body missing enabled; got keys %v", rawBody)
	}
	if _, ok := rawBody["gap"]; ok {
		t.Errorf("request body should omit gap when unset; got keys %v", rawBody)
	}
	if _, ok := rawBody["timeFrame"]; ok {
		t.Errorf("request body should omit timeFrame when unset; got keys %v", rawBody)
	}
}

// A 403 from the snapshot endpoint (org not enrolled in the beta flag) must not fail the whole service read.
func TestGetService_ToleratesSnapshotForbidden(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/snapshotConfiguration"):
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "feature not enabled"})
		case strings.HasSuffix(r.URL.Path, "/privateEndpointConfig"):
			_ = json.NewEncoder(w).Encode(ResponseWithResult[ServicePrivateEndpointConfig]{})
		case strings.HasSuffix(r.URL.Path, "/backupConfiguration"):
			_ = json.NewEncoder(w).Encode(ResponseWithResult[BackupConfiguration]{})
		case strings.HasSuffix(r.URL.Path, "/serviceQueryEndpoint"):
			_ = json.NewEncoder(w).Encode(ResponseWithResult[ServiceQueryEndpoint]{})
		default:
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", IsPrimary: boolPtr(true)}})
		}
	})

	service, err := client.GetService(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetService should tolerate a 403 from snapshotConfiguration, got: %v", err)
	}
	if service.SnapshotConfiguration != nil {
		t.Errorf("SnapshotConfiguration should be nil when the endpoint 403s; got %+v", service.SnapshotConfiguration)
	}
}
