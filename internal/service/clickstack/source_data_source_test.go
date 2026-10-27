package clickstack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func TestSourceDataSource_Read(t *testing.T) {
	t.Parallel()

	schemaResp := &fwdatasource.SchemaResponse{}
	(&sourceDataSource{}).Schema(context.Background(), fwdatasource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	const sources = `{"data":[
		{"id":"l1","name":"Logs","kind":"log","connection":"c1","from":{"databaseName":"otel","tableName":"otel_logs"}},
		{"id":"t1","name":"Traces","kind":"trace","connection":"c1","from":{"databaseName":"otel","tableName":"otel_traces"}},
		{"id":"d1","name":"Dup","kind":"log","connection":"c1","from":{"databaseName":"otel","tableName":"a"}},
		{"id":"d2","name":"Dup","kind":"trace","connection":"c1","from":{"databaseName":"otel","tableName":"b"}},
		{"id":"s1","name":"Same","kind":"log","connection":"c1","from":{"databaseName":"otel","tableName":"a"}},
		{"id":"s2","name":"Same","kind":"log","connection":"c1","from":{"databaseName":"otel","tableName":"b"}}
	]}`

	str := func(v string) tftypes.Value {
		if v == "" {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, v)
	}
	configObj := func(name, kind string) tftypes.Value {
		attrs := []string{teamAttr, nameAttr, "kind", idAttr, "connection_id", "database_name", "table_name"}
		types := map[string]tftypes.Type{}
		vals := map[string]tftypes.Value{}
		for _, a := range attrs {
			types[a] = tftypes.String
			vals[a] = str("")
		}
		vals[nameAttr], vals["kind"] = str(name), str(kind)
		return tftypes.NewValue(tftypes.Object{AttributeTypes: types}, vals)
	}

	cases := []struct {
		name, srcName, kind string
		wantErr             string
		wantID, wantKind    string
		wantTable           string
	}{
		{name: "match by name", srcName: "Traces", wantID: "t1", wantKind: "trace", wantTable: "otel_traces"},
		{name: "kind narrows duplicate names", srcName: "Dup", kind: "trace", wantID: "d2", wantKind: "trace", wantTable: "b"},
		{name: "duplicate names without kind", srcName: "Dup", wantErr: "Multiple Sources Found"},
		{name: "duplicate name and kind", srcName: "Same", kind: "log", wantErr: "Multiple Sources Found"},
		{name: "kind mismatch", srcName: "Logs", kind: "trace", wantErr: "Source Not Found"},
		{name: "unknown name", srcName: "Nope", wantErr: "Source Not Found"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sources))
	}))
	t.Cleanup(server.Close)
	c, err := client.New(server.URL, "test-key", server.Client())
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := &sourceDataSource{client: c}
			resp := &fwdatasource.ReadResponse{State: tfsdk.State{Schema: sch}}
			d.Read(context.Background(), fwdatasource.ReadRequest{
				Config: tfsdk.Config{Schema: sch, Raw: configObj(tc.srcName, tc.kind)},
			}, resp)

			if tc.wantErr != "" {
				if errs := resp.Diagnostics.Errors(); len(errs) != 1 || errs[0].Summary() != tc.wantErr {
					t.Fatalf("want one %q error, got %s", tc.wantErr, resp.Diagnostics)
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected error: %s", resp.Diagnostics)
			}
			var got sourceDataSourceModel
			resp.State.Get(context.Background(), &got)
			if got.ID.ValueString() != tc.wantID || got.Kind.ValueString() != tc.wantKind ||
				got.TableName.ValueString() != tc.wantTable || got.ConnectionID.ValueString() != "c1" ||
				got.DatabaseName.ValueString() != "otel" {
				t.Errorf("got %+v", got)
			}
		})
	}
}
