package clickstack

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func TestApplyDashboardBody_EmptyID(t *testing.T) {
	t.Parallel()
	var m dashboardResourceModel
	// Body has no "id" field; applyDashboardBody must return an error diagnostic.
	if diags := m.applyDashboardBody([]byte(`{"name":"D"}`)); !diags.HasError() {
		t.Error("expected HasError() == true when API body has no id, got no error")
	}
}

func TestApplyDashboardBody(t *testing.T) {
	t.Parallel()
	var m dashboardResourceModel
	body := []byte(`{"id":"d1","name":"D","tiles":[]}`)
	if diags := m.applyDashboardBody(body); diags.HasError() {
		t.Fatalf("applyDashboardBody: %s", diags)
	}
	if m.ID.ValueString() != "d1" {
		t.Errorf("expected id d1, got %q", m.ID.ValueString())
	}
	if m.NormalizedJSON.ValueString() != string(body) {
		t.Errorf("normalized_json not set")
	}
	if !m.DashboardJSON.IsNull() {
		t.Errorf("applyDashboardBody must not set DashboardJSON, got %q", m.DashboardJSON.ValueString())
	}
}

func TestParseDashboardJSON(t *testing.T) {
	t.Parallel()
	if err := parseDashboardJSON(`{"name":"D"}`); err != nil {
		t.Errorf("valid object rejected: %v", err)
	}
	if err := parseDashboardJSON(`[1,2]`); err == nil {
		t.Error("expected error for non-object JSON")
	}
	if err := parseDashboardJSON(`{bad`); err == nil {
		t.Error("expected error for invalid JSON")
	}
	t.Run("null rejected", func(t *testing.T) {
		t.Parallel()
		if err := parseDashboardJSON("null"); err == nil {
			t.Error("expected error for JSON null, got nil")
		}
	})
}

func TestParseDashboardImportID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, wantTeam, wantID string
		wantErr              bool
	}{
		{"d1", "", "d1", false},
		{"t1/d1", "t1", "d1", false},
		{"", "", "", true},
		{"t1/", "", "", true},
		{"/d1", "", "", true},
	}
	for _, tc := range cases {
		team, id, err := parseDashboardImportID(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("%q: err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			continue
		}
		if err == nil && (team != tc.wantTeam || id != tc.wantID) {
			t.Errorf("%q: got team=%q id=%q, want team=%q id=%q", tc.in, team, id, tc.wantTeam, tc.wantID)
		}
	}
}

func TestDashboardResource_Schema(t *testing.T) {
	t.Parallel()
	r := NewDashboardResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}
	for _, attr := range []string{"id", "team", "dashboard_json", "normalized_json"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
}

// dashboardValidateConfigRequest builds a ValidateConfigRequest whose config
// sets dashboard_json to the given string and leaves every other attribute
// null, matching the resource schema's attribute types.
func dashboardValidateConfigRequest(t *testing.T, dashboardJSON string) fwresource.ValidateConfigRequest {
	t.Helper()

	schemaResp := &fwresource.SchemaResponse{}
	(&dashboardResource{}).Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", schemaResp.Diagnostics)
	}

	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		idAttr:             tftypes.String,
		teamAttr:           tftypes.String,
		dashboardJSONAttr:  tftypes.String,
		normalizedJSONAttr: tftypes.String,
	}}
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		idAttr:             tftypes.NewValue(tftypes.String, nil),
		teamAttr:           tftypes.NewValue(tftypes.String, nil),
		dashboardJSONAttr:  tftypes.NewValue(tftypes.String, dashboardJSON),
		normalizedJSONAttr: tftypes.NewValue(tftypes.String, nil),
	})

	return fwresource.ValidateConfigRequest{
		Config: tfsdk.Config{Raw: raw, Schema: schemaResp.Schema},
	}
}

// validateEndpointHandler serves the given unenveloped /validate response body
// (the endpoint returns {"valid":...,"errors":[...]} with no {"data":...} wrapper).
func validateEndpointHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func TestDashboardResource_ValidateConfig(t *testing.T) {
	t.Parallel()

	const validBody = `{"name":"D","tiles":[]}`

	type wantDiag struct {
		severity       diag.Severity
		summary        string
		detailContains string
	}

	cases := []struct {
		name          string
		dashboardJSON string
		// handler, when non-nil, backs a stub API server the resource client
		// points at; nil leaves the client nil (early validation, pre-Configure).
		handler http.HandlerFunc
		want    []wantDiag
	}{
		{
			name:          "malformed dashboard_json is an attribute error",
			dashboardJSON: `{bad`,
			want: []wantDiag{
				{diag.SeverityError, "Invalid dashboard_json", "must be a JSON object"},
			},
		},
		{
			name:          "nil client skips API validation with no diagnostics",
			dashboardJSON: validBody,
		},
		{
			name:          "missing validate endpoint (404) warns validation skipped",
			dashboardJSON: validBody,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			want: []wantDiag{
				{diag.SeverityWarning, "Dashboard validation skipped", "validated on apply"},
			},
		},
		{
			name:          "generic API error warns validation unavailable",
			dashboardJSON: validBody,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				// 400 rather than 500 so a retrying HTTP client would not mask the failure.
				http.Error(w, `{"message":"boom"}`, http.StatusBadRequest)
			},
			want: []wantDiag{
				{diag.SeverityWarning, "Dashboard validation unavailable", "boom"},
			},
		},
		{
			name:          "invalid dashboard reports each error with its path",
			dashboardJSON: validBody,
			handler:       validateEndpointHandler(`{"valid":false,"errors":[{"path":"name","message":"Required"}]}`),
			want: []wantDiag{
				{diag.SeverityError, "Invalid dashboard configuration", "name: Required"},
			},
		},
		{
			name:          "invalid dashboard with no error details uses fallback error",
			dashboardJSON: validBody,
			handler:       validateEndpointHandler(`{"valid":false,"errors":[]}`),
			want: []wantDiag{
				{diag.SeverityError, "Invalid dashboard configuration", "returned no error details"},
			},
		},
		{
			name:          "valid dashboard produces no diagnostics",
			dashboardJSON: validBody,
			handler:       validateEndpointHandler(`{"valid":true,"errors":[],"normalized":{"name":"D","tiles":[]}}`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := &dashboardResource{}
			if tc.handler != nil {
				server := httptest.NewServer(tc.handler)
				t.Cleanup(server.Close)
				c, err := client.New(server.URL, "test-key", server.Client())
				if err != nil {
					t.Fatalf("client.New: %v", err)
				}
				r.client = c
			}

			resp := &fwresource.ValidateConfigResponse{}
			r.ValidateConfig(context.Background(), dashboardValidateConfigRequest(t, tc.dashboardJSON), resp)

			// ValidateConfig also emits the alpha warning; drop it so these
			// cases assert only the dashboard_json validation diagnostics.
			var got diag.Diagnostics
			for _, d := range resp.Diagnostics {
				if d.Summary() == "Alpha Resource" {
					continue
				}
				got = append(got, d)
			}

			if len(got) != len(tc.want) {
				t.Fatalf("got %d diagnostics, want %d: %s", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				d := got[i]
				if d.Severity() != w.severity {
					t.Errorf("diagnostic %d: severity = %v, want %v", i, d.Severity(), w.severity)
				}
				if d.Summary() != w.summary {
					t.Errorf("diagnostic %d: summary = %q, want %q", i, d.Summary(), w.summary)
				}
				if !strings.Contains(d.Detail(), w.detailContains) {
					t.Errorf("diagnostic %d: detail %q does not contain %q", i, d.Detail(), w.detailContains)
				}
			}
		})
	}
}

// dashboardTestSchema returns the resource schema for building request/response
// Plan and State values in unit tests.
func dashboardTestSchema(t *testing.T) rschema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	(&dashboardResource{}).Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}
	return resp.Schema
}

// dashboardObjectValue builds a tftypes object value for the resource's four
// attributes; a nil pointer produces a null attribute.
func dashboardObjectValue(id, team, dashJSON, normJSON *string) tftypes.Value {
	str := func(p *string) tftypes.Value {
		if p == nil {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, *p)
	}
	objType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		idAttr:             tftypes.String,
		teamAttr:           tftypes.String,
		dashboardJSONAttr:  tftypes.String,
		normalizedJSONAttr: tftypes.String,
	}}
	return tftypes.NewValue(objType, map[string]tftypes.Value{
		idAttr:             str(id),
		teamAttr:           str(team),
		dashboardJSONAttr:  str(dashJSON),
		normalizedJSONAttr: str(normJSON),
	})
}

// dashboardTestClient points a client at an httptest server running h.
func dashboardTestClient(t *testing.T, h http.Handler) *client.Client {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	c, err := client.New(server.URL, "test-key", server.Client())
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return c
}

func ptr(s string) *string { return &s }

func TestDashboardResource_Create(t *testing.T) {
	t.Parallel()
	const body = `{"name":"D","tiles":[]}`

	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr bool
		// wantID, when non-empty, is asserted against the resulting state.
		wantID string
	}{
		{
			name: "success populates id and normalized_json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":{"id":"d1","name":"D","tiles":[]}}`))
			},
			wantID: "d1",
		},
		{
			name: "api error surfaces diagnostic",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"message":"boom"}`, http.StatusBadRequest)
			},
			wantErr: true,
		},
		{
			name: "success body with no id is orphaned",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":{"name":"D","tiles":[]}}`))
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sch := dashboardTestSchema(t)
			r := &dashboardResource{client: dashboardTestClient(t, tc.handler)}
			resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: sch}}
			r.Create(context.Background(), fwresource.CreateRequest{
				Plan: tfsdk.Plan{Schema: sch, Raw: dashboardObjectValue(nil, nil, ptr(body), nil)},
			}, resp)

			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("HasError()=%v, want %v: %s", resp.Diagnostics.HasError(), tc.wantErr, resp.Diagnostics)
			}
			if tc.wantErr {
				return
			}
			var got dashboardResourceModel
			resp.State.Get(context.Background(), &got)
			if got.ID.ValueString() != tc.wantID {
				t.Errorf("id=%q, want %q", got.ID.ValueString(), tc.wantID)
			}
			if got.NormalizedJSON.IsNull() {
				t.Error("normalized_json not set")
			}
			if got.DashboardJSON.ValueString() != body {
				t.Errorf("dashboard_json=%q, want %q (config value must be preserved)", got.DashboardJSON.ValueString(), body)
			}
		})
	}
}

func TestDashboardResource_Read(t *testing.T) {
	t.Parallel()
	const serverBody = `{"id":"d1","name":"D","tiles":[]}`

	cases := []struct {
		name    string
		handler http.HandlerFunc
		// stateDashJSON is the dashboard_json already in state; nil models an
		// imported resource whose config value has not been read yet.
		stateDashJSON *string
		wantErr       bool
		wantRemoved   bool
		wantDashJSON  string
	}{
		{
			name:          "success updates normalized_json and preserves dashboard_json",
			handler:       func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":` + serverBody + `}`)) },
			stateDashJSON: ptr(`{"name":"D","tiles":[]}`),
			wantDashJSON:  `{"name":"D","tiles":[]}`,
		},
		{
			name:          "import backfills dashboard_json from server body",
			handler:       func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":` + serverBody + `}`)) },
			stateDashJSON: nil,
			wantDashJSON:  serverBody,
		},
		{
			name:          "not found removes resource from state",
			handler:       func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			stateDashJSON: ptr(serverBody),
			wantRemoved:   true,
		},
		{
			name: "api error surfaces diagnostic",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
			},
			stateDashJSON: ptr(serverBody),
			wantErr:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sch := dashboardTestSchema(t)
			r := &dashboardResource{client: dashboardTestClient(t, tc.handler)}
			resp := &fwresource.ReadResponse{State: tfsdk.State{Schema: sch}}
			r.Read(context.Background(), fwresource.ReadRequest{
				State: tfsdk.State{Schema: sch, Raw: dashboardObjectValue(ptr("d1"), nil, tc.stateDashJSON, ptr("stale"))},
			}, resp)

			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("HasError()=%v, want %v: %s", resp.Diagnostics.HasError(), tc.wantErr, resp.Diagnostics)
			}
			if tc.wantErr {
				return
			}
			if tc.wantRemoved {
				if !resp.State.Raw.IsNull() {
					t.Error("expected resource removed from state")
				}
				return
			}
			var got dashboardResourceModel
			resp.State.Get(context.Background(), &got)
			if got.DashboardJSON.ValueString() != tc.wantDashJSON {
				t.Errorf("dashboard_json=%q, want %q", got.DashboardJSON.ValueString(), tc.wantDashJSON)
			}
			if got.NormalizedJSON.ValueString() != serverBody {
				t.Errorf("normalized_json=%q, want %q", got.NormalizedJSON.ValueString(), serverBody)
			}
		})
	}
}

func TestDashboardResource_Update(t *testing.T) {
	t.Parallel()
	// Authored body has a named tile with no id; prior normalized state carries
	// the server-assigned id for that same-named tile, so the success case
	// exercises the tile-ID merge end to end.
	const body = `{"name":"D2","tiles":[{"name":"T"}]}`
	const priorNorm = `{"id":"d1","name":"D","tiles":[{"id":"srv-1","name":"T"}]}`

	cases := []struct {
		name        string
		handler     http.HandlerFunc
		wantErr     bool
		wantRemoved bool
	}{
		{
			name: "success merges tile id and updates state",
			handler: func(w http.ResponseWriter, r *http.Request) {
				// The merged tile id must reach the update request body — proves the
				// merge result (not the raw authored body) is what gets sent.
				raw, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(raw), `"srv-1"`) {
					t.Errorf("expected merged tile id srv-1 in update body, got %s", raw)
				}
				_, _ = w.Write([]byte(`{"data":{"id":"d1","name":"D2","tiles":[{"id":"srv-1","name":"T"}]}}`))
			},
		},
		{
			name:        "not found removes resource from state",
			handler:     func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
			wantRemoved: true,
		},
		{
			name: "api error surfaces diagnostic",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"message":"boom"}`, http.StatusBadRequest)
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sch := dashboardTestSchema(t)
			r := &dashboardResource{client: dashboardTestClient(t, tc.handler)}
			resp := &fwresource.UpdateResponse{State: tfsdk.State{Schema: sch}}
			r.Update(context.Background(), fwresource.UpdateRequest{
				Plan:  tfsdk.Plan{Schema: sch, Raw: dashboardObjectValue(ptr("d1"), nil, ptr(body), nil)},
				State: tfsdk.State{Schema: sch, Raw: dashboardObjectValue(ptr("d1"), nil, ptr(body), ptr(priorNorm))},
			}, resp)

			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("HasError()=%v, want %v: %s", resp.Diagnostics.HasError(), tc.wantErr, resp.Diagnostics)
			}
			if tc.wantErr {
				return
			}
			if tc.wantRemoved {
				if !resp.State.Raw.IsNull() {
					t.Error("expected resource removed from state")
				}
				return
			}
			var got dashboardResourceModel
			resp.State.Get(context.Background(), &got)
			if got.ID.ValueString() != "d1" {
				t.Errorf("id=%q, want d1", got.ID.ValueString())
			}
			if got.DashboardJSON.ValueString() != body {
				t.Errorf("dashboard_json=%q, want %q", got.DashboardJSON.ValueString(), body)
			}
		})
	}
}

func TestDashboardResource_Delete(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr bool
	}{
		{
			name:    "success",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
		},
		{
			name:    "not found is not an error",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) },
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
			sch := dashboardTestSchema(t)
			r := &dashboardResource{client: dashboardTestClient(t, tc.handler)}
			resp := &fwresource.DeleteResponse{State: tfsdk.State{Schema: sch}}
			r.Delete(context.Background(), fwresource.DeleteRequest{
				State: tfsdk.State{Schema: sch, Raw: dashboardObjectValue(ptr("d1"), nil, ptr(`{"name":"D"}`), ptr("n"))},
			}, resp)

			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("HasError()=%v, want %v: %s", resp.Diagnostics.HasError(), tc.wantErr, resp.Diagnostics)
			}
		})
	}
}
