// Package client implements the ClickStack HTTP API client. It is
// deliberately free of any terraform-plugin-framework types: it only speaks
// HTTP and JSON. The provider layer translates between Terraform models and
// the types defined here.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrNotFound is returned when the API responds with a 404 for a resource.
// Callers can use errors.Is to detect it, e.g. to drop a deleted resource
// from Terraform state.
var ErrNotFound = errors.New("not found")

// Client is a ClickStack API client authenticating with a Bearer API key.
type Client struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	// teamID, when non-empty, is sent as the x-hdx-team header to select the
	// active team. It is only honored by multi-team (EE) deployments, which
	// validate the API key's membership in the team server-side; single-team
	// (OSS) deployments ignore it. Empty means "use the API key's team".
	teamID string
}

// WithTeam returns a shallow copy of the Client scoped to teamID, so callers
// can target a specific team per request without mutating the shared client.
// An empty teamID (or one matching the current scope) returns the receiver
// unchanged, leaving team selection to the server.
func (c *Client) WithTeam(teamID string) *Client {
	if teamID == "" || teamID == c.teamID {
		return c
	}
	clone := *c
	clone.teamID = teamID
	return &clone
}

// New returns a Client for the API at endpoint. The endpoint is the base URL
// of the ClickStack API without the /api/v2 suffix, e.g.
// "https://api.hyperdx.io" or "http://localhost:8000".
func New(endpoint, apiKey string, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint %q: %w", endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("endpoint %q must use http or https", endpoint)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		httpClient: httpClient,
		endpoint:   strings.TrimRight(endpoint, "/"),
		apiKey:     apiKey,
	}, nil
}

// apiError is the error body returned by the ClickStack API.
type apiError struct {
	Message string `json:"message"`
}

// do sends an API request with an optional pre-encoded JSON body and returns
// the raw response body. Callers decode the result into their concrete
// response type.
func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.teamID != "" {
		req.Header.Set("x-hdx-team", c.teamID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing actionable on close failure

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s %s: %w", method, path, ErrNotFound)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var ae apiError
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) //nolint:errcheck // best-effort error body
		if jsonErr := json.Unmarshal(raw, &ae); jsonErr == nil && ae.Message != "" {
			return nil, fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, ae.Message)
		}
		return nil, fmt.Errorf("%s %s: unexpected status %d", method, path, resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: read response: %w", method, path, err)
	}

	return raw, nil
}
