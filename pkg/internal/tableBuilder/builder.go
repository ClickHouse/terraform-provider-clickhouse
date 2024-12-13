package tableBuilder

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

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
	err := t.QueryApiClient.RunQuery(ctx, table.querySpec(), nil)
	if err != nil {
		return err
	}

	return nil
}

func (t *builder) GetTable(ctx context.Context, name string) (*Table, error) {
	var srcTable struct {
		Name       string `ch:"name"`
		EngineFull string `ch:"engine_full"`
		PrimaryKey string `ch:"primary_key"`
		Comment    string `ch:"comment"`
	}

	err := t.QueryApiClient.RunQuery(
		ctx,
		fmt.Sprintf("SELECT name, engine_full, primary_key, comment FROM system.tables WHERE name='%s';", name),
		func(rows driver.Rows) error {
			if rows.Next() {
				return rows.ScanStruct(&srcTable)
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	engine, settings, err := parseEngineFull(srcTable.EngineFull)
	if err != nil {
		return nil, err
	}

	columns, err := t.getColumns(ctx, name)
	if err != nil {
		return nil, err
	}

	return &Table{
		Name:     srcTable.Name,
		Engine:   *engine,
		Columns:  columns,
		OrderBy:  srcTable.PrimaryKey,
		Comment:  srcTable.Comment,
		Settings: settings,
	}, nil
}

func (t *builder) DeleteTable(ctx context.Context, name string) error {
	return t.QueryApiClient.RunQuery(ctx, fmt.Sprintf("DROP TABLE %s", name), nil)
}

func (t *builder) SyncTable(ctx context.Context, table Table) error {
	existing, err := t.GetTable(ctx, table.Name)
	if err != nil {
		return err
	}

	for _, q := range existing.diffQueries(table) {
		err := t.QueryApiClient.RunQuery(ctx, q, nil)
		if err != nil {
			return err
		}
	}

	return nil
}
