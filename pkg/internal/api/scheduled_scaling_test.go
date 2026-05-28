package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func newScheduledScalingTestClient(t *testing.T, handler http.HandlerFunc) (*ClientImpl, *httptest.Server) {
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

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

func TestGetScheduledScaling(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/scalingSchedule"

	want := AutoScalingSchedule{
		Entries: []AutoScalingScheduleEntry{
			{
				ID:           "entry-1",
				Name:         "business hours",
				Weekdays:     []int{1, 2, 3, 4, 5},
				StartHourUtc: 8,
				EndHourUtc:   18,
				MinReplicas:  intPtr(3),
				MaxReplicas:  intPtr(3),
				IdleScaling:  boolPtr(false),
				IsActiveNow:  true,
			},
		},
		BaseConfig: &AutoScalingScheduleBaseConfig{
			MinReplicaMemoryGb: intPtr(8),
			MaxReplicaMemoryGb: intPtr(32),
			IdleScaling:        boolPtr(true),
			IdleTimeoutMinutes: intPtr(5),
		},
		ActiveEntryID: "entry-1",
	}

	client, _ := newScheduledScalingTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[AutoScalingSchedule]{Result: want})
	})

	got, err := client.GetScheduledScaling(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetScheduledScaling: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetScheduledScaling mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateScheduledScaling_PostsEntriesAndOmitsServerOnlyFields(t *testing.T) {
	update := AutoScalingScheduleUpdate{
		Entries: []AutoScalingScheduleEntry{
			{
				Name:               "overnight",
				Weekdays:           []int{0, 6},
				StartHourUtc:       22,
				EndHourUtc:         6,
				MinReplicaMemoryGb: intPtr(8),
				MaxReplicaMemoryGb: intPtr(8),
				MinReplicas:        intPtr(2),
				MaxReplicas:        intPtr(2),
				IdleScaling:        boolPtr(false),
				IdleTimeoutMinutes: intPtr(10),
			},
		},
	}

	var capturedBody map[string]any
	client, _ := newScheduledScalingTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q; want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		// Echo the entries back with a server-generated ID.
		response := AutoScalingSchedule{Entries: update.Entries}
		response.Entries[0].ID = "generated-id"
		response.Entries[0].IsActiveNow = false
		_ = json.NewEncoder(w).Encode(ResponseWithResult[AutoScalingSchedule]{Result: response})
	})

	got, err := client.UpdateScheduledScaling(context.Background(), "svc-1", update)
	if err != nil {
		t.Fatalf("UpdateScheduledScaling: %v", err)
	}
	if got.Entries[0].ID != "generated-id" {
		t.Errorf("entry ID = %q; want generated-id", got.Entries[0].ID)
	}

	// Verify the marshaled body did not contain id or isActiveNow (omitempty).
	entry := capturedBody["entries"].([]any)[0].(map[string]any)
	if _, ok := entry["id"]; ok {
		t.Errorf("request body unexpectedly contained 'id': %v", entry)
	}
	if _, ok := entry["isActiveNow"]; ok {
		t.Errorf("request body unexpectedly contained 'isActiveNow': %v", entry)
	}
}

func TestUpdateScheduledScaling_EmptyEntriesSerializesToEmptyArray(t *testing.T) {
	var captured string
	client, _ := newScheduledScalingTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = string(body)
		_ = json.NewEncoder(w).Encode(ResponseWithResult[AutoScalingSchedule]{Result: AutoScalingSchedule{Entries: []AutoScalingScheduleEntry{}}})
	})

	_, err := client.UpdateScheduledScaling(context.Background(), "svc-1", AutoScalingScheduleUpdate{Entries: []AutoScalingScheduleEntry{}})
	if err != nil {
		t.Fatalf("UpdateScheduledScaling: %v", err)
	}
	if !strings.Contains(captured, `"entries":[]`) {
		t.Errorf("request body should contain empty entries array, got: %s", captured)
	}
}

func TestDeleteScheduledScaling(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/scalingSchedule"
	client, _ := newScheduledScalingTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q; want DELETE", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		w.WriteHeader(http.StatusOK)
	})

	if err := client.DeleteScheduledScaling(context.Background(), "svc-1"); err != nil {
		t.Fatalf("DeleteScheduledScaling: %v", err)
	}
}
