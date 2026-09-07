package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

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
// Note this does NOT exercise the 429 exclusion in is4xxPermanent: that
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

// A service can 404 for the first few polls right after create: the API is
// eventually consistent, so the id the create call just returned may not be
// visible to every read yet. With TolerateNotFound the wait must absorb that
// window rather than failing the whole create on the first poll.
func TestWaitForServiceState_toleratesNotFoundWithinLimit(t *testing.T) {
	var calls int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if n := atomic.AddInt32(&calls, 1); n <= 3 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"NOT_FOUND","status":404}`))
			return
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", State: StateRunning}})
	})

	err := client.WaitForServiceState(context.Background(), "svc-1", func(state string) bool { return state == StateRunning }, 60, TolerateNotFound(5), withPollInterval(time.Millisecond))
	if err != nil {
		t.Fatalf("WaitForServiceState: want nil after 3 tolerated 404s, got %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 4 {
		t.Errorf("server received %d calls; want 4 (3x 404 then success)", n)
	}
}

// The tolerance is bounded: past the replication window a 404 means the
// service really is gone, so the wait must give up rather than burn the full
// maxWaitSeconds budget.
func TestWaitForServiceState_failsOnceNotFoundToleranceExhausted(t *testing.T) {
	var calls int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"NOT_FOUND","status":404}`))
	})

	err := client.WaitForServiceState(context.Background(), "svc-1", func(state string) bool { return state == StateRunning }, 5, TolerateNotFound(2), withPollInterval(time.Millisecond))
	if err == nil {
		t.Fatal("WaitForServiceState: want error once the 404 tolerance is exhausted, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("error = %v; want it to still match IsNotFound so delete paths can detect it", err)
	}
	// 2 tolerated + 1 that trips the permanent branch.
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Errorf("server received %d calls; want exactly 3 (tolerance 2, then give up)", n)
	}
}

// Without the option the previous behaviour stands: a 404 is terminal on the
// first poll. Only the post-create wait opts into tolerating it.
func TestWaitForServiceState_notFoundIsTerminalByDefault(t *testing.T) {
	var calls int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"NOT_FOUND","status":404}`))
	})

	err := client.WaitForServiceState(context.Background(), "svc-1", func(state string) bool { return state == StateRunning }, 5)
	if err == nil {
		t.Fatal("WaitForServiceState: want error, got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("error = %v; want IsNotFound to still match after backoff unwraps the permanent error", err)
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("WaitForServiceState made %d HTTP calls on a 404; want exactly 1", n)
	}
}

// waitForServiceRunning is a thin wrapper over WaitForServiceState. It is what
// the ClickPipes and UDF paths use after waking a service, and it had no
// coverage of its own — pin both the success condition and the inherited
// fail-fast behaviour.
func TestWaitForServiceRunning(t *testing.T) {
	t.Run("returns once the service reports running", func(t *testing.T) {
		var calls int32
		client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			state := StateRunning
			if atomic.AddInt32(&calls, 1) == 1 {
				// partially_running is explicitly NOT sufficient here.
				state = "partially_running"
			}
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", State: state}})
		})

		if err := client.waitForServiceRunning(context.Background(), "svc-1", 60, withPollInterval(time.Millisecond)); err != nil {
			t.Fatalf("waitForServiceRunning: %v", err)
		}
		if n := atomic.LoadInt32(&calls); n != 2 {
			t.Errorf("server received %d calls; want 2 (partially_running, then running)", n)
		}
	})

	t.Run("fails fast on a 403 instead of polling out the budget", func(t *testing.T) {
		var calls int32
		client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"FORBIDDEN","status":403}`))
		})

		if err := client.waitForServiceRunning(context.Background(), "svc-1", 25); err == nil {
			t.Fatal("waitForServiceRunning: want error, got nil")
		}
		if n := atomic.LoadInt32(&calls); n != 1 {
			t.Errorf("waitForServiceRunning made %d HTTP calls on a 403; want exactly 1", n)
		}
	})
}

// DeleteService only ever needs the service's state, both to decide whether a
// stop is required and to detect that the deletion finished. Neither should
// drag in the private endpoint config, backup configuration or query endpoint
// sub-resources, which need permissions a delete-scoped token may not hold.
func TestDeleteService_pollsLightweightEndpointsOnly(t *testing.T) {
	var gets, deletes int32
	var deleted atomic.Bool
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/org-1/services/svc-1" {
			t.Errorf("unexpected request to %q; DeleteService should only touch the base service endpoint", r.URL.Path)
		}
		switch r.Method {
		case http.MethodDelete:
			atomic.AddInt32(&deletes, 1)
			deleted.Store(true)
			_ = json.NewEncoder(w).Encode(ResponseWithResult[ServiceResponseResult]{
				Result: ServiceResponseResult{Service: Service{Id: "svc-1", State: StateStopped}},
			})
		default:
			atomic.AddInt32(&gets, 1)
			// Reads before the DELETE report the service as already stopped,
			// so no stop command is needed; reads after it report it as gone.
			if deleted.Load() {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"NOT_FOUND","status":404}`))
				return
			}
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", State: StateStopped}})
		}
	})

	svc, err := client.DeleteService(context.Background(), "svc-1")
	if err != nil {
		t.Fatalf("DeleteService: %v", err)
	}
	if svc == nil || svc.Id != "svc-1" {
		t.Errorf("DeleteService returned %+v; want the deleted service", svc)
	}
	if n := atomic.LoadInt32(&deletes); n != 1 {
		t.Errorf("issued %d DELETEs; want exactly 1", n)
	}
	// Initial state check, the stopped-state wait, then the deletion confirmation.
	if n := atomic.LoadInt32(&gets); n != 3 {
		t.Errorf("issued %d GETs; want 3 (state check, stopped-state wait, deletion confirmation)", n)
	}
}

// A 4xx while waiting for the deletion to land will never resolve by retrying.
// It must surface immediately rather than being retried for the full 5-minute
// budget, which is what "not deleted yet" looks like.
func TestDeleteService_failsFastOnForbiddenWhileConfirmingDeletion(t *testing.T) {
	var getsAfterDelete int32
	var deleted bool
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
			_ = json.NewEncoder(w).Encode(ResponseWithResult[ServiceResponseResult]{
				Result: ServiceResponseResult{Service: Service{Id: "svc-1", State: StateStopped}},
			})
			return
		}
		if !deleted {
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: "svc-1", State: StateStopped}})
			return
		}
		atomic.AddInt32(&getsAfterDelete, 1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"FORBIDDEN","status":403}`))
	})

	if _, err := client.DeleteService(context.Background(), "svc-1"); err == nil {
		t.Fatal("DeleteService: want error when the deletion check is forbidden, got nil")
	}
	if n := atomic.LoadInt32(&getsAfterDelete); n != 1 {
		t.Errorf("polled %d times after the DELETE; want exactly 1 (no retrying a 403)", n)
	}
}
