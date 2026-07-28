// Package client implements the ClickStack HTTP API client. It is
// deliberately free of any terraform-plugin-framework types: it only speaks
// HTTP and JSON. The provider layer translates between Terraform models and
// the types defined here.
package client

import (
	"bytes"
	"cmp"
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

// ErrCloudUnsupported is returned in cloud mode for concepts the Cloud API
// does not model — currently team scoping: a Cloud service is a single
// ClickStack team, so x-hdx-team has nothing to select. Endpoint coverage is
// deliberately NOT gated client-side: the Cloud API is still rolling out the
// full ClickStack surface, and a hard-coded allowlist would block newly
// exposed endpoints until a provider release. Unrouted paths surface as
// errRouteNotFound from the server instead.
var ErrCloudUnsupported = errors.New("not supported by ClickStack on ClickHouse Cloud")

// errRouteNotFound marks a 404 whose body is not the API's JSON error shape —
// the Cloud gateway's HTML "Cannot GET/POST" page for a path it does not
// serve. It must stay distinct from ErrNotFound: Terraform treats ErrNotFound
// on read as "resource deleted" and removes state.
var errRouteNotFound = errors.New("route not found")

// Client is a ClickStack API client. It speaks either to a self-hosted
// ClickStack API (Bearer API key, /api/v2 paths) or, in cloud mode, to
// ClickStack managed by ClickHouse Cloud through the Cloud OpenAPI (HTTP basic
// auth with a Cloud API key, org/service-scoped paths). do() translates paths,
// auth and response envelopes so per-resource methods are largely
// mode-agnostic; the rare exceptions branch on cloud explicitly (e.g.
// GetSource's list+filter fallback).
type Client struct {
	httpClient *http.Client
	endpoint   string
	apiKey     string
	// teamID, when non-empty, is sent as the x-hdx-team header to select the
	// active team. It is only honored by multi-team (EE) deployments, which
	// validate the API key's membership in the team server-side; single-team
	// (OSS) deployments ignore it. Empty means "use the API key's team".
	// Cloud mode never sends it: there a team is a Cloud service.
	teamID string
	// cloud mode credentials (HTTP basic auth with a Cloud API key).
	cloud       bool
	tokenKey    string
	tokenSecret string
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

// New returns a Client for a self-hosted ClickStack API at endpoint. The
// endpoint is the base URL of the ClickStack API without the /api/v2 suffix,
// e.g. "http://localhost:8000".
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

// NewCloud returns a Client for ClickStack managed by ClickHouse Cloud, served
// through the Cloud OpenAPI at
// {apiURL}/organizations/{organizationID}/services/{serviceID}/clickstack
// with HTTP basic auth (Cloud API key ID and secret). apiURL is the OpenAPI
// base, e.g. "https://api.clickhouse.cloud/v1". The Cloud API is still rolling
// out the full self-hosted surface; paths it does not serve yet fail with a
// route-not-found error from the server, not a client-side gate.
func NewCloud(apiURL, organizationID, serviceID, tokenKey, tokenSecret string, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("parse api url %q: %w", apiURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("api url %q must use http or https", apiURL)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		httpClient: httpClient,
		endpoint: strings.TrimRight(apiURL, "/") +
			"/organizations/" + url.PathEscape(organizationID) +
			"/services/" + url.PathEscape(serviceID) + "/clickstack",
		cloud:       true,
		tokenKey:    tokenKey,
		tokenSecret: tokenSecret,
	}, nil
}

// rewrapCloudEnvelope translates the Cloud OpenAPI success envelope
// {"status":…,"requestId":…,"result":X} into the self-hosted {"data":X}
// envelope, so the per-resource decoding is identical in both modes. Bodies
// without a result key (e.g. empty DELETE responses) pass through unchanged.
func rewrapCloudEnvelope(raw []byte) []byte {
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || env.Result == nil {
		return raw
	}
	out, err := json.Marshal(struct {
		Data json.RawMessage `json:"data"`
	}{env.Result})
	if err != nil {
		return raw
	}
	return out
}

// apiError is the error body returned by the API: self-hosted ClickStack uses
// "message", the Cloud OpenAPI uses "error".
type apiError struct {
	Message string `json:"message"`
	Error   string `json:"error"`
}

// do sends an API request with an optional pre-encoded JSON body and returns
// the raw response body. Callers decode the result into their concrete
// response type.
func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	reqPath := path
	if c.cloud {
		// Teams are a self-hosted (EE) concept; on Cloud each service is one
		// ClickStack. Failing here beats silently dropping the team scoping a
		// migrated config still carries.
		if c.teamID != "" {
			return nil, fmt.Errorf("%s %s: team %q: %w", method, path, c.teamID, ErrCloudUnsupported)
		}
		reqPath = strings.TrimPrefix(path, "/api/v2")
	}

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+reqPath, reqBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.cloud {
		req.SetBasicAuth(c.tokenKey, c.tokenSecret)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		if c.teamID != "" {
			req.Header.Set("x-hdx-team", c.teamID)
		}
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
		// The Cloud gateway 404s a genuinely missing resource with a JSON error
		// body, but a path it does not (yet) serve gets an HTML "Cannot GET"
		// page. Only the former may be treated as "resource deleted" —
		// Terraform removes state on ErrNotFound during reads and treats it as
		// already-deleted during destroys, so a routing 404 must stay distinct.
		if c.cloud {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) //nolint:errcheck // best-effort body sniff
			if !json.Valid(raw) {
				return nil, fmt.Errorf("%s %s: unexpected 404: %w (the Cloud API may not expose this ClickStack endpoint yet)", method, path, errRouteNotFound)
			}
		}
		return nil, fmt.Errorf("%s %s: %w", method, path, ErrNotFound)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var ae apiError
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096)) //nolint:errcheck // best-effort error body
		if jsonErr := json.Unmarshal(raw, &ae); jsonErr == nil && cmp.Or(ae.Message, ae.Error) != "" {
			return nil, fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, cmp.Or(ae.Message, ae.Error))
		}
		return nil, fmt.Errorf("%s %s: unexpected status %d", method, path, resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: read response: %w", method, path, err)
	}

	if c.cloud {
		raw = rewrapCloudEnvelope(raw)
	}
	return raw, nil
}
