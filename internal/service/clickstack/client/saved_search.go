package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const savedSearchesPath = "/api/v2/saved-searches"

// SavedSearch is a ClickStack saved search as exchanged with the v2 API.
//
// Filters is an opaque JSON passthrough: the pinned-sidebar-filter shapes are a
// union the provider does not model field-by-field, so the raw JSON is
// round-tripped verbatim to guarantee no filter is ever dropped by the
// full-replace PUT. Callers should send a non-null value (e.g. `[]`) since the
// PUT is a full replace.
type SavedSearch struct {
	ID            string          `json:"id,omitempty"`
	Name          string          `json:"name"`
	SourceID      string          `json:"sourceId"`
	Select        string          `json:"select"`
	Where         string          `json:"where"`
	WhereLanguage string          `json:"whereLanguage"`
	OrderBy       string          `json:"orderBy"`
	Tags          []string        `json:"tags"`
	Filters       json.RawMessage `json:"filters,omitempty"`
}

// savedSearchEnvelope wraps single-saved-search API responses.
type savedSearchEnvelope struct {
	Data SavedSearch `json:"data"`
}

// savedSearchListEnvelope wraps saved-search-list API responses.
type savedSearchListEnvelope struct {
	Data []SavedSearch `json:"data"`
}

// CreateSavedSearch creates a saved search and returns it as stored by the API.
func (c *Client) CreateSavedSearch(ctx context.Context, input SavedSearch) (*SavedSearch, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode saved search: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPost, savedSearchesPath, body)
	if err != nil {
		return nil, err
	}

	var resp savedSearchEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode saved search: %w", err)
	}
	return &resp.Data, nil
}

// GetSavedSearch fetches a saved search by ID. It returns an error wrapping
// ErrNotFound when the saved search does not exist.
func (c *Client) GetSavedSearch(ctx context.Context, id string) (*SavedSearch, error) {
	raw, err := c.do(ctx, http.MethodGet, savedSearchesPath+"/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}

	var resp savedSearchEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode saved search: %w", err)
	}
	return &resp.Data, nil
}

// ListSavedSearches fetches all saved searches for the authenticated team.
func (c *Client) ListSavedSearches(ctx context.Context) ([]SavedSearch, error) {
	raw, err := c.do(ctx, http.MethodGet, savedSearchesPath, nil)
	if err != nil {
		return nil, err
	}

	var resp savedSearchListEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode saved searches: %w", err)
	}
	return resp.Data, nil
}

// UpdateSavedSearch replaces a saved search by ID (the API PUT is a full
// replace) and returns the updated saved search. It returns an error wrapping
// ErrNotFound when the saved search does not exist.
func (c *Client) UpdateSavedSearch(ctx context.Context, id string, input SavedSearch) (*SavedSearch, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode saved search: %w", err)
	}

	raw, err := c.do(ctx, http.MethodPut, savedSearchesPath+"/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}

	var resp savedSearchEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode saved search: %w", err)
	}
	return &resp.Data, nil
}

// DeleteSavedSearch deletes a saved search by ID. It returns an error wrapping
// ErrNotFound when the saved search does not exist. The API also deletes any
// alerts attached to the saved search.
func (c *Client) DeleteSavedSearch(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, savedSearchesPath+"/"+url.PathEscape(id), nil)
	return err
}
