package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type BackupConfiguration struct {
	BackupPeriodInHours          *int32  `json:"backupPeriodInHours"`
	BackupRetentionPeriodInHours *int32  `json:"backupRetentionPeriodInHours"`
	BackupStartTime              *string `json:"backupStartTime"`
}

func (c *ClientImpl) GetBackupConfiguration(ctx context.Context, serviceId string) (*BackupConfiguration, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/backupConfiguration"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	backupConfigResponse := ResponseWithResult[BackupConfiguration]{}
	err = json.Unmarshal(body, &backupConfigResponse)
	if err != nil {
		return nil, err
	}

	// The API returns a default value of 24 for BackupPeriodInHours field even when BackupStartTime is set.
	// This is logically wrong as both fields can't be set at the same time.
	// We fix this by setting BackupPeriodInHours to nil in such case.
	if backupConfigResponse.Result.BackupStartTime != nil && *backupConfigResponse.Result.BackupStartTime != "" {
		backupConfigResponse.Result.BackupPeriodInHours = nil
	}

	return &backupConfigResponse.Result, nil
}

func (c *ClientImpl) UpdateBackupConfiguration(ctx context.Context, serviceId string, b BackupConfiguration) (*BackupConfiguration, error) {
	rb, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, "/backupConfiguration"), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	backupConfigResponse := ResponseWithResult[BackupConfiguration]{}
	err = json.Unmarshal(body, &backupConfigResponse)
	if err != nil {
		return nil, err
	}

	return &backupConfigResponse.Result, nil
}
