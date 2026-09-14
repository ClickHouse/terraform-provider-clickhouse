package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const QueryAPIEndpointOwnerType = "queryApiEndpoint"

type QueryAPIEndpointRequest struct {
	Name           string            `json:"name"`
	SQL            string            `json:"sql"`
	Database       string            `json:"database"`
	Parameters     map[string]string `json:"parameters"`
	APIKeyIDs      []string          `json:"apiKeyIds"`
	Roles          []string          `json:"roles"`
	AllowedOrigins []string          `json:"allowedOrigins"`
}

type QueryAPIEndpoint struct {
	QueryAPIEndpointRequest
	ID        string `json:"id"`
	URL       string `json:"url"`
	OwnerType string `json:"ownerType"`
}

func (c *ClientImpl) queryAPIEndpointsPath(serviceID, endpointID string) string {
	path := c.getServicePath(serviceID, "/query-api-endpoints")
	if endpointID != "" {
		path += "/" + endpointID
	}
	return path
}

func (c *ClientImpl) GetQueryAPIEndpoint(
	ctx context.Context,
	serviceID string,
	endpointID string,
) (*QueryAPIEndpoint, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		c.queryAPIEndpointsPath(serviceID, endpointID),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create get Query API endpoint request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[QueryAPIEndpoint]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode Query API endpoint %q: %w", endpointID, err)
	}
	return &response.Result, nil
}

func (c *ClientImpl) CreateQueryAPIEndpoint(
	ctx context.Context,
	serviceID string,
	endpoint QueryAPIEndpointRequest,
) (*QueryAPIEndpoint, error) {
	requestBody, err := json.Marshal(endpoint)
	if err != nil {
		return nil, fmt.Errorf("encode create Query API endpoint request: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		c.queryAPIEndpointsPath(serviceID, ""),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("create Query API endpoint request: %w", err)
	}

	body, err := c.doRequestWithAcceptedStatus(ctx, req, http.StatusCreated)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[QueryAPIEndpoint]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode created Query API endpoint: %w", err)
	}
	return &response.Result, nil
}

func (c *ClientImpl) UpdateQueryAPIEndpoint(
	ctx context.Context,
	serviceID string,
	endpointID string,
	endpoint QueryAPIEndpointRequest,
) (*QueryAPIEndpoint, error) {
	requestBody, err := json.Marshal(endpoint)
	if err != nil {
		return nil, fmt.Errorf("encode update Query API endpoint request: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPut,
		c.queryAPIEndpointsPath(serviceID, endpointID),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, fmt.Errorf("create update Query API endpoint request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[QueryAPIEndpoint]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode updated Query API endpoint %q: %w", endpointID, err)
	}
	return &response.Result, nil
}

func (c *ClientImpl) DeleteQueryAPIEndpoint(
	ctx context.Context,
	serviceID string,
	endpointID string,
) error {
	req, err := http.NewRequest(
		http.MethodDelete,
		c.queryAPIEndpointsPath(serviceID, endpointID),
		nil,
	)
	if err != nil {
		return fmt.Errorf("create delete Query API endpoint request: %w", err)
	}

	_, err = c.doRequest(ctx, req)
	return err
}
