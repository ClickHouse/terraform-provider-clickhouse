package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ReplicaAwareRouting struct {
	HTTPEnabled   *bool `json:"httpEnabled,omitempty"`
	NativeEnabled *bool `json:"nativeEnabled,omitempty"`
}

func (c *ClientImpl) GetReplicaAwareRouting(ctx context.Context, serviceId string) (*ReplicaAwareRouting, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/replicaAwareRouting"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[ReplicaAwareRouting]{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return &response.Result, nil
}

func (c *ClientImpl) UpdateReplicaAwareRouting(ctx context.Context, serviceId string, routing ReplicaAwareRouting) (*ReplicaAwareRouting, error) {
	rb, err := json.Marshal(routing)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, "/replicaAwareRouting"), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[ReplicaAwareRouting]{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return &response.Result, nil
}
