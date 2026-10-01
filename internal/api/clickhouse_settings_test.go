package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestListServiceClickhouseSettings_NormalizesValues(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickhouseSettings"

	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		_, _ = w.Write([]byte(`{"result":{"settings":[{"name":"compatibility","value":"26.2"},{"name":"max_query_size","value":262144}]}}`))
	})

	got, err := client.ListServiceClickhouseSettings(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("ListServiceClickhouseSettings: %v", err)
	}
	want := map[string]string{"compatibility": "26.2", "max_query_size": "262144"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListServiceClickhouseSettings mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateServiceClickhouseSettings_SendsNativeTypes(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickhouseSettings"

	var capturedBody map[string]map[string]any
	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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
		_, _ = w.Write([]byte(`{"result":{"settings":{"compatibility":"26.2","max_query_size":262144},"warnings":[{"name":"compatibility","message":"test first"}]}}`))
	})

	got, err := client.UpdateServiceClickhouseSettings(context.Background(), "svc-1", map[string]any{
		"compatibility":  "26.2",
		"max_query_size": int64(262144),
	})
	if err != nil {
		t.Fatalf("UpdateServiceClickhouseSettings: %v", err)
	}

	if v, ok := capturedBody["settings"]["compatibility"].(string); !ok || v != "26.2" {
		t.Errorf("request compatibility = %#v; want string 26.2", capturedBody["settings"]["compatibility"])
	}
	if v, ok := capturedBody["settings"]["max_query_size"].(float64); !ok || v != 262144 {
		t.Errorf("request max_query_size = %#v; want number 262144", capturedBody["settings"]["max_query_size"])
	}

	want := &ServiceClickhouseSettingsUpdateResult{
		Settings: map[string]string{"compatibility": "26.2", "max_query_size": "262144"},
		Warnings: []ServiceClickhouseSettingWarning{{Name: "compatibility", Message: "test first"}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("UpdateServiceClickhouseSettings mismatch (-want +got):\n%s", diff)
	}
}

func TestDeleteServiceClickhouseSetting(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickhouseSettings/max_query_size"
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

	if err := client.DeleteServiceClickhouseSetting(context.Background(), "svc-1", "max_query_size"); err != nil {
		t.Fatalf("DeleteServiceClickhouseSetting: %v", err)
	}
	if !sawDelete {
		t.Errorf("server did not see DELETE request")
	}
}

func TestGetServiceClickhouseSettingsSchema(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickhouseSettings/schema"

	client, _ := newUpgradeWindowTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		_, _ = w.Write([]byte(`{"result":{"settings":[{"name":"compatibility","type":"string","example":"24.8"},{"name":"max_query_size","type":"integer"}]}}`))
	})

	got, err := client.GetServiceClickhouseSettingsSchema(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetServiceClickhouseSettingsSchema: %v", err)
	}
	want := []ServiceClickhouseSettingSchemaEntry{
		{Name: "compatibility", Type: "string", Example: "24.8"},
		{Name: "max_query_size", Type: "integer"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("GetServiceClickhouseSettingsSchema mismatch (-want +got):\n%s", diff)
	}
}
