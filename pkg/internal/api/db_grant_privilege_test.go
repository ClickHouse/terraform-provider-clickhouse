package api

import (
	"reflect"
	"testing"

	"github.com/huandu/go-sqlbuilder"
)

func Test_grantPrivilegeQuery(t *testing.T) {
	tests := []struct {
		name           string
		grantPrivilege GrantPrivilege
		query          string
		args           []interface{}
		wantErr        bool
	}{
		{
			name: "ALL on all tables in default to user",
			grantPrivilege: GrantPrivilege{
				AccessType:      "ALL",
				DatabaseName:    toStrPtr("default"),
				TableName:       nil,
				ColumnName:      nil,
				GranteeUserName: toStrPtr("john"),
				GranteeRoleName: nil,
				GrantOption:     false,
			},
			query:   "GRANT ALL ON `default`.* TO `john`",
			args:    nil,
			wantErr: false,
		},
		{
			name: "ALL on all tables in default to role",
			grantPrivilege: GrantPrivilege{
				AccessType:      "ALL",
				DatabaseName:    toStrPtr("default"),
				TableName:       nil,
				ColumnName:      nil,
				GranteeUserName: nil,
				GranteeRoleName: toStrPtr("writer"),
				GrantOption:     false,
			},
			query:   "GRANT ALL ON `default`.* TO `writer`",
			args:    nil,
			wantErr: false,
		},
		{
			name: "ALL on specific table in default to user",
			grantPrivilege: GrantPrivilege{
				AccessType:      "ALL",
				DatabaseName:    toStrPtr("default"),
				TableName:       toStrPtr("tbl1"),
				ColumnName:      nil,
				GranteeUserName: toStrPtr("john"),
				GranteeRoleName: nil,
				GrantOption:     false,
			},
			query:   "GRANT ALL ON `default`.`tbl1` TO `john`",
			args:    nil,
			wantErr: false,
		},
		{
			name: "SELECT on specific column and specific table in default to user",
			grantPrivilege: GrantPrivilege{
				AccessType:      "SELECT",
				DatabaseName:    toStrPtr("default"),
				TableName:       toStrPtr("tbl1"),
				ColumnName:      toStrPtr("col1"),
				GranteeUserName: toStrPtr("john"),
				GranteeRoleName: nil,
				GrantOption:     false,
			},
			query:   "GRANT SELECT(col1) ON `default`.`tbl1` TO `john`",
			args:    nil,
			wantErr: false,
		},
		{
			name: "UPDATE TABLE on specific column and specific table in default to user",
			grantPrivilege: GrantPrivilege{
				AccessType:      "UPDATE TABLE",
				DatabaseName:    toStrPtr("default"),
				TableName:       toStrPtr("tbl1"),
				ColumnName:      toStrPtr("col1"),
				GranteeUserName: toStrPtr("john"),
				GranteeRoleName: nil,
				GrantOption:     false,
			},
			query:   "GRANT UPDATE TABLE(col1) ON `default`.`tbl1` TO `john`",
			args:    nil,
			wantErr: false,
		},
		{
			name: "ALL on specific column and specific table in default to user with grant option",
			grantPrivilege: GrantPrivilege{
				AccessType:      "ALL",
				DatabaseName:    toStrPtr("default"),
				TableName:       toStrPtr("tbl1"),
				ColumnName:      nil,
				GranteeUserName: toStrPtr("john"),
				GranteeRoleName: nil,
				GrantOption:     true,
			},
			query:   "GRANT ALL ON `default`.`tbl1` TO `john` WITH GRANT OPTION",
			args:    nil,
			wantErr: false,
		},
		{
			name: "No user nor role set",
			grantPrivilege: GrantPrivilege{
				AccessType:      "ALL",
				DatabaseName:    toStrPtr("default"),
				TableName:       nil,
				ColumnName:      nil,
				GranteeUserName: nil,
				GranteeRoleName: nil,
				GrantOption:     false,
			},
			query:   "",
			args:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, args, err := grantPrivilegeQuery(tt.grantPrivilege)

			sb := sqlbuilder.Build(query, args...)

			query, args = sb.Build()

			if (err != nil) != tt.wantErr {
				t.Errorf("grantPrivilegeQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if query != tt.query {
				t.Errorf("Query was %q, want %q", query, tt.query)
			}
			if !reflect.DeepEqual(args, tt.args) {
				t.Errorf("args were %v, want %v", args, tt.args)
			}
		})
	}
}

func Test_getGrantPrivilegeQuery(t *testing.T) {
	tests := []struct {
		name            string
		accessType      string
		database        *string
		table           *string
		column          *string
		granteeUserName *string
		granteeRoleName *string
		query           string
		args            []interface{}
		wantErr         bool
	}{
		{
			name:            "ALL on all tables in default to user",
			accessType:      "ALL",
			database:        toStrPtr("default"),
			table:           nil,
			column:          nil,
			granteeUserName: toStrPtr("john"),
			granteeRoleName: nil,
			query:           "SELECT access_type, database, table, column, user_name, role_name, toBool(grant_option) as grant_option FROM system.grants WHERE access_type = ? AND database = ? AND table IS NULL AND column IS NULL AND user_name = ?",
			args:            []interface{}{"ALL", "default", "john"},
			wantErr:         false,
		},
		{
			name:            "ALL on all tables in default to role",
			accessType:      "ALL",
			database:        toStrPtr("default"),
			table:           nil,
			column:          nil,
			granteeUserName: nil,
			granteeRoleName: toStrPtr("writer"),
			query:           "SELECT access_type, database, table, column, user_name, role_name, toBool(grant_option) as grant_option FROM system.grants WHERE access_type = ? AND database = ? AND table IS NULL AND column IS NULL AND role_name = ?",
			args:            []interface{}{"ALL", "default", "writer"},
			wantErr:         false,
		},
		{
			name:            "ALL on specific table in default to user",
			accessType:      "ALL",
			database:        toStrPtr("default"),
			table:           toStrPtr("tbl1"),
			column:          nil,
			granteeUserName: toStrPtr("john"),
			granteeRoleName: nil,
			query:           "SELECT access_type, database, table, column, user_name, role_name, toBool(grant_option) as grant_option FROM system.grants WHERE access_type = ? AND database = ? AND table = ? AND column IS NULL AND user_name = ?",
			args:            []interface{}{"ALL", "default", "tbl1", "john"},
			wantErr:         false,
		},
		{
			name:            "SELECT on specific column and specific table in default to user",
			accessType:      "SELECT",
			database:        toStrPtr("default"),
			table:           toStrPtr("tbl1"),
			column:          toStrPtr("col1"),
			granteeUserName: toStrPtr("john"),
			granteeRoleName: nil,
			query:           "SELECT access_type, database, table, column, user_name, role_name, toBool(grant_option) as grant_option FROM system.grants WHERE access_type = ? AND database = ? AND table = ? AND column = ? AND user_name = ?",
			args:            []interface{}{"SELECT", "default", "tbl1", "col1", "john"},
			wantErr:         false,
		},
		{
			name:            "UPDATE TABLE on specific column and specific table in default to user",
			accessType:      "UPDATE TABLE",
			database:        toStrPtr("default"),
			table:           toStrPtr("tbl1"),
			column:          toStrPtr("col1"),
			granteeUserName: toStrPtr("john"),
			granteeRoleName: nil,
			query:           "SELECT access_type, database, table, column, user_name, role_name, toBool(grant_option) as grant_option FROM system.grants WHERE access_type = ? AND database = ? AND table = ? AND column = ? AND user_name = ?",
			args:            []interface{}{"UPDATE TABLE", "default", "tbl1", "col1", "john"},
			wantErr:         false,
		},
		{
			name:            "No user nor role set",
			accessType:      "ALL",
			database:        toStrPtr("default"),
			table:           nil,
			column:          nil,
			granteeUserName: nil,
			granteeRoleName: nil,
			query:           "",
			args:            nil,
			wantErr:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, args, err := getGrantPrivilegeQuery(tt.accessType, tt.database, tt.table, tt.column, tt.granteeUserName, tt.granteeRoleName)

			sb := sqlbuilder.Build(query, args...)

			query, args = sb.Build()

			if (err != nil) != tt.wantErr {
				t.Errorf("getGrantPrivilegeQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if query != tt.query {
				t.Errorf("Query was %q, want %q", query, tt.query)
			}
			if !reflect.DeepEqual(args, tt.args) {
				t.Errorf("args were %v, want %v", args, tt.args)
			}
		})
	}
}

func Test_revokeGrantPrivilegeQuery(t *testing.T) {
	tests := []struct {
		name            string
		accessType      string
		database        *string
		table           *string
		column          *string
		granteeUserName *string
		granteeRoleName *string
		query           string
		args            []interface{}
		wantErr         bool
	}{
		{
			name:            "ALL on all tables in default to user",
			accessType:      "ALL",
			database:        toStrPtr("default"),
			table:           nil,
			column:          nil,
			granteeUserName: toStrPtr("john"),
			granteeRoleName: nil,
			query:           "REVOKE ALL ON `default`.* FROM `john`",
			args:            nil,
			wantErr:         false,
		},
		{
			name:            "ALL on all tables in default to role",
			accessType:      "ALL",
			database:        toStrPtr("default"),
			table:           nil,
			column:          nil,
			granteeUserName: nil,
			granteeRoleName: toStrPtr("writer"),
			query:           "REVOKE ALL ON `default`.* FROM `writer`",
			args:            nil,
			wantErr:         false,
		},
		{
			name:            "ALL on specific table in default to user",
			accessType:      "ALL",
			database:        toStrPtr("default"),
			table:           toStrPtr("tbl1"),
			column:          nil,
			granteeUserName: toStrPtr("john"),
			granteeRoleName: nil,
			query:           "REVOKE ALL ON `default`.`tbl1` FROM `john`",
			args:            nil,
			wantErr:         false,
		},
		{
			name:            "SELECT on specific column and specific table in default to user",
			accessType:      "SELECT",
			database:        toStrPtr("default"),
			table:           toStrPtr("tbl1"),
			column:          toStrPtr("col1"),
			granteeUserName: toStrPtr("john"),
			granteeRoleName: nil,
			query:           "REVOKE SELECT(col1) ON `default`.`tbl1` FROM `john`",
			args:            nil,
			wantErr:         false,
		},
		{
			name:            "UPDATE TABLE on specific column and specific table in default to user",
			accessType:      "UPDATE TABLE",
			database:        toStrPtr("default"),
			table:           toStrPtr("tbl1"),
			column:          toStrPtr("col1"),
			granteeUserName: toStrPtr("john"),
			granteeRoleName: nil,
			query:           "REVOKE UPDATE TABLE(col1) ON `default`.`tbl1` FROM `john`",
			args:            nil,
			wantErr:         false,
		},
		{
			name:            "No user nor role set",
			accessType:      "ALL",
			database:        toStrPtr("default"),
			table:           nil,
			column:          nil,
			granteeUserName: nil,
			granteeRoleName: nil,
			query:           "",
			args:            nil,
			wantErr:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, args, err := revokePrivilegeQuery(tt.accessType, tt.database, tt.table, tt.column, tt.granteeUserName, tt.granteeRoleName)

			sb := sqlbuilder.Build(query, args...)

			query, args = sb.Build()

			if (err != nil) != tt.wantErr {
				t.Errorf("revokePrivilegeQuery() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if query != tt.query {
				t.Errorf("Query was %q, want %q", query, tt.query)
			}
			if !reflect.DeepEqual(args, tt.args) {
				t.Errorf("args were %v, want %v", args, tt.args)
			}
		})
	}
}

func toStrPtr(s string) *string {
	return &s
}
