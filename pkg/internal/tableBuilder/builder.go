package tableBuilder

import (
	"context"
	"fmt"
	"strings"

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
	query := t.createTableQuery(table)
	err := t.QueryApiClient.RunQuery(ctx, query)
	if err != nil {
		return err
	}

	return nil
}

func (t *builder) createTableQuery(table Table) string {
	var columns []string
	for _, c := range table.Columns {
		colType := c.Type
		if c.Nullable {
			colType = fmt.Sprintf("Nullable(%s)", c.Type)
		}
		if c.Default != "" {
			colType = fmt.Sprintf("%s DEFAULT %s", colType, c.Default)
		}
		if c.Codec != "" {
			colType = fmt.Sprintf("%s CODEC(%s)", colType, c.Codec)
		}
		columns = append(columns, fmt.Sprintf("%s %s", c.Name, colType))
	}
	return fmt.Sprintf("CREATE TABLE %s (%s) ORDER BY %s;", table.Name, strings.Join(columns, ", "), table.OrderBy)
}
