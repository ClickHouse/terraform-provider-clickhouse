package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestListClickHouseSettings(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickhouseSettings"

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"settings":[
			{"name":"compatibility","value":"26.2"},
			{"name":"max_query_size","value":262144},
			{"name":"set_by_cloud","value":{"a":[1,2]}}
		]}}`))
	})

	got, err := client.ListClickHouseSettings(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("ListClickHouseSettings: %v", err)
	}
	want := []ClickHouseSetting{
		{Name: "compatibility", Value: ClickHouseSettingValue{Text: "26.2"}},
		{Name: "max_query_size", Value: ClickHouseSettingValue{Text: "262144", IsNumber: true}},
		{Name: "set_by_cloud", Value: ClickHouseSettingValue{Text: `{"a":[1,2]}`}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListClickHouseSettings mismatch (-want +got):\n%s", diff)
	}
}

func TestGetClickHouseSettingsSchema_ToleratesMixedValueTypes(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickhouseSettings/schema"

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"settings":[
			{"name":"compatibility","type":"string","enum":["25.8","26.2"],"default":"26.2","example":"24.8"},
			{"name":"max_query_size","type":"integer","enum":[0,1],"default":262144,"example":262144}
		]}}`))
	})

	got, err := client.GetClickHouseSettingsSchema(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetClickHouseSettingsSchema: %v", err)
	}
	want := []ClickHouseSettingSchema{
		{Name: "compatibility", Type: "string", Enum: []any{"25.8", "26.2"}, Default: "26.2", Example: "24.8"},
		{Name: "max_query_size", Type: "integer", Enum: []any{float64(0), float64(1)}, Default: float64(262144), Example: float64(262144)},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("GetClickHouseSettingsSchema mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateClickHouseSettings_SendsNativeJSONTypes(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickhouseSettings"

	var capturedBody map[string]map[string]any
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
		_, _ = w.Write([]byte(`{"result":{
			"settings":{"compatibility":"26.2","max_query_size":262144},
			"warnings":[{"name":"compatibility","message":"Changing the compatibility version can cause query failures."}]
		}}`))
	})

	got, err := client.UpdateClickHouseSettings(context.Background(), "svc-1", ClickHouseSettingsUpdate{
		Settings: map[string]ClickHouseSettingValue{
			"compatibility":  {Text: "26.2"},
			"max_query_size": {Text: "262144", IsNumber: true},
		},
	})
	if err != nil {
		t.Fatalf("UpdateClickHouseSettings: %v", err)
	}

	wantBody := map[string]map[string]any{
		"settings": {"compatibility": "26.2", "max_query_size": float64(262144)},
	}
	if diff := cmp.Diff(wantBody, capturedBody); diff != "" {
		t.Errorf("request body mismatch (-want +got):\n%s", diff)
	}
	want := &ClickHouseSettingsUpdateResult{
		Settings: map[string]ClickHouseSettingValue{
			"compatibility":  {Text: "26.2"},
			"max_query_size": {Text: "262144", IsNumber: true},
		},
		Warnings: []ClickHouseSettingWarning{
			{Name: "compatibility", Message: "Changing the compatibility version can cause query failures."},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("UpdateClickHouseSettings mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateClickHouseSettings_RejectsNonNumericNumber(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	})

	_, err := client.UpdateClickHouseSettings(context.Background(), "svc-1", ClickHouseSettingsUpdate{
		Settings: map[string]ClickHouseSettingValue{"max_query_size": {Text: "big", IsNumber: true}},
	})
	if err == nil {
		t.Fatal("UpdateClickHouseSettings: expected an error for a non-numeric number, got nil")
	}
}

func TestResetClickHouseSetting(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickhouseSettings/max_query_size"

	var sawDelete bool
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q; want DELETE", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		sawDelete = true
		w.WriteHeader(http.StatusOK)
	})

	if err := client.ResetClickHouseSetting(context.Background(), "svc-1", "max_query_size"); err != nil {
		t.Fatalf("ResetClickHouseSetting: %v", err)
	}
	if !sawDelete {
		t.Errorf("server did not see DELETE request")
	}
}

func TestResetClickHouseSetting_NotFound(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":404,"error":"not found"}`))
	})

	err := client.ResetClickHouseSetting(context.Background(), "svc-1", "max_query_size")
	if !IsNotFound(err) {
		t.Errorf("ResetClickHouseSetting error = %v; want a 404", err)
	}
}

func TestClickHouseSettingsClient_Errors(t *testing.T) {
	calls := map[string]func(*ClientImpl) error{
		"list": func(c *ClientImpl) error {
			_, err := c.ListClickHouseSettings(context.Background(), "svc-1")
			return err
		},
		"schema": func(c *ClientImpl) error {
			_, err := c.GetClickHouseSettingsSchema(context.Background(), "svc-1")
			return err
		},
		"update": func(c *ClientImpl) error {
			_, err := c.UpdateClickHouseSettings(context.Background(), "svc-1", ClickHouseSettingsUpdate{
				Settings: map[string]ClickHouseSettingValue{"compatibility": {Text: "26.2"}},
			})
			return err
		},
		"reset": func(c *ClientImpl) error {
			return c.ResetClickHouseSetting(context.Background(), "svc-1", "compatibility")
		},
	}

	cases := []struct {
		name   string
		call   string
		status int
		body   string
		check  func(error) bool
	}{
		{name: "list forbidden", call: "list", status: http.StatusForbidden, body: `{"status":403,"error":"forbidden"}`, check: IsForbidden},
		{name: "list malformed", call: "list", status: http.StatusOK, body: `{"result":{"settings":"nope"}}`},
		{name: "schema bad request", call: "schema", status: http.StatusBadRequest, body: `{"status":400,"error":"bad request"}`},
		{name: "schema malformed", call: "schema", status: http.StatusOK, body: `{"result":`},
		{name: "update bad request", call: "update", status: http.StatusBadRequest, body: `{"status":400,"error":"unknown setting"}`},
		{name: "update malformed", call: "update", status: http.StatusOK, body: `{"result":{"warnings":{}}}`, check: IsClickHouseSettingsResponseUnreadable},
		{name: "reset forbidden", call: "reset", status: http.StatusForbidden, body: `{"status":403,"error":"forbidden"}`, check: IsForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			err := calls[tc.call](client)
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if tc.check != nil && !tc.check(err) {
				t.Errorf("error = %v; did not match the expected status", err)
			}
		})
	}
}
