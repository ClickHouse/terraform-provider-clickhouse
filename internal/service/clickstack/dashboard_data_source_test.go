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

func TestDashboardDataSource_Schema(t *testing.T) {
	t.Parallel()
	d := NewDashboardDataSource()
	resp := &fwdatasource.SchemaResponse{}
	d.Schema(context.Background(), fwdatasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %s", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "team", "dashboard_json"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
}

func TestDashboardDataSource_Read(t *testing.T) {
	t.Parallel()

	schemaResp := &fwdatasource.SchemaResponse{}
	(&dashboardDataSource{}).Schema(context.Background(), fwdatasource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	configObj := func() tftypes.Value {
		return tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			idAttr: tftypes.String, teamAttr: tftypes.String, dashboardJSONAttr: tftypes.String,
		}}, map[string]tftypes.Value{
			idAttr:            tftypes.NewValue(tftypes.String, "d1"),
			teamAttr:          tftypes.NewValue(tftypes.String, nil),
			dashboardJSONAttr: tftypes.NewValue(tftypes.String, nil),
		})
	}

	cases := []struct {
		name         string
		handler      http.HandlerFunc
		wantErr      bool
		wantDashJSON string
	}{
		{
			name: "success sets dashboard_json from server body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":{"id":"d1","name":"D"}}`))
			},
			wantDashJSON: `{"id":"d1","name":"D"}`,
		},
		{
			name:    "not found is an error",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			wantErr: true,
		},
		{
			name: "api error surfaces diagnostic",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(tc.handler)
			t.Cleanup(server.Close)
			c, err := client.New(server.URL, "test-key", server.Client())
			if err != nil {
				t.Fatalf("client.New: %v", err)
			}
			d := &dashboardDataSource{client: c}
			resp := &fwdatasource.ReadResponse{State: tfsdk.State{Schema: sch}}
			d.Read(context.Background(), fwdatasource.ReadRequest{
				Config: tfsdk.Config{Schema: sch, Raw: configObj()},
			}, resp)

			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("HasError()=%v, want %v: %s", resp.Diagnostics.HasError(), tc.wantErr, resp.Diagnostics)
			}
			if tc.wantErr {
				return
			}
			var got dashboardDataSourceModel
			resp.State.Get(context.Background(), &got)
			if got.DashboardJSON.ValueString() != tc.wantDashJSON {
				t.Errorf("dashboard_json=%q, want %q", got.DashboardJSON.ValueString(), tc.wantDashJSON)
			}
		})
	}
}
