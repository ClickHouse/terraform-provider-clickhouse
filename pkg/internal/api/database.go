package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/huandu/go-sqlbuilder"

	sqlutil "github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/sql"
)

type Database struct {
	Name    string `json:"name"`
	Comment string `json:"comment"`
}

func (d *Database) createQuery() (string, []interface{}) {
	format := "CREATE DATABASE `$?`"
	args := []interface{}{
		sqlbuilder.Raw(sqlutil.EscapeBacktick(d.Name)),
	}

	if d.Comment != "" {
		format = fmt.Sprintf("%s COMMENT ${comment}", format)
		args = append(args, sqlbuilder.Named("comment", d.Comment))
	}
	sb := sqlbuilder.Build(format, args...)

	sql, args := sb.Build()

	return sql, args
}

func (d *Database) diffQueries(new Database) []string {
	queries := make([]string, 0)

	// There are no attributes that support being changed.

	return queries
}

func (c *ClientImpl) CreateDatabase(ctx context.Context, serviceID string, db Database) error {
	sql, args := db.createQuery()
	_, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return err
	}

	return nil
}

func (c *ClientImpl) GetDatabase(ctx context.Context, serviceID string, name string) (*Database, error) {
	sb := sqlbuilder.Build("SELECT name, comment FROM system.databases WHERE name=$?;", name)
	sql, args := sb.Build()

	body, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return nil, err
	}

	database := Database{}
	err = json.Unmarshal(body, &database)
	if err != nil {
		return nil, err
	}

	return &database, nil
}

func (c *ClientImpl) DeleteDatabase(ctx context.Context, serviceID string, name string) error {
	sb := sqlbuilder.Build("DROP DATABASE `$?`;", sqlbuilder.Raw(sqlutil.EscapeBacktick(name)))
	sql, args := sb.Build()
	_, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return err
	}

	return nil
}

func (c *ClientImpl) SyncDatabase(ctx context.Context, serviceID string, db Database) error {
	// There is no field in the Database spec that allows changing on the fly, so this function is a no-op.
	return nil
}
