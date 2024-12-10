package tableBuilder

import (
	"testing"
)

func TestTableBuilder_createTableQuery(t1 *testing.T) {
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
			},
			want: "CREATE TABLE tbl1 (col1 String);",
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
			},
			want: "CREATE TABLE tbl1 (col1 String, col2 UInt32);",
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &builder{}
			if got := t.createTableQuery(tt.table); got != tt.want {
				t1.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
