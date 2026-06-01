package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newReversePrivateEndpointTestClient(t *testing.T, handler http.HandlerFunc) (*ClientImpl, *httptest.Server) {
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

func TestCreateReversePrivateEndpoint_PostsGCPPSCAndCustomDNSMappings(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickpipesReversePrivateEndpoints"
	gcpServiceAttachment := "projects/my-project/regions/us-central1/serviceAttachments/my-service"
	request := CreateReversePrivateEndpoint{
		Description:          "gcp psc endpoint",
		Type:                 ReversePrivateEndpointTypeGCPPSCServiceAttachment,
		GCPServiceAttachment: &gcpServiceAttachment,
		CustomPrivateDNSMappings: []CustomPrivateDNSMapping{
			{PrivateDNSName: "my-service.example.com"},
		},
	}

	var capturedBody map[string]any
	client, _ := newReversePrivateEndpointTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q; want POST", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}

		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		response := ReversePrivateEndpoint{
			CreateReversePrivateEndpoint: request,
			ID:                           "rpe-1",
			ServiceID:                    "svc-1",
			EndpointID:                   "psc-endpoint",
			DNSNames:                     []string{"internal.example.com"},
			PrivateDNSNames:              []string{"private.example.com"},
			Status:                       ReversePrivateEndpointStatusReady,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ReversePrivateEndpoint]{Result: response})
	})

	got, err := client.CreateReversePrivateEndpoint(context.Background(), "svc-1", request)
	if err != nil {
		t.Fatalf("CreateReversePrivateEndpoint: %v", err)
	}

	if capturedBody["type"] != ReversePrivateEndpointTypeGCPPSCServiceAttachment {
		t.Errorf("type = %v; want %s", capturedBody["type"], ReversePrivateEndpointTypeGCPPSCServiceAttachment)
	}
	if capturedBody["gcpServiceAttachment"] != gcpServiceAttachment {
		t.Errorf("gcpServiceAttachment = %v; want %s", capturedBody["gcpServiceAttachment"], gcpServiceAttachment)
	}

	mappings, ok := capturedBody["customPrivateDnsMappings"].([]any)
	if !ok || len(mappings) != 1 {
		t.Fatalf("customPrivateDnsMappings = %#v; want one mapping", capturedBody["customPrivateDnsMappings"])
	}
	mapping, ok := mappings[0].(map[string]any)
	if !ok {
		t.Fatalf("customPrivateDnsMappings[0] = %#v; want object", mappings[0])
	}
	if mapping["privateDnsName"] != "my-service.example.com" {
		t.Errorf("privateDnsName = %v; want my-service.example.com", mapping["privateDnsName"])
	}

	if got.GCPServiceAttachment == nil || *got.GCPServiceAttachment != gcpServiceAttachment {
		t.Fatalf("GCPServiceAttachment = %v; want %s", got.GCPServiceAttachment, gcpServiceAttachment)
	}
	if len(got.CustomPrivateDNSMappings) != 1 || got.CustomPrivateDNSMappings[0].PrivateDNSName != "my-service.example.com" {
		t.Fatalf("CustomPrivateDNSMappings = %#v; want my-service.example.com", got.CustomPrivateDNSMappings)
	}
}
