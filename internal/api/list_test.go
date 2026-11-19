package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/pagination"
)

type listItem struct {
	ID string `json:"id"`
}

// cursorPages serves pages keyed by the request's cursor query param ("" is the
// first page) and records the query of every request.
func cursorPages(t *testing.T, pages map[string]string, queries *[]url.Values) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		*queries = append(*queries, r.URL.Query())
		body, ok := pages[r.URL.Query().Get("cursor")]
		if !ok {
			t.Errorf("unexpected cursor %q", r.URL.Query().Get("cursor"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, body)
	}
}

func TestListAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		query       url.Values
		pages       map[string]string
		want        []listItem
		wantQueries []url.Values
	}{
		{
			name:        "unpaginated endpoint",
			pages:       map[string]string{"": `{"status":200,"result":[{"id":"a"},{"id":"b"}]}`},
			want:        []listItem{{ID: "a"}, {ID: "b"}},
			wantQueries: []url.Values{{}},
		},
		{
			name: "walks every page",
			pages: map[string]string{
				"":   `{"result":[{"id":"a"}],"limit":1,"totalCount":3,"nextCursor":"c1"}`,
				"c1": `{"result":[{"id":"b"}],"limit":1,"totalCount":3,"nextCursor":"c2"}`,
				"c2": `{"result":[{"id":"c"}],"limit":1,"totalCount":3,"nextCursor":null}`,
			},
			want:        []listItem{{ID: "a"}, {ID: "b"}, {ID: "c"}},
			wantQueries: []url.Values{{}, {"cursor": {"c1"}}, {"cursor": {"c2"}}},
		},
		{
			name:  "filters are re-sent on every page, no limit is sent",
			query: url.Values{"filter": {"tag:a=1", "tag:b=2"}},
			pages: map[string]string{
				"":   `{"result":[{"id":"a"}],"nextCursor":"c1"}`,
				"c1": `{"result":[{"id":"b"}]}`,
			},
			want: []listItem{{ID: "a"}, {ID: "b"}},
			wantQueries: []url.Values{
				{"filter": {"tag:a=1", "tag:b=2"}},
				{"filter": {"tag:a=1", "tag:b=2"}, "cursor": {"c1"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var queries []url.Values
			c, _ := newTestClient(t, cursorPages(t, tt.pages, &queries))

			got, err := listAll[listItem](context.Background(), c, c.getOrgPath("/things"), tt.query)
			if err != nil {
				t.Fatalf("listAll: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("items = %+v, want %+v", got, tt.want)
			}
			if !reflect.DeepEqual(queries, tt.wantQueries) {
				t.Errorf("queries = %v, want %v", queries, tt.wantQueries)
			}
			// The caller's query must not be mutated by the cursor.
			if tt.query != nil && tt.query.Has("cursor") {
				t.Errorf("caller query mutated: %v", tt.query)
			}
		})
	}
}

func TestListAll_RepeatedCursor(t *testing.T) {
	t.Parallel()

	var queries []url.Values
	c, _ := newTestClient(t, cursorPages(t, map[string]string{
		"":   `{"result":[{"id":"a"}],"nextCursor":"c1"}`,
		"c1": `{"result":[{"id":"b"}],"nextCursor":"c1"}`,
	}, &queries))

	_, err := listAll[listItem](context.Background(), c, c.getOrgPath("/things"), nil)
	if !errors.Is(err, pagination.ErrNotConverged) {
		t.Fatalf("expected ErrNotConverged, got %v", err)
	}
}

func TestListAll_RequestErrorIsUnwrapped(t *testing.T) {
	t.Parallel()

	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":"not found"}`)
	})

	_, err := listAll[listItem](context.Background(), c, c.getOrgPath("/things"), nil)
	if !IsNotFound(err) {
		t.Fatalf("expected IsNotFound to match, got %v", err)
	}
}
