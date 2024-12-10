package queryApi

import (
	"context"
)

type Client interface {
	RunQuery(ctx context.Context, query string) error
}
