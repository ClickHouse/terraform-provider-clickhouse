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

func TestClickPipeKinesisSource_ProtobufSchemaJSON(t *testing.T) {
	protobufSchema := "c3ludGF4ID0gInByb3RvMyI7"

	payload, err := json.Marshal(ClickPipeKinesisSource{ProtobufSchema: &protobufSchema})

	require.NoError(t, err)
	var source map[string]any
	require.NoError(t, json.Unmarshal(payload, &source))
	assert.Equal(t, protobufSchema, source["protobufSchema"])
}

func TestClickPipeKinesisSource_OmitsUnsetProtobufSchema(t *testing.T) {
	payload, err := json.Marshal(ClickPipeKinesisSource{})

	require.NoError(t, err)
	assert.NotContains(t, string(payload), "protobufSchema")
}

func TestClickPipeKinesisSource_SchemaRegistryJSON(t *testing.T) {
	roleArn := "arn:aws:iam::123456789012:role/GlueRegistryAccess"
	payload, err := json.Marshal(ClickPipeKinesisSource{
		Format: ClickPipeAvroConfluentFormat,
		SchemaRegistry: &ClickPipeKinesisSchemaRegistry{
			Type:             ClickPipeKinesisSchemaRegistryTypeGlue,
			GlueRegion:       "us-east-1",
			GlueRegistryName: "orders-registry",
			GlueRoleArn:      &roleArn,
		},
	})
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"schemaRegistry":{"type":"glue","glueRegion":"us-east-1","glueRegistryName":"orders-registry","glueRoleArn":"`+roleArn+`"}`)
}

func TestClickPipeKinesisSource_OmitsUnsetSchemaRegistryAndRole(t *testing.T) {
	payload, err := json.Marshal(ClickPipeKinesisSource{Format: ClickPipeJSONEachRowFormat})
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "schemaRegistry")

	payload, err = json.Marshal(ClickPipeKinesisSchemaRegistry{Type: ClickPipeKinesisSchemaRegistryTypeGlue, GlueRegion: "us-east-1", GlueRegistryName: "orders-registry"})
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "glueRoleArn")
}
