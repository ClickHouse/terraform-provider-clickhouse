package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
)

const (
	ReversePrivateEndpointTypeVPCEndpointService = "VPC_ENDPOINT_SERVICE"
	ReversePrivateEndpointTypeVPCResource        = "VPC_RESOURCE"
	ReversePrivateEndpointTypeMSKMultiVPC        = "MSK_MULTI_VPC"
)

const (
	MSKAuthenticationSASLIAM   = "SASL_IAM"
	MSKAuthenticationSASLSCRAM = "SASL_SCRAM"
)

const (
	ReversePrivateEndpointStatusUnknown           = "Unknown"
	ReversePrivateEndpointStatusProvisioning      = "Provisioning"
	ReversePrivateEndpointStatusDeleting          = "Deleting"
	ReversePrivateEndpointStatusReady             = "Ready"
	ReversePrivateEndpointStatusFailed            = "Failed"
	ReversePrivateEndpointStatusPendingAcceptance = "PendingAcceptance"
	ReversePrivateEndpointStatusRejected          = "Rejected"
	ReversePrivateEndpointStatusExpired           = "Expired"
)

var (
	ReversePrivateEndpointTypes = []string{
		ReversePrivateEndpointTypeVPCEndpointService,
		ReversePrivateEndpointTypeVPCResource,
		ReversePrivateEndpointTypeMSKMultiVPC,
	}

	MSKAuthenticationTypes = []string{
		MSKAuthenticationSASLIAM,
		MSKAuthenticationSASLSCRAM,
	}

	ReversePrivateEndpointStatuses = []string{
		ReversePrivateEndpointStatusUnknown,
		ReversePrivateEndpointStatusProvisioning,
		ReversePrivateEndpointStatusDeleting,
		ReversePrivateEndpointStatusReady,
		ReversePrivateEndpointStatusFailed,
		ReversePrivateEndpointStatusPendingAcceptance,
		ReversePrivateEndpointStatusRejected,
		ReversePrivateEndpointStatusExpired,
	}
)

func (c *ClientImpl) GetReversePrivateEndpointPath(serviceId, reversePrivateEndpointId string) string {
	return c.getServicePath(serviceId, fmt.Sprintf("/clickpipesReversePrivateEndpoints/%s", reversePrivateEndpointId))
}

func (c *ClientImpl) ListReversePrivateEndpoints(ctx context.Context, serviceId string) ([]*ReversePrivateEndpoint, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/clickpipesReversePrivateEndpoints"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[[]ReversePrivateEndpoint]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ReversePrivateEndpoints: %w", err)
	}

	result := make([]*ReversePrivateEndpoint, len(response.Result))
	for i, rpe := range response.Result {
		// Copy for proper reference
		result[i] = &rpe
	}

	return result, nil
}

func (c *ClientImpl) GetReversePrivateEndpoint(ctx context.Context, serviceId, reversePrivateEndpointId string) (*ReversePrivateEndpoint, error) {
	req, err := http.NewRequest(http.MethodGet, c.GetReversePrivateEndpointPath(serviceId, reversePrivateEndpointId), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[ReversePrivateEndpoint]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ReversePrivateEndpoint: %w", err)
	}

	return &response.Result, nil
}

func (c *ClientImpl) CreateReversePrivateEndpoint(ctx context.Context, serviceId string, request CreateReversePrivateEndpoint) (*ReversePrivateEndpoint, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(request); err != nil {
		return nil, fmt.Errorf("failed to encode ReversePrivateEndpoint: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.getServicePath(serviceId, "/clickpipesReversePrivateEndpoints"), &payload)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[ReversePrivateEndpoint]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ReversePrivateEndpoint: %w", err)
	}

	return &response.Result, nil
}

func (c *ClientImpl) DeleteReversePrivateEndpoint(ctx context.Context, serviceId, reversePrivateEndpointId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.GetReversePrivateEndpointPath(serviceId, reversePrivateEndpointId), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
}

func (c *ClientImpl) WaitForReversePrivateEndpointState(ctx context.Context, serviceId string, reversePrivateEndpointId string, stateChecker func(string) bool, maxWaitSeconds uint64) (rpe *ReversePrivateEndpoint, err error) {
	checkState := func() error {
		rpe, err = c.GetReversePrivateEndpoint(ctx, serviceId, reversePrivateEndpointId)
		if err != nil {
			return err
		}

		if stateChecker(rpe.Status) {
			return nil
		}

		return fmt.Errorf("ClickPipe reverse private endpoint %s is in state %s", reversePrivateEndpointId, rpe.Status)
	}

	if maxWaitSeconds < 5 {
		maxWaitSeconds = 5
	}

	err = backoff.Retry(checkState, backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), maxWaitSeconds/5))
	return
}
