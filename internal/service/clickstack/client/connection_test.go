package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testAPIKey = "test-api-key"

// newTestClient returns a Client pointed at a httptest server running handler.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := New(server.URL, testAPIKey, server.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{name: "valid http", endpoint: "http://localhost:8000", wantErr: false},
		{name: "valid https", endpoint: "https://api.hyperdx.io", wantErr: false},
		{name: "trailing slash accepted", endpoint: "http://localhost:8000/", wantErr: false},
		{name: "invalid scheme", endpoint: "ftp://example.com", wantErr: true},
		{name: "not a url", endpoint: "://nope", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(tt.endpoint, testAPIKey, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("New(%q) error = %v, wantErr %v", tt.endpoint, err, tt.wantErr)
			}
		})
	}
}

func TestCreateConnection(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/connections" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
			t.Errorf("unexpected Authorization header: %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["name"] != "Test" || body["password"] != "secret" {
			t.Errorf("unexpected body: %v", body)
		}
		if _, ok := body["prometheusEndpoint"]; ok {
			t.Error("expected nil prometheusEndpoint to be omitted from create body")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"abc123","name":"Test","host":"http://ch:8123","username":"default"}}`)
	})

	conn, err := c.CreateConnection(context.Background(), CreateConnectionInput{
		Name:     "Test",
		Host:     "http://ch:8123",
		Username: "default",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}
	if conn.ID != "abc123" {
		t.Errorf("expected id abc123, got %q", conn.ID)
	}
}

func TestGetConnection(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/connections/abc123" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"abc123","name":"Test","host":"http://ch:8123","username":"default","hyperdxSettingPrefix":"hdx_"}}`)
	})

	conn, err := c.GetConnection(context.Background(), "abc123")
	if err != nil {
		t.Fatalf("GetConnection: %v", err)
	}
	if conn.HyperdxSettingPrefix == nil || *conn.HyperdxSettingPrefix != "hdx_" {
		t.Errorf("expected hyperdxSettingPrefix hdx_, got %v", conn.HyperdxSettingPrefix)
	}
	if conn.PrometheusEndpoint != nil {
		t.Errorf("expected nil prometheusEndpoint, got %v", *conn.PrometheusEndpoint)
	}
}

func TestGetConnection_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.GetConnection(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListConnections(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/connections" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"a"},{"id":"b"}]}`)
	})

	conns, err := c.ListConnections(context.Background())
	if err != nil {
		t.Fatalf("ListConnections: %v", err)
	}
	if len(conns) != 2 {
		t.Errorf("expected 2 connections, got %d", len(conns))
	}
}

func TestUpdateConnection_NullSemantics(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/connections/abc123" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		// nil password must be omitted (keep existing); nil prefix/endpoint
		// must be serialized as explicit null (clear existing).
		if _, ok := body["password"]; ok {
			t.Error("expected nil password to be omitted from update body")
		}
		if string(body["hyperdxSettingPrefix"]) != "null" {
			t.Errorf("expected hyperdxSettingPrefix null, got %s", body["hyperdxSettingPrefix"])
		}
		if string(body["prometheusEndpoint"]) != "null" {
			t.Errorf("expected prometheusEndpoint null, got %s", body["prometheusEndpoint"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"abc123","name":"Renamed","host":"http://ch:8123","username":"default"}}`)
	})

	conn, err := c.UpdateConnection(context.Background(), "abc123", UpdateConnectionInput{
		Name:     "Renamed",
		Host:     "http://ch:8123",
		Username: "default",
	})
	if err != nil {
		t.Fatalf("UpdateConnection: %v", err)
	}
	if conn.Name != "Renamed" {
		t.Errorf("expected name Renamed, got %q", conn.Name)
	}
}

func TestUpdateConnection_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.UpdateConnection(context.Background(), "missing", UpdateConnectionInput{})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteConnection(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v2/connections/abc123" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	})

	if err := c.DeleteConnection(context.Background(), "abc123"); err != nil {
		t.Fatalf("DeleteConnection: %v", err)
	}
}

func TestWithTeam_SetsHeader(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-hdx-team"); got != "team-123" {
			t.Errorf("expected x-hdx-team header team-123, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"abc123"}}`)
	})

	if _, err := c.WithTeam("team-123").GetConnection(context.Background(), "abc123"); err != nil {
		t.Fatalf("GetConnection: %v", err)
	}
}

func TestWithTeam_OmittedByDefault(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["X-Hdx-Team"]; ok {
			t.Errorf("expected x-hdx-team header to be absent, got %q", r.Header.Get("x-hdx-team"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"abc123"}}`)
	})

	// Empty team must not set the header, and must reuse the same client.
	if c.WithTeam("") != c {
		t.Error("expected WithTeam(\"\") to return the receiver unchanged")
	}
	if _, err := c.GetConnection(context.Background(), "abc123"); err != nil {
		t.Fatalf("GetConnection: %v", err)
	}
}

func TestDo_APIErrorMessage(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"Body validation failed: host: Required"}`)
	})

	_, err := c.CreateConnection(context.Background(), CreateConnectionInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "Body validation failed: host: Required"
	if got := err.Error(); !strings.Contains(got, want) {
		t.Errorf("expected error containing %q, got %q", want, got)
	}
}
