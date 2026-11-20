package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const testPostgresPrivateEndpointPath = testPostgresInstancePath + "/privateEndpoint"

func writePostgresPrivateEndpoint(t *testing.T, w http.ResponseWriter, endpoint PostgresPrivateEndpoint) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(ResponseWithResult[PostgresPrivateEndpoint]{Result: endpoint}); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

func TestPostgresPrivateEndpoint_Requests(t *testing.T) {
	want := PostgresPrivateEndpoint{
		State:               PostgresPrivateEndpointStateAvailable,
		EndpointServiceName: "com.amazonaws.vpce.us-east-1.vpce-svc-0123456789abcdef0",
		PrivateHostname:     "pg.private.example.com",
		SupportedRegions:    []string{"us-west-2"},
		AllowedVpcEndpoints: []PostgresAllowedVpcEndpoint{{VpcEndpointId: "vpce-0123456789abcdef0", Description: "app"}},
	}
	var requests []string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testPostgresPrivateEndpointPath {
			t.Errorf("path = %q; want %q", r.URL.Path, testPostgresPrivateEndpointPath)
		}
		assertBasicAuth(t, r)
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, r.Method+" "+string(body))
		writePostgresPrivateEndpoint(t, w, want)
	})
	ctx := context.Background()
	none := []PostgresAllowedVpcEndpoint{}
	regions := []string{"us-west-2"}

	for name, call := range map[string]func() (*PostgresPrivateEndpoint, error){
		"get": func() (*PostgresPrivateEndpoint, error) {
			return client.GetPostgresPrivateEndpoint(ctx, testPostgresID)
		},
		"create": func() (*PostgresPrivateEndpoint, error) {
			return client.CreatePostgresPrivateEndpoint(ctx, testPostgresID, PostgresPrivateEndpointCreate{})
		},
		"create with regions": func() (*PostgresPrivateEndpoint, error) {
			return client.CreatePostgresPrivateEndpoint(ctx, testPostgresID, PostgresPrivateEndpointCreate{SupportedRegions: regions})
		},
		"clear endpoints": func() (*PostgresPrivateEndpoint, error) {
			return client.UpdatePostgresPrivateEndpoint(ctx, testPostgresID, PostgresPrivateEndpointUpdate{AllowedVpcEndpoints: &none})
		},
		"set regions": func() (*PostgresPrivateEndpoint, error) {
			return client.UpdatePostgresPrivateEndpoint(ctx, testPostgresID, PostgresPrivateEndpointUpdate{SupportedRegions: &regions})
		},
	} {
		got, err := call()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if diff := cmp.Diff(want, *got); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", name, diff)
		}
	}

	for _, want := range []string{
		"GET ",
		"POST {}",
		`POST {"supportedRegions":["us-west-2"]}`,
		`PATCH {"allowedVpcEndpoints":[]}`,
		`PATCH {"supportedRegions":["us-west-2"]}`,
	} {
		if !slices.Contains(requests, want) {
			t.Errorf("no request %q in %q", want, requests)
		}
	}
}

func TestDeletePostgresPrivateEndpoint(t *testing.T) {
	for name, tc := range map[string]struct {
		status  int
		wantErr bool
	}{
		"deleted":         {status: http.StatusOK},
		"already deleted": {status: http.StatusNotFound},
		"request error":   {status: http.StatusBadRequest, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete || r.URL.Path != testPostgresPrivateEndpointPath {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{}`))
			})
			err := client.DeletePostgresPrivateEndpoint(context.Background(), testPostgresID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v; wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestWaitForPostgresPrivateEndpoint(t *testing.T) {
	available := func(e *PostgresPrivateEndpoint) bool {
		return e != nil && e.State == PostgresPrivateEndpointStateAvailable && e.EndpointServiceName != ""
	}
	gone := func(e *PostgresPrivateEndpoint) bool { return e == nil }

	for name, tc := range map[string]struct {
		responses []int // 200 = creating, 201 = available, other = that status
		done      func(*PostgresPrivateEndpoint) bool
		wantErr   string
		wantCalls int32
	}{
		"waits until available":       {responses: []int{200, 200, 201}, done: available, wantCalls: 3},
		"waits until gone":            {responses: []int{200, 404}, done: gone, wantCalls: 2},
		"retries a 404 while waiting": {responses: []int{404, 201}, done: available, wantCalls: 2},
		"reports the last state":      {responses: []int{200, 200, 200}, done: available, wantErr: "last seen state: creating", wantCalls: 3},
		"fails fast on a 403":         {responses: []int{403, 201}, done: available, wantErr: "status: 403", wantCalls: 1},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				i := int(calls.Add(1)) - 1
				status := tc.responses[min(i, len(tc.responses)-1)]
				switch status {
				case 200:
					writePostgresPrivateEndpoint(t, w, PostgresPrivateEndpoint{State: "creating"})
				case 201:
					writePostgresPrivateEndpoint(t, w, PostgresPrivateEndpoint{State: "available", EndpointServiceName: "svc"})
				default:
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"error":"x"}`))
				}
			})
			err := client.waitForPostgresPrivateEndpointWithInterval(context.Background(), testPostgresID, tc.done, time.Millisecond, uint64(len(tc.responses)-1))
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("err = %v; want it to contain %q", err, tc.wantErr)
			}
			if got := calls.Load(); got != tc.wantCalls {
				t.Errorf("calls = %d; want %d", got, tc.wantCalls)
			}
		})
	}
}
