package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ServiceQueryEndpoint struct {
	Id             string   `json:"id,omitempty"`
	Roles          []string `json:"roles"`
	OpenApiKeys    []string `json:"openApiKeys"`
	AllowedOrigins string   `json:"allowedOrigins"`
}

func (c *ClientImpl) GetQueryEndpoint(ctx context.Context, serviceID string) (*ServiceQueryEndpoint, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceID, "/serviceQueryEndpoint"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if IsNotFound(err) {
		// API respond with 404 when there are no service query endpoints.
		// We don't want to treat this as an error, but respond with nil.
		// This is a potential source of error, because if the `serviceID` is wrong
		// we don't catch the problem here.
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	serviceQueryEndpointResponse := ResponseWithResult[ServiceQueryEndpoint]{}
	err = json.Unmarshal(body, &serviceQueryEndpointResponse)
	if err != nil {
		return nil, err
	}

	return &serviceQueryEndpointResponse.Result, nil
}

func (c *ClientImpl) CreateQueryEndpoint(ctx context.Context, serviceID string, endpoint ServiceQueryEndpoint) (*ServiceQueryEndpoint, error) {
	rb, err := json.Marshal(endpoint)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.getServicePath(serviceID, "/serviceQueryEndpoint"), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[ServiceQueryEndpoint]{}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	return &response.Result, nil
}

func (c *ClientImpl) DeleteQueryEndpoint(ctx context.Context, serviceID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getServicePath(serviceID, "/serviceQueryEndpoint"), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	if IsNotFound(err) {
		// This is what we want
		return nil
	} else if err != nil {
		return err
	}

	return nil
}
