package clickstack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func TestAppResource_Metadata(t *testing.T) {
	t.Parallel()
	resp := &fwresource.MetadataResponse{}
	NewAppResource().Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "clickhouse"}, resp)
	if resp.TypeName != appResourceName {
		t.Errorf("expected %s, got %q", appResourceName, resp.TypeName)
	}
}

func appTestSchema(t *testing.T) rschema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	NewAppResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %s", resp.Diagnostics)
	}
	return resp.Schema
}

func TestAppResource_Schema(t *testing.T) {
	t.Parallel()
	s := appTestSchema(t)
	for _, a := range []string{idAttr, teamAttr, nameAttr, isSharedAttr, codeAttr, purposeAttr, versionIDAttr} {
		if _, ok := s.Attributes[a]; !ok {
			t.Errorf("missing attribute %q", a)
		}
	}
	if !s.Attributes[codeAttr].IsRequired() || !s.Attributes[nameAttr].IsRequired() {
		t.Error("name and code must be required")
	}
}

// appModel is a fully-known model with the given code and purpose.
func appModel(code, purpose string) appResourceModel {
	return appResourceModel{
		ID:        types.StringValue("a1"),
		Team:      types.StringNull(),
		Name:      types.StringValue("Checkout"),
		IsShared:  types.BoolValue(true),
		Code:      types.StringValue(code),
		Purpose:   types.StringValue(purpose),
		VersionID: types.StringValue("v1"),
	}
}

func TestAppResource_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		appName types.String
		wantErr bool
	}{
		{"plain", types.StringValue("Checkout health"), false},
		{"unknown", types.StringUnknown(), false},
		{"leading space", types.StringValue(" Checkout"), true},
		{"trailing newline", types.StringValue("Checkout\n"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := appModel("c", "")
			m.Name = tc.appName
			if got := m.validate().HasError(); got != tc.wantErr {
				t.Errorf("HasError() = %v, want %v", got, tc.wantErr)
			}
		})
	}
}

func TestAppResource_Validate_PrivateWarning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		isShared types.Bool
		wantWarn bool
	}{
		{"private", types.BoolValue(false), true},
		{"shared", types.BoolValue(true), false},
		{"default", types.BoolNull(), false},
		{"unknown", types.BoolUnknown(), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := appModel("c", "")
			m.IsShared = tc.isShared
			diags := m.validate()
			if diags.HasError() {
				t.Fatalf("unexpected error: %s", diags)
			}
			warned := len(diags.Warnings()) == 1 && diags.Warnings()[0].Summary() == privateAppWarningSummary
			if warned != tc.wantWarn || len(diags.Warnings()) > 1 {
				t.Errorf("warnings = %s, want private-app warning: %v", diags.Warnings(), tc.wantWarn)
			}
		})
	}
}

func TestAppResource_ValidateInput(t *testing.T) {
	t.Parallel()

	m := appModel("c", "p")
	m.Name = types.StringUnknown()
	m.IsShared = types.BoolNull()
	m.Purpose = types.StringValue("p")
	input, ok := m.validateInput()
	if !ok {
		t.Fatal("expected a validate body")
	}
	if input.Name != appValidateName || !input.IsShared || input.Code != "c" || input.Purpose != "p" {
		t.Errorf("unexpected body: %+v", input)
	}

	// A null purpose is sent as its default.
	m.Purpose = types.StringNull()
	if input, _ = m.validateInput(); input.Purpose != "" {
		t.Errorf("null purpose sent as %q", input.Purpose)
	}

	for name, mut := range map[string]func(*appResourceModel){
		"unknown code":      func(m *appResourceModel) { m.Code = types.StringUnknown() },
		"unknown purpose":   func(m *appResourceModel) { m.Purpose = types.StringUnknown() },
		"unknown is_shared": func(m *appResourceModel) { m.IsShared = types.BoolUnknown() },
	} {
		m := appModel("c", "p")
		mut(&m)
		if _, ok := m.validateInput(); ok {
			t.Errorf("%s: expected validation to be skipped", name)
		}
	}
}

func TestAppValidateDiagnostics(t *testing.T) {
	t.Parallel()

	nameErr := client.ValidateError{Path: "name", Message: "Required"}
	codeErr := client.ValidateError{Path: "code", Message: "App.tsx: Unexpected token"}
	otherErr := client.ValidateError{Path: "", Message: "Expected object"}
	unmappedErr := client.ValidateError{Path: "manifest.queries", Message: "Unrecognized key"}

	cases := []struct {
		name      string
		res       client.ValidateResult
		nameKnown bool
		want      []string // details, in order
	}{
		{"valid", client.ValidateResult{Valid: true}, true, nil},
		{"code error", client.ValidateResult{Errors: []client.ValidateError{codeErr}}, true, []string{codeErr.Message}},
		{"placeholder name errors dropped", client.ValidateResult{Errors: []client.ValidateError{nameErr, codeErr}}, false, []string{codeErr.Message}},
		{"known name errors kept", client.ValidateResult{Errors: []client.ValidateError{nameErr}}, true, []string{nameErr.Message}},
		{"top-level error", client.ValidateResult{Errors: []client.ValidateError{otherErr}}, true, []string{otherErr.Message}},
		// A path with no matching attribute is kept in the detail so the user can find it.
		{"unmapped path", client.ValidateResult{Errors: []client.ValidateError{unmappedErr}}, true, []string{"manifest.queries: Unrecognized key"}},
		{"no details", client.ValidateResult{}, true, []string{"returned no error details"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := appValidateDiagnostics(&tc.res, tc.nameKnown)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d diagnostics, want %d: %s", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				if got[i].Severity() != diag.SeverityError || !strings.Contains(got[i].Detail(), w) {
					t.Errorf("diagnostic %d = %s %q, want error containing %q", i, got[i].Severity(), got[i].Detail(), w)
				}
			}
		})
	}
}

// countingHandler counts requests and answers each with h.
func countingHandler(calls *atomic.Int32, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		h(w, r)
	}
}

func TestAppResource_ValidateConfig_NoAPICall(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	r := &appResource{client: dashboardTestClient(t, countingHandler(&calls, validateEndpointHandler(`{"valid":true,"errors":[]}`)))}

	cfg := appModel("export default function App() { return null; }", "")
	cfg.ID = types.StringNull()
	cfg.VersionID = types.StringNull()
	cfg.Name = types.StringValue(" padded")
	req := fwresource.ValidateConfigRequest{Config: tfsdk.Config{Schema: appTestSchema(t), Raw: appRaw(t, cfg)}}
	resp := &fwresource.ValidateConfigResponse{}
	r.ValidateConfig(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected the local name check to still run")
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("ValidateConfig made %d API calls, want 0", n)
	}
}

func TestNeedsAPIValidation(t *testing.T) {
	t.Parallel()
	state := appModel("c", "p")
	change := func(f func(*appResourceModel)) appResourceModel {
		m := appModel("c", "p")
		f(&m)
		return m
	}
	cases := []struct {
		name     string
		plan     appResourceModel
		hasState bool
		replaced bool
		want     bool
	}{
		{"create", appModel("c", "p"), false, false, true},
		{"unchanged", appModel("c", "p"), true, false, false},
		{"replacement", appModel("c", "p"), true, true, true},
		{"code change", appModel("c2", "p"), true, false, true},
		{"purpose change", appModel("c", "p2"), true, false, true},
		{"rename", change(func(m *appResourceModel) { m.Name = types.StringValue("Renamed") }), true, false, true},
		{"sharing change", change(func(m *appResourceModel) { m.IsShared = types.BoolValue(false) }), true, false, true},
		// Only the computed version_id differs: nothing new is sent.
		{"version_id only", change(func(m *appResourceModel) { m.VersionID = types.StringUnknown() }), true, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := needsAPIValidation(tc.plan, state, tc.hasState, tc.replaced); got != tc.want {
				t.Errorf("needsAPIValidation = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAppResource_ModifyPlan_Validation(t *testing.T) {
	t.Parallel()

	const compileError = `{"valid":false,"errors":[{"path":"code","message":"App.tsx: Unexpected token"}],"normalized":null}`
	valid := validateEndpointHandler(`{"valid":true,"errors":[],"normalized":{}}`)
	state := appModel("c", "p")
	unchanged := appModel("c", "p")
	unchanged.VersionID = types.StringUnknown()
	changed := appModel("c2", "p")
	changed.VersionID = types.StringUnknown()
	created := appModel("c", "p")
	created.ID = types.StringUnknown()
	created.VersionID = types.StringUnknown()

	cases := []struct {
		name      string
		plan      *appResourceModel // nil plans a destroy
		state     *appResourceModel // nil plans a create
		handler   http.HandlerFunc
		wantCalls int32
		want      []diag.Severity
		summary   string
	}{
		{"create is validated", &created, nil, valid, 1, nil, ""},
		{
			"compile error fails the plan", &created, nil, validateEndpointHandler(compileError), 1,
			[]diag.Severity{diag.SeverityError},
			"Invalid app configuration",
		},
		{"unchanged app is not sent", &unchanged, &state, validateEndpointHandler(compileError), 0, nil, ""},
		{
			"code change is validated", &changed, &state, validateEndpointHandler(compileError), 1,
			[]diag.Severity{diag.SeverityError},
			"Invalid app configuration",
		},
		{"destroy is not sent", nil, &state, valid, 0, nil, ""},
		{
			"missing endpoint warns", &created, nil,
			func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, 1,
			[]diag.Severity{diag.SeverityWarning},
			"App validation skipped",
		},
		{
			"apps disabled warns", &created, nil,
			func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"message":"Apps is not enabled for this team"}`, http.StatusForbidden)
			}, 1,
			[]diag.Severity{diag.SeverityWarning},
			"App validation unavailable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			s := appTestSchema(t)
			null := tftypes.NewValue(s.Type().TerraformType(ctx), nil)
			raw := func(m *appResourceModel) tftypes.Value {
				if m == nil {
					return null
				}
				return appRaw(t, *m)
			}
			var calls atomic.Int32
			r := &appResource{client: dashboardTestClient(t, countingHandler(&calls, tc.handler))}
			plan := tfsdk.Plan{Schema: s, Raw: raw(tc.plan)}
			resp := &fwresource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(ctx, fwresource.ModifyPlanRequest{
				Plan:  plan,
				State: tfsdk.State{Schema: s, Raw: raw(tc.state)},
			}, resp)

			if n := calls.Load(); n != tc.wantCalls {
				t.Errorf("validate calls = %d, want %d", n, tc.wantCalls)
			}
			if len(resp.Diagnostics) != len(tc.want) {
				t.Fatalf("got %d diagnostics, want %d: %s", len(resp.Diagnostics), len(tc.want), resp.Diagnostics)
			}
			for i, sev := range tc.want {
				d := resp.Diagnostics[i]
				if d.Severity() != sev || d.Summary() != tc.summary {
					t.Errorf("diagnostic %d = %s %q, want %s %q", i, d.Severity(), d.Summary(), sev, tc.summary)
				}
			}
		})
	}
}

func TestAppResource_ModifyPlan_VersionID(t *testing.T) {
	t.Parallel()

	// State may be stale, so even an update that matches state's code and
	// purpose can save a new version: version_id stays unknown on every update.
	state := appModel("c", "p")
	cases := []struct {
		name string
		plan appResourceModel
	}{
		{"rename", func() appResourceModel {
			m := appModel("c", "p")
			m.Name = types.StringValue("Renamed")
			return m
		}()},
		{"code change", appModel("c2", "p")},
		{"purpose change", appModel("c", "p2")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := appTestSchema(t)
			// The framework marks computed attributes unknown on any update.
			tc.plan.VersionID = types.StringUnknown()
			plan := tfsdk.Plan{Schema: s, Raw: appRaw(t, tc.plan)}
			resp := &fwresource.ModifyPlanResponse{Plan: plan}
			(&appResource{}).ModifyPlan(context.Background(), fwresource.ModifyPlanRequest{
				Plan:  plan,
				State: tfsdk.State{Schema: s, Raw: appRaw(t, state)},
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan: %s", resp.Diagnostics)
			}
			var got appResourceModel
			resp.Diagnostics.Append(resp.Plan.Get(context.Background(), &got)...)
			if !got.VersionID.IsUnknown() {
				t.Errorf("version_id = %s, want unknown", got.VersionID)
			}
		})
	}
}

func TestSharingChangeDiagnostics(t *testing.T) {
	t.Parallel()

	shared := appModel("c", "")
	private := appModel("c", "")
	private.IsShared = types.BoolValue(false)
	unknown := appModel("c", "")
	unknown.IsShared = types.BoolUnknown()

	cases := []struct {
		name      string
		plan      appResourceModel
		canChange *bool
		replaced  bool
		wantErr   bool
	}{
		{"not creator, changing", private, boolPtr(false), false, true},
		{"creator, changing", private, boolPtr(true), false, false},
		{"flag unknown, changing", private, nil, false, false},
		{"not creator, unchanged", shared, boolPtr(false), false, false},
		{"not creator, unknown", unknown, boolPtr(false), false, false},
		// A team change replaces the app; the caller creates the new one.
		{"not creator, changing on replace", private, boolPtr(false), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			diags := sharingChangeDiagnostics(tc.plan, shared, tc.canChange, tc.replaced)
			if diags.HasError() != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v: %s", diags.HasError(), tc.wantErr, diags)
			}
			if tc.wantErr && !strings.Contains(diags.Errors()[0].Detail(), "set is_shared back to true") {
				t.Errorf("detail should say how to revert: %q", diags.Errors()[0].Detail())
			}
		})
	}
}

// fakePrivate is an in-memory stand-in for the framework's private state.
type fakePrivate map[string][]byte

func (f fakePrivate) GetKey(_ context.Context, key string) ([]byte, diag.Diagnostics) {
	return f[key], nil
}

func (f fakePrivate) SetKey(_ context.Context, key string, value []byte) diag.Diagnostics {
	if len(value) == 0 {
		delete(f, key)
		return nil
	}
	f[key] = value
	return nil
}

func TestCanChangeIsShared_PrivateStateRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	priv := fakePrivate{}

	for _, want := range []bool{false, true} {
		if d := recordCanChangeIsShared(ctx, priv, &client.App{CanChangeIsShared: &want}); d.HasError() {
			t.Fatalf("record: %s", d)
		}
		got, d := storedCanChangeIsShared(ctx, priv)
		if d.HasError() || got == nil || *got != want {
			t.Errorf("stored = %v (%s), want %v", got, d, want)
		}
	}

	// A response without the field clears the recorded value, so a stale flag
	// can't block the plan.
	if d := recordCanChangeIsShared(ctx, priv, &client.App{}); d.HasError() {
		t.Fatalf("record: %s", d)
	}
	if got, _ := storedCanChangeIsShared(ctx, priv); got != nil {
		t.Errorf("stored = %v after a response without the field, want nil", *got)
	}

	// Unreadable data disables the check rather than failing the plan.
	priv[privateCanChangeIsShared] = []byte(`"yes"`)
	if got, d := storedCanChangeIsShared(ctx, priv); got != nil || d.HasError() {
		t.Errorf("stored = %v (%s) for unreadable data, want nil and no error", got, d)
	}
}

func TestAppResource_Configure(t *testing.T) {
	t.Parallel()
	c := dashboardTestClient(t, http.NotFoundHandler())

	cases := []struct {
		name       string
		data       any
		wantErr    string // summary of the expected error, "" for none
		wantClient bool
	}{
		// The framework configures resources before the provider has data.
		{name: "no provider data yet", data: nil},
		{name: "wrong type", data: "not provider data", wantErr: "Unexpected Resource Configure Type"},
		{name: "ClickStack not configured", data: &service.ProviderData{}, wantErr: "ClickStack not configured"},
		{name: "configured", data: &service.ProviderData{ClickStack: c}, wantClient: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &appResource{}
			resp := &fwresource.ConfigureResponse{}
			r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: tc.data}, resp)

			switch {
			case tc.wantErr == "" && resp.Diagnostics.HasError():
				t.Fatalf("unexpected error: %s", resp.Diagnostics)
			case tc.wantErr != "" && (!resp.Diagnostics.HasError() || resp.Diagnostics.Errors()[0].Summary() != tc.wantErr):
				t.Fatalf("diagnostics = %s, want error %q", resp.Diagnostics, tc.wantErr)
			}
			if got := r.client != nil; got != tc.wantClient {
				t.Errorf("client set = %v, want %v", got, tc.wantClient)
			}
		})
	}
}

func TestAppResource_Schema_BetaNote(t *testing.T) {
	t.Parallel()
	s := appTestSchema(t)
	if !strings.Contains(s.Description, "~> **Note:** This resource is in beta") {
		t.Errorf("Description lacks the beta note: %q", s.Description)
	}
}

func TestAppResource_ApplyApp_NoVersion(t *testing.T) {
	t.Parallel()
	var m appResourceModel
	m.applyApp(&client.App{ID: "a1", Name: "n", IsShared: true})
	if !m.Code.IsNull() || !m.VersionID.IsNull() {
		t.Errorf("expected null code and version_id, got %s / %s", m.Code, m.VersionID)
	}
	if m.Purpose.ValueString() != "" || m.Purpose.IsNull() {
		t.Errorf("expected empty purpose, got %s", m.Purpose)
	}
}

// appStubServer is an in-memory /api/v2/apps backend that versions an app the
// way the API does: a new version id only when code or purpose changes.
func appStubServer(t *testing.T) (http.HandlerFunc, *map[string]client.App) {
	t.Helper()
	apps := map[string]client.App{}
	versions := 0
	newVersion := func() *string {
		versions++
		v := fmt.Sprintf("v%d", versions)
		return &v
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/v2/apps/")
		var in client.AppInput
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &in)
		}
		write := func(a client.App) {
			b, _ := json.Marshal(map[string]client.App{"data": a})
			_, _ = w.Write(b)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/apps":
			a := client.App{ID: "a1", Name: in.Name, IsShared: in.IsShared, Code: &in.Code, Purpose: &in.Purpose, VersionID: newVersion()}
			apps[a.ID] = a
			write(a)
		case r.Method == http.MethodGet:
			a, ok := apps[id]
			if !ok {
				http.Error(w, `{"message":"App not found"}`, http.StatusNotFound)
				return
			}
			write(a)
		case r.Method == http.MethodPut:
			a, ok := apps[id]
			if !ok {
				http.Error(w, `{"message":"App not found"}`, http.StatusNotFound)
				return
			}
			if *a.Code != in.Code || *a.Purpose != in.Purpose {
				a.VersionID = newVersion()
			}
			a.Name, a.IsShared, a.Code, a.Purpose = in.Name, in.IsShared, &in.Code, &in.Purpose
			apps[id] = a
			write(a)
		case r.Method == http.MethodDelete:
			if _, ok := apps[id]; !ok {
				http.Error(w, `{"message":"App not found"}`, http.StatusNotFound)
				return
			}
			delete(apps, id)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}, &apps
}

func TestAppResource_CRUD(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := appTestSchema(t)
	handler, apps := appStubServer(t)
	r := &appResource{client: dashboardTestClient(t, handler)}

	// Create.
	planned := appModel("code-1", "")
	planned.ID = types.StringUnknown()
	planned.VersionID = types.StringUnknown()
	createResp := &fwresource.CreateResponse{State: tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}}
	r.Create(ctx, fwresource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: appRaw(t, planned)}}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %s", createResp.Diagnostics)
	}
	var created appResourceModel
	createResp.Diagnostics.Append(createResp.State.Get(ctx, &created)...)
	if created.ID.ValueString() != "a1" || created.VersionID.ValueString() != "v1" {
		t.Fatalf("unexpected created state: %+v", created)
	}

	// Rename only: no new version.
	renamed := created
	renamed.Name = types.StringValue("Renamed")
	updateResp := &fwresource.UpdateResponse{State: createResp.State}
	r.Update(ctx, fwresource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: appRaw(t, renamed)}}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %s", updateResp.Diagnostics)
	}
	var updated appResourceModel
	updateResp.Diagnostics.Append(updateResp.State.Get(ctx, &updated)...)
	if updated.Name.ValueString() != "Renamed" || updated.VersionID.ValueString() != "v1" {
		t.Errorf("rename changed the version or lost the name: %+v", updated)
	}

	// Drift from the app builder shows up on read.
	a := (*apps)["a1"]
	edited := "edited in builder"
	a.Code = &edited
	(*apps)["a1"] = a
	readResp := &fwresource.ReadResponse{State: updateResp.State}
	r.Read(ctx, fwresource.ReadRequest{State: updateResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %s", readResp.Diagnostics)
	}
	var read appResourceModel
	readResp.Diagnostics.Append(readResp.State.Get(ctx, &read)...)
	if read.Code.ValueString() != edited {
		t.Errorf("read code = %s, want drift %q", read.Code, edited)
	}

	// Delete, then a read drops the resource and a second delete is a no-op.
	deleteResp := &fwresource.DeleteResponse{}
	r.Delete(ctx, fwresource.DeleteRequest{State: readResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %s", deleteResp.Diagnostics)
	}
	goneResp := &fwresource.ReadResponse{State: readResp.State}
	r.Read(ctx, fwresource.ReadRequest{State: readResp.State}, goneResp)
	if goneResp.Diagnostics.HasError() {
		t.Fatalf("Read of a deleted app: %s", goneResp.Diagnostics)
	}
	if !goneResp.State.Raw.IsNull() {
		t.Error("expected Read to remove a deleted app from state")
	}
	r.Delete(ctx, fwresource.DeleteRequest{State: readResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Errorf("deleting a missing app: %s", deleteResp.Diagnostics)
	}
}

func TestAppResource_Update_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := appTestSchema(t)
	handler, _ := appStubServer(t)
	r := &appResource{client: dashboardTestClient(t, handler)}

	plan := tfsdk.Plan{Schema: s, Raw: appRaw(t, appModel("c", ""))}
	resp := &fwresource.UpdateResponse{State: tfsdk.State{Schema: s, Raw: appRaw(t, appModel("c", ""))}}
	r.Update(ctx, fwresource.UpdateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() || resp.Diagnostics.Errors()[0].Summary() != "App No Longer Exists" {
		t.Errorf("expected App No Longer Exists, got %s", resp.Diagnostics)
	}
}

// appRaw encodes a model as a tftypes value of the resource schema.
func appRaw(t *testing.T, m appResourceModel) tftypes.Value {
	t.Helper()
	ctx := context.Background()
	s := appTestSchema(t)
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &m); diags.HasError() {
		t.Fatalf("encode model: %s", diags)
	}
	return state.Raw
}
