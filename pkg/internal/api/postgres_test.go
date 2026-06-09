package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const (
	testOrgID                = "org-1"
	testPostgresID           = "pg-1"
	testPostgresInstancePath = "/organizations/org-1/postgres/pg-1"
	testPostgresListPath     = "/organizations/org-1/postgres"
	testPostgresSizeXLarge   = "r6gd.xlarge"
)

// newPostgresTestClient spins up an httptest.Server with the given handler
// and returns a *ClientImpl pointed at it. Mirrors newScheduledScalingTestClient.
func newPostgresTestClient(t *testing.T, handler http.HandlerFunc) (*ClientImpl, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(ClientConfig{
		ApiURL:         server.URL,
		OrganizationID: testOrgID,
		TokenKey:       "key",
		TokenSecret:    "secret",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client, server
}

func assertBasicAuth(t *testing.T, r *http.Request) {
	t.Helper()
	user, pass, ok := r.BasicAuth()
	if !ok {
		t.Errorf("expected basic auth; none present")
		return
	}
	if user != "key" || pass != "secret" {
		t.Errorf("basic auth = %q:%q; want key:secret", user, pass)
	}
}

// ----- GetPostgres ---------------------------------------------------------

func TestGetPostgres_HappyPath(t *testing.T) {
	expectedPath := testPostgresInstancePath
	want := Postgres{
		Id:        "pg-1",
		Name:      "my-pg",
		Provider:  "aws",
		Region:    "us-east-1",
		Size:      "r6gd.large",
		State:     PostgresStateRunning,
		CreatedAt: "2026-05-20T00:00:00Z",
		IsPrimary: true,
		Hostname:  "my-pg.example.com",
	}

	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		assertBasicAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: want})
	})

	got, err := client.GetPostgres(context.Background(), testPostgresID)
	if err != nil {
		t.Fatalf("GetPostgres: %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Errorf("GetPostgres mismatch (-want +got):\n%s", diff)
	}
}

func TestGetPostgres_NotFound(t *testing.T) {
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	})
	_, err := client.GetPostgres(context.Background(), testPostgresID)
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("IsNotFound(err) = false; want true (err=%v)", err)
	}
}

// ----- ListPostgres --------------------------------------------------------

func TestListPostgres_HappyPath(t *testing.T) {
	expectedPath := testPostgresListPath
	want := []PostgresListItem{
		{Id: "pg-1", Name: "one", Provider: "aws", Region: "us-east-1", State: "running", CreatedAt: "t1", IsPrimary: true},
		{Id: "pg-2", Name: "two", Provider: "aws", Region: "us-west-2", State: "creating", CreatedAt: "t2", IsPrimary: false},
	}

	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[[]PostgresListItem]{Result: want})
	})

	got, err := client.ListPostgres(context.Background())
	if err != nil {
		t.Fatalf("ListPostgres: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListPostgres mismatch (-want +got):\n%s", diff)
	}
}

// ----- CreatePostgres ------------------------------------------------------

func TestCreatePostgres_HappyPath_PersistsServerGeneratedPassword(t *testing.T) {
	expectedPath := testPostgresListPath
	pgServerSide := Postgres{
		Id:               "pg-new",
		Name:             "my-pg",
		Provider:         "aws",
		Region:           "us-east-1",
		Size:             "r6gd.large",
		State:            PostgresStateCreating,
		CreatedAt:        "2026-05-20T00:00:00Z",
		IsPrimary:        true,
		Password:         "server-generated-Aa1!secret",
		ConnectionString: "postgresql://user:server-generated-Aa1!secret@host/db",
	}

	var capturedBody PostgresCreate
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q; want POST", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: pgServerSide})
	})

	got, password, err := client.CreatePostgres(context.Background(), PostgresCreate{
		Name: "my-pg", Provider: "aws", Region: "us-east-1", Size: "r6gd.large",
	})
	if err != nil {
		t.Fatalf("CreatePostgres: %v", err)
	}
	if diff := cmp.Diff(&pgServerSide, got); diff != "" {
		t.Errorf("CreatePostgres returned instance mismatch (-want +got):\n%s", diff)
	}
	if password == "" {
		t.Fatal("expected server-generated password to be returned separately; got empty")
	}
	if password != "server-generated-Aa1!secret" {
		t.Errorf("password = %q; want server-generated", password)
	}
	if capturedBody.Name != "my-pg" || capturedBody.Provider != "aws" {
		t.Errorf("captured body wrong: %+v", capturedBody)
	}
}

func TestCreatePostgres_NoPasswordInResponse_ReturnsEmptyPasswordOut(t *testing.T) {
	// When the user supplied their own password in the create body, the server
	// doesn't echo a password back. The client method's second return must
	// be empty so callers can tell "server generated one for us" apart from
	// "we already have the password the user supplied."
	pgServerSide := Postgres{
		Id: "pg-new", Name: "my-pg", Provider: "aws", Region: "us-east-1",
		Size: "r6gd.large", State: PostgresStateCreating,
	}
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: pgServerSide})
	})
	_, password, err := client.CreatePostgres(context.Background(), PostgresCreate{
		Name: "my-pg", Provider: "aws", Region: "us-east-1", Size: "r6gd.large",
	})
	if err != nil {
		t.Fatalf("CreatePostgres: %v", err)
	}
	if password != "" {
		t.Errorf("password = %q; want empty (no password in server response)", password)
	}
}

// ----- UpdatePostgres ------------------------------------------------------

func TestUpdatePostgres_PatchesOnlyChangedFields(t *testing.T) {
	expectedPath := testPostgresInstancePath
	var capturedBody map[string]any
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q; want PATCH", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", Size: testPostgresSizeXLarge}})
	})
	_, err := client.UpdatePostgres(context.Background(), testPostgresID, PostgresUpdate{Size: testPostgresSizeXLarge})
	if err != nil {
		t.Fatalf("UpdatePostgres: %v", err)
	}
	if _, ok := capturedBody["name"]; ok {
		t.Errorf("PATCH body must not include name; got %v", capturedBody)
	}
	if got := capturedBody["size"]; got != testPostgresSizeXLarge {
		t.Errorf("size = %v; want r6gd.xlarge", got)
	}
}

func TestUpdatePostgres_SendsNameWhenSet(t *testing.T) {
	const newName = "renamed-pg"
	var capturedBody map[string]any
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", Name: newName}})
	})
	_, err := client.UpdatePostgres(context.Background(), testPostgresID, PostgresUpdate{Name: newName})
	if err != nil {
		t.Fatalf("UpdatePostgres: %v", err)
	}
	if got := capturedBody["name"]; got != newName {
		t.Errorf("name = %v; want %q", got, newName)
	}
}

// ----- DeletePostgres ------------------------------------------------------

func TestDeletePostgres_HappyPath(t *testing.T) {
	expectedPath := testPostgresInstancePath
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q; want DELETE", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		// Server returns 200 with body that has no `result` field.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"requestId":"req-1","status":200}`))
	})
	err := client.DeletePostgres(context.Background(), testPostgresID)
	if err != nil {
		t.Fatalf("DeletePostgres: %v", err)
	}
}

func TestDeletePostgres_NotFoundReturnsNil(t *testing.T) {
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	})
	err := client.DeletePostgres(context.Background(), testPostgresID)
	if err != nil {
		t.Errorf("DeletePostgres on 404 should be idempotent; got %v", err)
	}
}

func TestDeletePostgres_RetriesOn409(t *testing.T) {
	var calls atomic.Int32
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			http.Error(w, `{"error":"transient conflict"}`, http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"requestId":"req","status":200}`))
	})
	// Use a very short retry budget so this test runs fast. The client should
	// still succeed after two 409s.
	err := client.deletePostgresWithInterval(context.Background(), testPostgresID, 1*time.Millisecond, 5)
	if err != nil {
		t.Fatalf("DeletePostgres should retry on 409 and succeed; got %v", err)
	}
	if calls.Load() < 3 {
		t.Errorf("expected at least 3 calls (2x 409 + 1x 200); got %d", calls.Load())
	}
}

func TestDeletePostgres_RetriesOn409WithoutDependentSignal(t *testing.T) {
	// A 409 whose body mentions "replica" but not "depend" should NOT fail
	// fast — that's a transient conflict the retry loop can resolve. Guards
	// against the loose pattern match that an earlier draft of the heuristic
	// allowed (OR rather than AND).
	var calls atomic.Int32
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"replication slot still active; try again"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"requestId":"r","status":200}`))
	})
	err := client.deletePostgresWithInterval(context.Background(), testPostgresID, 1*time.Millisecond, 5)
	if err != nil {
		t.Fatalf("DeletePostgres should retry transient 409s containing 'replica' without 'depend'; got %v", err)
	}
	if calls.Load() < 3 {
		t.Errorf("expected ≥3 calls (2x 409 retried then 200); got %d", calls.Load())
	}
}

func TestDeletePostgres_FailsFastOnDependentReplica(t *testing.T) {
	var calls atomic.Int32
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// Server signals a dependent replica blocks deletion.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"cannot delete primary while dependent replicas exist","code":"DEPENDENT_REPLICA"}`))
	})
	err := client.deletePostgresWithInterval(context.Background(), testPostgresID, 5*time.Millisecond, 10)
	if err == nil {
		t.Fatal("expected error; got nil")
	}
	if !strings.Contains(err.Error(), "dependent") && !strings.Contains(err.Error(), "replica") {
		t.Errorf("expected error to mention dependent replica; got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected fail-fast (1 call); got %d calls", calls.Load())
	}
}

// ----- WaitForPostgresState ------------------------------------------------

func TestWaitForPostgresState_TransitionsToRunning(t *testing.T) {
	var calls atomic.Int32
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		state := PostgresStateCreating
		if n >= 3 {
			state = PostgresStateRunning
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", State: state}})
	})
	err := client.waitForPostgresStateWithInterval(context.Background(), testPostgresID,
		func(s string) bool { return s == PostgresStateRunning },
		1*time.Millisecond, 50)
	if err != nil {
		t.Fatalf("WaitForPostgresState: %v", err)
	}
	if calls.Load() < 3 {
		t.Errorf("expected at least 3 polls; got %d", calls.Load())
	}
}

func TestWaitForPostgresState_TimesOutWithLastSeenState(t *testing.T) {
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", State: PostgresStateCreating}})
	})
	err := client.waitForPostgresStateWithInterval(context.Background(), testPostgresID,
		func(s string) bool { return s == PostgresStateRunning },
		1*time.Millisecond, 3)
	if err == nil {
		t.Fatal("expected timeout error; got nil")
	}
	if !strings.Contains(err.Error(), PostgresStateCreating) {
		t.Errorf("error should mention last seen state %q; got %v", PostgresStateCreating, err)
	}
}

func TestWaitForPostgresState_PropagatesGetErrors(t *testing.T) {
	// If every poll's GetPostgres fails (e.g., instance deleted out-of-band,
	// token revoked), the helper must surface the real error rather than
	// rewriting it to the misleading "did not reach the expected state"
	// timeout message. Same shape as the LeaveAndReturn equivalent test.
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	})
	err := client.waitForPostgresStateWithInterval(context.Background(), testPostgresID,
		func(s string) bool { return s == PostgresStateRunning },
		1*time.Millisecond, 4)
	if err == nil {
		t.Fatal("expected error to propagate from failing GetPostgres; got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("expected 404 to propagate via IsNotFound; got %v", err)
	}
	if strings.Contains(err.Error(), "did not reach the expected state") {
		t.Errorf("real GetPostgres error must not be rewritten to the timeout message; got %v", err)
	}
}

func TestWaitForPostgresState_UnknownStateDoesNotCrash(t *testing.T) {
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", State: "something_brand_new"}})
	})
	err := client.waitForPostgresStateWithInterval(context.Background(), testPostgresID,
		func(s string) bool { return s == PostgresStateRunning },
		1*time.Millisecond, 2)
	if err == nil {
		t.Fatal("expected timeout error; got nil")
	}
	if !strings.Contains(err.Error(), "something_brand_new") {
		t.Errorf("error should mention the unknown state verbatim; got %v", err)
	}
}

// ----- WaitForPostgresMatch --------------------------------------------------

// matchSize returns a predicate that succeeds when the server reports
// state=running and the given size.
func matchSize(size string) func(*Postgres) bool {
	return func(pg *Postgres) bool {
		return pg.State == PostgresStateRunning && pg.Size == size
	}
}

func TestWaitForPostgresMatch_SucceedsOnConsecutivePredicateMatches(t *testing.T) {
	// Server returns the requested values cleanly throughout — the hot-reload
	// case (server applied the change before our first poll). Helper must
	// succeed after settleRetries consecutive matching polls.
	var calls atomic.Int32
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", State: PostgresStateRunning, Size: testPostgresSizeXLarge}})
	})
	err := client.waitForPostgresMatchWithInterval(context.Background(), testPostgresID,
		matchSize(testPostgresSizeXLarge), 1*time.Millisecond, 10, 3)
	if err != nil {
		t.Errorf("expected success after 3 consecutive matches; got %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("expected exactly 3 polls (settleRetries=3); got %d", got)
	}
}

func TestWaitForPostgresMatch_QueuedWorkWaitsForFieldFlip(t *testing.T) {
	// The race-trigger case the predicate-based helper is designed for:
	// server returns state=running but the field (size) hasn't flipped yet
	// because Ubicloud has queued the work. The helper must NOT succeed
	// until the field reflects the requested value.
	var calls atomic.Int32
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		size := "r6gd.large" // pre-PATCH steady state
		if n >= 4 {
			size = testPostgresSizeXLarge // Ubicloud finally committed the change
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", State: PostgresStateRunning, Size: size}})
	})
	err := client.waitForPostgresMatchWithInterval(context.Background(), testPostgresID,
		matchSize(testPostgresSizeXLarge), 1*time.Millisecond, 20, 3)
	if err != nil {
		t.Fatalf("expected success once field flipped + settled; got %v", err)
	}
	// Polls 1-3: size=large (no match, stable=0); polls 4-6: size=xlarge (3 matches → success).
	if got := calls.Load(); got != 6 {
		t.Errorf("expected exactly 6 polls (3 pre-flip, 3 post-flip); got %d", got)
	}
}

func TestWaitForPostgresMatch_TransientMatchResetsSettleWindow(t *testing.T) {
	// Server briefly shows the matching state mid-transition, then flips back
	// to non-matching, then matches stably. Settle must reset across the
	// inversion so success is anchored to STABLE matching, not transient.
	var calls atomic.Int32
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		size := testPostgresSizeXLarge
		if n == 2 {
			size = "r6gd.large" // transient inversion
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", State: PostgresStateRunning, Size: size}})
	})
	err := client.waitForPostgresMatchWithInterval(context.Background(), testPostgresID,
		matchSize(testPostgresSizeXLarge), 1*time.Millisecond, 20, 3)
	if err != nil {
		t.Fatalf("expected eventual success; got %v", err)
	}
	// poll1 match (1), poll2 mismatch (reset to 0), polls 3-5 match (stable=3 → success).
	if got := calls.Load(); got != 5 {
		t.Errorf("expected exactly 5 polls (transient inversion forces reset); got %d", got)
	}
}

func TestWaitForPostgresMatch_PropagatesGetErrors(t *testing.T) {
	// Polling repeatedly errors (instance deleted out-of-band, auth expired).
	// Helper must surface the error rather than silently returning nil.
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	})
	err := client.waitForPostgresMatchWithInterval(context.Background(), testPostgresID,
		matchSize(testPostgresSizeXLarge), 1*time.Millisecond, 4, 3)
	if err == nil {
		t.Fatal("expected error to propagate from failing GetPostgres; got nil")
	}
	if !IsNotFound(err) {
		t.Errorf("expected 404 to propagate via IsNotFound; got %v", err)
	}
}

func TestWaitForPostgresMatch_NeverMatchesTimesOut(t *testing.T) {
	// Field never flips (Ubicloud silently dropped the work, or the server
	// is in an unrecoverable state). Helper must time out rather than
	// silently report success.
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", State: PostgresStateRunning, Size: "r6gd.large"}})
	})
	err := client.waitForPostgresMatchWithInterval(context.Background(), testPostgresID,
		matchSize(testPostgresSizeXLarge), 1*time.Millisecond, 5, 3)
	if err == nil {
		t.Fatal("expected timeout error when predicate never matches; got nil")
	}
	if !strings.Contains(err.Error(), "did not match") {
		t.Errorf("error should mention failure to match; got %v", err)
	}
	if !strings.Contains(err.Error(), PostgresStateRunning) {
		t.Errorf("error should mention last seen state %q; got %v", PostgresStateRunning, err)
	}
}

func TestWaitForPostgresMatch_HonorsContextCancellation(t *testing.T) {
	// Predicate never matches → without ctx-aware backoff, the helper would
	// burn the full budget. With backoff.WithContext, cancellation bails fast.
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", State: PostgresStateRunning, Size: "r6gd.large"}})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := client.waitForPostgresMatchWithInterval(ctx, testPostgresID,
		matchSize(testPostgresSizeXLarge), 50*time.Millisecond, 600, 3)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from context timeout; got nil")
	}
	// Budget if cancellation were ignored: 600 × 50ms = 30s. Honoring ctx
	// should bail within a couple of poll intervals after the deadline.
	if elapsed > 1*time.Second {
		t.Errorf("cancellation should bail well before budget; took %v", elapsed)
	}
}

func TestWaitForPostgresState_HonorsContextCancellation(t *testing.T) {
	// Same guarantee for the simpler state-only helper.
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", State: PostgresStateCreating}})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := client.waitForPostgresStateWithInterval(ctx, testPostgresID,
		func(s string) bool { return s == PostgresStateRunning }, 50*time.Millisecond, 600)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from context timeout; got nil")
	}
	if elapsed > 1*time.Second {
		t.Errorf("cancellation should bail well before budget; took %v", elapsed)
	}
}

// ----- SetPostgresPassword -------------------------------------------------

func TestSetPostgresPassword_UserSuppliedReturnsEmpty(t *testing.T) {
	expectedPath := "/organizations/org-1/postgres/pg-1/password"
	var capturedBody PostgresPassword
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q; want PATCH", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		// Server returns empty password (user supplied it).
		_ = json.NewEncoder(w).Encode(ResponseWithResult[PostgresPassword]{Result: PostgresPassword{}})
	})
	supplied := "User-Supplied-Aa1!password"
	got, err := client.SetPostgresPassword(context.Background(), testPostgresID, PostgresPassword{Password: supplied})
	if err != nil {
		t.Fatalf("SetPostgresPassword: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil response struct")
	}
	if got.Password != "" {
		t.Errorf("expected empty password in response (user supplied); got %q", got.Password)
	}
	if capturedBody.Password != supplied {
		t.Errorf("server got body %+v; want password=%q", capturedBody, supplied)
	}
}

func TestSetPostgresPassword_ServerGeneratedReturnsValue(t *testing.T) {
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResponseWithResult[PostgresPassword]{Result: PostgresPassword{Password: "ServerGen-Aa1!password"}})
	})
	got, err := client.SetPostgresPassword(context.Background(), testPostgresID, PostgresPassword{})
	if err != nil {
		t.Fatalf("SetPostgresPassword: %v", err)
	}
	if got.Password == "" {
		t.Fatal("expected server-generated password; got empty")
	}
	if !strings.HasPrefix(got.Password, "ServerGen") {
		t.Errorf("password = %q; want server-generated value", got.Password)
	}
}

func TestSetPostgresPassword_IdempotencyForUserSuppliedValue(t *testing.T) {
	// User-supplied password is idempotent: server re-sets the same value
	// and returns an empty Password both times.
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ResponseWithResult[PostgresPassword]{Result: PostgresPassword{}})
	})
	supplied := "User-Supplied-Aa1!password"
	got1, err := client.SetPostgresPassword(context.Background(), testPostgresID, PostgresPassword{Password: supplied})
	if err != nil {
		t.Fatalf("first SetPostgresPassword: %v", err)
	}
	got2, err := client.SetPostgresPassword(context.Background(), testPostgresID, PostgresPassword{Password: supplied})
	if err != nil {
		t.Fatalf("second SetPostgresPassword: %v", err)
	}
	if got1.Password != "" || got2.Password != "" {
		t.Errorf("user-supplied PATCHes should both return empty Password; got %+v %+v", got1, got2)
	}
}

// ----- Config (Get / Replace POST / Update PATCH) -------------------------

func TestGetPostgresConfig_HappyPath(t *testing.T) {
	expectedPath := "/organizations/org-1/postgres/pg-1/config"
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		_, _ = w.Write([]byte(`{"result":{"pgConfig":{"max_connections":"200"},"pgBouncerConfig":{"default_pool_size":10}}}`))
	})
	got, err := client.GetPostgresConfig(context.Background(), testPostgresID)
	if err != nil {
		t.Fatalf("GetPostgresConfig: %v", err)
	}
	if got.PgConfig["max_connections"] != "200" {
		t.Errorf("PgConfig[max_connections] = %q; want 200", got.PgConfig["max_connections"])
	}
	if got.PgBouncerConfig["default_pool_size"] != "10" {
		t.Errorf("PgBouncerConfig[default_pool_size] = %q; want 10", got.PgBouncerConfig["default_pool_size"])
	}
}

func TestReplacePostgresConfig_PostsBothMaps(t *testing.T) {
	expectedPath := "/organizations/org-1/postgres/pg-1/config"
	var captured PostgresConfig
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q; want POST", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"result":{"pgConfig":{"max_connections":"200"},"pgBouncerConfig":{},"message":"restart required"}}`))
	})
	resp, err := client.ReplacePostgresConfig(context.Background(), testPostgresID, PostgresConfig{
		PgConfig:        PgConfigMap{"max_connections": "200"},
		PgBouncerConfig: PgConfigMap{},
	})
	if err != nil {
		t.Fatalf("ReplacePostgresConfig: %v", err)
	}
	if resp.Message != "restart required" {
		t.Errorf("Message should surface restart hint; got %q", resp.Message)
	}
	if captured.PgConfig["max_connections"] != "200" {
		t.Errorf("server got %+v; want max_connections=200", captured)
	}
}

func TestReplacePostgresConfig_AcceptsEmptyMaps(t *testing.T) {
	// Per Phase 0 Curl 3, runtime validator wins: {} is accepted by the server.
	// The client should send {} (not omitempty) so users can clear all parameters.
	var captured map[string]any
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"result":{"pgConfig":{},"pgBouncerConfig":{}}}`))
	})
	_, err := client.ReplacePostgresConfig(context.Background(), testPostgresID, PostgresConfig{
		PgConfig:        PgConfigMap{},
		PgBouncerConfig: PgConfigMap{},
	})
	if err != nil {
		t.Fatalf("ReplacePostgresConfig empty: %v", err)
	}
	pgConfig, ok := captured["pgConfig"].(map[string]any)
	if !ok {
		t.Fatalf("body must include pgConfig as object; got %v", captured)
	}
	if len(pgConfig) != 0 {
		t.Errorf("pgConfig must be empty object; got %v", pgConfig)
	}
	pgBouncer, ok := captured["pgBouncerConfig"].(map[string]any)
	if !ok {
		t.Fatalf("body must include pgBouncerConfig as object; got %v", captured)
	}
	if len(pgBouncer) != 0 {
		t.Errorf("pgBouncerConfig must be empty object; got %v", pgBouncer)
	}
}

// ----- Restore + Read Replica ---------------------------------------------

func TestRestorePostgres_HappyPath(t *testing.T) {
	expectedPath := "/organizations/org-1/postgres/source-id/restoredService"
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q; want POST", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-restored", Name: "restored", Provider: "aws", Region: "us-east-1"}})
	})
	got, err := client.RestorePostgres(context.Background(), "source-id", PostgresRestoreRequest{
		Name: "restored", RestoreTarget: "2026-05-20T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("RestorePostgres: %v", err)
	}
	if got.Id != "pg-restored" {
		t.Errorf("Id = %q; want pg-restored", got.Id)
	}
}

func TestCreatePostgresReadReplica_HappyPath(t *testing.T) {
	expectedPath := "/organizations/org-1/postgres/primary-id/readReplica"
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q; want POST", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-replica", IsPrimary: false}})
	})
	got, err := client.CreatePostgresReadReplica(context.Background(), "primary-id", PostgresReadReplicaRequest{Name: "replica"})
	if err != nil {
		t.Fatalf("CreatePostgresReadReplica: %v", err)
	}
	if got.IsPrimary {
		t.Errorf("replica should have IsPrimary=false; got %v", got.IsPrimary)
	}
}

// ----- CA certificates (raw response) ------------------------------------

func TestGetPostgresCaCertificates_ReturnsRawPEM(t *testing.T) {
	expectedPath := "/organizations/org-1/postgres/pg-1/caCertificates"
	pem := "-----BEGIN CERTIFICATE-----\nMIID...\n-----END CERTIFICATE-----\n"
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q; want GET", r.Method)
		}
		if r.URL.Path != expectedPath {
			t.Errorf("path = %q; want %q", r.URL.Path, expectedPath)
		}
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write([]byte(pem))
	})
	got, err := client.GetPostgresCaCertificates(context.Background(), testPostgresID)
	if err != nil {
		t.Fatalf("GetPostgresCaCertificates: %v", err)
	}
	if string(got) != pem {
		t.Errorf("raw PEM mismatch; got %q want %q", string(got), pem)
	}
}

// ----- Rate limit honored (sanity check on doRequest) -----------------------

func TestPostgres_RateLimit429HonorsResetHeader(t *testing.T) {
	var calls atomic.Int32
	start := time.Now()
	client, _ := newPostgresTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.Header().Set(ResponseHeaderRateLimitReset, "0")
			http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
			return
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1"}})
	})
	_, err := client.GetPostgres(context.Background(), testPostgresID)
	if err != nil {
		t.Fatalf("GetPostgres should retry past 429; got %v", err)
	}
	if calls.Load() < 2 {
		t.Errorf("expected ≥2 calls (429 then 200); got %d", calls.Load())
	}
	// X-RateLimit-Reset=0 should mean we retry almost immediately.
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("expected fast retry; took %v", elapsed)
	}
}

// ----- Misc: assert getPostgresPath works as advertised --------------------

func TestGetPostgresPath(t *testing.T) {
	c := &ClientImpl{BaseUrl: "https://example.com/v1", OrganizationId: "org-x"}
	if diff := cmp.Diff(c.getPostgresPath("", ""), "https://example.com/v1/organizations/org-x/postgres"); diff != "" {
		t.Errorf("getPostgresPath empty: %s", diff)
	}
	if diff := cmp.Diff(c.getPostgresPath("pg-1", ""), "https://example.com/v1/organizations/org-x/postgres/pg-1"); diff != "" {
		t.Errorf("getPostgresPath instance: %s", diff)
	}
	if diff := cmp.Diff(c.getPostgresPath("pg-1", "/config"), "https://example.com/v1/organizations/org-x/postgres/pg-1/config"); diff != "" {
		t.Errorf("getPostgresPath sub: %s", diff)
	}
}
