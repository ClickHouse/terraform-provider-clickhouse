package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

type RBACAllowDeny string

const (
	RBACAllowDenyAllow RBACAllowDeny = "ALLOW"
	RBACAllowDenyDeny  RBACAllowDeny = "DENY"
)

var RBACAllowDenyValues = []string{string(RBACAllowDenyAllow), string(RBACAllowDenyDeny)}

const (
	RBACRoleTypeSystem = "system"
	RBACRoleTypeCustom = "custom"
)

// Actor type prefixes used in the API's "type/id" actor format.
const (
	ActorTypeUser   = "user"
	ActorTypeAPIKey = "apiKey"
)

const (
	RBACPolicyRoleV2SQLConsoleReadonly = "sql-console-readonly"
	RBACPolicyRoleV2SQLConsoleAdmin    = "sql-console-admin"
)

var RBACPolicyRoleV2Values = []string{RBACPolicyRoleV2SQLConsoleReadonly, RBACPolicyRoleV2SQLConsoleAdmin}

type RBACPolicyTags struct {
	Grants []string `json:"grants,omitempty"`
	RoleV2 string   `json:"roleV2,omitempty"`
}

type RBACPolicy struct {
	ID          string          `json:"id"`
	RoleID      string          `json:"roleId"`
	TenantID    string          `json:"tenantId"`
	AllowDeny   RBACAllowDeny   `json:"allowDeny"`
	Permissions []string        `json:"permissions"`
	Resources   []string        `json:"resources"`
	Tags        *RBACPolicyTags `json:"tags"`
}

type RBACPolicyCreateRequest struct {
	AllowDeny   RBACAllowDeny   `json:"allowDeny"`
	Permissions []string        `json:"permissions"`
	Resources   []string        `json:"resources"`
	Tags        *RBACPolicyTags `json:"tags,omitempty"`
}

type RBACRole struct {
	ID        string       `json:"id"`
	TenantID  string       `json:"tenantId"`
	OwnerID   string       `json:"ownerId"`
	Name      string       `json:"name"`
	Type      string       `json:"type"`
	Actors    []string     `json:"actors"`
	Policies  []RBACPolicy `json:"policies"`
	CreatedAt string       `json:"createdAt"`
	UpdatedAt string       `json:"updatedAt"`
}

type RoleCreateRequest struct {
	Name     string                    `json:"name"`
	Actors   []string                  `json:"actors"`
	Policies []RBACPolicyCreateRequest `json:"policies"`
}

type RoleUpdateRequest struct {
	Name     string                     `json:"name,omitempty"`
	Actors   *[]string                  `json:"actors,omitempty"`
	Policies *[]RBACPolicyCreateRequest `json:"policies,omitempty"`
}

func (c *ClientImpl) ListRoles(ctx context.Context) ([]RBACRole, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/roles"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	rolesResponse := ResponseWithResult[[]RBACRole]{}
	if err = json.Unmarshal(body, &rolesResponse); err != nil {
		return nil, err
	}

	return rolesResponse.Result, nil
}

func (c *ClientImpl) GetRole(ctx context.Context, roleId string) (*RBACRole, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/roles/"+roleId), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	roleResponse := ResponseWithResult[RBACRole]{}
	if err = json.Unmarshal(body, &roleResponse); err != nil {
		return nil, err
	}

	return &roleResponse.Result, nil
}

func (c *ClientImpl) CreateRole(ctx context.Context, req RoleCreateRequest) (*RBACRole, error) {
	rb, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.getOrgPath("/roles"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	roleResponse := ResponseWithResult[RBACRole]{}
	if err = json.Unmarshal(body, &roleResponse); err != nil {
		return nil, err
	}

	return &roleResponse.Result, nil
}

func (c *ClientImpl) UpdateRole(ctx context.Context, roleId string, req RoleUpdateRequest) (*RBACRole, error) {
	rb, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(http.MethodPatch, c.getOrgPath("/roles/"+roleId), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	roleResponse := ResponseWithResult[RBACRole]{}
	if err = json.Unmarshal(body, &roleResponse); err != nil {
		return nil, err
	}

	return &roleResponse.Result, nil
}

func (c *ClientImpl) DeleteRole(ctx context.Context, roleId string) error {
	httpReq, err := http.NewRequest(http.MethodDelete, c.getOrgPath("/roles/"+roleId), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, httpReq)
	return err
}
