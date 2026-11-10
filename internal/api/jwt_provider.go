package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// JWTProvider mirrors the OpenAPI JwtProvider shape: a JWT authentication
// provider (JWKS mode) configured on a service. It is also the POST body.
type JWTProvider struct {
	Name       string  `json:"name"`
	Issuer     string  `json:"issuer"`
	Audience   string  `json:"audience"`
	JWKSURL    string  `json:"jwksUrl"`
	RolesClaim *string `json:"rolesClaim,omitempty"`
}

// JWTProviderUpdate is the PUT body. It replaces the whole provider
// configuration, so a nil RolesClaim removes an existing roles claim. The
// name is taken from the path.
type JWTProviderUpdate struct {
	Issuer     string  `json:"issuer"`
	Audience   string  `json:"audience"`
	JWKSURL    string  `json:"jwksUrl"`
	RolesClaim *string `json:"rolesClaim,omitempty"`
}

func (c *ClientImpl) jwtProvidersPath(serviceID, name string) string {
	path := c.getServicePath(serviceID, "/jwtProviders")
	if name != "" {
		path += "/" + url.PathEscape(name)
	}
	return path
}

func (c *ClientImpl) GetJWTProvider(ctx context.Context, serviceID, name string) (*JWTProvider, error) {
	req, err := http.NewRequest(http.MethodGet, c.jwtProvidersPath(serviceID, name), nil)
	if err != nil {
		return nil, fmt.Errorf("create get JWT provider request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[JWTProvider]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode JWT provider %q: %w", name, err)
	}
	return &response.Result, nil
}

func (c *ClientImpl) CreateJWTProvider(ctx context.Context, serviceID string, provider JWTProvider) (*JWTProvider, error) {
	requestBody, err := json.Marshal(provider)
	if err != nil {
		return nil, fmt.Errorf("encode create JWT provider request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.jwtProvidersPath(serviceID, ""), bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("create create JWT provider request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[JWTProvider]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode created JWT provider: %w", err)
	}
	return &response.Result, nil
}

func (c *ClientImpl) UpdateJWTProvider(ctx context.Context, serviceID, name string, update JWTProviderUpdate) (*JWTProvider, error) {
	requestBody, err := json.Marshal(update)
	if err != nil {
		return nil, fmt.Errorf("encode update JWT provider request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, c.jwtProvidersPath(serviceID, name), bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("create update JWT provider request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[JWTProvider]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode updated JWT provider %q: %w", name, err)
	}
	return &response.Result, nil
}

func (c *ClientImpl) DeleteJWTProvider(ctx context.Context, serviceID, name string) error {
	req, err := http.NewRequest(http.MethodDelete, c.jwtProvidersPath(serviceID, name), nil)
	if err != nil {
		return fmt.Errorf("create delete JWT provider request: %w", err)
	}

	_, err = c.doRequest(ctx, req)
	return err
}
