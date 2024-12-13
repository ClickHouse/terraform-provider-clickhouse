package queryApi

import (
	"context"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type clientImpl struct {
	baseUrl string
}

func New(BaseUrl string) (Client, error) {
	// TODO validate baseUrl is a valid URL

	return &clientImpl{
		baseUrl: BaseUrl,
	}, nil
}

func (c *clientImpl) RunQuery(ctx context.Context, query string, callback func(rows driver.Rows) error) error {
	// todo implement me
	conn, err := clickhouse.Open(&clickhouse.Options{
		Protocol: clickhouse.Native,
		Addr:     []string{"127.0.0.1:9000"},
	})
	if err != nil {
		return err
	}

	err = conn.Ping(ctx)
	if err != nil {
		return err
	}

	if callback != nil {
		rows, err := conn.Query(ctx, query, nil)
		if err != nil {
			return err
		}

		err = callback(rows)
		if err != nil {
			return err
		}
	} else {
		err = conn.Exec(ctx, query, nil)
		if err != nil {
			return err
		}
	}

	return nil
}
