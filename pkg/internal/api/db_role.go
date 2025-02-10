package api

import (
	"context"
	"encoding/json"

	"github.com/huandu/go-sqlbuilder"

	sqlutil "github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/sql"
)

type Role struct {
	Name string `json:"name"`
}

func (c *ClientImpl) CreateRole(ctx context.Context, serviceID string, role Role) (*Role, error) {
	format := "CREATE ROLE `$?`"
	args := []interface{}{
		sqlbuilder.Raw(sqlutil.EscapeBacktick(role.Name)),
	}

	sb := sqlbuilder.Build(format, args...)

	sql, args := sb.Build()

	_, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return nil, err
	}

	createdRole, err := c.GetRole(ctx, serviceID, role.Name)
	if err != nil {
		return nil, err
	}

	return createdRole, nil
}

func (c *ClientImpl) GetRole(ctx context.Context, serviceID string, name string) (*Role, error) {
	// Roles we create with terraform are by default created with the 'replicated' storage thus we filter the
	// select query to ensure we're not retrieving another role with the same name and a different storage type.
	format := "SELECT name FROM system.roles WHERE name = ${name} and storage = 'replicated'"
	args := []interface{}{
		sqlbuilder.Named("name", name),
	}

	sb := sqlbuilder.Build(format, args...)

	sql, args := sb.Build()

	data, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		// Role not found
		return nil, nil
	}

	role := Role{}

	err = json.Unmarshal(data, &role)
	if err != nil {
		return nil, err
	}

	return &role, nil
}

func (c *ClientImpl) DeleteRole(ctx context.Context, serviceID string, name string) error {
	format := "DROP ROLE IF EXISTS `$?`"
	args := []interface{}{
		sqlbuilder.Raw(sqlutil.EscapeBacktick(name)),
	}

	sb := sqlbuilder.Build(format, args...)

	sql, args := sb.Build()

	_, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return err
	}

	return nil
}
