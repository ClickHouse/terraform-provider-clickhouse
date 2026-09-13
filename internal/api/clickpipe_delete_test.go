package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDeleteClickPipe(t *testing.T) {
	for _, tc := range []struct {
		name         string
		deleteStatus int
		getStatuses  []int
		wantError    string
	}{
		{name: "waits for absence", deleteStatus: 200, getStatuses: []int{200, 200, 404}},
		{name: "already absent", deleteStatus: 404},
		{name: "delete forbidden", deleteStatus: 403, wantError: "status: 403"},
		{name: "poll unauthorized", deleteStatus: 200, getStatuses: []int{401}, wantError: "status: 401"},
		{name: "poll forbidden", deleteStatus: 200, getStatuses: []int{403}, wantError: "status: 403"},
		{name: "poll server failure", deleteStatus: 200, getStatuses: []int{503, 200, 404}},
		{name: "poll throttled", deleteStatus: 200, getStatuses: []int{429, 404}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var deletes, gets int
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != testClickPipesPath+"pipe-1" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				switch r.Method {
				case http.MethodDelete:
					deletes++
					w.WriteHeader(tc.deleteStatus)
				case http.MethodGet:
					gets++
					if gets > len(tc.getStatuses) {
						t.Error("unexpected extra GET")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					w.WriteHeader(tc.getStatuses[gets-1])
					state := "Deleting"
					if gets == 1 {
						state = "Running"
					}
					_, _ = w.Write([]byte(`{"result":{"id":"pipe-1","state":"` + state + `"}}`))
				default:
					t.Errorf("unexpected method: %s", r.Method)
				}
			})
			err := client.deleteClickPipeWithBudget(t.Context(), testServiceID, "pipe-1", 20*time.Second, time.Millisecond)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("delete: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v; want %s", err, tc.wantError)
			}
			if deletes != 1 || gets != len(tc.getStatuses) {
				t.Fatalf("DELETE calls = %d, GET calls = %d; want 1, %d", deletes, gets, len(tc.getStatuses))
			}
		})
	}
}

func TestDeleteClickPipeDeadline(t *testing.T) {
	for _, phase := range []string{"pending", "delete retry", "poll retry", "throttled", "in flight"} {
		t.Run(phase, func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if phase == "delete retry" || (phase == "poll retry" && r.Method == http.MethodGet) {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if phase == "throttled" {
					w.Header().Set(ResponseHeaderRateLimitReset, "120")
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				if phase == "in flight" {
					<-r.Context().Done()
					return
				}
				_, _ = w.Write([]byte(`{"result":{"state":"Deleting"}}`))
			})
			start := time.Now()
			err := client.deleteClickPipeWithBudget(t.Context(), testServiceID, "pipe-1", 100*time.Millisecond, time.Millisecond)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error = %v; want deadline exceeded", err)
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("deadline was not respected: %v", elapsed)
			}
		})
	}
}

func TestDeleteClickPipeCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			cancel()
		}
		_, _ = w.Write([]byte(`{"result":{"state":"Deleting"}}`))
	})
	err := client.DeleteClickPipe(ctx, testServiceID, "pipe-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v; want canceled", err)
	}
}

func TestDeleteClickPipeRetriesDelete(t *testing.T) {
	var deletes int
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes++
			if deletes == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	if err := client.DeleteClickPipe(t.Context(), testServiceID, "pipe-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deletes != 2 {
		t.Fatalf("DELETE calls = %d; want 2", deletes)
	}
}

func TestDeleteClickPipeMalformedResponse(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not JSON`))
	})
	err := client.DeleteClickPipe(t.Context(), testServiceID, "pipe-1")
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("error = %v; want invalid JSON error", err)
	}
}
