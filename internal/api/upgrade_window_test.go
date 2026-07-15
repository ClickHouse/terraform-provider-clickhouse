package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func newUpgradeWindowTestClient(t *testing.T, handler http.HandlerFunc) (*ClientImpl, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(ClientConfig{
		ApiURL:         server.URL,
		OrganizationID: "org-1",
		TokenKey:       "key",
		TokenSecret:    "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client, server
}

func TestGetUpgradeWindow(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/upgradeWindow"
	want := UpgradeWindow{Weekday: 3, StartHourUtc: 12, Duration: 6}

	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[UpgradeWindow]{Result: want})
	})

	got, err := client.GetUpgradeWindow(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetUpgradeWindow: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetUpgradeWindow mismatch (-want +got):\n%s", diff)
	}
}

func TestGetUpgradeWindow_NotFound(t *testing.T) {
	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	_, err := client.GetUpgradeWindow(context.Background(), "svc-1")
	if err == nil {
		t.Fatalf("GetUpgradeWindow: expected error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false; want true (err = %v)", err)
	}
}

func TestUpdateUpgradeWindow_OmitsDurationFromRequest(t *testing.T) {
	update := UpgradeWindowUpdate{Weekday: 1, StartHourUtc: 6}
	response := UpgradeWindow{Weekday: 1, StartHourUtc: 6, Duration: 6}

	var capturedBody map[string]any
	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q; want PUT", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[UpgradeWindow]{Result: response})
	})

	got, err := client.UpdateUpgradeWindow(context.Background(), "svc-1", update)
	if err != nil {
		t.Fatalf("UpdateUpgradeWindow: %v", err)
	}
	if _, ok := capturedBody["duration"]; ok {
		t.Errorf("request body should not contain duration: %v", capturedBody)
	}
	if w, ok := capturedBody["weekday"].(float64); !ok || int(w) != 1 {
		t.Errorf("request weekday = %v; want 1", capturedBody["weekday"])
	}
	if h, ok := capturedBody["startHourUtc"].(float64); !ok || int(h) != 6 {
		t.Errorf("request startHourUtc = %v; want 6", capturedBody["startHourUtc"])
	}
	if got.Duration != 6 {
		t.Errorf("response Duration = %d; want 6", got.Duration)
	}
}

func TestDeleteUpgradeWindow(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/upgradeWindow"
	var sawDelete bool
	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q; want DELETE", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		sawDelete = true
		w.WriteHeader(http.StatusOK)
	})

	if err := client.DeleteUpgradeWindow(context.Background(), "svc-1"); err != nil {
		t.Fatalf("DeleteUpgradeWindow: %v", err)
	}
	if !sawDelete {
		t.Errorf("server did not see DELETE request")
	}
}
