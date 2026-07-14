package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ServicePasswordUpdate struct {
	NewPasswordHash   string `json:"newPasswordHash,omitempty"`
	NewDoubleSha1Hash string `json:"newDoubleSha1Hash,omitempty"`
}

type ServicePasswordUpdateResult struct {
	Password string `json:"password,omitempty"`
}

func (c *ClientImpl) UpdateServicePassword(ctx context.Context, serviceId string, u ServicePasswordUpdate) (*ServicePasswordUpdateResult, error) {
	rb, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, "/password"), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ServicePasswordUpdateResult{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	return &serviceResponse, nil
}
