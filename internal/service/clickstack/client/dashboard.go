package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const dashboardsPath = "/api/v2/dashboards"

// ErrValidateUnsupported is returned when the /validate endpoint is absent
// (older API), so the provider can skip plan-time validation gracefully.
var ErrValidateUnsupported = errors.New("validate endpoint not available")

// ValidateError is one problem reported by the validate endpoint.
type ValidateError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidateResult is the response from POST /api/v2/dashboards/validate.
type ValidateResult struct {
	Valid  bool            `json:"valid"`
	Errors []ValidateError `json:"errors"`
}

type dashboardEnvelope struct {
	Data json.RawMessage `json:"data"`
}

// validateResponse is the wire form of a /validate reply. Valid is a pointer
// so "the body carried no verdict" is distinguishable from an explicit
// "valid": false — decoding a verdict-less body into ValidateResult yields the
// zero value, which reads as "invalid, and the API said why not", the worst
// possible default for a plan-time gate. Data captures the {"data":...}
// envelope the Cloud path arrives in.
type validateResponse struct {
	Valid  *bool           `json:"valid"`
	Errors []ValidateError `json:"errors"`
	Data   json.RawMessage `json:"data"`
}

// DashboardID extracts the "id" field from a dashboard body.
// It returns ("", nil) when the id field is absent or empty; callers are
// responsible for deciding whether an empty id is an error in their context.
func DashboardID(body json.RawMessage) (string, error) {
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", fmt.Errorf("decode dashboard id: %w", err)
	}
	return v.ID, nil
}

// CreateDashboard creates a dashboard and returns the server-assigned body.
func (c *Client) CreateDashboard(ctx context.Context, body json.RawMessage) (json.RawMessage, error) {
	raw, err := c.do(ctx, http.MethodPost, dashboardsPath, body)
	if err != nil {
		return nil, err
	}
	var env dashboardEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode dashboard: %w", err)
	}
	return env.Data, nil
}

// GetDashboard retrieves a dashboard by its ID and returns its body.
func (c *Client) GetDashboard(ctx context.Context, id string) (json.RawMessage, error) {
	raw, err := c.do(ctx, http.MethodGet, dashboardsPath+"/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var env dashboardEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode dashboard: %w", err)
	}
	return env.Data, nil
}

// UpdateDashboard replaces the dashboard with the given ID and returns the updated body.
func (c *Client) UpdateDashboard(ctx context.Context, id string, body json.RawMessage) (json.RawMessage, error) {
	raw, err := c.do(ctx, http.MethodPut, dashboardsPath+"/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}
	var env dashboardEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode dashboard: %w", err)
	}
	return env.Data, nil
}

// DeleteDashboard removes the dashboard with the given ID.
func (c *Client) DeleteDashboard(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, dashboardsPath+"/"+url.PathEscape(id), nil)
	return err
}

// ValidateDashboard posts the dashboard body to the validate endpoint. It
// returns ErrValidateUnsupported when the endpoint is absent (404).
//
// Self-hosted /validate responds with a bare {"valid","errors","normalized"}
// object, not the {"data":...} envelope the CRUD endpoints use. Cloud, though,
// wraps every response in {"result":...}, which do() rewraps to {"data":...} —
// so the verdict may arrive either bare or enveloped and both shapes must be
// handled.
//
// Every unrecognized shape must fail loudly rather than decode to the zero
// ValidateResult: the caller treats an error as "validation unavailable" (a
// warning, deferring to apply) but Valid=false as a hard plan failure, so a
// silent zero value blocks every plan with an unexplained "invalid dashboard".
func (c *Client) ValidateDashboard(ctx context.Context, body json.RawMessage) (*ValidateResult, error) {
	raw, err := c.do(ctx, http.MethodPost, dashboardsPath+"/validate", body)
	if err != nil {
		// Both 404 flavors mean the endpoint is absent: ErrNotFound
		// (self-hosted older API) and errRouteNotFound (Cloud gateway).
		if errors.Is(err, ErrNotFound) || errors.Is(err, errRouteNotFound) {
			return nil, ErrValidateUnsupported
		}
		return nil, err
	}
	var res validateResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decode validate result: %w", err)
	}
	// Unwrap one envelope level when the body carries no verdict of its own.
	// Keying on the missing verdict rather than on the presence of "data"
	// keeps a bare result that happens to have a "data" field from being
	// mistaken for an envelope and silently discarded.
	if res.Valid == nil && len(res.Data) > 0 {
		var inner validateResponse
		if err := json.Unmarshal(res.Data, &inner); err != nil {
			return nil, fmt.Errorf("decode validate result: %w", err)
		}
		res = inner
	}
	// No verdict in either shape means an unrecognized response, not a
	// rejected dashboard. Returning an error routes the caller to its
	// "validation unavailable" warning instead of blocking every plan with an
	// unexplained "invalid dashboard".
	if res.Valid == nil {
		return nil, fmt.Errorf(`decode validate result: no "valid" field in response`)
	}
	return &ValidateResult{Valid: *res.Valid, Errors: res.Errors}, nil
}
