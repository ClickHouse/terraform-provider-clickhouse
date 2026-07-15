package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// API key states, mirroring the `state` field of the keys API.
const (
	ApiKeyStateEnabled  = "enabled"
	ApiKeyStateDisabled = "disabled"
)

var ApiKeyStateValues = []string{ApiKeyStateEnabled, ApiKeyStateDisabled}

type IpAccessListEntry struct {
	Source      string `json:"source"`
	Description string `json:"description,omitempty"`
}

type ApiKey struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	State        string              `json:"state"`
	KeySuffix    string              `json:"keySuffix"`
	ExpireAt     string              `json:"expireAt"`
	CreatedAt    string              `json:"createdAt"`
	UsedAt       string              `json:"usedAt"`
	IpAccessList []IpAccessListEntry `json:"ipAccessList"`
}

// ApiKeyCreateRequest is the POST body. hashData is intentionally omitted so the
// server generates the secret and returns it once in the response.
type ApiKeyCreateRequest struct {
	Name  string `json:"name"`
	State string `json:"state,omitempty"`
	// No omitempty: a nil pointer must marshal to explicit null to clear the
	// expiry (null/empty means "never expires"). Adding omitempty here would
	// silently drop the field and leave the server value unchanged.
	ExpireAt     *string              `json:"expireAt"`
	IpAccessList *[]IpAccessListEntry `json:"ipAccessList,omitempty"`
}

type ApiKeyUpdateRequest struct {
	Name  string `json:"name,omitempty"`
	State string `json:"state,omitempty"`
	// No omitempty: a nil pointer must marshal to explicit null to clear the
	// expiry on update, otherwise a removed expire_at would leave the server
	// value in place and produce a permanent plan diff.
	ExpireAt     *string              `json:"expireAt"`
	IpAccessList *[]IpAccessListEntry `json:"ipAccessList,omitempty"`
}

// ApiKeyCreateResult is the "result" object of a create response. KeySecret is
// present only when the request omitted hashData.
type ApiKeyCreateResult struct {
	KeyID     string `json:"keyId"`
	KeySecret string `json:"keySecret"`
	Key       ApiKey `json:"key"`
}

// GetApiKeyID lists all keys and finds one by name, or (when name is nil) by
// matching KeySuffix against the provider's own token — a search used by the
// clickhouse_api_key_id data source when the key ID is not known up front. For
// a direct lookup by a known ID, use GetApiKey instead.
func (c *ClientImpl) GetApiKeyID(ctx context.Context, name *string) (*ApiKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/keys"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	apiKeysResponse := ResponseWithResult[[]ApiKey]{}
	err = json.Unmarshal(body, &apiKeysResponse)
	if err != nil {
		return nil, err
	}

	for _, key := range apiKeysResponse.Result {
		// We want to match a key by name
		if name != nil {
			if *name == key.Name {
				return &key, nil
			}
		} else {
			// Find id of the API key configured for this terraform provider run.
			// Since we don't know the ID of the API key on the client side, we need to a pattern match on the KeySuffix.
			// This is a very weak check, but that's all we have until https://github.com/ClickHouse/control-plane/issues/13294 is implemented.
			if strings.HasSuffix(c.TokenKey, key.KeySuffix) {
				return &key, nil
			}
		}
	}

	errorMsg := "key not found"

	if name != nil {
		errorMsg = fmt.Sprintf("API key named %q was not found", *name)
	}

	return nil, fmt.Errorf("%s", errorMsg)
}

func (c *ClientImpl) CreateApiKey(ctx context.Context, req ApiKeyCreateRequest) (*ApiKeyCreateResult, error) {
	rb, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, c.getOrgPath("/keys"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[ApiKeyCreateResult]{}
	if err = json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

// GetApiKey fetches a single key directly by its ID, for the clickhouse_api_key
// resource's Read (which already knows the ID from state). A missing key yields
// the API's 404, so callers can use api.IsNotFound to drop it from state. See
// GetApiKeyID for the name/suffix search the data source uses.
func (c *ClientImpl) GetApiKey(ctx context.Context, keyId string) (*ApiKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/keys/"+keyId), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[ApiKey]{}
	if err = json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

func (c *ClientImpl) UpdateApiKey(ctx context.Context, keyId string, req ApiKeyUpdateRequest) (*ApiKey, error) {
	rb, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(http.MethodPatch, c.getOrgPath("/keys/"+keyId), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, httpReq)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[ApiKey]{}
	if err = json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp.Result, nil
}

func (c *ClientImpl) DeleteApiKey(ctx context.Context, keyId string) error {
	httpReq, err := http.NewRequest(http.MethodDelete, c.getOrgPath("/keys/"+keyId), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, httpReq)
	return err
}
