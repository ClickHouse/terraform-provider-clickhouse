package queryApi

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type Client interface {
	RunQuery(ctx context.Context, query string, callback func(rows driver.Rows) error) error
}
