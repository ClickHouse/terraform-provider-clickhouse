package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func (c *ClientImpl) GetSSHKeyPath(serviceId, sshKeyId string) string {
	return c.getServicePath(serviceId, fmt.Sprintf("/clickpipesSshKeyResources/%s", sshKeyId))
}

func (c *ClientImpl) ListSSHKeys(ctx context.Context, serviceId string) ([]*SSHKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/clickpipesSshKeyResources"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[[]SSHKey]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SSHKeys: %w", err)
	}

	result := make([]*SSHKey, len(response.Result))
	for i := range response.Result {
		sshKey := response.Result[i]
		result[i] = &sshKey
	}

	return result, nil
}

func (c *ClientImpl) GetSSHKey(ctx context.Context, serviceId, sshKeyId string) (*SSHKey, error) {
	req, err := http.NewRequest(http.MethodGet, c.GetSSHKeyPath(serviceId, sshKeyId), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[SSHKey]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SSHKey: %w", err)
	}

	return &response.Result, nil
}

func (c *ClientImpl) CreateSSHKey(ctx context.Context, serviceId string, request CreateSSHKey) (*SSHKey, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(request); err != nil {
		return nil, fmt.Errorf("failed to encode SSHKey: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.getServicePath(serviceId, "/clickpipesSshKeyResources"), &payload)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[SSHKey]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SSHKey: %w", err)
	}

	return &response.Result, nil
}

func (c *ClientImpl) DeleteSSHKey(ctx context.Context, serviceId, sshKeyId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.GetSSHKeyPath(serviceId, sshKeyId), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
}

// ValidateSSHKey triggers a connectivity check against the bastion using the
// stored key. The server returns the resource with an updated status (active or
// failed). Validation requires the public key to already be installed on the
// bastion, so it is a day-2 operation rather than part of resource creation.
func (c *ClientImpl) ValidateSSHKey(ctx context.Context, serviceId, sshKeyId string) (*SSHKey, error) {
	req, err := http.NewRequest(http.MethodPost, c.GetSSHKeyPath(serviceId, sshKeyId)+"/validate", bytes.NewBufferString("{}"))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[SSHKey]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal SSHKey: %w", err)
	}

	return &response.Result, nil
}
