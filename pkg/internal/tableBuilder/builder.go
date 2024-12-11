package tableBuilder

import (
	"context"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/queryApi"
)

type builder struct {
	QueryApiClient queryApi.Client
}

func New(queryApiClient queryApi.Client) (Builder, error) {
	// todo validate queryApiClient is not nil

	return &builder{QueryApiClient: queryApiClient}, nil
}

func (t *builder) CreateTable(ctx context.Context, table Table) error {
	err := t.QueryApiClient.RunQuery(ctx, table.querySpec())
	if err != nil {
		return err
	}

	return nil
}
