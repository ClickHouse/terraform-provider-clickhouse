package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestGetTeam(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/team" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"team1","name":"Acme","defaultUserRole":"role1"}}`)
	})

	team, err := c.GetTeam(context.Background())
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if team.ID != "team1" || team.Name != "Acme" {
		t.Errorf("unexpected team: %+v", team)
	}
	if team.DefaultUserRole == nil || *team.DefaultUserRole != "role1" {
		t.Errorf("expected defaultUserRole role1, got %v", team.DefaultUserRole)
	}
}

func TestGetTeam_NullDefaultUserRole(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"team1","name":"Acme","defaultUserRole":null}}`)
	})

	team, err := c.GetTeam(context.Background())
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if team.DefaultUserRole != nil {
		t.Errorf("expected nil defaultUserRole, got %v", *team.DefaultUserRole)
	}
}

func TestSetDefaultUserRole(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v2/team/defaultUserRole" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["roleId"] != "role2" {
			t.Errorf("unexpected roleId: %q", body["roleId"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"defaultUserRole":"role2"}}`)
	})

	got, err := c.SetDefaultUserRole(context.Background(), "role2")
	if err != nil {
		t.Fatalf("SetDefaultUserRole: %v", err)
	}
	if got == nil || *got != "role2" {
		t.Errorf("expected role2, got %v", got)
	}
}

func TestListTeamMembers(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/team/members" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"u1","email":"a@b.com","roleId":"r1","roleName":"Admin","isCurrentUser":true},{"id":"u2","email":"c@d.com","roleId":"r2","roleName":"Member","isVirtual":true,"accessKey":"key"}]}`)
	})

	members, err := c.ListTeamMembers(context.Background())
	if err != nil {
		t.Fatalf("ListTeamMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if members[0].Email != "a@b.com" || members[0].RoleID != "r1" {
		t.Errorf("unexpected member[0]: %+v", members[0])
	}
	if members[1].AccessKey == nil || *members[1].AccessKey != "key" {
		t.Errorf("expected access key on virtual member, got %v", members[1].AccessKey)
	}
}

func TestInviteTeamMember_Pending(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/team/invitation" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["email"] != "new@user.com" || body["roleId"] != "r1" {
			t.Errorf("unexpected body: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"email":"new@user.com","invitationId":"inv1","userId":null,"status":"pending","url":"https://app/join-team?token=x"}}`)
	})

	res, err := c.InviteTeamMember(context.Background(), InviteTeamMemberInput{
		Email:  "new@user.com",
		RoleID: "r1",
	})
	if err != nil {
		t.Fatalf("InviteTeamMember: %v", err)
	}
	if res.Status != "pending" {
		t.Errorf("expected pending, got %q", res.Status)
	}
	if res.InvitationID == nil || *res.InvitationID != "inv1" {
		t.Errorf("expected invitationId inv1, got %v", res.InvitationID)
	}
	if res.UserID != nil {
		t.Errorf("expected nil userId, got %v", *res.UserID)
	}
}

func TestInviteTeamMember_NoRole(t *testing.T) {
	t.Parallel()

	// OSS has no RBAC: an empty RoleID must be omitted from the request body.
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if _, ok := body["roleId"]; ok {
			t.Errorf("expected roleId to be omitted, got body: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"email":"oss@user.com","invitationId":"inv2","userId":null,"status":"pending","url":"https://app/join-team?token=y"}}`)
	})

	if _, err := c.InviteTeamMember(context.Background(), InviteTeamMemberInput{Email: "oss@user.com"}); err != nil {
		t.Fatalf("InviteTeamMember: %v", err)
	}
}

func TestInviteTeamMember_Active(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"email":"existing@user.com","invitationId":null,"userId":"u9","status":"active","url":""}}`)
	})

	res, err := c.InviteTeamMember(context.Background(), InviteTeamMemberInput{
		Email:  "existing@user.com",
		RoleID: "r1",
	})
	if err != nil {
		t.Fatalf("InviteTeamMember: %v", err)
	}
	if res.Status != "active" {
		t.Errorf("expected active, got %q", res.Status)
	}
	if res.UserID == nil || *res.UserID != "u9" {
		t.Errorf("expected userId u9, got %v", res.UserID)
	}
}

func TestListInvitations(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/team/invitations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"inv1","email":"a@b.com","roleId":"r1"}]}`)
	})

	invites, err := c.ListInvitations(context.Background())
	if err != nil {
		t.Fatalf("ListInvitations: %v", err)
	}
	if len(invites) != 1 || invites[0].ID != "inv1" {
		t.Errorf("unexpected invitations: %+v", invites)
	}
}

func TestDeleteInvitation_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	err := c.DeleteInvitation(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateMemberRole(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/team/members/u1/role" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["roleId"] != "r2" {
			t.Errorf("unexpected roleId: %q", body["roleId"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"userId":"u1","roleId":"r2"}}`)
	})

	if err := c.UpdateMemberRole(context.Background(), "u1", "r2"); err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
}

func TestRemoveMember(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v2/team/members/u1/role" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"success":true}}`)
	})

	if err := c.RemoveMember(context.Background(), "u1"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
}
