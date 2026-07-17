package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestCreateRole(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v2/roles" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["name"] != "Editor" {
			t.Errorf("unexpected body: %v", body)
		}
		perms, ok := body["permissions"].([]any)
		if !ok || len(perms) != 1 {
			t.Fatalf("expected 1 permission, got %v", body["permissions"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"role1","name":"Editor","isPredefined":false,"permissions":[{"action":"read","subject":"Dashboard","integration":"mongodb","inverted":false}]}}`)
	})

	role, err := c.CreateRole(context.Background(), CreateRoleInput{
		Name: "Editor",
		Permissions: []Permission{
			{Action: "read", Subject: "Dashboard", Integration: "mongodb", Inverted: boolPtr(false)},
		},
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if role.ID != "role1" {
		t.Errorf("expected id role1, got %q", role.ID)
	}
	if role.IsPredefined {
		t.Error("expected IsPredefined false")
	}
	if len(role.Permissions) != 1 || role.Permissions[0].Subject != "Dashboard" {
		t.Errorf("unexpected permissions: %v", role.Permissions)
	}
}

func TestCreateRole_WithConditions(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body struct {
			Permissions []map[string]json.RawMessage `json:"permissions"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if len(body.Permissions) != 1 {
			t.Fatalf("expected 1 permission, got %d", len(body.Permissions))
		}
		if string(body.Permissions[0]["conditions"]) != `{"name":"Prod"}` {
			t.Errorf("unexpected conditions: %s", body.Permissions[0]["conditions"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"role2","name":"Cond","isPredefined":false,"permissions":[{"action":"read","subject":"Source","integration":"mongodb","conditions":{"name":"Prod"}}]}}`)
	})

	role, err := c.CreateRole(context.Background(), CreateRoleInput{
		Name: "Cond",
		Permissions: []Permission{
			{Action: "read", Subject: "Source", Integration: "mongodb", Conditions: json.RawMessage(`{"name":"Prod"}`)},
		},
	})
	if err != nil {
		t.Fatalf("CreateRole: %v", err)
	}
	if string(role.Permissions[0].Conditions) != `{"name":"Prod"}` {
		t.Errorf("unexpected conditions round-trip: %s", role.Permissions[0].Conditions)
	}
}

func TestGetRole_NotFound(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := c.GetRole(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListRoles(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/roles" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"a","name":"Admin","isPredefined":true},{"id":"b","name":"Member","isPredefined":true}]}`)
	})

	roles, err := c.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

func TestUpdateRole(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v2/roles/role1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]json.RawMessage
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		// Permissions are always sent; nil name is omitted.
		if _, ok := body["permissions"]; !ok {
			t.Error("expected permissions in update body")
		}
		if _, ok := body["name"]; ok {
			t.Error("expected nil name to be omitted from update body")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"role1","name":"Editor","isPredefined":false,"permissions":[]}}`)
	})

	_, err := c.UpdateRole(context.Background(), "role1", UpdateRoleInput{
		Permissions: []Permission{},
	})
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
}

func TestDeleteRole(t *testing.T) {
	t.Parallel()

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v2/roles/role1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"success":true}}`)
	})

	if err := c.DeleteRole(context.Background(), "role1"); err != nil {
		t.Fatalf("DeleteRole: %v", err)
	}
}
