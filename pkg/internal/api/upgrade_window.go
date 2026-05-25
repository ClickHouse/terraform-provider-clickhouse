package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// UpgradeWindow mirrors the OpenAPI public UpgradeWindowV1 shape: a weekly
// recurring window during which the data plane is allowed to perform service
// upgrades.
type UpgradeWindow struct {
	Weekday      int `json:"weekday"`
	StartHourUtc int `json:"startHourUtc"`
	Duration     int `json:"duration"`
}

// UpgradeWindowUpdate is the PUT request body. The server fixes `duration` so
// it is intentionally omitted from the request — matching the OpenAPI
// UpgradeWindowV1PutRequest shape.
type UpgradeWindowUpdate struct {
	Weekday      int `json:"weekday"`
	StartHourUtc int `json:"startHourUtc"`
}

func (c *ClientImpl) GetUpgradeWindow(ctx context.Context, serviceId string) (*UpgradeWindow, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/upgradeWindow"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[UpgradeWindow]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Result, nil
}

func (c *ClientImpl) UpdateUpgradeWindow(ctx context.Context, serviceId string, u UpgradeWindowUpdate) (*UpgradeWindow, error) {
	rb, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPut, c.getServicePath(serviceId, "/upgradeWindow"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[UpgradeWindow]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Result, nil
}

func (c *ClientImpl) DeleteUpgradeWindow(ctx context.Context, serviceId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getServicePath(serviceId, "/upgradeWindow"), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
}
