package tableBuilder

import (
	"reflect"
	"testing"
)

func Test_parseEngineFull(t *testing.T) {
	tests := []struct {
		name         string
		engineFull   string
		wantEngine   *Engine
		wantSettings map[string]string
		wantErr      bool
	}{
		{
			name:       "Full test",
			engineFull: "CollapsingMergeTree(sign) ORDER BY id SETTINGS index_granularity = 1024, test = true",
			wantEngine: &Engine{
				Name:   "CollapsingMergeTree",
				Params: []string{"sign"},
			},
			wantSettings: map[string]string{
				"index_granularity": "1024",
				"test":              "true",
			},
			wantErr: false,
		},
		{
			name:       "No settings",
			engineFull: "CollapsingMergeTree(sign) ORDER BY id",
			wantEngine: &Engine{
				Name:   "CollapsingMergeTree",
				Params: []string{"sign"},
			},
			wantSettings: nil,
			wantErr:      false,
		},
		{
			name:       "No params",
			engineFull: "MergeTree ORDER BY id",
			wantEngine: &Engine{
				Name:   "MergeTree",
				Params: nil,
			},
			wantSettings: nil,
			wantErr:      false,
		},
		{
			name:       "Multiple params",
			engineFull: "MergeTree(one, two, three) ORDER BY id",
			wantEngine: &Engine{
				Name:   "MergeTree",
				Params: []string{"one", "two", "three"},
			},
			wantSettings: nil,
			wantErr:      false,
		},
		{
			name:         "No order by",
			engineFull:   "MergeTree(one, two, three)",
			wantEngine:   nil,
			wantSettings: nil,
			wantErr:      true,
		},
		{
			name:         "Invalid settings",
			engineFull:   "MergeTree ORDER BY id SETTINGS wrong",
			wantEngine:   nil,
			wantSettings: nil,
			wantErr:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := parseEngineFull(tt.engineFull)
			if (err != nil) != tt.wantErr {
				t.Errorf("error got = %v, want %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.wantEngine) {
				t.Errorf("Engine got = %v, want %v", got, tt.wantEngine)
			}
			if !reflect.DeepEqual(got1, tt.wantSettings) {
				t.Errorf("Settings got = %v, want %v", got1, tt.wantSettings)
			}
		})
	}
}
