package tableBuilder

import (
	"testing"
)

func TestColumn_querySpec(t *testing.T) {
	tests := []struct {
		name   string
		Column Column
		want   string
	}{
		{
			name: "Simple",
			Column: Column{
				Name: "id",
				Type: "UInt32",
			},
			want: "id UInt32",
		},
		{
			name: "Nullable",
			Column: Column{
				Name:     "id",
				Type:     "UInt32",
				Nullable: true,
			},
			want: "id Nullable(UInt32)",
		},
		{
			name: "Default",
			Column: Column{
				Name:    "id",
				Type:    "UInt32",
				Default: ptr("3"),
			},
			want: "id UInt32 DEFAULT 3",
		},
		{
			name: "Materialized",
			Column: Column{
				Name:         "id",
				Type:         "UInt32",
				Materialized: ptr("3"),
			},
			want: "id UInt32 MATERIALIZED 3",
		},
		{
			name: "Alias",
			Column: Column{
				Name:  "id",
				Type:  "UInt32",
				Alias: ptr("other_field + 1"),
			},
			want: "id UInt32 ALIAS other_field + 1",
		},
		{
			name: "Ephemeral",
			Column: Column{
				Name:      "id",
				Type:      "UInt32",
				Ephemeral: true,
			},
			want: "id UInt32 EPHEMERAL",
		},
		{
			name: "Comment",
			Column: Column{
				Name:    "id",
				Type:    "UInt32",
				Comment: ptr("comment"),
			},
			want: "id UInt32 COMMENT 'comment'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.Column.querySpec(); got != tt.want {
				t.Errorf("querySpec() = %v, want %v", got, tt.want)
			}
		})
	}
}

func ptr[T any](val T) *T {
	return &val
}
