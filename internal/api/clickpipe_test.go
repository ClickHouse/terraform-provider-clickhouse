package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGetClickPipesServiceContext(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/organizations/org-1/services/svc-1/clickpipes/context" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[ClickPipesServiceContext]{
			Result: ClickPipesServiceContext{GCPWorkloadIdentity: ClickPipesGCPWorkloadIdentityContext{
				Supported: true,
				Ready:     boolPtr(true),
				Principal: strPtr("tenant@clickpipes.iam.gserviceaccount.com"),
			}},
		})
	})

	got, err := client.GetClickPipesServiceContext(context.Background(), testServiceID)
	if err != nil {
		t.Fatalf("GetClickPipesServiceContext: %v", err)
	}
	if !got.GCPWorkloadIdentity.Supported || got.GCPWorkloadIdentity.Ready == nil || !*got.GCPWorkloadIdentity.Ready {
		t.Fatalf("unexpected workload identity context: %+v", got.GCPWorkloadIdentity)
	}
	if got.GCPWorkloadIdentity.Principal == nil || *got.GCPWorkloadIdentity.Principal != "tenant@clickpipes.iam.gserviceaccount.com" {
		t.Errorf("principal = %v", got.GCPWorkloadIdentity.Principal)
	}
}

func TestWaitForClickPipesGCPWorkloadIdentity(t *testing.T) {
	t.Run("returns ready identity", func(t *testing.T) {
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"result":{"gcpWorkloadIdentity":{"supported":true,"ready":true,"principal":"tenant@example.com"}}}`))
		})

		got, err := client.WaitForClickPipesGCPWorkloadIdentity(context.Background(), testServiceID, time.Second)
		if err != nil {
			t.Fatalf("WaitForClickPipesGCPWorkloadIdentity: %v", err)
		}
		if got.Principal == nil || *got.Principal != "tenant@example.com" {
			t.Errorf("principal = %v", got.Principal)
		}
	})

	t.Run("rejects unsupported identity without waiting", func(t *testing.T) {
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"result":{"gcpWorkloadIdentity":{"supported":false}}}`))
		})

		_, err := client.WaitForClickPipesGCPWorkloadIdentity(context.Background(), testServiceID, time.Second)
		if err == nil || !strings.Contains(err.Error(), "not supported") {
			t.Fatalf("error = %v; want unsupported error", err)
		}
	})

	t.Run("times out while identity is not ready", func(t *testing.T) {
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"result":{"gcpWorkloadIdentity":{"supported":true,"ready":false}}}`))
		})

		_, err := client.WaitForClickPipesGCPWorkloadIdentity(context.Background(), testServiceID, 10*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "timed out after 10ms") {
			t.Fatalf("error = %v; want timeout error", err)
		}
	})
}

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
			if stateUpdate.Command != serviceStateCommandAwake {
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

		case r.Method == http.MethodGet && r.URL.Path == testServiceInstancePath:
			// The service is still idle, so nobody else woke it up.
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: testServiceID, State: StateIdle}})

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

func TestCreateClickPipe_ContinuesWhenAnotherWakeWon(t *testing.T) {
	var createCalls, wakeCalls, stateGetCalls int

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == testClickPipesPath:
			createCalls++
			if wakeCalls == 0 {
				w.WriteHeader(http.StatusFailedDependency)
				fmt.Fprintf(w, idle424BodyFormat, StateIdle)
				return
			}
			_ = json.NewEncoder(w).Encode(ResponseWithResult[ClickPipe]{Result: ClickPipe{ID: "pipe-1", Name: "my-pipe", State: ClickPipeProvisioningState}})

		case r.Method == http.MethodPatch && r.URL.Path == testServiceStatePath:
			wakeCalls++
			// Another pipe for the same service already woke it up, so this
			// request's own wake attempt loses the race.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"requestId":"x","error":"BAD_REQUEST: Service must be in idle state to awake","status":400}`))

		case r.Method == http.MethodGet && r.URL.Path == testServiceInstancePath:
			stateGetCalls++
			state := StateRunning
			if stateGetCalls == 1 {
				state = StateAwaking
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
		t.Errorf("create calls = %d; want 2", createCalls)
	}
	if wakeCalls != 1 {
		t.Errorf("wake calls = %d; want 1", wakeCalls)
	}
	if stateGetCalls < 2 {
		t.Errorf("service state polls = %d; want at least 2 (race check + wait)", stateGetCalls)
	}
}

func TestCreateClickPipe_WaitsForAwakingServiceAndRetries(t *testing.T) {
	var createCalls, stateGetCalls int

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == testClickPipesPath:
			createCalls++
			if createCalls == 1 {
				w.WriteHeader(http.StatusFailedDependency)
				fmt.Fprintf(w, idle424BodyFormat, StateAwaking)
				return
			}
			_ = json.NewEncoder(w).Encode(ResponseWithResult[ClickPipe]{Result: ClickPipe{ID: "pipe-1", Name: "my-pipe", State: ClickPipeProvisioningState}})

		case r.Method == http.MethodGet && r.URL.Path == testServiceInstancePath:
			stateGetCalls++
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: testServiceID, State: StateRunning}})

		case r.Method == http.MethodPatch && r.URL.Path == testServiceStatePath:
			t.Error("already-awaking service must not receive another awake command")
			w.WriteHeader(http.StatusInternalServerError)

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
		t.Errorf("create calls = %d; want 2", createCalls)
	}
	if stateGetCalls < 1 {
		t.Errorf("service state polls = %d; want at least 1", stateGetCalls)
	}
}

func TestCreateClickPipe_ConcurrentCreatesOnIdleService(t *testing.T) {
	const pipeCount = 3

	var (
		mu           sync.Mutex
		serviceState = StateIdle
		wakeCalls    int
	)

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == testClickPipesPath:
			if serviceState != StateRunning {
				w.WriteHeader(http.StatusFailedDependency)
				fmt.Fprintf(w, idle424BodyFormat, serviceState)
				return
			}
			var gotPipe ClickPipe
			_ = json.NewDecoder(r.Body).Decode(&gotPipe)
			_ = json.NewEncoder(w).Encode(ResponseWithResult[ClickPipe]{Result: ClickPipe{ID: gotPipe.Name, Name: gotPipe.Name, State: ClickPipeProvisioningState}})

		case r.Method == http.MethodPatch && r.URL.Path == testServiceStatePath:
			wakeCalls++
			if serviceState != StateIdle {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"requestId":"x","error":"BAD_REQUEST: Service must be in idle state to awake","status":400}`))
				return
			}
			serviceState = StateAwaking
			_, _ = w.Write([]byte(`{}`))

		case r.Method == http.MethodGet && r.URL.Path == testServiceInstancePath:
			state := serviceState
			// The service finishes waking up once it has been polled while awaking.
			if serviceState == StateAwaking {
				serviceState = StateRunning
			}
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Service]{Result: Service{Id: testServiceID, State: state}})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	errs := make([]error, pipeCount)
	var wg sync.WaitGroup
	for i := range pipeCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = client.CreateClickPipe(context.Background(), testServiceID, ClickPipe{Name: fmt.Sprintf("pipe-%d", i)})
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("CreateClickPipe(pipe-%d): %v", i, err)
		}
	}
	if wakeCalls < 1 {
		t.Errorf("wake calls = %d; want at least 1", wakeCalls)
	}
}

func TestUpdateClickPipeSettings_PreservesKafkaReadCommittedBoolean(t *testing.T) {
	for _, value := range []bool{true, false} {
		t.Run(fmt.Sprintf("%t", value), func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPut || r.URL.Path != testClickPipesPath+"pipe-1/settings" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}

				var settings map[string]any
				if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
					t.Fatalf("decoding settings body: %v", err)
				}
				if got, ok := settings["kafka_read_committed"].(bool); !ok || got != value {
					t.Errorf("kafka_read_committed = %#v; want boolean %t", settings["kafka_read_committed"], value)
				}

				_ = json.NewEncoder(w).Encode(ResponseWithResult[map[string]any]{Result: settings})
			})

			settings, err := client.UpdateClickPipeSettings(
				context.Background(),
				testServiceID,
				"pipe-1",
				map[string]any{"kafka_read_committed": value},
			)
			if err != nil {
				t.Fatalf("UpdateClickPipeSettings: %v", err)
			}
			if got, ok := settings["kafka_read_committed"].(bool); !ok || got != value {
				t.Errorf("response kafka_read_committed = %#v; want boolean %t", settings["kafka_read_committed"], value)
			}
		})
	}
}
