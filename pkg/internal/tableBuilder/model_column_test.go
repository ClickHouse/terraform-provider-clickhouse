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
				Default: "3",
			},
			want: "id UInt32 DEFAULT 3",
		},
		{
			name: "Materialized",
			Column: Column{
				Name:         "id",
				Type:         "UInt32",
				Materialized: "3",
			},
			want: "id UInt32 MATERIALIZED 3",
		},
		{
			name: "Alias",
			Column: Column{
				Name:  "id",
				Type:  "UInt32",
				Alias: "other_field + 1",
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
			name: "Codec",
			Column: Column{
				Name:  "id",
				Type:  "UInt32",
				Codec: "TEST",
			},
			want: "id UInt32 CODEC(TEST)",
		},
		{
			name: "TTL",
			Column: Column{
				Name: "id",
				Type: "UInt32",
				TTL: &TTL{
					TimeColumn: "ts",
					Interval:   "1 hour",
				},
			},
			want: "id UInt32 TTL ts + INTERVAL 1 hour",
		},
		{
			name: "Comment",
			Column: Column{
				Name:    "id",
				Type:    "UInt32",
				Comment: "comment",
			},
			want: "id UInt32 COMMENT 'comment'",
		},
		{
			name: "Comment and Codec",
			Column: Column{
				Name:    "id",
				Type:    "UInt32",
				Comment: "comment",
				Codec:   "codec",
			},
			want: "id UInt32 COMMENT 'comment' CODEC(codec)",
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
