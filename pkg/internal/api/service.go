package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
)

type ServiceStateUpdate struct {
	Command string `json:"command"`
}

type ServiceResponseResult struct {
	Service  Service `json:"service"`
	Password string  `json:"password"`
}

type ServiceBody struct {
	Service Service `json:"service"`
}

// GetService - Returns service by ID
func (c *ClientImpl) GetService(ctx context.Context, serviceId string) (*Service, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, ""), nil)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ResponseWithResult[Service]{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	service := serviceResponse.Result

	endpointConfigResponse, err := c.GetServicePrivateEndpointConfig(ctx, serviceId)
	if err != nil {
		return nil, err
	}

	service.PrivateEndpointConfig = endpointConfigResponse

	backupConfiguration, err := c.GetBackupConfiguration(ctx, service.Id)
	if err != nil {
		return nil, err
	}

	service.BackupConfiguration = backupConfiguration

	queryEndpoints, err := c.GetQueryEndpoint(ctx, service.Id)
	if err != nil {
		return nil, err
	}

	service.QueryAPIEndpoints = queryEndpoints

	return &service, nil
}

func (c *ClientImpl) CreateService(ctx context.Context, s Service) (*Service, string, error) {
	// Needed until we have alignment between service creation and replicaScaling calls.
	s.FixMemoryBounds()
	rb, err := json.Marshal(s)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequest(http.MethodPost, c.getServicePath("", ""), strings.NewReader(string(rb)))
	if err != nil {
		return nil, "", err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, "", err
	}

	serviceResponse := ResponseWithResult[ServiceResponseResult]{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, "", err
	}

	return &serviceResponse.Result.Service, serviceResponse.Result.Password, nil
}

func (c *ClientImpl) WaitForServiceState(ctx context.Context, serviceId string, stateChecker func(string) bool, maxWaitSeconds int) error {
	// Wait until service is in desired state
	checkState := func() error {
		service, err := c.GetService(ctx, serviceId)
		if is5xx(err) {
			// 500s are automatically retried in `GetService`.
			// If we get it here, we consider it an unrecoverable error.
			return backoff.Permanent(err)
		} else if err != nil {
			return err
		}

		if stateChecker(service.State) {
			return nil
		}

		return fmt.Errorf("service %s is in state %s", serviceId, service.State)
	}

	if maxWaitSeconds < 5 {
		maxWaitSeconds = 5
	}

	err := backoff.Retry(checkState, backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), uint64(maxWaitSeconds/5))) //nolint:gosec
	if err != nil {
		return err
	}

	return nil
}

func (c *ClientImpl) UpdateService(ctx context.Context, serviceId string, s ServiceUpdate) (*Service, error) {
	rb, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, ""), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ResponseWithResult[Service]{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	return &serviceResponse.Result, nil
}

func (c *ClientImpl) DeleteService(ctx context.Context, serviceId string) (*Service, error) {
	service, err := c.GetService(ctx, serviceId)
	if IsNotFound(err) {
		// That is what we want
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	if service.State != StateStopped && service.State != StateStopping {
		rb, _ := json.Marshal(ServiceStateUpdate{
			Command: "stop",
		})
		req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, "/state"), strings.NewReader(string(rb)))
		if err != nil {
			return nil, err
		}

		_, err = c.doRequest(ctx, req)
		if IsNotFound(err) {
			// That is what we want
			return nil, nil
		} else if err != nil {
			return nil, err
		}
	}

	err = c.WaitForServiceState(ctx, serviceId, func(state string) bool { return state == StateStopped }, 10*60)
	if IsNotFound(err) {
		// That is what we want
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodDelete, c.getServicePath(serviceId, ""), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if IsNotFound(err) {
		// That is what we want
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	serviceResponse := ResponseWithResult[ServiceResponseResult]{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	// Wait until service is deleted
	checkDeleted := func() error {
		_, err := c.GetService(ctx, serviceId)
		if IsNotFound(err) {
			// That is what we want
			return nil
		} else if err != nil {
			return err
		}

		return fmt.Errorf("service %s is not deleted yet", serviceId)
	}

	// Wait for up to 5 minutes for the service to be deleted
	err = backoff.Retry(checkDeleted, backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), 60))
	if err != nil {
		return nil, fmt.Errorf("service %s was not deleted in the allocated time", serviceId)
	}

	return &serviceResponse.Result.Service, nil
}

func (c *ClientImpl) RotateTDEKey(ctx context.Context, serviceId string, keyId string) error {
	rb, err := json.Marshal(ServiceKeyRotation{TransparentDataEncryptionKeyId: keyId})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, ""), strings.NewReader(string(rb)))
	if err != nil {
		return err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return err
	}

	serviceResponse := ResponseWithResult[Service]{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return err
	}

	return nil
}
