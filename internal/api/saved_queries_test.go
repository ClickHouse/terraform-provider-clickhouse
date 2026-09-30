package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCreateSavedQuery(t *testing.T) {
	request := SavedQueryRequest{
		Name:       "daily-actives",
		SQL:        "SELECT count() FROM events WHERE date = {date:Date}",
		Database:   "analytics",
		Parameters: map[string]string{"date": "2026-08-13"},
	}
	want := SavedQuery{
		SavedQueryRequest: request,
		ID:                "query-1",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s; want POST", r.Method)
		}
		if r.URL.Path != "/organizations/provider-org/services/service-1/saved-queries" {
			t.Errorf("path = %s; want saved-query collection", r.URL.Path)
		}

		var got SavedQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if diff := cmp.Diff(request, got); diff != "" {
			t.Errorf("request mismatch (-want +got):\n%s", diff)
		}

		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(ResponseWithResult[SavedQuery]{Result: want}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{
		ApiURL:         server.URL,
		OrganizationID: "provider-org",
		TokenKey:       "key",
		TokenSecret:    "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	got, err := client.CreateSavedQuery(context.Background(), "service-1", request)
	if err != nil {
		t.Fatalf("CreateSavedQuery: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("saved query mismatch (-want +got):\n%s", diff)
	}
}

func TestGetSavedQuery(t *testing.T) {
	want := SavedQuery{
		SavedQueryRequest: SavedQueryRequest{
			Name:       "daily-actives",
			SQL:        "SELECT count() FROM events",
			Database:   "analytics",
			Parameters: map[string]string{"date": "Date"},
		},
		ID: "query-1",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s; want GET", r.Method)
		}
		if r.URL.Path != "/organizations/provider-org/services/service-1/saved-queries/query-1" {
			t.Errorf("path = %s; want saved-query resource", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(ResponseWithResult[SavedQuery]{Result: want}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{
		ApiURL:         server.URL,
		OrganizationID: "provider-org",
		TokenKey:       "key",
		TokenSecret:    "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	got, err := client.GetSavedQuery(context.Background(), "service-1", "query-1")
	if err != nil {
		t.Fatalf("GetSavedQuery: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("saved query mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateSavedQuery(t *testing.T) {
	request := SavedQueryRequest{
		Name:       "daily-actives-updated",
		SQL:        "SELECT uniq(user_id) FROM events",
		Database:   "analytics",
		Parameters: map[string]string{"date": "Date"},
	}
	want := SavedQuery{
		SavedQueryRequest: request,
		ID:                "query-1",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s; want PUT", r.Method)
		}
		if r.URL.Path != "/organizations/provider-org/services/service-1/saved-queries/query-1" {
			t.Errorf("path = %s; want saved-query resource", r.URL.Path)
		}

		var got SavedQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if diff := cmp.Diff(request, got); diff != "" {
			t.Errorf("request mismatch (-want +got):\n%s", diff)
		}
		if err := json.NewEncoder(w).Encode(ResponseWithResult[SavedQuery]{Result: want}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{
		ApiURL:         server.URL,
		OrganizationID: "provider-org",
		TokenKey:       "key",
		TokenSecret:    "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	got, err := client.UpdateSavedQuery(context.Background(), "service-1", "query-1", request)
	if err != nil {
		t.Fatalf("UpdateSavedQuery: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("saved query mismatch (-want +got):\n%s", diff)
	}
}

func TestDeleteSavedQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s; want DELETE", r.Method)
		}
		if r.URL.Path != "/organizations/provider-org/services/service-1/saved-queries/query-1" {
			t.Errorf("path = %s; want saved-query resource", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(ClientConfig{
		ApiURL:         server.URL,
		OrganizationID: "provider-org",
		TokenKey:       "key",
		TokenSecret:    "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := client.DeleteSavedQuery(context.Background(), "service-1", "query-1"); err != nil {
		t.Fatalf("DeleteSavedQuery: %v", err)
	}
}

func TestGetSavedQueryNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
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

	_, err = client.GetSavedQuery(context.Background(), "service-1", "missing")
	if !IsNotFound(err) {
		t.Fatalf("GetSavedQuery error = %v; want 404", err)
	}
}

func TestCreateSavedQueryRequiresCreatedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
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

	_, err = client.CreateSavedQuery(context.Background(), "service-1", SavedQueryRequest{})
	if err == nil {
		t.Fatal("CreateSavedQuery should reject a 200 response")
	}
}
