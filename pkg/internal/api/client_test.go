package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestNewClient(t *testing.T) {
	testClient := &ClientImpl{
		BaseUrl: "https://api.clickhouse.cloud/v1",
		HttpClient: &http.Client{
			Timeout: time.Minute * 5,
		},
		OrganizationId: "10ead720-7ca1-48c9-aaf7-7230f42b56c0",
		TokenKey:       "dE8jvpSRVurZCLcLZllb",
		TokenSecret:    "4b1dZbh9bFV9uHQ7Aay4vHHbsTL1HkD2CyZyFBlOLc",
	}

	client, err := NewClient(ClientConfig{
		ApiURL:         testClient.BaseUrl,
		OrganizationID: testClient.OrganizationId,
		TokenKey:       testClient.TokenKey,
		TokenSecret:    testClient.TokenSecret,
	})
	if err != nil {
		t.Fatalf("new client err: %v", err)
	}
	if diff := cmp.Diff(testClient, client); diff != "" {
		t.Errorf("NewClient() mismatch (-want +got):\n%s", diff)
	}
	orgPath := "https://api.clickhouse.cloud/v1/organizations/10ead720-7ca1-48c9-aaf7-7230f42b56c0"
	if diff := cmp.Diff(client.getOrgPath(""), orgPath); diff != "" {
		t.Errorf("getOrgPath() mismatch (-want +got):\n%s", diff)
	}
	servicePath := "https://api.clickhouse.cloud/v1/organizations/10ead720-7ca1-48c9-aaf7-7230f42b56c0/services"
	if diff := cmp.Diff(client.getServicePath("", ""), servicePath); diff != "" {
		t.Errorf("getServicePath() mismatch (-want +got):\n%s", diff)
	}
	servicePath = "https://api.clickhouse.cloud/v1/organizations/10ead720-7ca1-48c9-aaf7-7230f42b56c0/services/1234"
	if diff := cmp.Diff(client.getServicePath("1234", ""), servicePath); diff != "" {
		t.Errorf("getServicePath() mismatch (-want +got):\n%s", diff)
	}
}
