package resource

import (
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
)

// isClickPipeStoppedState gates the pause-before-edit for CDC table_mappings
// changes and is the state we wait for. It must recognize only the terminal
// paused states (Stopped/Paused), not transitional (Stopping/Pausing) or active
// states, or an edit could be issued against a pipe that is not yet editable.
func TestIsClickPipeStoppedState(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		state    string
		expected bool
	}{
		"stopped":               {state: api.ClickPipeStoppedState, expected: true},
		"paused":                {state: api.ClickPipePausedState, expected: true},
		"running":               {state: api.ClickPipeRunningState, expected: false},
		"stopping-transitional": {state: api.ClickPipeStoppingState, expected: false},
		"pausing-transitional":  {state: api.ClickPipePausingState, expected: false},
		"snapshot":              {state: api.ClickPipeSnapShotState, expected: false},
		"provisioning":          {state: api.ClickPipeProvisioningState, expected: false},
		"failed":                {state: api.ClickPipeFailedState, expected: false},
		"empty":                 {state: "", expected: false},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := isClickPipeStoppedState(tc.state); got != tc.expected {
				t.Errorf("isClickPipeStoppedState(%q) = %v, want %v", tc.state, got, tc.expected)
			}
		})
	}
}
