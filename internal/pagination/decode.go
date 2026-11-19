package pagination

import (
	"encoding/json"
	"fmt"
)

// openAPIPage is the ClickHouse Cloud OpenAPI list envelope:
//
//	{"status":200,"requestId":"…","result":[…],"limit":250,"totalCount":9999,"nextCursor":"…"}
//
// An endpoint that is not paginated yet returns only {"result":[…]}, which
// decodes as a single, last page.
type openAPIPage[T any] struct {
	Result     []T     `json:"result"`
	Limit      int     `json:"limit"`
	TotalCount *int    `json:"totalCount"`
	NextCursor *string `json:"nextCursor"`
}

// DecodeOpenAPIPage decodes a ClickHouse Cloud OpenAPI list response.
func DecodeOpenAPIPage[T any](body []byte) (Page[T], error) {
	var env openAPIPage[T]
	if err := json.Unmarshal(body, &env); err != nil {
		return Page[T]{}, fmt.Errorf("decode page: %w", err)
	}
	page := Page[T]{
		Items:      env.Result,
		TotalCount: env.TotalCount,
		Limit:      env.Limit,
	}
	if env.NextCursor != nil {
		page.NextCursor = *env.NextCursor
	}
	return page, nil
}
