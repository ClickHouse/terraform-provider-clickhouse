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

func newJWTProviderTestClient(t *testing.T, handler http.HandlerFunc) *ClientImpl {
	t.Helper()
	server := httptest.NewServer(handler)
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
	return client
}

func writeJWTProviderResult(t *testing.T, w http.ResponseWriter, provider JWTProvider) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(ResponseWithResult[JWTProvider]{Result: provider}); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func testJWTProvider() JWTProvider {
	rolesClaim := "clickhouse:roles"
	return JWTProvider{
		Name:       "my-oidc-provider",
		Issuer:     "https://auth.example.com",
		Audience:   "my-service",
		JWKSURL:    "https://auth.example.com/.well-known/jwks.json",
		RolesClaim: &rolesClaim,
	}
}

func TestCreateJWTProvider(t *testing.T) {
	want := testJWTProvider()

	client := newJWTProviderTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s; want POST", r.Method)
		}
		if r.URL.Path != "/organizations/provider-org/services/service-1/jwtProviders" {
			t.Errorf("path = %s; want JWT provider collection", r.URL.Path)
		}

		var got JWTProvider
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("request mismatch (-want +got):\n%s", diff)
		}

		writeJWTProviderResult(t, w, want)
	})

	got, err := client.CreateJWTProvider(context.Background(), "service-1", want)
	if err != nil {
		t.Fatalf("CreateJWTProvider: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("provider mismatch (-want +got):\n%s", diff)
	}
}

func TestCreateJWTProviderOmitsUnsetRolesClaim(t *testing.T) {
	provider := testJWTProvider()
	provider.RolesClaim = nil

	client := newJWTProviderTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		if strings.Contains(string(body), "rolesClaim") {
			t.Errorf("request body = %s; want no rolesClaim", body)
		}
		writeJWTProviderResult(t, w, provider)
	})

	got, err := client.CreateJWTProvider(context.Background(), "service-1", provider)
	if err != nil {
		t.Fatalf("CreateJWTProvider: %v", err)
	}
	if got.RolesClaim != nil {
		t.Errorf("rolesClaim = %q; want nil", *got.RolesClaim)
	}
}

func TestGetJWTProvider(t *testing.T) {
	want := testJWTProvider()

	client := newJWTProviderTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s; want GET", r.Method)
		}
		if r.URL.Path != "/organizations/provider-org/services/service-1/jwtProviders/my-oidc-provider" {
			t.Errorf("path = %s; want JWT provider item", r.URL.Path)
		}
		writeJWTProviderResult(t, w, want)
	})

	got, err := client.GetJWTProvider(context.Background(), "service-1", "my-oidc-provider")
	if err != nil {
		t.Fatalf("GetJWTProvider: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("provider mismatch (-want +got):\n%s", diff)
	}
}

func TestGetJWTProviderNotFound(t *testing.T) {
	client := newJWTProviderTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":404,"error":"NOT_FOUND: no JWT provider named \"missing\" exists"}`))
	})

	_, err := client.GetJWTProvider(context.Background(), "service-1", "missing")
	if !IsNotFound(err) {
		t.Fatalf("err = %v; want a 404", err)
	}
}

func TestUpdateJWTProvider(t *testing.T) {
	update := JWTProviderUpdate{
		Issuer:   "https://auth.example.com",
		Audience: "another-service",
		JWKSURL:  "https://auth.example.com/.well-known/jwks.json",
	}
	want := JWTProvider{
		Name:     "my-oidc-provider",
		Issuer:   update.Issuer,
		Audience: update.Audience,
		JWKSURL:  update.JWKSURL,
	}

	client := newJWTProviderTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s; want PUT", r.Method)
		}
		if r.URL.Path != "/organizations/provider-org/services/service-1/jwtProviders/my-oidc-provider" {
			t.Errorf("path = %s; want JWT provider item", r.URL.Path)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		// The name comes from the path and a nil roles claim is left out, which clears it.
		wantBody := map[string]any{
			"issuer":   update.Issuer,
			"audience": update.Audience,
			"jwksUrl":  update.JWKSURL,
		}
		if diff := cmp.Diff(wantBody, got); diff != "" {
			t.Errorf("request mismatch (-want +got):\n%s", diff)
		}

		writeJWTProviderResult(t, w, want)
	})

	got, err := client.UpdateJWTProvider(context.Background(), "service-1", "my-oidc-provider", update)
	if err != nil {
		t.Fatalf("UpdateJWTProvider: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("provider mismatch (-want +got):\n%s", diff)
	}
}

func TestDeleteJWTProvider(t *testing.T) {
	called := false
	client := newJWTProviderTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s; want DELETE", r.Method)
		}
		if r.URL.Path != "/organizations/provider-org/services/service-1/jwtProviders/my-oidc-provider" {
			t.Errorf("path = %s; want JWT provider item", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status":200}`))
	})

	if err := client.DeleteJWTProvider(context.Background(), "service-1", "my-oidc-provider"); err != nil {
		t.Fatalf("DeleteJWTProvider: %v", err)
	}
	if !called {
		t.Fatal("DELETE was not sent")
	}
}
