package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClickPipeDestinationTableDefinition_TTLJSON(t *testing.T) {
	ttl := "toDateTime(event_time) + INTERVAL 30 DAY"

	payload, err := json.Marshal(ClickPipeDestinationTableDefinition{
		Engine: ClickPipeDestinationTableEngine{Type: "MergeTree"},
		TTL:    &ttl,
	})

	require.NoError(t, err)
	var definition map[string]any
	require.NoError(t, json.Unmarshal(payload, &definition))
	assert.Equal(t, ttl, definition["ttl"])
}

func TestClickPipeDestinationTableDefinition_OmitsUnsetTTL(t *testing.T) {
	payload, err := json.Marshal(ClickPipeDestinationTableDefinition{
		Engine: ClickPipeDestinationTableEngine{Type: "MergeTree"},
	})

	require.NoError(t, err)
	assert.NotContains(t, string(payload), "ttl")
}
