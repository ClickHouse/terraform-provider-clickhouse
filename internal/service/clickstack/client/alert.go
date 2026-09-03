package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const alertsPath = "/api/v2/alerts"

// Alert sources accepted by the external v2 API. A saved-search alert evaluates
// a saved search; a tile alert evaluates one dashboard tile (line, stacked bar,
// or number). The API also has an internal-only "inline" source that is not
// exposed externally yet.
const (
	AlertSourceSavedSearch = "saved_search"
	AlertSourceTile        = "tile"
)

// AlertChannelWebhook is the webhook channel type.
const AlertChannelWebhook = "webhook"

// AlertChannel is an alert's notification channel. Only the webhook type exists
// today; the shape mirrors the API so additional channel types slot in later.
type AlertChannel struct {
	Type      string `json:"type"`
	WebhookID string `json:"webhookId,omitempty"`
}

// Alert is a ClickStack alert as exchanged with the v2 API.
//
// It models only the write-schema fields plus the server-assigned id. The
// server-managed transient fields (state, silenced, executionErrors) are
// deliberately not modeled: they are absent from the request body, and because
// the API's PUT is a partial $set of the write-schema fields, they are
// preserved server-side across updates (a user-set silence is never clobbered).
//
// The PUT is a partial $set (not a full replace), so omitting a write-schema key
// does NOT uniformly clear it — behavior is per-field:
//   - name, message, note, numConsecutiveWindows: the server coerces an omitted
//     value to null, so `omitempty` here correctly clears the field on removal.
//   - scheduleStartAt: always sent (no omitempty) — nil -> null clears it, and
//     the server then forces scheduleOffsetMinutes to 0. Because it is always
//     sent, omitting scheduleOffsetMinutes clears it (server sets 0) rather than
//     keeping the old value; the provider treats a returned offset of 0 as unset.
//     The provider also omits the offset entirely whenever scheduleStartAt is set
//     so the two mutually-exclusive fields are never sent together.
//   - groupBy: the server keeps the previous value when omitted and rejects an
//     explicit null, so the provider models it as Optional+Computed (sticky) —
//     removing it from config is a no-op (recreate to reset).
//   - thresholdMax: kept when omitted and rejects null; only sent for range
//     threshold types (and applyAlert does not reconcile it for other types).
//   - savedSearchId, dashboardId, tileId: an omitted id keeps the current target,
//     which is why the resource rejects "" at plan time instead of sending it.
type Alert struct {
	ID            string       `json:"id,omitempty"`
	Source        string       `json:"source"`
	Channel       AlertChannel `json:"channel"`
	Interval      string       `json:"interval"`
	Threshold     float64      `json:"threshold"`
	ThresholdType string       `json:"thresholdType"`
	ThresholdMax  *float64     `json:"thresholdMax,omitempty"`
	// Exactly one target is set, matching Source. All three are omitempty so the
	// unused pair is never sent; the API strips keys outside the chosen source's
	// branch, so sending them would only mislead a reader of the request.
	SavedSearchID         string  `json:"savedSearchId,omitempty"`
	DashboardID           string  `json:"dashboardId,omitempty"`
	TileID                string  `json:"tileId,omitempty"`
	GroupBy               *string `json:"groupBy,omitempty"`
	Name                  *string `json:"name,omitempty"`
	Message               *string `json:"message,omitempty"`
	Note                  *string `json:"note,omitempty"`
	NumConsecutiveWindows *int    `json:"numConsecutiveWindows,omitempty"`
	ScheduleOffsetMinutes *int    `json:"scheduleOffsetMinutes,omitempty"`
	// ScheduleStartAt is always serialized (no omitempty): a nil pointer sends
	// JSON null, which the API treats as "clear" (and then forces the offset to
	// 0). Omitting it would instead preserve the previous value. The provider
	// always sends it so removing schedule_start_at from config propagates.
	ScheduleStartAt *string `json:"scheduleStartAt"`
}

// alertEnvelope wraps single-alert API responses.
type alertEnvelope struct {
	Data Alert `json:"data"`
}

// CreateAlert creates an alert and returns it as stored by the API. Source
// defaults to saved_search when unset.
func (c *Client) CreateAlert(ctx context.Context, input Alert) (*Alert, error) {
	input.Source = defaultSource(input.Source)
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode alert: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPost, alertsPath, body)
	if err != nil {
		return nil, err
	}

	var resp alertEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode alert: %w", err)
	}
	return &resp.Data, nil
}

// GetAlert fetches an alert by ID. It returns an error wrapping ErrNotFound when
// the alert does not exist.
func (c *Client) GetAlert(ctx context.Context, id string) (*Alert, error) {
	raw, err := c.do(ctx, http.MethodGet, alertsPath+"/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}

	var resp alertEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode alert: %w", err)
	}
	return &resp.Data, nil
}

// UpdateAlert updates an alert by ID and returns the updated alert. Source
// defaults to saved_search when unset. The API PUT is a partial $set of the
// write-schema fields (see the Alert doc for which omitted fields clear vs. are
// kept), not a whole-document replace. It returns an error wrapping ErrNotFound
// when the alert does not exist.
func (c *Client) UpdateAlert(ctx context.Context, id string, input Alert) (*Alert, error) {
	input.Source = defaultSource(input.Source)
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode alert: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPut, alertsPath+"/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}

	var resp alertEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode alert: %w", err)
	}
	return &resp.Data, nil
}

// DeleteAlert deletes an alert by ID. It returns an error wrapping ErrNotFound
// when the alert does not exist (e.g. it was already cascade-deleted with its
// saved search).
func (c *Client) DeleteAlert(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, alertsPath+"/"+url.PathEscape(id), nil)
	return err
}

// defaultSource keeps callers that predate tile alerts working: an empty source
// still means a saved-search alert.
func defaultSource(s string) string {
	if s == "" {
		return AlertSourceSavedSearch
	}
	return s
}
