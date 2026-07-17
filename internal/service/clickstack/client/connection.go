package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const connectionsPath = "/api/v2/connections"

// Connection is a ClickHouse connection as returned by the ClickStack API.
// The password is write-only and never returned by the API.
type Connection struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	Host                 string  `json:"host"`
	Username             string  `json:"username"`
	HyperdxSettingPrefix *string `json:"hyperdxSettingPrefix,omitempty"`
	PrometheusEndpoint   *string `json:"prometheusEndpoint,omitempty"`
}

// CreateConnectionInput is the request body for creating a connection.
// Optional fields are omitted from the request when nil.
type CreateConnectionInput struct {
	Name                 string  `json:"name"`
	Host                 string  `json:"host"`
	Username             string  `json:"username"`
	Password             string  `json:"password"`
	HyperdxSettingPrefix *string `json:"hyperdxSettingPrefix,omitempty"`
	PrometheusEndpoint   *string `json:"prometheusEndpoint,omitempty"`
}

// UpdateConnectionInput is the request body for updating a connection.
// HyperdxSettingPrefix and PrometheusEndpoint are always serialized so that
// nil is sent as JSON null, which the API treats as "clear the existing
// value". Password is omitted when nil, which the API treats as "keep the
// existing password".
type UpdateConnectionInput struct {
	Name                 string  `json:"name"`
	Host                 string  `json:"host"`
	Username             string  `json:"username"`
	Password             *string `json:"password,omitempty"`
	HyperdxSettingPrefix *string `json:"hyperdxSettingPrefix"`
	PrometheusEndpoint   *string `json:"prometheusEndpoint"`
}

// connectionEnvelope wraps single-connection API responses.
type connectionEnvelope struct {
	Data Connection `json:"data"`
}

// connectionListEnvelope wraps connection-list API responses.
type connectionListEnvelope struct {
	Data []Connection `json:"data"`
}

// CreateConnection creates a connection and returns it as stored by the API.
func (c *Client) CreateConnection(ctx context.Context, input CreateConnectionInput) (*Connection, error) {
	// The request intentionally carries the ClickHouse password: this is the
	// API for provisioning credentials. It is sent over the authenticated
	// HTTPS connection and never logged.
	body, err := json.Marshal(input) //nolint:gosec // G117: password in body is the API contract
	if err != nil {
		return nil, fmt.Errorf("encode connection: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPost, connectionsPath, body)
	if err != nil {
		return nil, err
	}

	var resp connectionEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode connection: %w", err)
	}
	return &resp.Data, nil
}

// GetConnection fetches a connection by ID. It returns an error wrapping
// ErrNotFound when the connection does not exist.
func (c *Client) GetConnection(ctx context.Context, id string) (*Connection, error) {
	raw, err := c.do(ctx, http.MethodGet, connectionsPath+"/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}

	var resp connectionEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode connection: %w", err)
	}
	return &resp.Data, nil
}

// ListConnections fetches all connections for the authenticated team.
func (c *Client) ListConnections(ctx context.Context) ([]Connection, error) {
	raw, err := c.do(ctx, http.MethodGet, connectionsPath, nil)
	if err != nil {
		return nil, err
	}

	var resp connectionListEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode connections: %w", err)
	}
	return resp.Data, nil
}

// UpdateConnection updates a connection by ID and returns the updated
// connection. It returns an error wrapping ErrNotFound when the connection
// does not exist.
func (c *Client) UpdateConnection(ctx context.Context, id string, input UpdateConnectionInput) (*Connection, error) {
	// See CreateConnection: the password in the body is the API contract.
	body, err := json.Marshal(input) //nolint:gosec // G117: password in body is the API contract
	if err != nil {
		return nil, fmt.Errorf("encode connection: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPut, connectionsPath+"/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}

	var resp connectionEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode connection: %w", err)
	}
	return &resp.Data, nil
}

// DeleteConnection deletes a connection by ID. It returns an error wrapping
// ErrNotFound when the connection does not exist.
func (c *Client) DeleteConnection(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, connectionsPath+"/"+url.PathEscape(id), nil)
	return err
}
