package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// Invitation represents an organization invitation returned by the ClickHouse
// Cloud API. Note the JSON key for the identifier is "id" (not "invitationId",
// which is only the path parameter / operationId name).
type Invitation struct {
	ID            string               `json:"id"`
	Email         string               `json:"email"`
	Role          string               `json:"role"`
	AssignedRoles []MemberAssignedRole `json:"assignedRoles"`
	CreatedAt     string               `json:"createdAt"`
	ExpireAt      string               `json:"expireAt"`
}

// CreateInvitationRequest is the body for POST /invitations. Prefer
// AssignedRoleIds; Role is a deprecated legacy alternative ("admin"/"developer").
type CreateInvitationRequest struct {
	Email           string   `json:"email"`
	AssignedRoleIds []string `json:"assignedRoleIds,omitempty"`
	Role            string   `json:"role,omitempty"`
}

func (c *ClientImpl) ListInvitations(ctx context.Context) ([]Invitation, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/invitations"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[[]Invitation]{}
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Result, nil
}

func (c *ClientImpl) GetInvitation(ctx context.Context, invitationId string) (*Invitation, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/invitations/"+invitationId), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[Invitation]{}
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Result, nil
}

func (c *ClientImpl) CreateInvitation(ctx context.Context, req CreateInvitationRequest) (*Invitation, error) {
	rb, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.getOrgPath("/invitations"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[Invitation]{}
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Result, nil
}

func (c *ClientImpl) DeleteInvitation(ctx context.Context, invitationId string) error {
	httpReq, err := http.NewRequest(http.MethodDelete, c.getOrgPath("/invitations/"+invitationId), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, httpReq)
	return err
}
