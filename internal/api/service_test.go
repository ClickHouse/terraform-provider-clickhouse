package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// A protected service must not be stopped by a delete that cannot succeed: DeleteService returns before any write.
func TestDeleteService_protectedServiceFailsBeforeStopping(t *testing.T) {
	var writes int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			atomic.AddInt32(&writes, 1)
			t.Errorf("unexpected %s %s; a protected service must not be stopped or deleted", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/organizations/org-1/services/svc-1":
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", State: StateRunning, DeletionProtection: boolPtr(true)}})
		case "/organizations/org-1/services/svc-1/privateEndpointConfig":
			_, _ = w.Write([]byte(`{"result":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, err := client.DeleteService(context.Background(), "svc-1")
	if !errors.Is(err, ErrServiceDeletionProtected) {
		t.Fatalf("DeleteService error = %v; want ErrServiceDeletionProtected", err)
	}
	if errors.Is(err, ErrDeleteRefusedAfterStop) {
		t.Errorf("DeleteService error = %v; must not say the service was stopped", err)
	}
	if n := atomic.LoadInt32(&writes); n != 0 {
		t.Errorf("DeleteService sent %d write requests; want 0", n)
	}
}

// The protection 409 never clears by waiting, unlike the 409 for a service still stopping, so it is not retried.
func TestDeleteService_protection409IsNotRetried(t *testing.T) {
	var deletes int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			atomic.AddInt32(&deletes, 1)
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"CONFLICT: Deletion protection is enabled for this service. Turn it off before deleting the service.","status":409}`))
		case r.URL.Path == "/organizations/org-1/services/svc-1":
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", State: StateStopped}})
		case r.URL.Path == "/organizations/org-1/services/svc-1/privateEndpointConfig":
			_, _ = w.Write([]byte(`{"result":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, err := client.DeleteService(context.Background(), "svc-1")
	if !errors.Is(err, ErrServiceDeletionProtected) {
		t.Fatalf("DeleteService error = %v; want ErrServiceDeletionProtected", err)
	}
	if errors.Is(err, ErrDeleteRefusedAfterStop) {
		t.Errorf("DeleteService error = %v; the service was already stopped, so it must not say DeleteService stopped it", err)
	}
	if !strings.Contains(err.Error(), "CONFLICT: Deletion protection is enabled for this service") {
		t.Errorf("DeleteService error = %v; want it to carry the API's 409 message", err)
	}
	if n := atomic.LoadInt32(&deletes); n != 1 {
		t.Errorf("DeleteService sent %d DELETE requests; want exactly 1 (no retries)", n)
	}
}

type deleteFlowCalls struct{ stops, deletes int32 }

// newDeleteFlowClient serves a running service that stops on the stop PATCH and answers each DELETE with deleteStatus and deleteBody.
func newDeleteFlowClient(t *testing.T, protection *bool, deleteStatus int, deleteBody string) (*ClientImpl, *deleteFlowCalls) {
	t.Helper()
	calls := &deleteFlowCalls{}
	var stopped, deleted atomic.Bool
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/organizations/org-1/services/svc-1/state":
			atomic.AddInt32(&calls.stops, 1)
			stopped.Store(true)
			_, _ = w.Write([]byte(`{"result":{}}`))
		case r.Method == http.MethodDelete:
			atomic.AddInt32(&calls.deletes, 1)
			if deleteStatus < 300 {
				deleted.Store(true)
			}
			w.WriteHeader(deleteStatus)
			_, _ = w.Write([]byte(deleteBody))
		case deleted.Load():
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/organizations/org-1/services/svc-1":
			state := StateRunning
			if stopped.Load() {
				state = StateStopped
			}
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", State: state, DeletionProtection: protection}})
		case r.URL.Path == "/organizations/org-1/services/svc-1/privateEndpointConfig":
			_, _ = w.Write([]byte(`{"result":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return client, calls
}

// Flagged organizations report deletionProtection=false on every unprotected service; that must not block the delete.
func TestDeleteService_unprotectedServiceIsStoppedAndDeleted(t *testing.T) {
	client, calls := newDeleteFlowClient(t, boolPtr(false), http.StatusOK, `{"result":{}}`)

	if _, err := client.DeleteService(context.Background(), "svc-1"); err != nil {
		t.Fatalf("DeleteService error = %v; want nil", err)
	}
	if n := atomic.LoadInt32(&calls.stops); n != 1 {
		t.Errorf("DeleteService sent %d stop requests; want 1", n)
	}
	if n := atomic.LoadInt32(&calls.deletes); n != 1 {
		t.Errorf("DeleteService sent %d DELETE requests; want 1", n)
	}
}

// Protection turned on after the GET: DeleteService has already stopped the service, and the error says so.
func TestDeleteService_protection409AfterStopSaysStopped(t *testing.T) {
	client, calls := newDeleteFlowClient(t, nil, http.StatusConflict,
		`{"error":"CONFLICT: Deletion protection is enabled for this service. Turn it off before deleting the service.","status":409}`)

	_, err := client.DeleteService(context.Background(), "svc-1")
	if !errors.Is(err, ErrServiceDeletionProtected) {
		t.Fatalf("DeleteService error = %v; want ErrServiceDeletionProtected", err)
	}
	if !errors.Is(err, ErrDeleteRefusedAfterStop) {
		t.Errorf("DeleteService error = %v; want ErrDeleteRefusedAfterStop", err)
	}
	if n := atomic.LoadInt32(&calls.stops); n != 1 {
		t.Errorf("DeleteService sent %d stop requests; want 1", n)
	}
	if n := atomic.LoadInt32(&calls.deletes); n != 1 {
		t.Errorf("DeleteService sent %d DELETE requests; want exactly 1 (no retries)", n)
	}
}

// A horizontal create must pass the mode, the replica band, and the fixed (min == max) per-replica memory
// through on POST /services, and must not fabricate any vertical-only fields (num_replicas or the deprecated
// totals — FixMemoryBounds should be a no-op when the band is present).
func TestCreateService_horizontalBand(t *testing.T) {
	var gotBody map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"result":{"service":{"id":"svc-1","name":"svc"},"password":"p"}}`))
	})

	_, _, err := client.CreateService(context.Background(), Service{
		Name:               "svc",
		Provider:           "aws",
		Region:             "us-east-1",
		Tier:               TierPPv2,
		AutoscalingMode:    "horizontal",
		MinReplicas:        intPtr(2),
		MaxReplicas:        intPtr(6),
		MinReplicaMemoryGb: intPtr(16),
		MaxReplicaMemoryGb: intPtr(16),
	})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	if gotBody["autoscalingMode"] != "horizontal" ||
		gotBody["minReplicas"] != float64(2) || gotBody["maxReplicas"] != float64(6) ||
		gotBody["minReplicaMemoryGb"] != float64(16) || gotBody["maxReplicaMemoryGb"] != float64(16) {
		t.Errorf("POST body missing the horizontal mode/band/memory: %v", gotBody)
	}
	// Horizontal must not carry a fixed replica count or the deprecated totals — FixMemoryBounds is a no-op here.
	for _, k := range []string{"numReplicas", "minTotalMemoryGb", "maxTotalMemoryGb"} {
		if _, present := gotBody[k]; present {
			t.Errorf("POST body unexpectedly carried vertical-only field %q for a horizontal create: %v", k, gotBody)
		}
	}
}

func TestListServices_HappyPath_NoFilter(t *testing.T) {
	want := []Service{
		{Id: "svc-1", Name: "one", Provider: "aws", Region: "us-east-1", State: "running", ClickHouseVersion: "24.5", CreatedAt: "t1"},
		{Id: "svc-2", Name: "two", Provider: "gcp", Region: "us-central1", State: "idle", ClickHouseVersion: "24.5", CreatedAt: "t2"},
	}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/services" {
			t.Errorf("path = %q; want /organizations/org-1/services", r.URL.Path)
		}
		if got := r.URL.Query()["filter"]; len(got) != 0 {
			t.Errorf("filter = %v; want none", got)
		}
		assertBasicAuth(t, r)
		_ = json.NewEncoder(w).Encode(ResponseWithResult[[]Service]{Result: want})
	})

	got, err := client.ListServices(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListServices mismatch (-want +got):\n%s", diff)
	}
}

func TestListServices_SendsTagFilters(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query()["filter"]
		sort.Strings(got)
		want := []string{"tag:Env=prod", "tag:Team=data"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("filter params mismatch (-want +got):\n%s", diff)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[[]Service]{Result: []Service{}})
	})

	_, err := client.ListServices(context.Background(), []string{"tag:Env=prod", "tag:Team=data"})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
}

func TestListServices_APIError(t *testing.T) {
	// Use a non-5xx status: doRequest retries 5xx with backoff (up to
	// MaxElapsedTime), which would blow the test timeout. A 403 surfaces the
	// error immediately, which is what we want to assert.
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	})
	if _, err := client.ListServices(context.Background(), nil); err == nil {
		t.Fatal("expected error; got nil")
	}
}

// GetServiceBase must fetch only the core service object with a single request —
// no private-endpoint-config, backup, or query-endpoint enrichment calls (unlike
// GetService).
func TestGetServiceBase_SingleRequestNoEnrichment(t *testing.T) {
	var calls int32
	want := Service{Id: "svc-1", Name: "svc", Provider: "aws", Region: "us-east-1", State: "running"}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != "/organizations/org-1/services/svc-1" {
			t.Errorf("path = %q; want /organizations/org-1/services/svc-1", r.URL.Path)
		}
		assertBasicAuth(t, r)
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: want})
	})

	got, err := client.GetServiceBase(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("GetServiceBase: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetServiceBase mismatch (-want +got):\n%s", diff)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("GetServiceBase made %d HTTP calls; want exactly 1 (no enrichment)", n)
	}
}

// WaitForServiceState only needs the service's state field, so it must poll
// the lightweight base endpoint and never fan out to the private endpoint
// config, backup configuration, or query endpoints sub-resources — those
// require extra permissions that a caller polling right after creating a
// service may not have.
func TestWaitForServiceState_pollsLightweightEndpointOnly(t *testing.T) {
	var calls int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/organizations/org-1/services/svc-1" {
			t.Errorf("unexpected request to %q; WaitForServiceState should only poll the base service endpoint", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", State: StateRunning}})
	})

	err := client.WaitForServiceState(context.Background(), "svc-1", func(state string) bool { return state == StateRunning }, 10)
	if err != nil {
		t.Fatalf("WaitForServiceState: %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("WaitForServiceState made %d HTTP calls; want exactly 1", n)
	}
}

// A 4xx (e.g. a permission the caller doesn't have) can never resolve by
// retrying. WaitForServiceState must fail immediately instead of retrying it
// for the full maxWaitSeconds, indistinguishable from the service genuinely
// still being in its starting state.
func TestWaitForServiceState_failsFastOn403(t *testing.T) {
	var calls int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"FORBIDDEN","status":403}`))
	})

	err := client.WaitForServiceState(context.Background(), "svc-1", func(state string) bool { return state == StateRunning }, 25)
	if err == nil {
		t.Fatal("WaitForServiceState: want error, got nil")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("WaitForServiceState made %d HTTP calls on a 403; want exactly 1 (no retries)", n)
	}
}

// Transient 429s are absorbed by doRequest's own retry (within its
// 61-second budget) and never reach the poll loop's error handling, so the
// wait still succeeds rather than failing fast the way a 403 does.
//
// Note this does NOT exercise the 429 exclusion in Is4xxPermanent: that
// only matters for 429s sustained past doRequest's MaxElapsedTime, which
// isn't reachable here without making that budget injectable.
// TestIs4xxPermanent is what guards the exclusion itself.
func TestWaitForServiceState_transient429IsAbsorbedByDoRequest(t *testing.T) {
	var calls int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			// No X-RateLimit-Reset header: doRequest's own retry falls back
			// to an immediate retry (resetSeconds defaults to 0), so this
			// test resolves quickly rather than waiting on real rate-limit
			// timing.
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"TOO_MANY_REQUESTS","status":429}`))
			return
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", State: StateRunning}})
	})

	err := client.WaitForServiceState(context.Background(), "svc-1", func(state string) bool { return state == StateRunning }, 25)
	if err != nil {
		t.Fatalf("WaitForServiceState: want nil error after transient 429s, got %v", err)
	}
	if n := atomic.LoadInt32(&calls); n < 3 {
		t.Errorf("server received %d calls; want at least 3 (2x 429 then success)", n)
	}
}

// A cancelled context must stop the poll loop immediately rather than running
// out the full maxWaitSeconds budget. Without backoff.WithContext the retry
// loop ignores cancellation entirely.
func TestWaitForServiceState_stopsOnContextCancel(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{
			Result: Service{Id: "svc-1", State: StateProvisioning},
		})
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := client.WaitForServiceState(ctx, "svc-1", func(s string) bool { return s == StateRunning }, 60)
	if err == nil {
		t.Fatal("want error from cancelled context, got nil")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %s; want an immediate return on a cancelled context", elapsed)
	}
}
