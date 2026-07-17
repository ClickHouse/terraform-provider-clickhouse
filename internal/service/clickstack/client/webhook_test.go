package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCreateWebhook(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/webhooks" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
			t.Errorf("unexpected Authorization header: %q", got)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["service"] != "generic" || body["name"] != "pd" || body["url"] != "https://example.com/hook" {
			t.Errorf("unexpected body: %v", body)
		}
		// Write-only secret maps are sent on create.
		headers, ok := body["headers"].(map[string]any)
		if !ok || headers["Authorization"] != "Bearer sekret" {
			t.Errorf("expected headers in create body, got %v", body["headers"])
		}

		w.Header().Set("Content-Type", "application/json")
		// The response omits the write-only headers/queryParams.
		_, _ = io.WriteString(w, `{"data":{"id":"wh1","service":"generic","name":"pd","url":"https://example.com/hook"}}`)
	})

	wh, err := c.CreateWebhook(context.Background(), Webhook{
		Service: "generic",
		Name:    "pd",
		URL:     "https://example.com/hook",
		Headers: map[string]string{"Authorization": "Bearer sekret"},
	})
	if err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	if wh.ID != "wh1" {
		t.Errorf("expected id wh1, got %q", wh.ID)
	}
	if wh.Headers != nil {
		t.Errorf("expected headers absent on response, got %v", wh.Headers)
	}
}

func TestGetWebhook_FiltersList(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/webhooks" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"a","service":"slack","name":"a","url":"u"},{"id":"b","service":"slack","name":"b","url":"u"}]}`)
	})

	wh, err := c.GetWebhook(context.Background(), "b")
	if err != nil {
		t.Fatalf("GetWebhook: %v", err)
	}
	if wh.Name != "b" {
		t.Errorf("expected webhook b, got %q", wh.Name)
	}
}

func TestGetWebhook_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"a"}]}`)
	})

	_, err := c.GetWebhook(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestGetWebhook_Paginated proves a webhook on a later page is still found: the
// first page is full (webhookListPageSize entries) so the client requests the
// next page, where the target lives.
func TestGetWebhook_Paginated(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var env webhookListEnvelope
		if r.URL.Query().Get("offset") == "0" {
			// A full page of filler so the client requests the next page.
			env.Data = make([]Webhook, webhookListPageSize)
			for i := range env.Data {
				env.Data[i] = Webhook{ID: fmt.Sprintf("filler-%d", i)}
			}
			env.Meta = &listMeta{Total: webhookListPageSize + 1, Limit: webhookListPageSize}
		} else {
			env.Data = []Webhook{{ID: "target", Name: "found", URL: "u"}}
		}
		if err := json.NewEncoder(w).Encode(env); err != nil {
			t.Fatalf("encode: %v", err)
		}
	})

	wh, err := c.GetWebhook(context.Background(), "target")
	if err != nil {
		t.Fatalf("GetWebhook paginated: %v", err)
	}
	if wh.Name != "found" {
		t.Errorf("expected paginated webhook 'found', got %q", wh.Name)
	}
}

func TestUpdateWebhook(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/webhooks/wh1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"wh1","service":"generic","name":"renamed","url":"u"}}`)
	})

	desc := "desc"
	wh, err := c.UpdateWebhook(context.Background(), "wh1", Webhook{
		Service:     "generic",
		Name:        "renamed",
		URL:         "u",
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("UpdateWebhook: %v", err)
	}
	if wh.Name != "renamed" {
		t.Errorf("expected renamed, got %q", wh.Name)
	}
}

func TestDeleteWebhook_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	err := c.DeleteWebhook(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteWebhook_ReferencedByAlert(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"message":"webhook is referenced by an alert"}`)
	})

	err := c.DeleteWebhook(context.Background(), "wh1")
	if err == nil {
		t.Fatal("expected error for referenced webhook")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("409 should not map to ErrNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), "referenced by an alert") {
		t.Errorf("expected API message in error, got %v", err)
	}
}

func TestWebhook_WithTeamHeader(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-hdx-team"); got != "team-9" {
			t.Errorf("expected x-hdx-team team-9, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[]}`)
	})

	if _, err := c.WithTeam("team-9").ListWebhooks(context.Background()); err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
}

// TestListWebhooks_ServerCapsPageSize proves paging continues when the server
// returns fewer rows than the requested limit but reports a larger meta total —
// a fixed short-page break would stop early and drop later webhooks.
func TestListWebhooks_ServerCapsPageSize(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var env webhookListEnvelope
		env.Meta = &listMeta{Total: 3, Limit: webhookListPageSize}
		switch r.URL.Query().Get("offset") {
		case "0":
			env.Data = []Webhook{{ID: "a"}, {ID: "b"}} // server caps at 2 though 3 exist
		default:
			env.Data = []Webhook{{ID: "c"}}
		}
		if err := json.NewEncoder(w).Encode(env); err != nil {
			t.Fatalf("encode: %v", err)
		}
	})

	whs, err := c.ListWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(whs) != 3 {
		t.Errorf("expected all 3 webhooks across capped pages, got %d", len(whs))
	}
}

// TestListWebhooks_OffsetIgnoredTerminates proves the loop terminates (rather than
// looping forever) when the server ignores offset and returns the same full page
// with no meta.
func TestListWebhooks_OffsetIgnoredTerminates(t *testing.T) {
	t.Parallel()

	page := make([]Webhook, webhookListPageSize)
	for i := range page {
		page[i] = Webhook{ID: fmt.Sprintf("id-%d", i)}
	}
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Same full page every time, no meta -> would loop forever without the
		// non-advancing (no-new-IDs) guard.
		if err := json.NewEncoder(w).Encode(webhookListEnvelope{Data: page}); err != nil {
			t.Fatalf("encode: %v", err)
		}
	})

	whs, err := c.ListWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(whs) != webhookListPageSize {
		t.Errorf("expected exactly one page of unique webhooks, got %d", len(whs))
	}
}

// TestListWebhooks_CappedPageNoMeta proves paging continues when the server caps
// its page size below the requested limit AND returns no meta — the short first
// page must not terminate the loop, or a live webhook on a later page is missed.
func TestListWebhooks_CappedPageNoMeta(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var env webhookListEnvelope // no meta
		switch r.URL.Query().Get("offset") {
		case "0":
			env.Data = []Webhook{{ID: "a"}, {ID: "b"}} // capped at 2, no meta
		case "2":
			env.Data = []Webhook{{ID: "target", Name: "found", URL: "u"}}
		default:
			env.Data = nil // empty -> terminate
		}
		if err := json.NewEncoder(w).Encode(env); err != nil {
			t.Fatalf("encode: %v", err)
		}
	})

	// Assert against ListWebhooks (not GetWebhook, whose early-exit could mask a
	// pager that fails to accumulate): all three across the capped no-meta pages.
	whs, err := c.ListWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ListWebhooks across capped no-meta pages: %v", err)
	}
	if len(whs) != 3 {
		t.Fatalf("expected 3 webhooks across capped no-meta pages, got %d", len(whs))
	}
	if whs[2].Name != "found" {
		t.Errorf("expected later-page webhook accumulated, got %q", whs[2].Name)
	}
}
