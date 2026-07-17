package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const rolesPath = "/api/v2/roles"

// Permission is a single CASL permission rule on a Role. Conditions is an
// integration-specific JSON object (MongoDB filter or ClickHouse row policy)
// kept opaque so the provider does not need to model every variant.
type Permission struct {
	Action      string          `json:"action"`
	Subject     string          `json:"subject"`
	Integration string          `json:"integration"`
	Inverted    *bool           `json:"inverted,omitempty"`
	Fields      []string        `json:"fields,omitempty"`
	Conditions  json.RawMessage `json:"conditions,omitempty"`
}

// Role is an RBAC role as returned by the ClickStack API. Predefined roles
// (Admin, Member, ReadOnly) have IsPredefined true and cannot be modified or
// deleted.
type Role struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Description  *string      `json:"description,omitempty"`
	Permissions  []Permission `json:"permissions"`
	IsPredefined bool         `json:"isPredefined"`
}

// CreateRoleInput is the request body for creating a custom role.
type CreateRoleInput struct {
	Name        string       `json:"name"`
	Description *string      `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
}

// UpdateRoleInput is the request body for updating a custom role. Permissions
// is always sent; Name and Description are omitted when nil so the existing
// value is kept.
type UpdateRoleInput struct {
	Name        *string      `json:"name,omitempty"`
	Description *string      `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
}

type roleEnvelope struct {
	Data Role `json:"data"`
}

type roleListEnvelope struct {
	Data []Role `json:"data"`
}

// CreateRole creates a custom role and returns it as stored by the API.
func (c *Client) CreateRole(ctx context.Context, input CreateRoleInput) (*Role, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode role: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPost, rolesPath, body)
	if err != nil {
		return nil, err
	}

	var resp roleEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode role: %w", err)
	}
	return &resp.Data, nil
}

// GetRole fetches a role by ID. It returns an error wrapping ErrNotFound when
// the role does not exist.
func (c *Client) GetRole(ctx context.Context, id string) (*Role, error) {
	raw, err := c.do(ctx, http.MethodGet, rolesPath+"/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}

	var resp roleEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode role: %w", err)
	}
	return &resp.Data, nil
}

// ListRoles fetches all roles for the authenticated team, including predefined
// roles.
func (c *Client) ListRoles(ctx context.Context) ([]Role, error) {
	raw, err := c.do(ctx, http.MethodGet, rolesPath, nil)
	if err != nil {
		return nil, err
	}

	var resp roleListEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode roles: %w", err)
	}
	return resp.Data, nil
}

// UpdateRole updates a custom role by ID and returns the updated role. It
// returns an error wrapping ErrNotFound when the role does not exist.
func (c *Client) UpdateRole(ctx context.Context, id string, input UpdateRoleInput) (*Role, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode role: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPut, rolesPath+"/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}

	var resp roleEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode role: %w", err)
	}
	return &resp.Data, nil
}

// DeleteRole deletes a custom role by ID. It returns an error wrapping
// ErrNotFound when the role does not exist.
func (c *Client) DeleteRole(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, rolesPath+"/"+url.PathEscape(id), nil)
	return err
}
