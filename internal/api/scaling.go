package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ReplicaScalingUpdate struct {
	IdleScaling        *bool `json:"idleScaling,omitempty"` // bool pointer so that `false`` is not omitted
	MinReplicaMemoryGb *int  `json:"minReplicaMemoryGb,omitempty"`
	MaxReplicaMemoryGb *int  `json:"maxReplicaMemoryGb,omitempty"`
	NumReplicas        *int  `json:"numReplicas,omitempty"`
	IdleTimeoutMinutes *int  `json:"idleTimeoutMinutes,omitempty"`
}

func (c *ClientImpl) UpdateReplicaScaling(ctx context.Context, serviceId string, s ReplicaScalingUpdate) (*Service, error) {
	rb, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, "/replicaScaling"), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ResponseWithResult[Service]{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	return &serviceResponse.Result, nil
}
