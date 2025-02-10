package api

import (
	"context"
	"encoding/json"

	"github.com/huandu/go-sqlbuilder"

	sqlutil "github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/sql"
)

type User struct {
	Name               string `json:"name"`
	PasswordSha256Hash string `json:"-"`
}

func (c *ClientImpl) CreateUser(ctx context.Context, serviceID string, user User) (*User, error) {
	format := "CREATE USER `$?` IDENTIFIED WITH sha256_hash BY ${hash}"
	args := []interface{}{
		sqlbuilder.Raw(sqlutil.EscapeBacktick(user.Name)),
		sqlbuilder.Named("hash", user.PasswordSha256Hash),
	}

	sb := sqlbuilder.Build(format, args...)

	sql, args := sb.Build()

	_, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return nil, err
	}

	createdUser, err := c.GetUser(ctx, serviceID, user.Name)
	if err != nil {
		return nil, err
	}

	// We don't get the SHA back from clickhouse, so we assume the operation was successful and return it to the client.
	createdUser.PasswordSha256Hash = user.PasswordSha256Hash

	return createdUser, nil
}

func (c *ClientImpl) GetUser(ctx context.Context, serviceID string, name string) (*User, error) {
	// Users we create with terraform are by default created with the 'replicated' storage thus we filter the
	// select query to ensure we're not retrieving another user with the same username and a different storage type.
	format := "SELECT name FROM system.users WHERE name = ${name} and storage = 'replicated'"
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
		// User not found
		return nil, nil
	}

	user := User{}

	err = json.Unmarshal(data, &user)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (c *ClientImpl) DeleteUser(ctx context.Context, serviceID string, name string) error {
	format := "DROP USER IF EXISTS `$?`"
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
