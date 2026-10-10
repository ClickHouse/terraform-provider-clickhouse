package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestUpdateReversePrivateEndpoint_PatchesCustomDNSMappings(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickpipesReversePrivateEndpoints/rpe-1"
	mappings := []CustomPrivateDNSMapping{
		{PrivateDNSName: "one.example.com", TargetID: "rcfg-097648d8068504966"},
		{PrivateDNSName: "two.example.com"},
	}
	request := UpdateReversePrivateEndpoint{CustomPrivateDNSMappings: &mappings}

	var capturedBody map[string]any
	client, _ := newReversePrivateEndpointTestClient(t, func(w http.ResponseWriter, r *http.Request) {
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

		response := ReversePrivateEndpoint{
			CreateReversePrivateEndpoint: CreateReversePrivateEndpoint{
				CustomPrivateDNSMappings: mappings,
			},
			ID:        "rpe-1",
			ServiceID: "svc-1",
			Status:    ReversePrivateEndpointStatusReady,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ReversePrivateEndpoint]{Result: response})
	})

	got, err := client.UpdateReversePrivateEndpoint(context.Background(), "svc-1", "rpe-1", request)
	if err != nil {
		t.Fatalf("UpdateReversePrivateEndpoint: %v", err)
	}

	capturedMappings, ok := capturedBody["customPrivateDnsMappings"].([]any)
	if !ok || len(capturedMappings) != 2 {
		t.Fatalf("customPrivateDnsMappings = %#v; want two mappings", capturedBody["customPrivateDnsMappings"])
	}
	mapping, ok := capturedMappings[0].(map[string]any)
	if !ok {
		t.Fatalf("customPrivateDnsMappings[0] = %#v; want object", capturedMappings[0])
	}
	if mapping["privateDnsName"] != "one.example.com" {
		t.Errorf("privateDnsName = %v; want one.example.com", mapping["privateDnsName"])
	}
	if mapping["targetId"] != "rcfg-097648d8068504966" {
		t.Errorf("targetId = %v; want rcfg-097648d8068504966", mapping["targetId"])
	}
	defaultMapping, ok := capturedMappings[1].(map[string]any)
	if !ok {
		t.Fatalf("customPrivateDnsMappings[1] = %#v; want object", capturedMappings[1])
	}
	if _, exists := defaultMapping["targetId"]; exists {
		t.Errorf("mapping without a target must omit targetId: %#v", defaultMapping)
	}
	if _, exists := defaultMapping["internalDnsName"]; exists {
		t.Errorf("internalDnsName must not be sent: %#v", defaultMapping)
	}

	if len(got.CustomPrivateDNSMappings) != 2 || got.CustomPrivateDNSMappings[1].PrivateDNSName != "two.example.com" {
		t.Fatalf("CustomPrivateDNSMappings = %#v; want two mappings", got.CustomPrivateDNSMappings)
	}
	if got.CustomPrivateDNSMappings[0].TargetID != "rcfg-097648d8068504966" {
		t.Fatalf("TargetID = %q; want rcfg-097648d8068504966", got.CustomPrivateDNSMappings[0].TargetID)
	}
}

func TestGetReversePrivateEndpoint_DecodesDNSTargetsAndPrivateDNSMappings(t *testing.T) {
	expectedPath := "/organizations/org-1/services/svc-1/clickpipesReversePrivateEndpoints/rpe-1"
	client, _ := newReversePrivateEndpointTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != expectedPath {
			t.Errorf("request = %s %s; want GET %s", r.Method, r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{
			"id":"rpe-1",
			"type":"VPC_RESOURCE",
			"status":"Ready",
			"dnsNames":["vpce-1.rcfg-00.example.on.aws"],
			"dnsTargets":[
				{"id":"rcfg-00","kind":"RESOURCE_CONFIGURATION","internalDnsName":"vpce-1.rcfg-00.example.on.aws"},
				{"id":"rcfg-01","kind":"RESOURCE_CONFIGURATION","internalDnsName":"vpce-1.rcfg-01.example.on.aws"}
			],
			"privateDnsMappings":[{"privateDnsName":"node-00.example.com","internalDnsName":"vpce-1.rcfg-00.example.on.aws"}],
			"customPrivateDnsMappings":[{"privateDnsName":"node-01-pri.example.com","targetId":"rcfg-01"}]
		}}`)
	})

	got, err := client.GetReversePrivateEndpoint(context.Background(), "svc-1", "rpe-1")
	if err != nil {
		t.Fatalf("GetReversePrivateEndpoint: %v", err)
	}

	wantTargets := []DNSTarget{
		{ID: "rcfg-00", Kind: "RESOURCE_CONFIGURATION", InternalDNSName: "vpce-1.rcfg-00.example.on.aws"},
		{ID: "rcfg-01", Kind: "RESOURCE_CONFIGURATION", InternalDNSName: "vpce-1.rcfg-01.example.on.aws"},
	}
	if !reflect.DeepEqual(got.DNSTargets, wantTargets) {
		t.Fatalf("DNSTargets = %#v; want %#v", got.DNSTargets, wantTargets)
	}
	wantPrivate := []PrivateDNSMapping{{PrivateDNSName: "node-00.example.com", InternalDNSName: "vpce-1.rcfg-00.example.on.aws"}}
	if !reflect.DeepEqual(got.PrivateDNSMappings, wantPrivate) {
		t.Fatalf("PrivateDNSMappings = %#v; want %#v", got.PrivateDNSMappings, wantPrivate)
	}
	wantCustom := []CustomPrivateDNSMapping{{PrivateDNSName: "node-01-pri.example.com", TargetID: "rcfg-01"}}
	if !reflect.DeepEqual(got.CustomPrivateDNSMappings, wantCustom) {
		t.Fatalf("CustomPrivateDNSMappings = %#v; want %#v", got.CustomPrivateDNSMappings, wantCustom)
	}
}

func TestUpdateReversePrivateEndpoint_PreservesEmptyCustomDNSMappings(t *testing.T) {
	mappings := []CustomPrivateDNSMapping{}
	request := UpdateReversePrivateEndpoint{CustomPrivateDNSMappings: &mappings}

	var capturedBody map[string]any
	client, _ := newReversePrivateEndpointTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		response := ReversePrivateEndpoint{
			CreateReversePrivateEndpoint: CreateReversePrivateEndpoint{
				CustomPrivateDNSMappings: mappings,
			},
			ID:        "rpe-1",
			ServiceID: "svc-1",
			Status:    ReversePrivateEndpointStatusReady,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ReversePrivateEndpoint]{Result: response})
	})

	_, err := client.UpdateReversePrivateEndpoint(context.Background(), "svc-1", "rpe-1", request)
	if err != nil {
		t.Fatalf("UpdateReversePrivateEndpoint: %v", err)
	}

	capturedMappings, ok := capturedBody["customPrivateDnsMappings"].([]any)
	if !ok {
		t.Fatalf("customPrivateDnsMappings = %#v; want empty array", capturedBody["customPrivateDnsMappings"])
	}
	if len(capturedMappings) != 0 {
		t.Fatalf("customPrivateDnsMappings length = %d; want 0", len(capturedMappings))
	}
}
