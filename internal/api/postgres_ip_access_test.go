package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostgresCreateIpAccessEncoding(t *testing.T) {
	for _, tc := range []struct {
		name string
		list *[]PostgresIpAccess
		want string
	}{
		{"omitted", nil, ""},
		{"empty", &[]PostgresIpAccess{}, "[]"},
		{
			"configured", &[]PostgresIpAccess{{Source: "192.0.2.1", PortRange: "5432", Description: ""}},
			`[{"source":"192.0.2.1","portRange":"5432","description":""}]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(PostgresCreate{IpAccessList: tc.list})
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &fields))
			require.Equal(t, tc.want, string(fields["ipAccessList"]))
		})
	}
}

func TestPostgresIpAccessReadAndPatch(t *testing.T) {
	rules := []PostgresIpAccess{
		{Id: "fr-original", Source: "192.0.2.1/32", PortRange: "5432", Description: "Office"},
	}
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, testPostgresInstancePath, r.URL.Path)
		if r.Method == http.MethodPatch {
			var body map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Len(t, body, 1)
			require.JSONEq(t, `{"add":[{"id":"fr-original","source":"192.0.2.1/32","portRange":"5432","description":""}],"remove":[{"id":"fr-other"}]}`, string(body["ipAccessList"]))
			rules[0].Description = ""
		} else {
			require.Equal(t, http.MethodGet, r.Method)
		}
		require.NoError(t, json.NewEncoder(w).Encode(ResponseWithResult[Postgres]{
			Result: Postgres{Id: testPostgresID, IpAccessList: rules},
		}))
	})

	got, err := client.GetPostgres(context.Background(), testPostgresID)
	require.NoError(t, err)
	require.Equal(t, rules, got.IpAccessList)
	updated := rules[0]
	updated.Description = ""
	got, err = client.UpdatePostgres(context.Background(), testPostgresID, PostgresUpdate{
		IpAccessList: &PostgresIpAccessUpdate{
			Add: []PostgresIpAccess{updated}, Remove: []PostgresIpAccessRuleReference{{Id: "fr-other"}},
		},
	})
	require.NoError(t, err)
	require.Empty(t, got.IpAccessList[0].Description)
}

func TestPostgresIpAccessMissingVersusEmpty(t *testing.T) {
	for _, body := range []string{`{}`, `{"ipAccessList":null}`} {
		var pg Postgres
		require.NoError(t, json.Unmarshal([]byte(body), &pg))
		require.Nil(t, pg.IpAccessList)
	}
	var pg Postgres
	require.NoError(t, json.Unmarshal([]byte(`{"ipAccessList":[]}`), &pg))
	require.NotNil(t, pg.IpAccessList)
	require.Empty(t, pg.IpAccessList)
}
