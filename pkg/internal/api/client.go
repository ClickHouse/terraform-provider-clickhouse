package api

import (
	"net/http"
	"time"
)

type ClientImpl struct {
	BaseUrl         string
	QueryAPIBaseUrl string
	HttpClient      *http.Client
	OrganizationId  string
	TokenKey        string
	TokenSecret     string
}

func NewClient(apiUrl string, queryApiBaseUrl string, organizationId string, tokenKey string, tokenSecret string) (Client, error) {
	client := &ClientImpl{
		BaseUrl:         apiUrl,
		QueryAPIBaseUrl: queryApiBaseUrl,
		HttpClient: &http.Client{
			Timeout: time.Second * 30,
		},
		OrganizationId: organizationId,
		TokenKey:       tokenKey,
		TokenSecret:    tokenSecret,
	}

	return client, nil
}
