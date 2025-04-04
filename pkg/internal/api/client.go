package api

import (
	"fmt"
	"net/http"
	"time"
)

type ClientImpl struct {
	BaseUrl        string
	HttpClient     *http.Client
	OrganizationId string
	TokenKey       string
	TokenSecret    string
}

type ClientConfig struct {
	ApiURL         string
	OrganizationID string
	TokenKey       string
	TokenSecret    string
	Timeout        time.Duration
}

func NewClient(config ClientConfig) (*ClientImpl, error) {
	if config.ApiURL == "" {
		return nil, fmt.Errorf("ApiURL cannot be empty")
	}
	if config.OrganizationID == "" {
		return nil, fmt.Errorf("OrganizationID cannot be empty")
	}
	if config.TokenKey == "" {
		return nil, fmt.Errorf("TokenKey cannot be empty")
	}
	if config.TokenSecret == "" {
		return nil, fmt.Errorf("TokenSecret cannot be empty")
	}
	if config.Timeout == 0 {
		config.Timeout = time.Minute * 5
	}

	client := &ClientImpl{
		BaseUrl: config.ApiURL,
		HttpClient: &http.Client{
			Timeout: config.Timeout,
		},
		OrganizationId: config.OrganizationID,
		TokenKey:       config.TokenKey,
		TokenSecret:    config.TokenSecret,
	}

	return client, nil
}
