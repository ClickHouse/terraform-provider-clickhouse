package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const teamPath = "/api/v2/team"

// Team holds the mutable team settings exposed by the ClickStack API. The team
// itself is provisioned out-of-band (at signup); only its settings are
// manageable here. DefaultUserRole is nil when the team has no configured
// default new-user role.
type Team struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	DefaultUserRole *string `json:"defaultUserRole"`
}

// TeamMember is a user (real or virtual) with an assigned role in the team.
type TeamMember struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	Name          *string `json:"name,omitempty"`
	IsVirtual     bool    `json:"isVirtual"`
	RoleID        string  `json:"roleId"`
	RoleName      string  `json:"roleName"`
	IsCurrentUser bool    `json:"isCurrentUser"`
	AccessKey     *string `json:"accessKey,omitempty"`
}

// TeamInvitation is a pending invitation to join the team.
type TeamInvitation struct {
	ID     string  `json:"id"`
	Email  string  `json:"email"`
	Name   *string `json:"name,omitempty"`
	RoleID string  `json:"roleId"`
}

// InviteTeamMemberInput is the request body for inviting a member. RoleID is
// omitted when empty: OSS deployments have no RBAC and accept invitations
// without a role.
type InviteTeamMemberInput struct {
	Email  string  `json:"email"`
	RoleID string  `json:"roleId,omitempty"`
	Name   *string `json:"name,omitempty"`
}

// InviteResult is the outcome of an invitation. When the invitee already has
// an account the role is assigned immediately (Status "active", UserID set);
// otherwise a pending invitation is created (Status "pending", InvitationID
// and URL set).
type InviteResult struct {
	Email        string  `json:"email"`
	InvitationID *string `json:"invitationId"`
	UserID       *string `json:"userId"`
	Status       string  `json:"status"`
	URL          string  `json:"url"`
}

type teamEnvelope struct {
	Data Team `json:"data"`
}

type teamMemberListEnvelope struct {
	Data []TeamMember `json:"data"`
}

type teamInvitationListEnvelope struct {
	Data []TeamInvitation `json:"data"`
}

type inviteResultEnvelope struct {
	Data InviteResult `json:"data"`
}

type defaultUserRoleEnvelope struct {
	Data struct {
		DefaultUserRole *string `json:"defaultUserRole"`
	} `json:"data"`
}

// GetTeam fetches the team settings for the authenticated team.
func (c *Client) GetTeam(ctx context.Context) (*Team, error) {
	raw, err := c.do(ctx, http.MethodGet, teamPath, nil)
	if err != nil {
		return nil, err
	}

	var resp teamEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode team: %w", err)
	}
	return &resp.Data, nil
}

// SetDefaultUserRole sets the role assigned to new users joining the team and
// returns the resulting default role ID.
func (c *Client) SetDefaultUserRole(ctx context.Context, roleID string) (*string, error) {
	body, err := json.Marshal(struct {
		RoleID string `json:"roleId"`
	}{RoleID: roleID})
	if err != nil {
		return nil, fmt.Errorf("encode default user role: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPatch, teamPath+"/defaultUserRole", body)
	if err != nil {
		return nil, err
	}

	var resp defaultUserRoleEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode default user role: %w", err)
	}
	return resp.Data.DefaultUserRole, nil
}

// ListTeamMembers fetches the team's members and their assigned roles.
func (c *Client) ListTeamMembers(ctx context.Context) ([]TeamMember, error) {
	raw, err := c.do(ctx, http.MethodGet, teamPath+"/members", nil)
	if err != nil {
		return nil, err
	}

	var resp teamMemberListEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode team members: %w", err)
	}
	return resp.Data, nil
}

// InviteTeamMember invites a user to the team with the given role.
func (c *Client) InviteTeamMember(ctx context.Context, input InviteTeamMemberInput) (*InviteResult, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode invitation: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPost, teamPath+"/invitation", body)
	if err != nil {
		return nil, err
	}

	var resp inviteResultEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode invitation: %w", err)
	}
	return &resp.Data, nil
}

// ListInvitations fetches the team's pending invitations.
func (c *Client) ListInvitations(ctx context.Context) ([]TeamInvitation, error) {
	raw, err := c.do(ctx, http.MethodGet, teamPath+"/invitations", nil)
	if err != nil {
		return nil, err
	}

	var resp teamInvitationListEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode invitations: %w", err)
	}
	return resp.Data, nil
}

// DeleteInvitation deletes a pending invitation by ID. It returns an error
// wrapping ErrNotFound when the invitation does not exist.
func (c *Client) DeleteInvitation(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, teamPath+"/invitation/"+url.PathEscape(id), nil)
	return err
}

// UpdateMemberRole updates the role assigned to a team member.
func (c *Client) UpdateMemberRole(ctx context.Context, userID, roleID string) error {
	body, err := json.Marshal(struct {
		RoleID string `json:"roleId"`
	}{RoleID: roleID})
	if err != nil {
		return fmt.Errorf("encode member role: %w", err)
	}

	_, err = c.do(ctx, http.MethodPut, teamPath+"/members/"+url.PathEscape(userID)+"/role", body)
	return err
}

// RemoveMember removes a member's role assignment, removing them from the team.
// It returns an error wrapping ErrNotFound when the member is not assigned.
func (c *Client) RemoveMember(ctx context.Context, userID string) error {
	_, err := c.do(ctx, http.MethodDelete, teamPath+"/members/"+url.PathEscape(userID)+"/role", nil)
	return err
}
