package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// PostgresPrivateEndpointStateAvailable is the state of a private endpoint
// service that VPC endpoints can connect to.
const PostgresPrivateEndpointStateAvailable = "available"

const postgresPrivateEndpointPath = "/privateEndpoint"

var (
	postgresPrivateEndpointPollInterval = 10 * time.Second
	errPostgresPrivateEndpointNotDone   = errors.New("postgres private endpoint not yet in the expected state")
)

// PostgresAllowedVpcEndpoint is an AWS VPC endpoint allowed to connect to a
// Postgres private endpoint service.
type PostgresAllowedVpcEndpoint struct {
	VpcEndpointId string `json:"vpcEndpointId"`
	Description   string `json:"description"`
}

// PostgresPrivateEndpoint is the AWS private endpoint service of a Postgres
// service. EndpointServiceName and PrivateHostname are empty until the API
// sets them.
type PostgresPrivateEndpoint struct {
	State               string                       `json:"state"`
	EndpointServiceName string                       `json:"endpointServiceName,omitempty"`
	PrivateHostname     string                       `json:"privateHostname,omitempty"`
	SupportedRegions    []string                     `json:"supportedRegions"`
	AllowedVpcEndpoints []PostgresAllowedVpcEndpoint `json:"allowedVpcEndpoints"`
}

// PostgresPrivateEndpointCreate is the body of a create request.
// SupportedRegions are AWS regions other than the service's own.
type PostgresPrivateEndpointCreate struct {
	SupportedRegions []string `json:"supportedRegions,omitempty"`
}

// PostgresPrivateEndpointUpdate is the body of an update request. Each field
// that is set replaces that list in full; a nil field is left unchanged.
type PostgresPrivateEndpointUpdate struct {
	AllowedVpcEndpoints *[]PostgresAllowedVpcEndpoint `json:"allowedVpcEndpoints,omitempty"`
	SupportedRegions    *[]string                     `json:"supportedRegions,omitempty"`
}

// GetPostgresPrivateEndpoint fetches the private endpoint service of a
// Postgres service. A 404 response yields an error for which IsNotFound
// returns true.
func (c *ClientImpl) GetPostgresPrivateEndpoint(ctx context.Context, postgresId string) (*PostgresPrivateEndpoint, error) {
	req, err := http.NewRequest(http.MethodGet, c.getPostgresPath(postgresId, postgresPrivateEndpointPath), nil)
	if err != nil {
		return nil, err
	}
	return c.doPostgresPrivateEndpointRequest(ctx, req)
}

// CreatePostgresPrivateEndpoint creates the private endpoint service of a
// Postgres service. It returns while the service is still `creating`.
func (c *ClientImpl) CreatePostgresPrivateEndpoint(ctx context.Context, postgresId string, body PostgresPrivateEndpointCreate) (*PostgresPrivateEndpoint, error) {
	return c.sendPostgresPrivateEndpoint(ctx, http.MethodPost, postgresId, body)
}

// UpdatePostgresPrivateEndpoint replaces the allowed VPC endpoints and/or the
// supported regions of the private endpoint service of a Postgres service.
func (c *ClientImpl) UpdatePostgresPrivateEndpoint(ctx context.Context, postgresId string, body PostgresPrivateEndpointUpdate) (*PostgresPrivateEndpoint, error) {
	return c.sendPostgresPrivateEndpoint(ctx, http.MethodPatch, postgresId, body)
}

// DeletePostgresPrivateEndpoint schedules deletion of the private endpoint
// service of a Postgres service. 404 → nil (idempotent).
func (c *ClientImpl) DeletePostgresPrivateEndpoint(ctx context.Context, postgresId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getPostgresPath(postgresId, postgresPrivateEndpointPath), nil)
	if err != nil {
		return err
	}
	if _, err := c.doRequest(ctx, req); err != nil && !IsNotFound(err) {
		return err
	}
	return nil
}

// WaitForPostgresPrivateEndpoint polls until done returns true. done receives
// nil once the private endpoint service no longer exists, so the same helper
// waits for creation and for deletion. 5xx and permanent 4xx errors bail
// immediately; other errors retry until the budget runs out.
func (c *ClientImpl) WaitForPostgresPrivateEndpoint(ctx context.Context, postgresId string, done func(*PostgresPrivateEndpoint) bool, maxWaitSeconds int) error {
	maxRetries := uint64(maxWaitSeconds / int(postgresPrivateEndpointPollInterval/time.Second)) //nolint:gosec
	return c.waitForPostgresPrivateEndpointWithInterval(ctx, postgresId, done, postgresPrivateEndpointPollInterval, maxRetries)
}

// waitForPostgresPrivateEndpointWithInterval is the test seam for
// WaitForPostgresPrivateEndpoint.
func (c *ClientImpl) waitForPostgresPrivateEndpointWithInterval(ctx context.Context, postgresId string, done func(*PostgresPrivateEndpoint) bool, interval time.Duration, maxRetries uint64) error {
	lastSeenState := "unknown"
	check := func() error {
		endpoint, err := c.GetPostgresPrivateEndpoint(ctx, postgresId)
		switch {
		case IsNotFound(err):
			endpoint, lastSeenState = nil, "not found"
		case is5xx(err) || Is4xxPermanent(err):
			return backoff.Permanent(err)
		case err != nil:
			return err
		default:
			lastSeenState = endpoint.State
		}
		if done(endpoint) {
			return nil
		}
		return errPostgresPrivateEndpointNotDone
	}
	if maxRetries < 1 {
		maxRetries = 1
	}
	b := backoff.WithContext(backoff.NewConstantBackOff(interval), ctx)
	err := backoff.Retry(check, backoff.WithMaxRetries(b, maxRetries))
	if errors.Is(err, errPostgresPrivateEndpointNotDone) {
		return fmt.Errorf("private endpoint of postgres %s did not reach the expected state in the allocated time (last seen state: %s)", postgresId, lastSeenState)
	}
	return err
}

func (c *ClientImpl) sendPostgresPrivateEndpoint(ctx context.Context, method, postgresId string, body any) (*PostgresPrivateEndpoint, error) {
	rb, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, c.getPostgresPath(postgresId, postgresPrivateEndpointPath), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}
	return c.doPostgresPrivateEndpointRequest(ctx, req)
}

func (c *ClientImpl) doPostgresPrivateEndpointRequest(ctx context.Context, req *http.Request) (*PostgresPrivateEndpoint, error) {
	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	resp := ResponseWithResult[PostgresPrivateEndpoint]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Postgres private endpoint: %w", err)
	}
	return &resp.Result, nil
}
