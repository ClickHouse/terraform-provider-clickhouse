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
	// IsPrometheusEndpoint is a pointer because the API omits it for a
	// connection that never set it. Treat an absent value as false.
	IsPrometheusEndpoint *bool `json:"isPrometheusEndpoint,omitempty"`
	// AgentClickhouseRoleName is the ClickHouse role applied to queries AI
	// agents run on their own on the connection. EE only: the API omits it when unset,
	// and OSS servers never return it.
	AgentClickhouseRoleName *string `json:"agentClickhouseRoleName,omitempty"`
}

// CreateConnectionInput is the request body for creating a connection.
// Optional fields are omitted from the request when nil.
type CreateConnectionInput struct {
	Name                    string  `json:"name"`
	Host                    string  `json:"host"`
	Username                string  `json:"username"`
	Password                string  `json:"password"`
	HyperdxSettingPrefix    *string `json:"hyperdxSettingPrefix,omitempty"`
	IsPrometheusEndpoint    bool    `json:"isPrometheusEndpoint"`
	AgentClickhouseRoleName *string `json:"agentClickhouseRoleName,omitempty"`
}

// UpdateConnectionInput is the request body for updating a connection.
// HyperdxSettingPrefix is always serialized so that nil is sent as JSON null,
// which the API treats as "clear the existing value". IsPrometheusEndpoint is
// always serialized because the API keeps the stored value when it is omitted,
// so false has to be sent to turn it off. Password is omitted when nil, which
// the API treats as "keep the existing password". AgentClickhouseRoleName is
// also omitted when nil ("keep the existing role"); a pointer to "" clears it.
// It is not always serialized like HyperdxSettingPrefix because servers that
// predate the field (OSS, older EE) should never see it unless it is in use.
type UpdateConnectionInput struct {
	Name                    string  `json:"name"`
	Host                    string  `json:"host"`
	Username                string  `json:"username"`
	Password                *string `json:"password,omitempty"`
	HyperdxSettingPrefix    *string `json:"hyperdxSettingPrefix"`
	IsPrometheusEndpoint    bool    `json:"isPrometheusEndpoint"`
	AgentClickhouseRoleName *string `json:"agentClickhouseRoleName,omitempty"`
}

// connectionEnvelope wraps single-connection API responses. Warning is a
// non-fatal message the API may attach to a create or update, e.g. when the
// agent role is not granted to the connection's ClickHouse user.
type connectionEnvelope struct {
	Data    Connection `json:"data"`
	Warning string     `json:"warning,omitempty"`
}

// connectionListEnvelope wraps connection-list API responses.
type connectionListEnvelope struct {
	Data []Connection `json:"data"`
}

// CreateConnection creates a connection and returns it as stored by the API,
// along with any non-fatal warning the API attached to the response.
func (c *Client) CreateConnection(ctx context.Context, input CreateConnectionInput) (*Connection, string, error) {
	// The request intentionally carries the ClickHouse password: this is the
	// API for provisioning credentials. It is sent over the authenticated
	// HTTPS connection and never logged.
	body, err := json.Marshal(input) //nolint:gosec // G117: password in body is the API contract
	if err != nil {
		return nil, "", fmt.Errorf("encode connection: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPost, connectionsPath, body)
	if err != nil {
		return nil, "", err
	}

	var resp connectionEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, "", fmt.Errorf("decode connection: %w", err)
	}
	return &resp.Data, resp.Warning, nil
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
// connection, along with any non-fatal warning the API attached to the
// response. It returns an error wrapping ErrNotFound when the connection does
// not exist.
func (c *Client) UpdateConnection(ctx context.Context, id string, input UpdateConnectionInput) (*Connection, string, error) {
	// See CreateConnection: the password in the body is the API contract.
	body, err := json.Marshal(input) //nolint:gosec // G117: password in body is the API contract
	if err != nil {
		return nil, "", fmt.Errorf("encode connection: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPut, connectionsPath+"/"+url.PathEscape(id), body)
	if err != nil {
		return nil, "", err
	}

	var resp connectionEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, "", fmt.Errorf("decode connection: %w", err)
	}
	return &resp.Data, resp.Warning, nil
}

// DeleteConnection deletes a connection by ID. It returns an error wrapping
// ErrNotFound when the connection does not exist.
func (c *Client) DeleteConnection(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, connectionsPath+"/"+url.PathEscape(id), nil)
	return err
}
