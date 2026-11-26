package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCreateByocInfrastructure_HappyPath(t *testing.T) {
	want := ByocInfrastructure{
		Id:            "4a2c7b1e-0000-4000-8000-000000000001",
		State:         ByocStateProvisioning,
		AccountId:     "123456789012",
		RegionId:      "us-east-2",
		CloudProvider: "aws",
		DisplayName:   "byoc-prod",
	}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q; want POST", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/byocInfrastructure" {
			t.Errorf("path = %q; want /organizations/org-1/byocInfrastructure", r.URL.Path)
		}
		assertBasicAuth(t, r)

		body, _ := io.ReadAll(r.Body)
		got := map[string]any{}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if got["regionId"] != "us-east-2" || got["accountId"] != "123456789012" {
			t.Errorf("body = %v; want regionId/accountId set", got)
		}
		if got["externalId"] != "ch-abcdef0123456789" {
			t.Errorf("externalId = %v; want ch-abcdef0123456789", got["externalId"])
		}
		if _, present := got["vpcId"]; present {
			t.Errorf("vpcId should be omitted when unset; body = %s", body)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructure]{Result: want})
	})

	got, err := client.CreateByocInfrastructure(context.Background(), ByocInfrastructureCreateRequest{
		RegionId:    "us-east-2",
		AccountId:   "123456789012",
		DisplayName: strPtr("byoc-prod"),
		ExternalId:  strPtr("ch-abcdef0123456789"),
		Tags:        map[string]string{"team": "data"},
	})
	if err != nil {
		t.Fatalf("CreateByocInfrastructure: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("CreateByocInfrastructure mismatch (-want +got):\n%s", diff)
	}
}

func TestCreateByocInfrastructure_DoesNotRetry(t *testing.T) {
	var requests atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"transient"}`))
	})

	_, err := client.CreateByocInfrastructure(context.Background(), ByocInfrastructureCreateRequest{
		RegionId:  "us-east-2",
		AccountId: "123456789012",
	})
	if err == nil {
		t.Fatal("CreateByocInfrastructure must fail on a 5xx response")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d; the non-idempotent create must never be retried", got)
	}
}

func TestValidateByocInfrastructure_HappyPath(t *testing.T) {
	want := ByocInfrastructureValidation{
		CloudProvider: "aws",
		Supported:     true,
		AllPassed:     false,
		AnyPassed:     true,
		Checks: []ByocInfrastructureValidationCheck{
			{Name: "Create EKS cluster", Action: "eks:CreateCluster", Group: "base", Allowed: false, Reason: "implicitDeny"},
		},
	}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q; want POST", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/byocInfrastructure/validate" {
			t.Errorf("path = %q; want /organizations/org-1/byocInfrastructure/validate", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		got := map[string]any{}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		// The validate endpoint does not accept displayName; the dedicated
		// request type must make sending it impossible.
		if _, present := got["displayName"]; present {
			t.Errorf("displayName must never be sent to validate; body = %s", body)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructureValidation]{Result: want})
	})

	got, err := client.ValidateByocInfrastructure(context.Background(), ByocInfrastructureValidateRequest{
		RegionId:  "us-east-2",
		AccountId: "123456789012",
	})
	if err != nil {
		t.Fatalf("ValidateByocInfrastructure: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("ValidateByocInfrastructure mismatch (-want +got):\n%s", diff)
	}
}

func TestGetByocInfrastructure_HappyPath(t *testing.T) {
	want := ByocInfrastructureDetails{
		Id:                       "4a2c7b1e-0000-4000-8000-000000000001",
		State:                    ByocStateReady,
		AccountId:                "123456789012",
		RegionId:                 "us-east-2",
		CloudProvider:            "aws",
		DisplayName:              "byoc-prod",
		EnablePrivateLink:        boolPtr(true),
		EnablePublicLoadBalancer: boolPtr(false),
		VpcCidrRange:             strPtr("10.0.0.0/16"),
		VpcAvailabilityZoneList:  []string{"us-east-2a", "us-east-2b", "us-east-2c"},
		IsByoVpc:                 boolPtr(false),
	}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/byocInfrastructure/"+want.Id {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructureDetails]{Result: want})
	})

	got, err := client.GetByocInfrastructure(context.Background(), want.Id)
	if err != nil {
		t.Fatalf("GetByocInfrastructure: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetByocInfrastructure mismatch (-want +got):\n%s", diff)
	}
}

func TestCreateByocInfrastructure_RestrictedSendsAccessModelAndFacts(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got := map[string]any{}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if got["accessModel"] != ByocAccessModelRestricted {
			t.Errorf("accessModel = %v; want restricted", got["accessModel"])
		}
		facts, ok := got["restrictedFacts"].(map[string]any)
		if !ok {
			t.Fatalf("restrictedFacts missing; body = %s", body)
		}
		want := map[string]any{
			"clusterName":     "ra-cluster",
			"imageMode":       ByocImageModeDirect,
			"executorRoleArn": "arn:aws:iam::123456789012:role/executor",
			"nodeRoleArns":    []any{"arn:aws:iam::123456789012:role/node"},
			"dataBucket":      "data",
			"backupBucket":    "backup",
			"storageRoleArn":  "arn:aws:iam::123456789012:role/storage",
		}
		if diff := cmp.Diff(want, facts); diff != "" {
			t.Errorf("restrictedFacts mismatch, unset optionals must be omitted (-want +got):\n%s", diff)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructure]{Result: ByocInfrastructure{
			Id: "byoc-1", State: ByocStateAwaitingConnector, AccessModel: ByocAccessModelRestricted,
		}})
	})

	got, err := client.CreateByocInfrastructure(context.Background(), ByocInfrastructureCreateRequest{
		RegionId:    "us-east-2",
		AccountId:   "123456789012",
		AccessModel: strPtr(ByocAccessModelRestricted),
		RestrictedFacts: &ByocRestrictedFacts{
			ClusterName:     "ra-cluster",
			ImageMode:       ByocImageModeDirect,
			ExecutorRoleArn: strPtr("arn:aws:iam::123456789012:role/executor"),
			NodeRoleArns:    []string{"arn:aws:iam::123456789012:role/node"},
			DataBucket:      "data",
			BackupBucket:    "backup",
			StorageRoleArn:  "arn:aws:iam::123456789012:role/storage",
		},
	})
	if err != nil {
		t.Fatalf("CreateByocInfrastructure: %v", err)
	}
	if got.State != ByocStateAwaitingConnector || !IsRestrictedAccess(got.AccessModel) {
		t.Errorf("got %+v; want a restricted infrastructure awaiting its connector", got)
	}
}

func TestGetByocInfrastructure_RestrictedDecodesFactsAndConnector(t *testing.T) {
	raw := `{"result":{"id":"byoc-1","state":"infra-ready","accountId":"123456789012","regionId":"us-east-2",
		"cloudProvider":"aws","displayName":"ra","enablePrivateLink":false,"accessModel":"restricted",
		"restrictedFacts":{"clusterName":"ra-cluster","platformNodeGroup":"system","availabilityZones":["us-east-2a","us-east-2b","us-east-2c"],
			"imageMode":"bundle","imageRegistry":"registry.example.com/ch","dataBucket":"data","backupBucket":"backup",
			"storageRoleArn":"arn:aws:iam::123456789012:role/storage","enableMonitoring":false},
		"connector":{"lastHeartbeatAt":"2026-01-01T00:00:00Z","platformState":"ready",
			"components":[{"name":"executor","state":"connected","version":"1.2.3","lastHeartbeatAt":"2026-01-01T00:00:00Z"}]}}}`
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(raw))
	})

	got, err := client.GetByocInfrastructure(context.Background(), "byoc-1")
	if err != nil {
		t.Fatalf("GetByocInfrastructure: %v", err)
	}
	wantFacts := &ByocRestrictedFacts{
		ClusterName:       "ra-cluster",
		PlatformNodeGroup: strPtr("system"),
		AvailabilityZones: []string{"us-east-2a", "us-east-2b", "us-east-2c"},
		ImageMode:         ByocImageModeBundle,
		ImageRegistry:     strPtr("registry.example.com/ch"),
		DataBucket:        "data",
		BackupBucket:      "backup",
		StorageRoleArn:    "arn:aws:iam::123456789012:role/storage",
		EnableMonitoring:  boolPtr(false),
	}
	if diff := cmp.Diff(wantFacts, got.RestrictedFacts); diff != "" {
		t.Errorf("restrictedFacts mismatch (-want +got):\n%s", diff)
	}
	wantConnector := &ByocConnector{
		LastHeartbeatAt: strPtr("2026-01-01T00:00:00Z"),
		PlatformState:   strPtr("ready"),
		Components: []ByocConnectorComponent{
			{Name: "executor", State: "connected", Version: strPtr("1.2.3"), LastHeartbeatAt: strPtr("2026-01-01T00:00:00Z")},
		},
	}
	if diff := cmp.Diff(wantConnector, got.Connector); diff != "" {
		t.Errorf("connector mismatch (-want +got):\n%s", diff)
	}
	if !IsRestrictedAccess(got.AccessModel) {
		t.Errorf("accessModel = %q; want restricted", got.AccessModel)
	}
}

func TestCreateByocInfrastructureEnrollmentToken(t *testing.T) {
	cases := []struct {
		name     string
		ttl      *int64
		wantBody string
	}{
		{name: "default ttl sends an empty object", wantBody: `{}`},
		{name: "explicit ttl", ttl: func() *int64 { v := int64(7200); return &v }(), wantBody: `{"ttlSeconds":7200}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %q; want POST", r.Method)
				}
				if r.URL.Path != "/organizations/org-1/byocInfrastructure/byoc-1/enrollmentToken" {
					t.Errorf("path = %q", r.URL.Path)
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != tc.wantBody {
					t.Errorf("body = %s; want %s", body, tc.wantBody)
				}
				_, _ = w.Write([]byte(`{"result":{"token":"enroll-secret","expiresAt":"2026-01-01T04:00:00Z"}}`))
			})

			got, err := client.CreateByocInfrastructureEnrollmentToken(context.Background(), "byoc-1", ByocEnrollmentTokenRequest{TtlSeconds: tc.ttl})
			if err != nil {
				t.Fatalf("CreateByocInfrastructureEnrollmentToken: %v", err)
			}
			want := &ByocEnrollmentToken{Token: "enroll-secret", ExpiresAt: "2026-01-01T04:00:00Z"}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("token mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCreateByocInfrastructure_ConflictIsClassifiable(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"CONFLICT: a BYOC infrastructure is already registered"}`))
	})

	_, err := client.CreateByocInfrastructure(context.Background(), ByocInfrastructureCreateRequest{
		RegionId:  "us-east-2",
		AccountId: "123456789012",
	})
	if !IsConflict(err) {
		t.Fatalf("err = %v; want IsConflict so the resource can suggest an import", err)
	}
}

func TestCreateByocInfrastructureEnrollmentToken_DoesNotRetry(t *testing.T) {
	var requests atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"transient"}`))
	})

	_, err := client.CreateByocInfrastructureEnrollmentToken(context.Background(), "byoc-1", ByocEnrollmentTokenRequest{})
	if err == nil {
		t.Fatal("CreateByocInfrastructureEnrollmentToken must fail on a 5xx response")
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("requests = %d; a mint whose response was lost must never be replayed", got)
	}
}

func TestCreateByocInfrastructureEnrollmentToken_ConflictUntilTrustAdmitted(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"cluster trust is not admitted yet"}`))
	})

	_, err := client.CreateByocInfrastructureEnrollmentToken(context.Background(), "byoc-1", ByocEnrollmentTokenRequest{})
	if !IsConflict(err) {
		t.Fatalf("err = %v; want IsConflict so callers can wait for trust admission", err)
	}
}

func TestGetByocInfrastructure_NotFound(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"NOT_FOUND: BYOC infrastructure not found."}`))
	})

	_, err := client.GetByocInfrastructure(context.Background(), "missing")
	if !IsNotFound(err) {
		t.Fatalf("err = %v; want IsNotFound", err)
	}
}

func TestUpdateByocInfrastructure_SendsOnlySetFields(t *testing.T) {
	want := ByocInfrastructure{Id: "byoc-1", State: ByocStateReady, DisplayName: "renamed"}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q; want PATCH", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/byocInfrastructure/byoc-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		got := map[string]any{}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if got["displayName"] != "renamed" {
			t.Errorf("displayName = %v; want renamed", got["displayName"])
		}
		if got["enablePrivateLink"] != false {
			t.Errorf("enablePrivateLink = %v; want false", got["enablePrivateLink"])
		}
		for _, absent := range []string{"enablePrivateLoadBalancer", "enablePublicLoadBalancer", "gcpPscSubnetId", "tags"} {
			if _, present := got[absent]; present {
				t.Errorf("%s should be omitted when unset; body = %s", absent, body)
			}
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructure]{Result: want})
	})

	got, err := client.UpdateByocInfrastructure(context.Background(), "byoc-1", ByocInfrastructureUpdateRequest{
		DisplayName:       strPtr("renamed"),
		EnablePrivateLink: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("UpdateByocInfrastructure: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("UpdateByocInfrastructure mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateByocInfrastructure_EmptyTagsSerializesAsEmptyObject(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got := map[string]any{}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		tags, present := got["tags"]
		if !present {
			t.Errorf("tags must be sent when explicitly set to an empty map; body = %s", body)
		} else if m, ok := tags.(map[string]any); !ok || len(m) != 0 {
			t.Errorf("tags = %v; want empty object", tags)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructure]{Result: ByocInfrastructure{Id: "byoc-1"}})
	})

	emptyTags := map[string]string{}
	if _, err := client.UpdateByocInfrastructure(context.Background(), "byoc-1", ByocInfrastructureUpdateRequest{
		Tags: &emptyTags,
	}); err != nil {
		t.Fatalf("UpdateByocInfrastructure: %v", err)
	}
}

func TestDeleteByocInfrastructure_HappyPath(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q; want DELETE", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/byocInfrastructure/byoc-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 200})
	})

	if err := client.DeleteByocInfrastructure(context.Background(), "byoc-1"); err != nil {
		t.Fatalf("DeleteByocInfrastructure: %v", err)
	}
}

func TestGetByocInfrastructureTags_NilTagsYieldEmptyMap(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/org-1/byocInfrastructure/byoc-1/tags" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"result":{"tags":null}}`))
	})

	got, err := client.GetByocInfrastructureTags(context.Background(), "byoc-1")
	if err != nil {
		t.Fatalf("GetByocInfrastructureTags: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("tags = %v; want empty non-nil map", got)
	}
}

func TestGetByocInfrastructurePrivateEndpointConfig_HappyPath(t *testing.T) {
	want := ByocInfrastructurePrivateEndpointConfig{
		EndpointName:       "com.amazonaws.vpce.us-east-2.vpce-svc-0123",
		PrivateDnsHostname: "abc.vpce.example.byoc.clickhouse.cloud",
	}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/org-1/byocInfrastructure/byoc-1/privateEndpointConfig" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructurePrivateEndpointConfig]{Result: want})
	})

	got, err := client.GetByocInfrastructurePrivateEndpointConfig(context.Background(), "byoc-1")
	if err != nil {
		t.Fatalf("GetByocInfrastructurePrivateEndpointConfig: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetByocInfrastructurePrivateEndpointConfig mismatch (-want +got):\n%s", diff)
	}
}

func TestGetByocInfrastructureCustomerAuthorizedAccess_HappyPath(t *testing.T) {
	start := "2026-06-01T12:00:00.000Z"
	end := "2026-06-01T16:00:00.000Z"
	want := ByocInfrastructureCustomerAuthorizedAccess{
		Enabled:            true,
		DurationSeconds:    14400,
		StartTimestamp:     &start,
		Allowed:            true,
		WindowEndTimestamp: &end,
	}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/byocInfrastructure/byoc-1/customerAuthorizedAccess" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructureCustomerAuthorizedAccess]{Result: want})
	})

	got, err := client.GetByocInfrastructureCustomerAuthorizedAccess(context.Background(), "byoc-1")
	if err != nil {
		t.Fatalf("GetByocInfrastructureCustomerAuthorizedAccess: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetByocInfrastructureCustomerAuthorizedAccess mismatch (-want +got):\n%s", diff)
	}
}

func TestUpdateByocInfrastructureCustomerAuthorizedAccess_OmitsAbsentStart(t *testing.T) {
	want := ByocInfrastructureCustomerAuthorizedAccess{Enabled: false, Allowed: false}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q; want PATCH", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/byocInfrastructure/byoc-1/customerAuthorizedAccess" {
			t.Errorf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		got := map[string]any{}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("request body is not JSON: %v", err)
		}
		if got["enabled"] != false {
			t.Errorf("enabled = %v; want false", got["enabled"])
		}
		if got["durationSeconds"] != float64(0) {
			t.Errorf("durationSeconds = %v; want 0", got["durationSeconds"])
		}
		if _, present := got["startTimestamp"]; present {
			t.Errorf("startTimestamp should be omitted when unset; body = %s", body)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructureCustomerAuthorizedAccess]{Result: want})
	})

	got, err := client.UpdateByocInfrastructureCustomerAuthorizedAccess(context.Background(), "byoc-1", ByocInfrastructureCustomerAuthorizedAccessUpdateRequest{Enabled: false})
	if err != nil {
		t.Fatalf("UpdateByocInfrastructureCustomerAuthorizedAccess: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("UpdateByocInfrastructureCustomerAuthorizedAccess mismatch (-want +got):\n%s", diff)
	}
}

func TestWaitForByocInfrastructureState_WaitsUntilReady(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		state := ByocStateProvisioning
		if calls.Add(1) >= 2 {
			state = ByocStateReady
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ByocInfrastructureDetails]{
			Result: ByocInfrastructureDetails{Id: "byoc-1", State: state},
		})
	})

	err := client.WaitForByocInfrastructureState(context.Background(), "byoc-1", func(state string) bool {
		return state != ByocStateProvisioning
	}, 30)
	if err != nil {
		t.Fatalf("WaitForByocInfrastructureState: %v", err)
	}
	if calls.Load() < 2 {
		t.Errorf("calls = %d; want at least 2", calls.Load())
	}
}

func TestWaitForByocInfrastructureState_404IsTerminal(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"NOT_FOUND: BYOC infrastructure not found."}`))
	})

	err := client.WaitForByocInfrastructureState(context.Background(), "byoc-1", func(state string) bool {
		return state == ByocStateTerminated
	}, 60)
	if !IsNotFound(err) {
		t.Fatalf("err = %v; want IsNotFound", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d; want 1 (404 must not be retried)", calls.Load())
	}
}
