package clickstack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	rschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func strPtr(s string) *string { return &s }

func TestTeamMemberResource_Schema(t *testing.T) {
	t.Parallel()

	r := NewTeamMemberResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "team", "email", "name", "role_id", "status", "invite_url"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected resource schema to contain attribute %q", attr)
		}
	}

	roleID := resp.Schema.Attributes["role_id"]
	if !roleID.IsOptional() {
		t.Error("expected role_id to be optional (OSS has no RBAC)")
	}
	// Computed lets state track a server-assigned role (e.g. the team default
	// on RBAC deployments) without a perpetual diff when role_id is omitted.
	if !roleID.IsComputed() {
		t.Error("expected role_id to be computed")
	}

	inviteURL, ok := resp.Schema.Attributes["invite_url"]
	if !ok {
		t.Fatal("invite_url attribute missing")
	}
	if !inviteURL.IsSensitive() {
		t.Error("expected invite_url attribute to be sensitive")
	}
}

func TestResolveRoleID(t *testing.T) {
	t.Parallel()

	// Server returns a role: use it.
	if got := resolveRoleID(types.StringValue("old"), "new"); got.ValueString() != "new" {
		t.Errorf("server role ignored: got %q, want new", got.ValueString())
	}

	// Server returns empty (OSS, no RBAC): a prior non-empty role must survive
	// rather than being reset to empty (drift-preservation guard).
	if got := resolveRoleID(types.StringValue("old"), ""); got.ValueString() != "old" {
		t.Errorf("prior role not preserved on empty response: got %q, want old", got.ValueString())
	}

	// Server empty and prior null: stays null (no config-vs-state drift).
	if got := resolveRoleID(types.StringNull(), ""); !got.IsNull() {
		t.Errorf("null prior not preserved: got %q", got.ValueString())
	}

	// Role omitted in config, RBAC server returns the team default role:
	// state tracks the server role, and role_id being Computed means the
	// tracked value causes no diff against the null config.
	if got := resolveRoleID(types.StringNull(), "team-default"); got.ValueString() != "team-default" {
		t.Errorf("server default role not adopted: got %q, want team-default", got.ValueString())
	}
}

// TestTeamMemberResource_Read drives the Read handler against a stub API,
// exercising resolveRoleID through both the active-member and pending-invite
// branches (it is otherwise only unit-tested in isolation).
func TestTeamMemberResource_Read(t *testing.T) {
	t.Parallel()

	const email = "user@example.com"

	memberSchema := func() rschema.Schema {
		resp := &fwresource.SchemaResponse{}
		(&teamMemberResource{}).Schema(context.Background(), fwresource.SchemaRequest{}, resp)
		return resp.Schema
	}()

	// stateWithRole builds a prior state with the given (nullable) role_id.
	stateWithRole := func(priorRole *string) tftypes.Value {
		null := tftypes.NewValue(tftypes.String, nil)
		role := null
		if priorRole != nil {
			role = tftypes.NewValue(tftypes.String, *priorRole)
		}
		return tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			idAttr: tftypes.String, teamAttr: tftypes.String, emailAttr: tftypes.String,
			nameAttr: tftypes.String, roleIDAttr: tftypes.String, statusAttr: tftypes.String, inviteURLAttr: tftypes.String,
		}}, map[string]tftypes.Value{
			idAttr: tftypes.NewValue(tftypes.String, "old-id"), teamAttr: null,
			emailAttr: tftypes.NewValue(tftypes.String, email), nameAttr: null,
			roleIDAttr: role, statusAttr: null, inviteURLAttr: null,
		})
	}

	// mux serves the members list, then falls back to invitations. members/invs
	// are raw JSON arrays for the respective "data" envelopes.
	newClient := func(t *testing.T, members, invs string) *client.Client {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/v2/team/members", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":` + members + `}`))
		})
		mux.HandleFunc("/api/v2/team/invitations", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":` + invs + `}`))
		})
		server := httptest.NewServer(mux)
		t.Cleanup(server.Close)
		c, err := client.New(server.URL, "test-key", server.Client())
		if err != nil {
			t.Fatalf("client.New: %v", err)
		}
		return c
	}

	cases := []struct {
		name        string
		priorRole   *string
		members     string
		invs        string
		wantRemoved bool
		wantStatus  string
		wantRole    string // "" asserts the role_id is null
	}{
		{
			name:       "active member adopts server role",
			priorRole:  strPtr("old"),
			members:    `[{"id":"u1","email":"` + email + `","roleId":"admin"}]`,
			wantStatus: memberStatusActive,
			wantRole:   "admin",
		},
		{
			name:       "active member with empty server role preserves prior (OSS drift guard)",
			priorRole:  strPtr("old"),
			members:    `[{"id":"u1","email":"` + email + `","roleId":""}]`,
			wantStatus: memberStatusActive,
			wantRole:   "old",
		},
		{
			name:       "pending invite adopts invite role",
			priorRole:  nil,
			members:    `[]`,
			invs:       `[{"id":"inv1","email":"` + email + `","roleId":"viewer"}]`,
			wantStatus: memberStatusPending,
			wantRole:   "viewer",
		},
		{
			name:        "neither member nor invite removes resource",
			priorRole:   strPtr("old"),
			members:     `[]`,
			invs:        `[]`,
			wantRemoved: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &teamMemberResource{client: newClient(t, tc.members, tc.invs)}
			resp := &fwresource.ReadResponse{State: tfsdk.State{Schema: memberSchema}}
			r.Read(context.Background(), fwresource.ReadRequest{
				State: tfsdk.State{Schema: memberSchema, Raw: stateWithRole(tc.priorRole)},
			}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %s", resp.Diagnostics)
			}
			if tc.wantRemoved {
				if !resp.State.Raw.IsNull() {
					t.Error("expected resource removed from state")
				}
				return
			}
			var got teamMemberResourceModel
			resp.State.Get(context.Background(), &got)
			if got.Status.ValueString() != tc.wantStatus {
				t.Errorf("status=%q, want %q", got.Status.ValueString(), tc.wantStatus)
			}
			if tc.wantRole == "" {
				if !got.RoleID.IsNull() {
					t.Errorf("role_id=%q, want null", got.RoleID.ValueString())
				}
			} else if got.RoleID.ValueString() != tc.wantRole {
				t.Errorf("role_id=%q, want %q", got.RoleID.ValueString(), tc.wantRole)
			}
		})
	}
}

func TestApplyInviteResult(t *testing.T) {
	t.Parallel()

	// Active result: id is the user ID, invite URL is cleared.
	var active teamMemberResourceModel
	applyInviteResult(&active, &client.InviteResult{
		Status: memberStatusActive,
		UserID: strPtr("user-1"),
		URL:    "",
	})
	if active.Status.ValueString() != memberStatusActive {
		t.Errorf("expected active status, got %q", active.Status.ValueString())
	}
	if active.ID.ValueString() != "user-1" {
		t.Errorf("expected id user-1, got %q", active.ID.ValueString())
	}
	if active.InviteURL.ValueString() != "" {
		t.Errorf("expected empty invite_url, got %q", active.InviteURL.ValueString())
	}

	// Pending result: id is the invitation ID, invite URL is populated.
	var pending teamMemberResourceModel
	applyInviteResult(&pending, &client.InviteResult{
		Status:       memberStatusPending,
		InvitationID: strPtr("inv-1"),
		URL:          "https://app/join-team?token=x",
	})
	if pending.Status.ValueString() != memberStatusPending {
		t.Errorf("expected pending status, got %q", pending.Status.ValueString())
	}
	if pending.ID.ValueString() != "inv-1" {
		t.Errorf("expected id inv-1, got %q", pending.ID.ValueString())
	}
	if pending.InviteURL.ValueString() != "https://app/join-team?token=x" {
		t.Errorf("unexpected invite_url: %q", pending.InviteURL.ValueString())
	}

	// An unknown role_id (Computed attribute omitted from config on create)
	// must be normalized to null: the invite API returns no role, and an
	// unknown value in state fails apply with "value is unknown after apply".
	unknown := teamMemberResourceModel{RoleID: types.StringUnknown()}
	applyInviteResult(&unknown, &client.InviteResult{Status: memberStatusActive, UserID: strPtr("user-1")})
	if !unknown.RoleID.IsNull() {
		t.Errorf("expected unknown role_id to be normalized to null, got %v", unknown.RoleID)
	}

	// A known role_id passes through untouched.
	known := teamMemberResourceModel{RoleID: types.StringValue("role-1")}
	applyInviteResult(&known, &client.InviteResult{Status: memberStatusActive, UserID: strPtr("user-1")})
	if known.RoleID.ValueString() != "role-1" {
		t.Errorf("expected role_id role-1 to be preserved, got %q", known.RoleID.ValueString())
	}
}
