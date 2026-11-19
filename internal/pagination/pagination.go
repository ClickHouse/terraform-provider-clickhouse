// Package pagination walks cursor-paginated list endpoints to exhaustion.
//
// It implements the client side of the "Pagination for OpenAPI list
// resources" RFC: list responses are decoded into a normalized Page shape and
// a single walker follows nextCursor until the last page, so every list is
// consumed the same way.
//
// The package is deliberately free of terraform-plugin-framework types: it
// only knows about pages and items.
package pagination

import (
	"context"
	"errors"
	"fmt"
)

// DefaultMaxPages bounds a walk against a server that never converges.
const DefaultMaxPages = 10000

// ErrNotConverged is returned when a walk hits its page cap, or when the server
// hands back a cursor it already returned (which would loop forever).
var ErrNotConverged = errors.New("pagination did not converge")

// Page is the normalized shape of one page of a list response.
type Page[T any] struct {
	// Items are the elements of this page.
	Items []T
	// NextCursor is the opaque cursor of the next page. Empty means this is the
	// last page.
	NextCursor string
	// TotalCount is the number of elements in the whole collection, when the
	// server reports it.
	TotalCount *int
	// Limit is the page size the server applied, when it reports it.
	Limit int
}

// Request identifies the page a Fetcher must retrieve.
type Request struct {
	// Cursor is the cursor of the page to fetch. Empty means the first page.
	Cursor string
	// Limit is the requested page size. Zero means "do not send a limit" and
	// let the server apply its default.
	Limit int
}

// Fetcher retrieves and decodes one page.
type Fetcher[T any] func(ctx context.Context, req Request) (Page[T], error)

// Options configure a walk.
type Options struct {
	// PageSize is sent as Request.Limit. Zero lets the server pick.
	PageSize int
	// MaxPages caps the number of pages fetched. Zero means DefaultMaxPages.
	MaxPages int
}

// Each walks every page and calls fn for each item. fn returns true to stop
// the walk early.
//
// An empty NextCursor ends the walk. A cursor that was already returned, or
// reaching MaxPages, is an error wrapping ErrNotConverged rather than an
// infinite loop. The walk stops as soon as ctx is done.
func Each[T any](ctx context.Context, fetch Fetcher[T], fn func(T) bool, opts Options) error {
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = DefaultMaxPages
	}

	seenCursors := map[string]bool{}
	req := Request{Limit: opts.PageSize}

	for pages := 0; ; pages++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if pages >= maxPages {
			return fmt.Errorf("%w after %d pages", ErrNotConverged, maxPages)
		}

		page, err := fetch(ctx, req)
		if err != nil {
			return err
		}

		for _, item := range page.Items {
			if fn(item) {
				return nil
			}
		}

		if page.NextCursor == "" {
			return nil
		}
		if seenCursors[page.NextCursor] {
			return fmt.Errorf("%w: cursor %q returned twice", ErrNotConverged, page.NextCursor)
		}
		seenCursors[page.NextCursor] = true
		req.Cursor = page.NextCursor
	}
}

// All walks every page and returns all items.
func All[T any](ctx context.Context, fetch Fetcher[T], opts Options) ([]T, error) {
	var all []T
	err := Each(ctx, fetch, func(item T) bool {
		all = append(all, item)
		return false
	}, opts)
	if err != nil {
		return nil, err
	}
	return all, nil
}
