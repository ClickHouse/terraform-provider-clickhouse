package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCreateDashboard_JSON(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/dashboards" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"name":"D"`) {
			t.Errorf("body not forwarded: %s", body)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"d1","name":"D","tiles":[]}}`)
	})
	got, err := c.CreateDashboard(context.Background(), json.RawMessage(`{"name":"D","tiles":[]}`))
	if err != nil {
		t.Fatalf("CreateDashboard: %v", err)
	}
	id, _ := DashboardID(got)
	if id != "d1" {
		t.Errorf("expected id d1, got %q (body %s)", id, got)
	}
}

func TestGetDashboard_NotFound_JSON(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/dashboards/missing" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := c.GetDashboard(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateDashboard_JSON(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/dashboards/d1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"name":"D2"`) {
			t.Errorf("body not forwarded: %s", body)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"d1","name":"D2"}}`)
	})
	got, err := c.UpdateDashboard(context.Background(), "d1", json.RawMessage(`{"name":"D2","tiles":[]}`))
	if err != nil {
		t.Fatalf("UpdateDashboard: %v", err)
	}
	id, _ := DashboardID(got)
	if id != "d1" {
		t.Errorf("expected id d1, got %q (body %s)", id, got)
	}
}

func TestDeleteDashboard_JSON(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v2/dashboards/d1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.DeleteDashboard(context.Background(), "d1"); err != nil {
		t.Fatalf("DeleteDashboard: %v", err)
	}
}

func TestValidateDashboard(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/dashboards/validate" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"valid":false,"errors":[{"path":"tiles.0.config","message":"bad"}],"normalized":null}`)
	})
	res, err := c.ValidateDashboard(context.Background(), json.RawMessage(`{"name":"D","tiles":[]}`))
	if err != nil {
		t.Fatalf("ValidateDashboard: %v", err)
	}
	if res.Valid || len(res.Errors) != 1 || res.Errors[0].Path != "tiles.0.config" {
		t.Errorf("unexpected result: %+v", res)
	}
}

func TestValidateDashboard_Unsupported(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	if _, err := c.ValidateDashboard(context.Background(), json.RawMessage(`{}`)); !errors.Is(err, ErrValidateUnsupported) {
		t.Errorf("expected ErrValidateUnsupported, got %v", err)
	}
}
