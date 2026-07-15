package api

import (
	"context"
	"encoding/json"
	"net/http"
)

type MemberAssignedRole struct {
	RoleID   string `json:"roleId"`
	RoleName string `json:"roleName"`
	RoleType string `json:"roleType"`
}

type Member struct {
	UserID        string               `json:"userId"`
	Name          string               `json:"name"`
	Email         string               `json:"email"`
	Role          string               `json:"role"`
	JoinedAt      string               `json:"joinedAt"`
	AssignedRoles []MemberAssignedRole `json:"assignedRoles"`
}

func (c *ClientImpl) ListMembers(ctx context.Context) ([]Member, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/members"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[[]Member]{}
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Result, nil
}

func (c *ClientImpl) GetMember(ctx context.Context, userID string) (*Member, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/members/"+userID), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[Member]{}
	if err = json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Result, nil
}
