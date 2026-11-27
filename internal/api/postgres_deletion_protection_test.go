package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestUpdatePostgres_SendsDeletionProtection(t *testing.T) {
	var captured map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", DeletionProtection: boolPtr(true)}})
	})
	got, err := client.UpdatePostgres(context.Background(), testPostgresID, PostgresUpdate{DeletionProtection: boolPtr(true)})
	if err != nil {
		t.Fatalf("UpdatePostgres: %v", err)
	}
	if captured["deletionProtection"] != true {
		t.Errorf("deletionProtection = %v; want true", captured["deletionProtection"])
	}
	if got.DeletionProtection == nil || !*got.DeletionProtection {
		t.Errorf("response deletionProtection = %v; want true", got.DeletionProtection)
	}
}

func TestUpdatePostgres_OmitsUnsetDeletionProtection(t *testing.T) {
	var captured map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1"}})
	})
	if _, err := client.UpdatePostgres(context.Background(), testPostgresID, PostgresUpdate{Size: testPostgresSizeXLarge}); err != nil {
		t.Fatalf("UpdatePostgres: %v", err)
	}
	if _, ok := captured["deletionProtection"]; ok {
		t.Errorf("PATCH body must omit deletionProtection when unset; got %v", captured)
	}
}

func TestDeletePostgres_FailsFastOnDeletionProtectionMessage(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q; want only DELETE when the message names deletion protection", r.Method)
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"CONFLICT: Deletion protection is enabled for this service. Turn it off before deleting the service.","status":409}`))
	})
	err := client.deletePostgresWithInterval(context.Background(), testPostgresID, time.Millisecond, 10)
	if !errors.Is(err, ErrPostgresDeletionProtected) {
		t.Fatalf("want ErrPostgresDeletionProtected; got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("want fail-fast (1 call); got %d", calls.Load())
	}
}

func TestDeletePostgres_FailsFastWhenGetReportsProtection(t *testing.T) {
	var deletes atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", DeletionProtection: boolPtr(true)}})
			return
		}
		deletes.Add(1)
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"CONFLICT: service is protected","status":409}`))
	})
	err := client.deletePostgresWithInterval(context.Background(), testPostgresID, time.Millisecond, 10)
	if !errors.Is(err, ErrPostgresDeletionProtected) {
		t.Fatalf("want ErrPostgresDeletionProtected; got %v", err)
	}
	if deletes.Load() != 1 {
		t.Errorf("want fail-fast (1 DELETE); got %d", deletes.Load())
	}
}

func TestDeletePostgres_RetriesConflictWhenNotProtected(t *testing.T) {
	var deletes atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{Result: Postgres{Id: "pg-1", DeletionProtection: boolPtr(false)}})
			return
		}
		if deletes.Add(1) <= 2 {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"transient conflict","status":409}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":200}`))
	})
	if err := client.deletePostgresWithInterval(context.Background(), testPostgresID, time.Millisecond, 10); err != nil {
		t.Fatalf("want retry then success; got %v", err)
	}
	if deletes.Load() != 3 {
		t.Errorf("want 3 DELETEs; got %d", deletes.Load())
	}
}

func TestPostgresListItem_DecodesDeletionProtection(t *testing.T) {
	var items []PostgresListItem
	if err := json.Unmarshal([]byte(`[{"id":"a","deletionProtection":true},{"id":"b"}]`), &items); err != nil {
		t.Fatal(err)
	}
	if items[0].DeletionProtection == nil || !*items[0].DeletionProtection {
		t.Errorf("item a: want true, got %v", items[0].DeletionProtection)
	}
	if items[1].DeletionProtection != nil {
		t.Errorf("item b: want nil when omitted, got %v", *items[1].DeletionProtection)
	}
}
