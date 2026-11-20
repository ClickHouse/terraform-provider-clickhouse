package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The PATCH contract for source.postgres.mutualTls has three states: omitted
// keeps the stored client certificate, an object replaces it and an explicit
// JSON null removes it.
func TestClickPipePostgresSource_MutualTLSJSON(t *testing.T) {
	tests := []struct {
		name      string
		mutualTLS *Nullable[ClickPipePostgresMutualTLS]
		want      string
	}{
		{
			name:      "omitted keeps the stored certificate",
			mutualTLS: nil,
			want:      `{"host":"pg.example.com"}`,
		},
		{
			name:      "object replaces the stored certificate",
			mutualTLS: NullableValue(ClickPipePostgresMutualTLS{Certificate: "CERT", PrivateKey: "KEY"}),
			want:      `{"host":"pg.example.com","mutualTls":{"certificate":"CERT","privateKey":"KEY"}}`,
		},
		{
			name:      "explicit null removes the stored certificate",
			mutualTLS: NullableNull[ClickPipePostgresMutualTLS](),
			want:      `{"host":"pg.example.com","mutualTls":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(ClickPipeUpdate{
				Source: &ClickPipeSource{
					Postgres: &ClickPipePostgresSource{Host: "pg.example.com", MutualTLS: tt.mutualTLS},
				},
			})
			require.NoError(t, err)

			var update struct {
				Source struct {
					Postgres json.RawMessage `json:"postgres"`
				} `json:"source"`
			}
			require.NoError(t, json.Unmarshal(payload, &update))
			assert.JSONEq(t, tt.want, string(update.Source.Postgres))
			// Assert key presence too so the null/omitted distinction is unmistakable.
			assert.Equal(t, tt.mutualTLS != nil, containsKey(t, update.Source.Postgres, "mutualTls"))
		})
	}
}

func containsKey(t *testing.T, raw json.RawMessage, key string) bool {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	_, ok := fields[key]
	return ok
}

// GET never returns mutualTls, but decoding must tolerate every shape.
func TestClickPipePostgresSource_MutualTLSUnmarshal(t *testing.T) {
	var absent ClickPipePostgresSource
	require.NoError(t, json.Unmarshal([]byte(`{"host":"pg.example.com"}`), &absent))
	assert.Nil(t, absent.MutualTLS)

	var null ClickPipePostgresSource
	require.NoError(t, json.Unmarshal([]byte(`{"mutualTls":null}`), &null))
	assert.Nil(t, null.MutualTLS)

	var set ClickPipePostgresSource
	require.NoError(t, json.Unmarshal([]byte(`{"mutualTls":{"certificate":"CERT","privateKey":"KEY"}}`), &set))
	require.NotNil(t, set.MutualTLS)
	require.False(t, set.MutualTLS.IsNull())
	assert.Equal(t, ClickPipePostgresMutualTLS{Certificate: "CERT", PrivateKey: "KEY"}, *set.MutualTLS.Value)
}

func TestNullable_RoundTrip(t *testing.T) {
	type wrapper struct {
		Field *Nullable[string] `json:"field,omitempty"`
	}

	payload, err := json.Marshal(wrapper{Field: NullableNull[string]()})
	require.NoError(t, err)
	assert.JSONEq(t, `{"field":null}`, string(payload))
	assert.True(t, NullableNull[string]().IsNull())

	payload, err = json.Marshal(wrapper{Field: NullableValue("v")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"field":"v"}`, string(payload))

	// Unmarshalling directly into a Nullable (not through a pointer field, where
	// encoding/json short-circuits null) must also handle null.
	n := Nullable[string]{Value: new(string)}
	require.NoError(t, n.UnmarshalJSON([]byte(" null ")))
	assert.True(t, n.IsNull())
	assert.Error(t, n.UnmarshalJSON([]byte(`{"not":"a string"}`)))
}
