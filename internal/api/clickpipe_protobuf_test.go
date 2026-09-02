package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClickPipeKafkaSource_ProtobufSchemaJSON(t *testing.T) {
	protobufSchema := "c3ludGF4ID0gXCJwcm90bzNcIjs="

	payload, err := json.Marshal(ClickPipeKafkaSource{ProtobufSchema: &protobufSchema})

	require.NoError(t, err)
	assert.JSONEq(t, `{"protobufSchema":"c3ludGF4ID0gXCJwcm90bzNcIjs="}`, string(payload))
}

func TestClickPipeKafkaSource_OmitsUnsetProtobufSchema(t *testing.T) {
	payload, err := json.Marshal(ClickPipeKafkaSource{})

	require.NoError(t, err)
	assert.NotContains(t, string(payload), "protobufSchema")
}
