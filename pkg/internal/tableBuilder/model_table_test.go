package tableBuilder

import (
	"testing"
)

func TestTable_querySpec(t1 *testing.T) {
	tests := []struct {
		name  string
		table Table
		want  string
	}{
		{
			name: "Simple case",
			table: Table{
				Name: "tbl1",
				Columns: []Column{
					{
						Name: "col1",
						Type: "String",
					},
				},
				OrderBy: "col1",
			},
			want: "CREATE TABLE tbl1 (col1 String) ORDER BY col1;",
		},
		{
			name: "Two columns",
			table: Table{
				Name: "tbl1",
				Columns: []Column{
					{
						Name: "col1",
						Type: "String",
					},
					{
						Name: "col2",
						Type: "UInt32",
					},
				},
				OrderBy: "col1",
			},
			want: "CREATE TABLE tbl1 (col1 String, col2 UInt32) ORDER BY col1;",
		},
		{
			name: "Nullable column",
			table: Table{
				Name: "tbl1",
				Columns: []Column{
					{
						Name:     "col1",
						Type:     "String",
						Nullable: true,
					},
				},
				OrderBy: "col1",
			},
			want: "CREATE TABLE tbl1 (col1 Nullable(String)) ORDER BY col1;",
		},
		{
			name: "Default for column",
			table: Table{
				Name: "tbl1",
				Columns: []Column{
					{
						Name:    "col1",
						Type:    "String",
						Default: ptr("def1"),
					},
				},
				OrderBy: "col1",
			},
			want: "CREATE TABLE tbl1 (col1 String DEFAULT def1) ORDER BY col1;",
		},
		{
			name: "Comment for column",
			table: Table{
				Name: "tbl1",
				Columns: []Column{
					{
						Name:    "col1",
						Type:    "String",
						Comment: ptr("comm1"),
					},
				},
				OrderBy: "col1",
			},
			want: "CREATE TABLE tbl1 (col1 String COMMENT 'comm1') ORDER BY col1;",
		},
		{
			name: "Settings",
			table: Table{
				Name: "tbl1",
				Columns: []Column{
					{
						Name: "col1",
						Type: "String",
					},
				},
				OrderBy: "col1",
				Settings: map[string]string{
					"sett1": "123",
				},
			},
			want: "CREATE TABLE tbl1 (col1 String) ORDER BY col1 SETTINGS sett1=123;",
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			if got := tt.table.querySpec(); got != tt.want {
				t1.Errorf("querySpec() = %v, want %v", got, tt.want)
			}
		})
	}
}
