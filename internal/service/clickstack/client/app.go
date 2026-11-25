package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const appsPath = "/api/v2/apps"

// AppInput is the body of an app create, update or validate request. Update is
// a full replace: every field is sent, and IsShared must always be explicit so
// a private app is never shared by omission.
type AppInput struct {
	Name     string `json:"name"`
	IsShared bool   `json:"isShared"`
	Code     string `json:"code"`
	Purpose  string `json:"purpose"`
}

// App is a ClickStack app as returned by the v2 API. Code, Purpose and
// VersionID are null for an app that has no version yet (one created in the
// app builder whose first build never saved). The server-derived manifest is
// not decoded: the provider does not expose it.
//
// CanChangeIsShared says whether the caller may change IsShared: only the
// app's creator can. The API always returns it; it is a pointer so a response
// missing it (a proxy stripping it, or a future API change) decodes as unknown
// rather than as false, which would block every sharing change at plan time.
type App struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	IsShared          bool    `json:"isShared"`
	Code              *string `json:"code"`
	Purpose           *string `json:"purpose"`
	VersionID         *string `json:"versionId"`
	CanChangeIsShared *bool   `json:"canChangeIsShared"`
}

type appEnvelope struct {
	Data App `json:"data"`
}

// decodeApp decodes a single-app {"data":...} response.
func decodeApp(raw []byte) (*App, error) {
	var resp appEnvelope
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decode app: %w", err)
	}
	return &resp.Data, nil
}

// CreateApp creates an app. The server compiles the code first; a compile
// error is a 400 whose message the returned error carries.
func (c *Client) CreateApp(ctx context.Context, input AppInput) (*App, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode app: %w", err)
	}
	raw, err := c.do(ctx, http.MethodPost, appsPath, body)
	if err != nil {
		return nil, err
	}
	return decodeApp(raw)
}

// GetApp fetches an app with its current code and purpose. It returns an error
// wrapping ErrNotFound when the app does not exist or the caller cannot see it
// (a private app owned by another user: the API hides those entirely).
func (c *Client) GetApp(ctx context.Context, id string) (*App, error) {
	raw, err := c.do(ctx, http.MethodGet, appsPath+"/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	return decodeApp(raw)
}

// UpdateApp replaces an app's name, sharing, code and purpose. A new version is
// saved only when the code or purpose changes. It returns an error wrapping
// ErrNotFound when the app does not exist.
func (c *Client) UpdateApp(ctx context.Context, id string, input AppInput) (*App, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode app: %w", err)
	}
	raw, err := c.do(ctx, http.MethodPut, appsPath+"/"+url.PathEscape(id), body)
	if err != nil {
		return nil, err
	}
	return decodeApp(raw)
}

// DeleteApp deletes an app with its version history. It returns an error
// wrapping ErrNotFound when the app does not exist.
func (c *Client) DeleteApp(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, appsPath+"/"+url.PathEscape(id), nil)
	return err
}

// ValidateApp checks an app body, compiling its code, without saving it. It
// returns ErrValidateUnsupported when the endpoint is absent (404).
func (c *Client) ValidateApp(ctx context.Context, input AppInput) (*ValidateResult, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode app: %w", err)
	}
	return c.validate(ctx, appsPath+"/validate", body)
}
