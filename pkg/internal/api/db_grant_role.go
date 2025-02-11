package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/huandu/go-sqlbuilder"

	sqlutil "github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/sql"
)

type GrantRole struct {
	RoleName        string  `json:"granted_role_name"`
	GranteeUserName *string `json:"user_name"`
	GranteeRoleName *string `json:"role_name"`
	AdminOption     bool    `json:"with_admin_option"`
}

func (c *ClientImpl) GrantRole(ctx context.Context, serviceID string, grantRole GrantRole) (*GrantRole, error) {
	format := "GRANT `$?` TO `$?`"
	if grantRole.AdminOption {
		format = fmt.Sprintf("%s WITH ADMIN OPTION", format)
	}
	args := []interface{}{
		sqlbuilder.Raw(sqlutil.EscapeBacktick(grantRole.RoleName)),
	}

	if grantRole.GranteeUserName != nil {
		args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*grantRole.GranteeUserName)))
	} else if grantRole.GranteeRoleName != nil {
		args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*grantRole.GranteeRoleName)))
	} else {
		return nil, fmt.Errorf("either GranteeUserName or GranteeRoleName must be set")
	}

	sb := sqlbuilder.Build(format, args...)

	sql, args := sb.Build()

	_, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return nil, err
	}

	createdGrant, err := c.GetGrantRole(ctx, serviceID, grantRole.RoleName, grantRole.GranteeUserName, grantRole.GranteeRoleName)
	if err != nil {
		return nil, err
	}

	return createdGrant, nil
}

func (c *ClientImpl) GetGrantRole(ctx context.Context, serviceID string, grantedRoleName string, granteeUserName *string, granteeRoleName *string) (*GrantRole, error) {
	var fieldName, fieldValue string
	if granteeUserName != nil {
		fieldName = "user_name"
		fieldValue = *granteeUserName
	} else if granteeRoleName != nil {
		fieldName = "role_name"
		fieldValue = *granteeRoleName
	} else {
		return nil, fmt.Errorf("either GranteeUserName or GranteeRoleName must be set")
	}

	format := fmt.Sprintf("SELECT granted_role_name,user_name,role_name,toBool(with_admin_option) as with_admin_option FROM system.role_grants WHERE granted_role_name = ${granted_role_name} and %s = ${field_value}", fieldName)
	args := []interface{}{
		sqlbuilder.Named("granted_role_name", grantedRoleName),
		sqlbuilder.Named("field_value", fieldValue),
	}

	sb := sqlbuilder.Build(format, args...)

	sql, args := sb.Build()

	data, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		// Grant not found
		return nil, nil
	}

	grant := GrantRole{}

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

	return &grant, nil
}

func (c *ClientImpl) RevokeGrantRole(ctx context.Context, serviceID string, grantedRoleName string, granteeUserName *string, granteeRoleName *string) error {
	format := "REVOKE `$?` FROM `$?`"
	args := []interface{}{
		sqlbuilder.Raw(sqlutil.EscapeBacktick(grantedRoleName)),
	}

	if granteeUserName != nil {
		args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*granteeUserName)))
	} else if granteeRoleName != nil {
		args = append(args, sqlbuilder.Raw(sqlutil.EscapeBacktick(*granteeRoleName)))
	} else {
		return fmt.Errorf("either GranteeUserName or GranteeRoleName must be set")
	}

	sb := sqlbuilder.Build(format, args...)

	sql, args := sb.Build()

	_, err := c.runQuery(ctx, serviceID, sql, args...)
	if err != nil {
		return err
	}

	return nil
}
