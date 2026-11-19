package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/pagination"
)

// queryParamCursor is the query parameter carrying the opaque cursor of the
// page to fetch, per the OpenAPI pagination RFC.
const queryParamCursor = "cursor"

// eachItem walks every page of a cursor-paginated OpenAPI list endpoint and
// calls fn for each item; fn returns true to stop early. query holds the
// endpoint's own filters and is re-sent unchanged on every page, as the RFC
// requires. No limit is sent, so the server default page size applies.
//
// An endpoint that does not paginate yet returns a single page with no
// nextCursor, so this is safe to use on any list endpoint.
func eachItem[T any](ctx context.Context, c *ClientImpl, path string, query url.Values, fn func(T) bool) error {
	fetch := func(ctx context.Context, page pagination.Request) (pagination.Page[T], error) {
		q := url.Values{}
		for k, v := range query {
			q[k] = append([]string(nil), v...)
		}
		if page.Cursor != "" {
			q.Set(queryParamCursor, page.Cursor)
		}

		req, err := http.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			return pagination.Page[T]{}, err
		}
		req.URL.RawQuery = q.Encode()

		body, err := c.doRequest(ctx, req)
		if err != nil {
			return pagination.Page[T]{}, err
		}
		return pagination.DecodeOpenAPIPage[T](body)
	}

	err := pagination.Each(ctx, fetch, fn, pagination.Options{})
	// Request errors are returned unwrapped: IsNotFound and friends match on
	// the "status: NNN" prefix of the error message.
	if errors.Is(err, pagination.ErrNotConverged) {
		return fmt.Errorf("list %s: %w", path, err)
	}
	return err
}

// listAll returns every item of a cursor-paginated OpenAPI list endpoint. See
// eachItem.
func listAll[T any](ctx context.Context, c *ClientImpl, path string, query url.Values) ([]T, error) {
	var all []T
	err := eachItem(ctx, c, path, query, func(item T) bool {
		all = append(all, item)
		return false
	})
	if err != nil {
		return nil, err
	}
	return all, nil
}
