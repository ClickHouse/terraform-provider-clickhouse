package pagination

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

type item struct{ ID string }

func items(ids ...string) []item {
	out := make([]item, len(ids))
	for i, id := range ids {
		out[i] = item{ID: id}
	}
	return out
}

func ids(in []item) []string {
	out := make([]string, len(in))
	for i, it := range in {
		out[i] = it.ID
	}
	return out
}

func intPtr(i int) *int { return &i }

// cursorServer serves pages keyed by the requested cursor ("" is the first page).
func cursorServer(t *testing.T, pages map[string]Page[item], reqs *[]Request) Fetcher[item] {
	t.Helper()
	return func(_ context.Context, req Request) (Page[item], error) {
		*reqs = append(*reqs, req)
		p, ok := pages[req.Cursor]
		if !ok {
			t.Fatalf("unexpected cursor %q", req.Cursor)
		}
		return p, nil
	}
}

func TestAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pages     map[string]Page[item]
		opts      Options
		want      []string
		wantReqs  []Request
		wantError error
	}{
		{
			name:     "unpaginated endpoint is a single last page",
			pages:    map[string]Page[item]{"": {Items: items("a", "b")}},
			want:     []string{"a", "b"},
			wantReqs: []Request{{}},
		},
		{
			name:     "empty collection",
			pages:    map[string]Page[item]{"": {}},
			want:     []string{},
			wantReqs: []Request{{}},
		},
		{
			name: "follows cursors to the last page",
			pages: map[string]Page[item]{
				"":   {Items: items("a", "b"), NextCursor: "c1"},
				"c1": {Items: items("c", "d"), NextCursor: "c2"},
				"c2": {Items: items("e")},
			},
			want:     []string{"a", "b", "c", "d", "e"},
			wantReqs: []Request{{}, {Cursor: "c1"}, {Cursor: "c2"}},
		},
		{
			name: "empty page with a cursor does not stop the walk",
			pages: map[string]Page[item]{
				"":   {NextCursor: "c1"},
				"c1": {Items: items("a")},
			},
			want:     []string{"a"},
			wantReqs: []Request{{}, {Cursor: "c1"}},
		},
		{
			name: "page size is sent on every request",
			pages: map[string]Page[item]{
				"":   {Items: items("a"), NextCursor: "c1"},
				"c1": {Items: items("b")},
			},
			opts:     Options{PageSize: 50},
			want:     []string{"a", "b"},
			wantReqs: []Request{{Limit: 50}, {Cursor: "c1", Limit: 50}},
		},
		{
			name: "repeated cursor is an error",
			pages: map[string]Page[item]{
				"":   {Items: items("a"), NextCursor: "c1"},
				"c1": {Items: items("b"), NextCursor: "c1"},
			},
			wantError: ErrNotConverged,
		},
		{
			name: "page cap",
			pages: map[string]Page[item]{
				"":   {Items: items("a"), NextCursor: "c1"},
				"c1": {Items: items("b"), NextCursor: "c2"},
				"c2": {Items: items("c")},
			},
			opts:      Options{MaxPages: 2},
			wantError: ErrNotConverged,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var reqs []Request
			got, err := All(context.Background(), cursorServer(t, tt.pages, &reqs), tt.opts)
			if tt.wantError != nil {
				if !errors.Is(err, tt.wantError) {
					t.Fatalf("expected %v, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("All: %v", err)
			}
			if g := ids(got); !reflect.DeepEqual(g, tt.want) {
				t.Errorf("items = %v, want %v", g, tt.want)
			}
			if !reflect.DeepEqual(reqs, tt.wantReqs) {
				t.Errorf("requests = %+v, want %+v", reqs, tt.wantReqs)
			}
		})
	}
}

func TestEach_EarlyStop(t *testing.T) {
	t.Parallel()

	var reqs []Request
	fetch := cursorServer(t, map[string]Page[item]{
		"":   {Items: items("a", "b"), NextCursor: "c1"},
		"c1": {Items: items("c", "d"), NextCursor: "c2"},
	}, &reqs)

	var seen []string
	err := Each(context.Background(), fetch, func(i item) bool {
		seen = append(seen, i.ID)
		return i.ID == "c"
	}, Options{})
	if err != nil {
		t.Fatalf("Each: %v", err)
	}
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(seen, want) {
		t.Errorf("seen = %v, want %v", seen, want)
	}
	if len(reqs) != 2 {
		t.Errorf("expected 2 requests, got %d", len(reqs))
	}
}

func TestEach_FetchError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	calls := 0
	fetch := func(_ context.Context, _ Request) (Page[item], error) {
		calls++
		if calls == 2 {
			return Page[item]{}, boom
		}
		return Page[item]{Items: items(strconv.Itoa(calls)), NextCursor: "c" + strconv.Itoa(calls)}, nil
	}
	if _, err := All(context.Background(), fetch, Options{}); !errors.Is(err, boom) {
		t.Fatalf("expected boom, got %v", err)
	}
}

func TestEach_ContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	fetch := func(_ context.Context, _ Request) (Page[item], error) {
		calls++
		cancel()
		return Page[item]{Items: items("a"), NextCursor: "c" + strconv.Itoa(calls)}, nil
	}
	if _, err := All(ctx, fetch, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 fetch before cancellation took effect, got %d", calls)
	}
}
