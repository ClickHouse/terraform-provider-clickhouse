package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestCreateSavedSearch(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/saved-searches" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["name"] != "errors" || body["sourceId"] != "src1" {
			t.Errorf("unexpected body: %v", body)
		}
		if body["whereLanguage"] != "lucene" {
			t.Errorf("expected whereLanguage lucene, got %v", body["whereLanguage"])
		}
		// tags must serialize as [] (full-replace reset), not null.
		tags, ok := body["tags"].([]any)
		if !ok || len(tags) != 0 {
			t.Errorf("expected empty tags array, got %v", body["tags"])
		}
		// filters passed through verbatim.
		filters, ok := body["filters"].([]any)
		if !ok || len(filters) != 1 {
			t.Errorf("expected filters array of 1, got %v", body["filters"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"ss1","name":"errors","sourceId":"src1","whereLanguage":"lucene","tags":[],"filters":[{"type":"sql","condition":"x"}]}}`)
	})

	ss, err := c.CreateSavedSearch(context.Background(), SavedSearch{
		Name:          "errors",
		SourceID:      "src1",
		WhereLanguage: "lucene",
		Tags:          []string{},
		Filters:       json.RawMessage(`[{"type":"sql","condition":"x"}]`),
	})
	if err != nil {
		t.Fatalf("CreateSavedSearch: %v", err)
	}
	if ss.ID != "ss1" {
		t.Errorf("expected id ss1, got %q", ss.ID)
	}
}

func TestGetSavedSearch_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.GetSavedSearch(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetSavedSearch_FiltersRoundTrip(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// A filter shape the provider does not model field-by-field; it must
		// survive verbatim.
		_, _ = io.WriteString(w, `{"data":{"id":"ss1","name":"n","sourceId":"s","filters":[{"type":"sql_ast","operator":"and","left":{},"right":{}}]}}`)
	})

	ss, err := c.GetSavedSearch(context.Background(), "ss1")
	if err != nil {
		t.Fatalf("GetSavedSearch: %v", err)
	}
	if !json.Valid(ss.Filters) {
		t.Fatalf("filters not valid JSON: %s", ss.Filters)
	}
	var got []map[string]any
	if err := json.Unmarshal(ss.Filters, &got); err != nil {
		t.Fatalf("unmarshal filters: %v", err)
	}
	if len(got) != 1 || got[0]["type"] != "sql_ast" {
		t.Errorf("sql_ast filter not preserved: %v", got)
	}
}

func TestUpdateSavedSearch(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/saved-searches/ss1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"ss1","name":"renamed","sourceId":"s"}}`)
	})

	ss, err := c.UpdateSavedSearch(context.Background(), "ss1", SavedSearch{Name: "renamed", SourceID: "s", Tags: []string{}})
	if err != nil {
		t.Fatalf("UpdateSavedSearch: %v", err)
	}
	if ss.Name != "renamed" {
		t.Errorf("expected renamed, got %q", ss.Name)
	}
}

func TestDeleteSavedSearch(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v2/saved-searches/ss1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	})

	if err := c.DeleteSavedSearch(context.Background(), "ss1"); err != nil {
		t.Fatalf("DeleteSavedSearch: %v", err)
	}
}
