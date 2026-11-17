package clickstack

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func TestConnectionResource_Schema(t *testing.T) {
	t.Parallel()

	r := NewConnectionResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "team", "name", "host", "username", "password", "hyperdx_setting_prefix", "is_prometheus_endpoint", "agent_clickhouse_role_name"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected resource schema to contain attribute %q", attr)
		}
	}

	password, ok := resp.Schema.Attributes["password"]
	if !ok {
		t.Fatal("password attribute missing")
	}
	if !password.IsSensitive() {
		t.Error("expected password attribute to be sensitive")
	}
}

// The API omits isPrometheusEndpoint for a connection that never set it. It
// must read as false, the attribute's default, so the connection does not plan
// a change.
func TestConnectionResource_ApplyIsPrometheusEndpoint(t *testing.T) {
	t.Parallel()

	yes, no := true, false
	for name, tc := range map[string]struct {
		in   *bool
		want bool
	}{
		"absent": {nil, false},
		"false":  {&no, false},
		"true":   {&yes, true},
	} {
		var m connectionResourceModel
		m.applyConnection(&client.Connection{IsPrometheusEndpoint: tc.in})
		if !m.IsPrometheusEndpoint.Equal(types.BoolValue(tc.want)) {
			t.Errorf("%s: is_prometheus_endpoint=%s, want %v", name, m.IsPrometheusEndpoint, tc.want)
		}
	}
}

func connectionTestSchema(t *testing.T) rschema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	NewConnectionResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}
	return resp.Schema
}

// connectionTestModel returns a fully known model with the given agent role.
func connectionTestModel(role types.String) connectionResourceModel {
	return connectionResourceModel{
		ID:                      types.StringValue("c1"),
		Team:                    types.StringNull(),
		Name:                    types.StringValue("Test"),
		Host:                    types.StringValue("http://ch:8123"),
		Username:                types.StringValue("default"),
		Password:                types.StringNull(),
		HyperdxSettingPrefix:    types.StringNull(),
		IsPrometheusEndpoint:    types.BoolValue(false),
		AgentClickhouseRoleName: role,
	}
}

func TestConnectionResource_AgentRoleValidator(t *testing.T) {
	t.Parallel()

	attr, ok := connectionTestSchema(t).Attributes["agent_clickhouse_role_name"].(rschema.StringAttribute)
	if !ok {
		t.Fatal("agent_clickhouse_role_name is not a string attribute")
	}
	if !attr.Optional || attr.Computed {
		t.Error("expected agent_clickhouse_role_name to be optional and not computed")
	}

	for value, wantErr := range map[string]bool{
		"agent_ro":   false,
		"Agent_RO_2": false,
		"_":          false,
		"":           true,
		"agent-ro":   true,
		"agent ro":   true,
		"agent.ro":   true,
		"role;DROP":  true,
		"`agent_ro`": true,
		"agent_ro\n": true,
	} {
		resp := &validator.StringResponse{}
		for _, v := range attr.Validators {
			v.ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("agent_clickhouse_role_name"),
				ConfigValue: types.StringValue(value),
			}, resp)
		}
		if resp.Diagnostics.HasError() != wantErr {
			t.Errorf("%q: HasError()=%v, want %v", value, resp.Diagnostics.HasError(), wantErr)
		}
	}
}

// An unset role must be null whether the API omits it or returns "", so an
// omitted attribute never plans a change, including right after import.
func TestConnectionResource_ApplyAgentRole(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		in   *string
		want types.String
	}{
		"absent": {nil, types.StringNull()},
		"empty":  {ptr(""), types.StringNull()},
		"set":    {ptr("agent_ro"), types.StringValue("agent_ro")},
	} {
		m := connectionTestModel(types.StringValue("stale"))
		m.applyConnection(&client.Connection{AgentClickhouseRoleName: tc.in})
		if !m.AgentClickhouseRoleName.Equal(tc.want) {
			t.Errorf("%s: agent_clickhouse_role_name=%s, want %s", name, m.AgentClickhouseRoleName, tc.want)
		}
	}
}

func TestAgentRoleUpdateValue(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		plan, state types.String
		want        *string
	}{
		"unset stays omitted": {types.StringNull(), types.StringNull(), nil},
		"set":                 {types.StringValue("a"), types.StringNull(), ptr("a")},
		"changed":             {types.StringValue("b"), types.StringValue("a"), ptr("b")},
		"removed clears":      {types.StringNull(), types.StringValue("a"), ptr("")},
	} {
		got := agentRoleUpdateValue(tc.plan, tc.state)
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("%s: got %q, want omitted", name, *got)
		case tc.want != nil && (got == nil || *got != *tc.want):
			t.Errorf("%s: got %v, want %q", name, got, *tc.want)
		}
	}
}

func TestConnectionResource_Create_AgentRole(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		planned      types.String
		response     string
		wantSent     *string
		wantState    types.String
		wantWarnings []string
	}{
		{
			name:      "unset is omitted and stays null",
			planned:   types.StringNull(),
			response:  `{"data":{"id":"c1","name":"Test","host":"http://ch:8123","username":"default"}}`,
			wantState: types.StringNull(),
		},
		{
			name:      "set is sent and read back",
			planned:   types.StringValue("agent_ro"),
			wantSent:  ptr("agent_ro"),
			response:  `{"data":{"id":"c1","name":"Test","host":"http://ch:8123","username":"default","agentClickhouseRoleName":"agent_ro"}}`,
			wantState: types.StringValue("agent_ro"),
		},
		{
			name:         "api warning is surfaced",
			planned:      types.StringValue("agent_ro"),
			wantSent:     ptr("agent_ro"),
			response:     `{"data":{"id":"c1","name":"Test","host":"http://ch:8123","username":"default","agentClickhouseRoleName":"agent_ro"},"warning":"role not granted"}`,
			wantState:    types.StringValue("agent_ro"),
			wantWarnings: []string{"ClickStack Connection Warning"},
		},
		{
			// OSS drops the field: keep the planned value so the apply does not
			// fail as inconsistent, and warn that it was not applied.
			name:         "server without the field keeps plan and warns",
			planned:      types.StringValue("agent_ro"),
			wantSent:     ptr("agent_ro"),
			response:     `{"data":{"id":"c1","name":"Test","host":"http://ch:8123","username":"default"}}`,
			wantState:    types.StringValue("agent_ro"),
			wantWarnings: []string{"Agent ClickHouse Role Not Applied"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			sch := connectionTestSchema(t)
			r := &connectionResource{client: dashboardTestClient(t, connectionRoleHandler(t, http.MethodPost, tc.wantSent, tc.response))}

			plan := tfsdk.Plan{Schema: sch}
			m := connectionTestModel(tc.planned)
			m.ID = types.StringUnknown()
			plan.Set(ctx, &m)
			resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: sch}}
			r.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)

			assertConnectionDiags(t, resp.Diagnostics, tc.wantWarnings)
			var got connectionResourceModel
			resp.State.Get(ctx, &got)
			if !got.AgentClickhouseRoleName.Equal(tc.wantState) {
				t.Errorf("agent_clickhouse_role_name=%s, want %s", got.AgentClickhouseRoleName, tc.wantState)
			}
		})
	}
}

func TestConnectionResource_Update_AgentRole(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		prior        types.String
		planned      types.String
		response     string
		wantSent     *string
		wantState    types.String
		wantWarnings []string
	}{
		{
			name:      "unset stays omitted",
			prior:     types.StringNull(),
			planned:   types.StringNull(),
			response:  `{"data":{"id":"c1","name":"Test","host":"http://ch:8123","username":"default"}}`,
			wantState: types.StringNull(),
		},
		{
			name:      "removing from config clears it",
			prior:     types.StringValue("agent_ro"),
			planned:   types.StringNull(),
			wantSent:  ptr(""),
			response:  `{"data":{"id":"c1","name":"Test","host":"http://ch:8123","username":"default"}}`,
			wantState: types.StringNull(),
		},
		{
			name:         "change is sent and warning surfaced",
			prior:        types.StringValue("agent_ro"),
			planned:      types.StringValue("agent_rw"),
			wantSent:     ptr("agent_rw"),
			response:     `{"data":{"id":"c1","name":"Test","host":"http://ch:8123","username":"default","agentClickhouseRoleName":"agent_rw"},"warning":"role not granted"}`,
			wantState:    types.StringValue("agent_rw"),
			wantWarnings: []string{"ClickStack Connection Warning"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			sch := connectionTestSchema(t)
			r := &connectionResource{client: dashboardTestClient(t, connectionRoleHandler(t, http.MethodPut, tc.wantSent, tc.response))}

			state := tfsdk.State{Schema: sch}
			priorModel := connectionTestModel(tc.prior)
			state.Set(ctx, &priorModel)
			plan := tfsdk.Plan{Schema: sch}
			planModel := connectionTestModel(tc.planned)
			plan.Set(ctx, &planModel)
			resp := &fwresource.UpdateResponse{State: state}
			r.Update(ctx, fwresource.UpdateRequest{Plan: plan, State: state}, resp)

			assertConnectionDiags(t, resp.Diagnostics, tc.wantWarnings)
			var got connectionResourceModel
			resp.State.Get(ctx, &got)
			if !got.AgentClickhouseRoleName.Equal(tc.wantState) {
				t.Errorf("agent_clickhouse_role_name=%s, want %s", got.AgentClickhouseRoleName, tc.wantState)
			}
		})
	}
}

// Read reports the server value as-is, so a role cleared out of band on an EE
// server shows up as drift, and an import of a connection without a role
// leaves the attribute null.
func TestConnectionResource_Read_AgentRole(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		prior    types.String
		response string
		want     types.String
	}{
		"server value wins": {
			types.StringValue("agent_ro"),
			`{"data":{"id":"c1","agentClickhouseRoleName":"agent_rw"}}`,
			types.StringValue("agent_rw"),
		},
		"cleared out of band is drift": {
			types.StringValue("agent_ro"),
			`{"data":{"id":"c1"}}`,
			types.StringNull(),
		},
		"import without role is null": {
			types.StringNull(),
			`{"data":{"id":"c1","agentClickhouseRoleName":""}}`,
			types.StringNull(),
		},
		"import with role": {
			types.StringNull(),
			`{"data":{"id":"c1","agentClickhouseRoleName":"agent_ro"}}`,
			types.StringValue("agent_ro"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			sch := connectionTestSchema(t)
			r := &connectionResource{client: dashboardTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.response))
			}))}

			state := tfsdk.State{Schema: sch}
			m := connectionTestModel(tc.prior)
			state.Set(ctx, &m)
			resp := &fwresource.ReadResponse{State: state}
			r.Read(ctx, fwresource.ReadRequest{State: state}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %s", resp.Diagnostics)
			}
			var got connectionResourceModel
			resp.State.Get(ctx, &got)
			if !got.AgentClickhouseRoleName.Equal(tc.want) {
				t.Errorf("agent_clickhouse_role_name=%s, want %s", got.AgentClickhouseRoleName, tc.want)
			}
		})
	}
}

// connectionRoleHandler asserts the method and the agentClickhouseRoleName in
// the request body (nil wantSent means the field must be omitted), then
// replies with response.
func connectionRoleHandler(t *testing.T, method string, wantSent *string, response string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			t.Errorf("method=%s, want %s", r.Method, method)
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		got, ok := body["agentClickhouseRoleName"]
		switch {
		case wantSent == nil && ok:
			t.Errorf("expected agentClickhouseRoleName omitted, got %s", got)
		case wantSent != nil:
			var s string
			if err := json.Unmarshal(got, &s); err != nil || s != *wantSent {
				t.Errorf("agentClickhouseRoleName=%s, want %q", got, *wantSent)
			}
		}
		_, _ = w.Write([]byte(response))
	}
}

// assertConnectionDiags fails on any error and checks the warning summaries.
func assertConnectionDiags(t *testing.T, diags diag.Diagnostics, wantWarnings []string) {
	t.Helper()
	if diags.HasError() {
		t.Fatalf("unexpected errors: %s", diags)
	}
	warnings := diags.Warnings()
	if len(warnings) != len(wantWarnings) {
		t.Fatalf("warnings=%v, want summaries %v", warnings, wantWarnings)
	}
	for i, w := range warnings {
		if w.Summary() != wantWarnings[i] {
			t.Errorf("warning[%d]=%q, want %q", i, w.Summary(), wantWarnings[i])
		}
	}
}
