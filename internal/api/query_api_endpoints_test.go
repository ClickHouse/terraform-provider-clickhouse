package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestQueryAPIEndpointClient(t *testing.T) {
	ctx := context.Background()
	request := QueryAPIEndpointRequest{
		Name:           "daily-actives",
		SQL:            "SELECT count() FROM events WHERE date = {date:Date}",
		Database:       "analytics",
		Parameters:     map[string]string{"date": "2026-08-13"},
		APIKeyIDs:      []string{"key-1"},
		Roles:          []string{"analytics_reader"},
		AllowedOrigins: []string{"https://example.com"},
	}
	endpoint := QueryAPIEndpoint{
		QueryAPIEndpointRequest: request,
		ID:                      "endpoint-1",
		URL:                     "https://queries.clickhouse.cloud/run/endpoint-1",
		OwnerType:               QueryAPIEndpointOwnerType,
	}

	const collectionPath = "/organizations/provider-org/services/service-1/query-api-endpoints"
	const resourcePath = collectionPath + "/endpoint-1"

	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)

		switch r.Method {
		case http.MethodPost, http.MethodPut:
			var got QueryAPIEndpointRequest
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if diff := cmp.Diff(request, got); diff != "" {
				t.Errorf("request mismatch (-want +got):\n%s", diff)
			}
		case http.MethodGet, http.MethodDelete:
		default:
			t.Errorf("unexpected method %s", r.Method)
		}

		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		if r.Method != http.MethodDelete {
			if err := json.NewEncoder(w).Encode(ResponseWithResult[QueryAPIEndpoint]{Result: endpoint}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
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

	created, err := client.CreateQueryAPIEndpoint(ctx, "service-1", request)
	if err != nil {
		t.Fatalf("CreateQueryAPIEndpoint: %v", err)
	}
	if diff := cmp.Diff(&endpoint, created); diff != "" {
		t.Errorf("created endpoint mismatch (-want +got):\n%s", diff)
	}

	got, err := client.GetQueryAPIEndpoint(ctx, "service-1", "endpoint-1")
	if err != nil {
		t.Fatalf("GetQueryAPIEndpoint: %v", err)
	}
	if diff := cmp.Diff(&endpoint, got); diff != "" {
		t.Errorf("read endpoint mismatch (-want +got):\n%s", diff)
	}

	updated, err := client.UpdateQueryAPIEndpoint(ctx, "service-1", "endpoint-1", request)
	if err != nil {
		t.Fatalf("UpdateQueryAPIEndpoint: %v", err)
	}
	if diff := cmp.Diff(&endpoint, updated); diff != "" {
		t.Errorf("updated endpoint mismatch (-want +got):\n%s", diff)
	}

	if err := client.DeleteQueryAPIEndpoint(ctx, "service-1", "endpoint-1"); err != nil {
		t.Fatalf("DeleteQueryAPIEndpoint: %v", err)
	}

	wantRequests := []string{
		http.MethodPost + " " + collectionPath,
		http.MethodGet + " " + resourcePath,
		http.MethodPut + " " + resourcePath,
		http.MethodDelete + " " + resourcePath,
	}
	if diff := cmp.Diff(wantRequests, requests); diff != "" {
		t.Errorf("requests mismatch (-want +got):\n%s", diff)
	}
}

func TestGetQueryAPIEndpointNotFound(t *testing.T) {
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

	_, err = client.GetQueryAPIEndpoint(context.Background(), "service-1", "missing")
	if !IsNotFound(err) {
		t.Fatalf("GetQueryAPIEndpoint error = %v; want 404", err)
	}
}

func TestCreateQueryAPIEndpointRequiresCreatedStatus(t *testing.T) {
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

	_, err = client.CreateQueryAPIEndpoint(context.Background(), "service-1", QueryAPIEndpointRequest{})
	if err == nil {
		t.Fatal("CreateQueryAPIEndpoint should reject a 200 response")
	}
}
