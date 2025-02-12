package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/huandu/go-sqlbuilder"

	sqlutil "github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/sql"
)

type GrantPrivilege struct {
	AccessType      string  `json:"access_type"`
	DatabaseName    string  `json:"database"`
	TableName       *string `json:"table"`
	ColumnName      *string `json:"column"`
	GranteeUserName *string `json:"user_name"`
	GranteeRoleName *string `json:"role_name"`
	GrantOption     bool    `json:"grant_option"`
}

func (c *ClientImpl) GrantPrivilege(ctx context.Context, serviceID string, grantPrivilege GrantPrivilege) (*GrantPrivilege, error) {
	query, args, err := grantPrivilegeQuery(grantPrivilege)
	if err != nil {
		return nil, err
	}

	sb := sqlbuilder.Build(query, args...)

	sql, args := sb.Build()

	_, err = c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return nil, err
	}

	createdGrant, err := c.GetGrantPrivilege(ctx, serviceID, grantPrivilege.AccessType, grantPrivilege.DatabaseName, grantPrivilege.TableName, grantPrivilege.ColumnName, grantPrivilege.GranteeUserName, grantPrivilege.GranteeRoleName)
	if err != nil {
		return nil, err
	}

	return createdGrant, nil
}

func (c *ClientImpl) GetGrantPrivilege(ctx context.Context, serviceID string, accessType string, database string, table *string, column *string, granteeUserName *string, granteeRoleName *string) (*GrantPrivilege, error) {
	query, args, err := getGrantPrivilegeQuery(accessType, database, table, column, granteeUserName, granteeRoleName)
	if err != nil {
		return nil, err
	}

	sb := sqlbuilder.Build(query, args...)
	sql, args := sb.Build()

	data, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		// Grant not found
		return nil, nil
	}

	grant := GrantPrivilege{}

	err = json.Unmarshal(data, &grant)
	if err != nil {
		return nil, err
	}

	if grant.GranteeUserName != nil && *grant.GranteeUserName == "" {
		grant.GranteeUserName = nil
	}
	if grant.GranteeRoleName != nil && *grant.GranteeRoleName == "" {
		grant.GranteeRoleName = nil
	}
	if grant.TableName != nil && *grant.TableName == "" {
		grant.TableName = nil
	}
	if grant.ColumnName != nil && *grant.ColumnName == "" {
		grant.ColumnName = nil
	}

	return &grant, nil
}

func (c *ClientImpl) RevokeGrantPrivilege(ctx context.Context, serviceID string, accessType string, database string, table *string, column *string, granteeUserName *string, granteeRoleName *string) error {
	query, args, err := revokePrivilegeQuery(accessType, database, table, column, granteeUserName, granteeRoleName)
	if err != nil {
		return err
	}

	sb := sqlbuilder.Build(query, args...)

	sql, args := sb.Build()

	_, err = c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return err
	}

	return nil
}

func grantPrivilegeQuery(grantPrivilege GrantPrivilege) (string, []interface{}, error) {
	query := "GRANT $?"
	args := make([]interface{}, 0)

	// Privilege
	{
		accessString := grantPrivilege.AccessType
		if grantPrivilege.ColumnName != nil {
			accessString = fmt.Sprintf("%s(%s)", accessString, *grantPrivilege.ColumnName)
		}

		args = append(args, sqlbuilder.Raw(accessString))
	}

	// Target database/table
	{
		args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(grantPrivilege.DatabaseName)))

		if grantPrivilege.TableName == nil {
			query = fmt.Sprintf("%s ON `$?`.*", query)
		} else {
			query = fmt.Sprintf("%s ON `$?`.`$?`", query)
			args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*grantPrivilege.TableName)))
		}
	}

	// Grantee
	{
		query = fmt.Sprintf("%s TO `$?`", query)

		if grantPrivilege.GranteeUserName != nil {
			args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*grantPrivilege.GranteeUserName)))
		} else if grantPrivilege.GranteeRoleName != nil {
			args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*grantPrivilege.GranteeRoleName)))
		} else {
			return "", nil, fmt.Errorf("either GranteeUserName or GranteeRoleName must be set")
		}
	}

	// Grant option
	{
		if grantPrivilege.GrantOption {
			query = fmt.Sprintf("%s WITH GRANT OPTION", query)
		}
	}

	return query, args, nil
}

func getGrantPrivilegeQuery(accessType string, database string, table *string, column *string, granteeUserName *string, granteeRoleName *string) (string, []interface{}, error) {
	query := "SELECT access_type, database, table, column, user_name, role_name, toBool(grant_option) as grant_option FROM system.grants WHERE access_type = ${access_type} AND database = ${database}"
	args := []interface{}{
		sqlbuilder.Named("access_type", accessType),
		sqlbuilder.Named("database", database),
	}

	if table != nil {
		query = fmt.Sprintf("%s AND table = ${table}", query)
		args = append(args, sqlbuilder.Named("table", *table))
	}

	if column != nil {
		query = fmt.Sprintf("%s AND column = ${column}", query)
		args = append(args, sqlbuilder.Named("column", *column))
	}

	if granteeUserName != nil {
		query = fmt.Sprintf("%s AND user_name = ${value}", query)
		args = append(args, sqlbuilder.Named("value", *granteeUserName))
	} else if granteeRoleName != nil {
		query = fmt.Sprintf("%s AND role_name = ${value}", query)
		args = append(args, sqlbuilder.Named("value", *granteeRoleName))
	} else {
		return "", nil, fmt.Errorf("either GranteeUserName or GranteeRoleName must be set")
	}

	return query, args, nil
}

func revokePrivilegeQuery(accessType string, database string, table *string, column *string, granteeUserName *string, granteeRoleName *string) (string, []interface{}, error) {
	query := "REVOKE $?"
	args := make([]interface{}, 0)

	// Privilege
	{
		accessString := accessType
		if column != nil {
			accessString = fmt.Sprintf("%s(%s)", accessString, *column)
		}

		args = append(args, sqlbuilder.Raw(accessString))
	}

	// Target database/table
	{
		args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(database)))

		if table == nil {
			query = fmt.Sprintf("%s ON `$?`.*", query)
		} else {
			query = fmt.Sprintf("%s ON `$?`.`$?`", query)
			args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*table)))
		}
	}

	// Grantee
	{
		query = fmt.Sprintf("%s FROM `$?`", query)

		if granteeUserName != nil {
			args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*granteeUserName)))
		} else if granteeRoleName != nil {
			args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*granteeRoleName)))
		} else {
			return "", nil, fmt.Errorf("either GranteeUserName or GranteeRoleName must be set")
		}
	}

	return query, args, nil
}
