package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	ApiKeyStateEnabled  = "enabled"
	ApiKeyStateDisabled = "disabled"
)

type ApiKey struct {
	ID             string     `json:"id,omitempty"`
	State          string     `json:"state,omitempty"`
	ExpirationDate *time.Time `json:"expireAt,omitempty"`

	Name  string   `json:"name"`
	Roles []string `json:"roles"`

	KeySuffix string `json:"keySuffix,omitempty"`
}

type ApiKeyUpdate struct {
	Name           string     `json:"name,omitempty"`
	Roles          []string   `json:"roles,omitempty"`
	State          string     `json:"state,omitempty"`
	ExpirationDate *time.Time `json:"expireAt,omitempty"`
}

type ApiKeyResponseResult struct {
	Key       ApiKey `json:"key"`
	KeyID     string `json:"keyId"`
	KeySecret string `json:"keySecret"`
}

func (c *ClientImpl) GetCurrentApiKey(ctx context.Context) (*ApiKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.getKeysPath(), nil)
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
		// Find id of the API key configured for this terraform provider run.
		// Since we don't know the ID of the API key on the client side, we need to a pattern match on the KeySuffix.
		// This is a very weak check, but that's all we have until https://github.com/ClickHouse/control-plane/issues/13294 is implemented.
		if strings.HasSuffix(c.TokenKey, key.KeySuffix) {
			return &key, nil
		}
	}

	return nil, fmt.Errorf("key not found") //nolint
}

func (c *ClientImpl) GetApiKey(ctx context.Context, id string) (*ApiKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.getKeyPath(id), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	apiKeysResponse := ResponseWithResult[ApiKey]{}
	err = json.Unmarshal(body, &apiKeysResponse)
	if err != nil {
		return nil, err
	}

	return &apiKeysResponse.Result, nil
}

func (c *ClientImpl) CreateApiKey(ctx context.Context, key ApiKey) (*ApiKey, string, string, error) {
	rb, err := json.Marshal(key)
	if err != nil {
		return nil, "", "", err
	}

	req, err := http.NewRequest(http.MethodPost, c.getKeysPath(), strings.NewReader(string(rb)))
	if err != nil {
		return nil, "", "", err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, "", "", err
	}

	createKeyResponse := ResponseWithResult[ApiKeyResponseResult]{}
	err = json.Unmarshal(body, &createKeyResponse)
	if err != nil {
		return nil, "", "", err
	}

	return &createKeyResponse.Result.Key, createKeyResponse.Result.KeyID, createKeyResponse.Result.KeySecret, nil
}

func (c *ClientImpl) UpdateApiKey(ctx context.Context, id string, update ApiKeyUpdate) (*ApiKey, error) {
	rb, err := json.Marshal(update)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getKeyPath(id), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	keyResponse := ResponseWithResult[ApiKey]{}
	err = json.Unmarshal(body, &keyResponse)
	if err != nil {
		return nil, err
	}

	return &keyResponse.Result, nil
}

func (c *ClientImpl) DeleteApiKey(ctx context.Context, id string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getKeyPath(id), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	if IsNotFound(err) {
		// That is what we want
		return nil
	} else if err != nil {
		return err
	}

	return nil
}
