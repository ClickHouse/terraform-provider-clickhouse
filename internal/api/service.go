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
// GetServiceBase fetches only the core service object (GET /services/{id}). It
// does not make the additional sub-resource calls GetService makes (private
// endpoint config, backup configuration, query endpoints). Use it when those
// enriched fields are not needed.
func (c *ClientImpl) GetServiceBase(ctx context.Context, serviceId string) (*Service, error) {
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
	return &service, nil
}

func (c *ClientImpl) GetService(ctx context.Context, serviceId string) (*Service, error) {
	service, err := c.GetServiceBase(ctx, serviceId)
	if err != nil {
		return nil, err
	}

	endpointConfigResponse, err := c.GetServicePrivateEndpointConfig(ctx, serviceId)
	if err != nil {
		return nil, err
	}

	service.PrivateEndpointConfig = endpointConfigResponse

	// Only primary services have backup settings.
	if service.IsPrimary != nil && *service.IsPrimary {
		backupConfiguration, err := c.GetBackupConfiguration(ctx, service.Id)
		if err != nil {
			return nil, err
		}

		service.BackupConfiguration = backupConfiguration
	}

	queryEndpoints, err := c.GetQueryEndpoint(ctx, service.Id)
	if err != nil {
		return nil, err
	}

	service.QueryAPIEndpoints = queryEndpoints

	return service, nil
}

// ListServices - Returns all services in the organization, optionally
// narrowed by repeated `filter` query params (e.g. tag filters).
func (c *ClientImpl) ListServices(ctx context.Context, filters []string) ([]Service, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/services"), nil)
	if err != nil {
		return nil, err
	}
	if len(filters) > 0 {
		q := req.URL.Query()
		for _, f := range filters {
			q.Add("filter", f)
		}
		req.URL.RawQuery = q.Encode()
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[[]Service]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal services list: %w", err)
	}
	return resp.Result, nil
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
	// Always rewrite the backup ID as we need to make Terraform state to work by API does not return it backup ID
	serviceResponse.Result.Service.BackupID = s.BackupID

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

// getServiceState returns just the service state. Unlike GetService it does
// not fan out to the private endpoint config, backup configuration and query
// endpoints sub-resources, so it is cheap enough to poll.
func (c *ClientImpl) getServiceState(ctx context.Context, serviceId string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, ""), nil)
	if err != nil {
		return "", err
	}
	body, err := c.doRequest(ctx, req)
	if err != nil {
		return "", err
	}

	serviceResponse := ResponseWithResult[Service]{}
	if err := json.Unmarshal(body, &serviceResponse); err != nil {
		return "", err
	}

	return serviceResponse.Result.State, nil
}

// wakeService asks the API to un-idle the service. The "awake" command is a
// no-op on a service that is already running or waking up.
func (c *ClientImpl) wakeService(ctx context.Context, serviceId string) error {
	rb, err := json.Marshal(ServiceStateUpdate{
		Command: "awake",
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, "/state"), strings.NewReader(string(rb)))
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
}

// waitForServiceRunning polls the service state until it is running — the
// only state in which the ClickPipes API accepts creations and updates
// (partially_running is NOT sufficient). It is a lightweight alternative to
// WaitForServiceState for internal use (e.g. after wakeService), polling only
// the state instead of the full GetService fan-out.
func (c *ClientImpl) waitForServiceRunning(ctx context.Context, serviceId string, maxWaitSeconds int) error {
	checkState := func() error {
		state, err := c.getServiceState(ctx, serviceId)
		if is5xx(err) {
			// 500s are automatically retried in `doRequest`.
			// If we get it here, we consider it an unrecoverable error.
			return backoff.Permanent(err)
		} else if err != nil {
			return err
		}

		if state == StateRunning {
			return nil
		}

		return fmt.Errorf("service %s is in state %s", serviceId, state)
	}

	if maxWaitSeconds < 5 {
		maxWaitSeconds = 5
	}

	return backoff.Retry(checkState, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), uint64(maxWaitSeconds/5)), ctx)) //nolint:gosec
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

	// The API may return 409 Conflict if the service is still transitioning
	// on the backend despite reporting as stopped. Retry the DELETE in that case.
	var body []byte
	deleteService := func() error {
		req, err := http.NewRequest(http.MethodDelete, c.getServicePath(serviceId, ""), nil)
		if err != nil {
			return backoff.Permanent(err)
		}
		body, err = c.doRequest(ctx, req)
		if IsNotFound(err) {
			// That is what we want; signal success so the caller can return nil.
			return nil
		}
		if IsConflict(err) {
			// Service is still transitioning; retry.
			return err
		}
		if err != nil {
			return backoff.Permanent(err)
		}
		return nil
	}

	err = backoff.Retry(deleteService, backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Second), 90))
	if IsNotFound(err) {
		// That is what we want
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if body == nil {
		// Deleted before we could read a response body (404 path above exited early).
		return nil, nil
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
