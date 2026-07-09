package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

const (
	testServiceID = "svc-1"
	// getClickPipePath appends the (empty) pipe ID after the slash on create.
	testClickPipesPath      = "/organizations/org-1/services/svc-1/clickpipes/"
	testServiceInstancePath = "/organizations/org-1/services/svc-1"
	testServiceStatePath    = "/organizations/org-1/services/svc-1/state"
	idle424BodyFormat       = `{"requestId":"x","error":"FAILED_DEPENDENCY: ClickPipe creation is allowed only when the ClickHouse service is running. Current state: %s","status":424}`
)

// ----- doClickPipeRequest idle-service wake (issue #376) --------------------

func TestCreateClickPipe_WakesIdleServiceAndRetries(t *testing.T) {
	var createCalls, wakeCalls, stateGetCalls int

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == testClickPipesPath:
			createCalls++
			var gotPipe ClickPipe
			if err := json.NewDecoder(r.Body).Decode(&gotPipe); err != nil {
				t.Errorf("decoding create body (call %d): %v", createCalls, err)
			}
			if gotPipe.Name != "my-pipe" {
				t.Errorf("create body (call %d) name = %q; want my-pipe (body must be replayed on retry)", createCalls, gotPipe.Name)
			}
			if wakeCalls == 0 {
				w.WriteHeader(http.StatusFailedDependency)
				fmt.Fprintf(w, idle424BodyFormat, "idle")
				return
			}
			_ = json.NewEncoder(w).Encode(ResponseWithResult[ClickPipe]{Result: ClickPipe{ID: "pipe-1", Name: "my-pipe", State: ClickPipeProvisioningState}})

		case r.Method == http.MethodPatch && r.URL.Path == testServiceStatePath:
			wakeCalls++
			var stateUpdate ServiceStateUpdate
			if err := json.NewDecoder(r.Body).Decode(&stateUpdate); err != nil {
				t.Errorf("decoding state update body: %v", err)
			}
			if stateUpdate.Command != "awake" {
				t.Errorf("state command = %q; want awake", stateUpdate.Command)
			}
			_, _ = w.Write([]byte(`{}`))

		case r.Method == http.MethodGet && r.URL.Path == testServiceInstancePath:
			stateGetCalls++
			state := "idle"
			if wakeCalls > 0 {
				state = StateRunning
			}
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: testServiceID, State: state}})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	pipe, err := client.CreateClickPipe(context.Background(), testServiceID, ClickPipe{Name: "my-pipe"})
	if err != nil {
		t.Fatalf("CreateClickPipe: %v", err)
	}
	if pipe.ID != "pipe-1" {
		t.Errorf("pipe ID = %q; want pipe-1", pipe.ID)
	}
	if createCalls != 2 {
		t.Errorf("create calls = %d; want 2 (initial 424 + retry after wake)", createCalls)
	}
	if wakeCalls != 1 {
		t.Errorf("wake calls = %d; want 1", wakeCalls)
	}
	if stateGetCalls < 1 {
		t.Errorf("service state polls = %d; want at least 1", stateGetCalls)
	}
}

func TestCreateClickPipe_StoppedService424_DoesNotWake(t *testing.T) {
	var createCalls int

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == testClickPipesPath:
			createCalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusFailedDependency)
			fmt.Fprintf(w, idle424BodyFormat, "stopped")

		default:
			// A stopped service was stopped deliberately: the provider must
			// neither wake it nor touch any other endpoint.
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, err := client.CreateClickPipe(context.Background(), testServiceID, ClickPipe{Name: "my-pipe"})
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !strings.HasPrefix(err.Error(), "status: 424") {
		t.Errorf("error = %v; want the original 424 passed through", err)
	}
	if createCalls != 1 {
		t.Errorf("create calls = %d; want 1 (no retry)", createCalls)
	}
}

func TestCreateClickPipe_WakeFails_ReturnsError(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == testClickPipesPath:
			w.WriteHeader(http.StatusFailedDependency)
			fmt.Fprintf(w, idle424BodyFormat, "idle")

		case r.Method == http.MethodPatch && r.URL.Path == testServiceStatePath:
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	_, err := client.CreateClickPipe(context.Background(), testServiceID, ClickPipe{Name: "my-pipe"})
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !strings.Contains(err.Error(), "waking it up failed") {
		t.Errorf("error = %v; want wake failure to be surfaced", err)
	}
}
