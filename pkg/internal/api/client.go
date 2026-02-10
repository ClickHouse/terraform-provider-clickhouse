package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type ClientImpl struct {
	BaseUrl        string
	HttpClient     *http.Client
	OrganizationId string
	TokenKey       string
	TokenSecret    string

	// Track if organization resource has been registered
	orgResourceMutex      sync.Mutex
	orgResourceRegistered bool
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
		OrganizationId:        config.OrganizationID,
		TokenKey:              config.TokenKey,
		TokenSecret:           config.TokenSecret,
		orgResourceRegistered: false,
	}

	return client, nil
}

// RegisterOrganizationResource attempts to register an organization resource.
// Returns an error if one is already registered.
func (c *ClientImpl) RegisterOrganizationResource() error {
	c.orgResourceMutex.Lock()
	defer c.orgResourceMutex.Unlock()

	if c.orgResourceRegistered {
		return fmt.Errorf("only one clickhouse_organization_settings resource is allowed per provider configuration")
	}

	c.orgResourceRegistered = true
	return nil
}
