package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type SnapshotConfiguration struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Gap       *int32 `json:"gap,omitempty"`
	TimeFrame *int32 `json:"timeFrame,omitempty"`
}

func (c *ClientImpl) GetSnapshotConfiguration(ctx context.Context, serviceId string) (*SnapshotConfiguration, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/snapshotConfiguration"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	snapshotConfigResponse := ResponseWithResult[SnapshotConfiguration]{}
	err = json.Unmarshal(body, &snapshotConfigResponse)
	if err != nil {
		return nil, err
	}

	return &snapshotConfigResponse.Result, nil
}

func (c *ClientImpl) UpdateSnapshotConfiguration(ctx context.Context, serviceId string, s SnapshotConfiguration) (*SnapshotConfiguration, error) {
	rb, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, "/snapshotConfiguration"), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	snapshotConfigResponse := ResponseWithResult[SnapshotConfiguration]{}
	err = json.Unmarshal(body, &snapshotConfigResponse)
	if err != nil {
		return nil, err
	}

	return &snapshotConfigResponse.Result, nil
}
