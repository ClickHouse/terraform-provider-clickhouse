package tableBuilder

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func (t *builder) getColumns(ctx context.Context, tableName string) ([]Column, error) {
	type chColumn struct {
		Name              string `ch:"name"`
		Type              string `ch:"type"`
		DefaultKind       string `ch:"default_type"`
		DefaultExpression string `ch:"default_expression"`
		Comment           string `ch:"comment"`
		TTL               string `ch:"ttl_expression"`
		Codec             string `ch:"codec_expression"`
	}
	columns := make([]Column, 0)

	err := t.QueryApiClient.RunQuery(ctx, fmt.Sprintf("DESCRIBE TABLE %s;", tableName),
		func(rows driver.Rows) error {
			for rows.Next() {
				var col chColumn
				err := rows.ScanStruct(&col)
				if err != nil {
					return err
				}

				column := Column{
					Name: col.Name,
				}
				if strings.HasPrefix(col.Type, "Nullable") {
					column.Nullable = true
					column.Type = strings.TrimSuffix(strings.TrimPrefix(col.Type, "Nullable("), ")")
				} else {
					column.Type = col.Type
				}

				if col.Comment != "" {
					column.Comment = &col.Comment
				}

				var defaultExpression *string
				if col.DefaultExpression != "" {
					defaultExpression = &col.DefaultExpression
				}

				switch col.DefaultKind {
				case "ALIAS":
					column.Alias = defaultExpression
				case "DEFAULT":
					column.Default = defaultExpression
				case "EPHEMERAL":
					column.Ephemeral = true
				case "MATERIALIZED":
					column.Materialized = defaultExpression
				}

				if err != nil {
					return err
				}

				columns = append(columns, column)
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	return columns, nil
}
