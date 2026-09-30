package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func reversePrivateEndpointReady(endpoint *api.ReversePrivateEndpoint, requireDNSNames bool) (bool, error) {
	switch endpoint.Status {
	case api.ReversePrivateEndpointStatusFailed, api.ReversePrivateEndpointStatusRejected, api.ReversePrivateEndpointStatusExpired:
		return false, fmt.Errorf("ClickPipe reverse private endpoint %s reached terminal state %s", endpoint.ID, endpoint.Status)
	case api.ReversePrivateEndpointStatusReady:
		return !requireDNSNames || len(endpoint.DNSNames) > 0, nil
	default:
		return false, nil
	}
}

func waitForReversePrivateEndpointReady(ctx context.Context, client *api.ClientImpl, serviceID, endpointID string, requireDNSNames bool) (*api.ReversePrivateEndpoint, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var endpoint *api.ReversePrivateEndpoint
	err := backoff.Retry(func() error {
		var err error
		endpoint, err = client.GetReversePrivateEndpoint(ctx, serviceID, endpointID)
		if err != nil {
			return err
		}
		ready, err := reversePrivateEndpointReady(endpoint, requireDNSNames)
		if err != nil {
			return backoff.Permanent(err)
		}
		if ready {
			return nil
		}
		return fmt.Errorf("ClickPipe reverse private endpoint %s is not ready (status %s, %d DNS names)", endpointID, endpoint.Status, len(endpoint.DNSNames))
	}, backoff.WithContext(backoff.NewConstantBackOff(5*time.Second), ctx))
	return endpoint, err
}
