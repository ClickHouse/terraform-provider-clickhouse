package clickstack

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func TestApplyDashboardBody_EmptyID(t *testing.T) {
	t.Parallel()
	var m dashboardResourceModel
	// Body has no "id" field; applyDashboardBody must return an error diagnostic.
	if diags := m.applyDashboardBody(context.Background(), []byte(`{"name":"D"}`)); !diags.HasError() {
		t.Error("expected HasError() == true when API body has no id, got no error")
	}
}

func TestApplyDashboardBody(t *testing.T) {
	t.Parallel()
	var m dashboardResourceModel
	body := []byte(`{"id":"d1","name":"D","tiles":[]}`)
	if diags := m.applyDashboardBody(context.Background(), body); diags.HasError() {
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
	for _, attr := range []string{"id", "team", "dashboard_json", "normalized_json", "tile_ids"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q", attr)
		}
	}
	if a, ok := resp.Schema.Attributes["tile_ids"]; ok && !a.IsComputed() {
		t.Error("tile_ids must be computed: the server assigns tile ids, the author cannot")
	}
}

func TestTileIDsByName(t *testing.T) {
	t.Parallel()
	body := []byte(`{"id":"d1","tiles":[
		{"id":"t1","name":"Errors"},
		{"id":"t2","name":"Dup"},{"id":"t3","name":"Dup"},
		{"id":"t4","name":""},
		{"id":"t5"}
	]}`)
	got, err := tileIDsByName(body)
	if err != nil {
		t.Fatalf("tileIDsByName: %v", err)
	}
	if len(got) != 1 || got["Errors"] != "t1" {
		t.Errorf("tileIDsByName = %v, want {Errors: t1} (duplicate and blank names excluded)", got)
	}
}

func TestApplyDashboardBody_TileIDs(t *testing.T) {
	t.Parallel()
	var m dashboardResourceModel
	if d := m.applyDashboardBody(context.Background(), []byte(`{"id":"d1","tiles":[{"id":"t1","name":"Errors"}]}`)); d.HasError() {
		t.Fatalf("applyDashboardBody: %s", d)
	}
	elems := m.TileIDs.Elements()
	v, ok := elems["Errors"]
	if !ok || v.(types.String).ValueString() != "t1" {
		t.Errorf("tile_ids = %v, want {Errors: t1}", elems)
	}
}

// TestApplyDashboardBody_UnreadableTiles: the id decode reads only "id", so a
// body with an unreadable tiles array still reaches the tile_ids step. It must
// error — on update the plan has already promised known tile_ids entries, so an
// empty map would come back as an "inconsistent result after apply" instead.
func TestApplyDashboardBody_UnreadableTiles(t *testing.T) {
	t.Parallel()
	var m dashboardResourceModel
	diags := m.applyDashboardBody(context.Background(), []byte(`{"id":"d1","tiles":{"nope":true}}`))
	if !diags.HasError() {
		t.Fatalf("applyDashboardBody = %s, want an error for an unreadable tiles array", diags)
	}
}

func TestTileIDsPlanModifier(t *testing.T) {
	t.Parallel()
	// Prior server body: two named tiles with the ids the server assigned.
	const prior = `{"id":"d1","tiles":[{"id":"srv-a","name":"A"},{"id":"srv-b","name":"B"}]}`
	knownA := types.StringValue("srv-a")

	cases := []struct {
		name string
		// prior is normalized_json in the prior state; nil models a create, where
		// there is no prior state object at all.
		prior *string
		// priorAuthored is dashboard_json in the prior state; nil uses a body that
		// differs from every plan below, so the "nothing authored changed" early
		// return only fires in the case that sets it.
		priorAuthored *string
		// priorTileIDs is the tile_ids map in the prior state (req.StateValue).
		priorTileIDs map[string]attr.Value
		plan         string
		// want is the expected planned map; nil means the modifier must leave the
		// incoming plan value alone.
		want map[string]attr.Value
		// wantPriorMap asserts the modifier handed back req.StateValue verbatim.
		wantPriorMap bool
	}{
		{
			name:  "unchanged tile set keeps every id known",
			prior: ptr(prior),
			plan:  `{"tiles":[{"name":"A"},{"name":"B"}]}`,
			want:  map[string]attr.Value{"A": knownA, "B": types.StringValue("srv-b")},
		},
		{
			name:  "added tile is unknown while existing ids stay known",
			prior: ptr(prior),
			plan:  `{"tiles":[{"name":"A"},{"name":"B"},{"name":"C"}]}`,
			want: map[string]attr.Value{
				"A": knownA, "B": types.StringValue("srv-b"), "C": types.StringUnknown(),
			},
		},
		{
			name:  "renamed tile drops the old name and is unknown under the new one",
			prior: ptr(prior),
			plan:  `{"tiles":[{"name":"A"},{"name":"B2"}]}`,
			want:  map[string]attr.Value{"A": knownA, "B2": types.StringUnknown()},
		},
		{
			name:  "authored id the server knows is kept even under a new name",
			prior: ptr(prior),
			plan:  `{"tiles":[{"id":"srv-b","name":"Renamed"}]}`,
			want:  map[string]attr.Value{"Renamed": types.StringValue("srv-b")},
		},
		{
			name:  "unknown authored id yields to the name match",
			prior: ptr(prior),
			plan:  `{"tiles":[{"id":"nope","name":"A"}]}`,
			want:  map[string]attr.Value{"A": knownA},
		},
		{
			name:  "unknown authored id with no name match is unknown",
			prior: ptr(prior),
			plan:  `{"tiles":[{"id":"nope","name":"C"}]}`,
			want:  map[string]attr.Value{"C": types.StringUnknown()},
		},
		{
			name:  "duplicate planned names are excluded from the map",
			prior: ptr(prior),
			plan:  `{"tiles":[{"name":"A"},{"name":"dup"},{"name":"dup"}]}`,
			want:  map[string]attr.Value{"A": knownA},
		},
		{
			// A blank-named tile inserted above tile A must not take srv-a by
			// position — the name match claims it first, so A keeps its known id.
			name:  "a blank-named tile cannot take a named tile's id by position",
			prior: ptr(`{"id":"d1","tiles":[{"id":"srv-a","name":"A"},{"id":"srv-b","name":""}]}`),
			plan:  `{"tiles":[{"name":""},{"name":"A"}]}`,
			want:  map[string]attr.Value{"A": knownA},
		},
		{
			// Refreshed state: the UI added tile C, so normalized_json and tile_ids
			// know it but the authored body does not. Nothing authored changed, so
			// the prior map must survive verbatim and leave the UI edit undetected —
			// recomputing would drop C and plan an update that deletes it.
			name:          "no authored change keeps the prior map, UI drift and all",
			prior:         ptr(`{"id":"d1","tiles":[{"id":"srv-a","name":"A"},{"id":"srv-c","name":"C"}]}`),
			priorAuthored: ptr(`{ "tiles" : [ {"name":"A"} ] }`),
			priorTileIDs:  map[string]attr.Value{"A": knownA, "C": types.StringValue("srv-c")},
			plan:          `{"tiles":[{"name":"A"}]}`,
			wantPriorMap:  true,
		},
		{
			// Safety net in plannedTileIDs: with no prior tiles array the merge
			// returns the authored body untouched, so an authored id nothing
			// recognises must still plan as unknown.
			name:  "authored id with no prior tiles array is unknown",
			prior: ptr(`{"id":"d1"}`),
			plan:  `{"tiles":[{"id":"stale","name":"A"}]}`,
			want:  map[string]attr.Value{"A": types.StringUnknown()},
		},
		{
			name:  "malformed planned body leaves the map unknown",
			prior: ptr(prior),
			plan:  `{bad`,
			want:  nil,
		},
		{
			name:  "create leaves the planned value untouched",
			prior: nil,
			plan:  `{"tiles":[{"name":"A"}]}`,
			want:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sch := dashboardTestSchema(t)
			priorAuthored := tc.priorAuthored
			if priorAuthored == nil {
				priorAuthored = ptr(`{"tiles":[]}`)
			}
			stateValue := types.MapNull(types.StringType)
			if tc.priorTileIDs != nil {
				stateValue = types.MapValueMust(types.StringType, tc.priorTileIDs)
			}
			stateRaw := dashboardObjectValue(ptr("d1"), nil, priorAuthored, tc.prior)
			if tc.prior == nil {
				stateRaw = tftypes.NewValue(dashboardObjectType, nil)
			}
			// tile_ids is unknown in the proposed plan, matching the PlanValue the
			// framework hands the modifier.
			planRaw := tftypes.NewValue(dashboardObjectType, map[string]tftypes.Value{
				idAttr:             tftypes.NewValue(tftypes.String, "d1"),
				teamAttr:           tftypes.NewValue(tftypes.String, nil),
				dashboardJSONAttr:  tftypes.NewValue(tftypes.String, tc.plan),
				normalizedJSONAttr: tftypes.NewValue(tftypes.String, nil),
				tileIDsAttr:        tftypes.NewValue(tileIDsTFType, tftypes.UnknownValue),
			})
			req := planmodifier.MapRequest{
				Path:       path.Root(tileIDsAttr),
				StateValue: stateValue,
				PlanValue:  types.MapUnknown(types.StringType),
				State:      tfsdk.State{Schema: sch, Raw: stateRaw},
				Plan:       tfsdk.Plan{Schema: sch, Raw: planRaw},
			}
			resp := &planmodifier.MapResponse{PlanValue: req.PlanValue}
			tileIDsPlanModifier{}.PlanModifyMap(context.Background(), req, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("PlanModifyMap: %s", resp.Diagnostics)
			}
			if tc.wantPriorMap {
				if !resp.PlanValue.Equal(req.StateValue) {
					t.Errorf("plan value = %v, want the prior state map (%v)", resp.PlanValue, req.StateValue)
				}
				return
			}
			if tc.want == nil {
				if !resp.PlanValue.Equal(req.PlanValue) {
					t.Errorf("plan value = %v, want it left unmodified (%v)", resp.PlanValue, req.PlanValue)
				}
				return
			}
			want := types.MapValueMust(types.StringType, tc.want)
			if !resp.PlanValue.Equal(want) {
				t.Errorf("plan value = %v, want %v", resp.PlanValue, want)
			}
		})
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

	raw := tftypes.NewValue(dashboardObjectType, map[string]tftypes.Value{
		idAttr:             tftypes.NewValue(tftypes.String, nil),
		teamAttr:           tftypes.NewValue(tftypes.String, nil),
		dashboardJSONAttr:  tftypes.NewValue(tftypes.String, dashboardJSON),
		normalizedJSONAttr: tftypes.NewValue(tftypes.String, nil),
		tileIDsAttr:        tftypes.NewValue(tileIDsTFType, nil),
	})

	return fwresource.ValidateConfigRequest{
		Config: tfsdk.Config{Raw: raw, Schema: schemaResp.Schema},
	}
}

// tileIDsTFType and dashboardObjectType mirror the resource schema, so every
// Config/Plan/State value built here has the exact shape the framework expects.
var (
	tileIDsTFType       = tftypes.Map{ElementType: tftypes.String}
	dashboardObjectType = tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		idAttr:             tftypes.String,
		teamAttr:           tftypes.String,
		dashboardJSONAttr:  tftypes.String,
		normalizedJSONAttr: tftypes.String,
		tileIDsAttr:        tileIDsTFType,
	}}
)

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

			// ValidateConfig also emits the beta warning; drop it so these
			// cases assert only the dashboard_json validation diagnostics.
			var got diag.Diagnostics
			for _, d := range resp.Diagnostics {
				if d.Summary() == "Beta Resource" {
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

// dashboardObjectValue builds a tftypes object value for the resource's
// attributes; a nil pointer produces a null attribute. tile_ids is always null:
// it is computed, and every code path under test recomputes it from the server
// body rather than reading it.
func dashboardObjectValue(id, team, dashJSON, normJSON *string) tftypes.Value {
	str := func(p *string) tftypes.Value {
		if p == nil {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, *p)
	}
	return tftypes.NewValue(dashboardObjectType, map[string]tftypes.Value{
		idAttr:             str(id),
		teamAttr:           str(team),
		dashboardJSONAttr:  str(dashJSON),
		normalizedJSONAttr: str(normJSON),
		tileIDsAttr:        tftypes.NewValue(tileIDsTFType, nil),
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
		// wantNormalized defaults to serverBody when empty.
		wantNormalized string
	}{
		{
			name:          "success updates normalized_json and preserves dashboard_json",
			handler:       func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":` + serverBody + `}`)) },
			stateDashJSON: ptr(`{"name":"D","tiles":[]}`),
			wantDashJSON:  `{"name":"D","tiles":[]}`,
		},
		{
			// The backfilled body drops the dashboard id (the write path rejects
			// it) but keeps everything else the server returned.
			name:          "import backfills dashboard_json from server body",
			handler:       func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":` + serverBody + `}`)) },
			stateDashJSON: nil,
			wantDashJSON:  `{"name":"D","tiles":[]}`,
		},
		{
			// Filter ids go the same way as the dashboard id, and for the same
			// reason: the write path refuses to take them back.
			name: "import strips filter ids from the backfilled body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":{"id":"d1","name":"D","filters":[{"id":"f1","name":"Env"}],"tiles":[]}}`))
			},
			stateDashJSON: nil,
			wantDashJSON:  `{"filters":[{"name":"Env"}],"name":"D","tiles":[]}`,
			// normalized_json keeps the ids: update reads the filter ids back out
			// of it, because the Cloud API demands one on every filter there.
			wantNormalized: `{"id":"d1","name":"D","filters":[{"id":"f1","name":"Env"}],"tiles":[]}`,
		},
		{
			// The API exports aggregation fields its own write schema rejects, so
			// the backfilled body drops them or every plan fails validation.
			name: "import drops select fields the write schema rejects",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":{"id":"d1","name":"D","tiles":[{"name":"T","config":{"select":[{"aggFn":"count","level":0.9,"valueExpression":"Duration"}]}}]}}`))
			},
			stateDashJSON:  nil,
			wantDashJSON:   `{"name":"D","tiles":[{"config":{"select":[{"aggFn":"count"}]},"name":"T"}]}`,
			wantNormalized: `{"id":"d1","name":"D","tiles":[{"name":"T","config":{"select":[{"aggFn":"count","level":0.9,"valueExpression":"Duration"}]}}]}`,
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
			wantNormalized := tc.wantNormalized
			if wantNormalized == "" {
				wantNormalized = serverBody
			}
			if got.NormalizedJSON.ValueString() != wantNormalized {
				t.Errorf("normalized_json=%q, want %q", got.NormalizedJSON.ValueString(), wantNormalized)
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
			// tile_ids must survive the write into real schema-typed state — it is
			// what dependent alerts read their tile_id from.
			if v := got.TileIDs.Elements()["T"]; v == nil || !v.Equal(types.StringValue("srv-1")) {
				t.Errorf("tile_ids=%v, want {T: srv-1}", got.TileIDs)
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
