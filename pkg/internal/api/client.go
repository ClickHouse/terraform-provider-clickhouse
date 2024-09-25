package api

import (
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

func NewClient(apiUrl string, organizationId string, tokenKey string, tokenSecret string) (*ClientImpl, error) {
	client := &ClientImpl{
		BaseUrl: apiUrl,
		HttpClient: &http.Client{
			Timeout: time.Second * 30,
		},
		OrganizationId: organizationId,
		TokenKey:       tokenKey,
		TokenSecret:    tokenSecret,
	}

	return client, nil
}
