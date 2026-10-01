package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ServiceClickhouseSettingWarning mirrors ServiceClickhouseSettingWarning in
// the OpenAPI spec: a notice the server returns for settings whose change may
// be disruptive (e.g. `compatibility`).
type ServiceClickhouseSettingWarning struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// ServiceClickhouseSettingsUpdateResult is the PATCH response. Setting values
// are normalized to their string form: the API returns either a JSON string or
// a JSON integer.
type ServiceClickhouseSettingsUpdateResult struct {
	Settings map[string]string
	Warnings []ServiceClickhouseSettingWarning
}

// ServiceClickhouseSettingSchemaEntry mirrors ServiceClickhouseSettingSchemaEntry
// in the OpenAPI spec.
type ServiceClickhouseSettingSchemaEntry struct {
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	Description       string  `json:"description,omitempty"`
	Enum              []int64 `json:"enum,omitempty"`
	Warning           string  `json:"warning,omitempty"`
	DeprecationNotice string  `json:"deprecationNotice,omitempty"`
	Example           string  `json:"example,omitempty"`
}

type serviceClickhouseSetting struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type serviceClickhouseSettingsList struct {
	Settings []serviceClickhouseSetting `json:"settings"`
}

type serviceClickhouseSettingsPatchRequest struct {
	Settings map[string]any `json:"settings"`
}

type serviceClickhouseSettingsPatchResponse struct {
	Settings map[string]json.RawMessage        `json:"settings"`
	Warnings []ServiceClickhouseSettingWarning `json:"warnings"`
}

type serviceClickhouseSettingsSchema struct {
	Settings []ServiceClickhouseSettingSchemaEntry `json:"settings"`
}

// ListServiceClickhouseSettings returns the explicitly configured ClickHouse
// settings of a service, keyed by name. Settings left at their platform
// default are not included.
func (c *ClientImpl) ListServiceClickhouseSettings(ctx context.Context, serviceId string) (map[string]string, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/clickhouseSettings"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[serviceClickhouseSettingsList]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	settings := make(map[string]string, len(response.Result.Settings))
	for _, s := range response.Result.Settings {
		value, err := clickhouseSettingValueString(s.Value)
		if err != nil {
			return nil, fmt.Errorf("setting %q: %w", s.Name, err)
		}
		settings[s.Name] = value
	}

	return settings, nil
}

// UpdateServiceClickhouseSettings sets one or more ClickHouse settings. Each
// value must be a string or an integer, matching the setting's type in
// GetServiceClickhouseSettingsSchema.
func (c *ClientImpl) UpdateServiceClickhouseSettings(ctx context.Context, serviceId string, settings map[string]any) (*ServiceClickhouseSettingsUpdateResult, error) {
	rb, err := json.Marshal(serviceClickhouseSettingsPatchRequest{Settings: settings})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, "/clickhouseSettings"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[serviceClickhouseSettingsPatchResponse]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	result := &ServiceClickhouseSettingsUpdateResult{
		Settings: make(map[string]string, len(response.Result.Settings)),
		Warnings: response.Result.Warnings,
	}
	for name, raw := range response.Result.Settings {
		value, err := clickhouseSettingValueString(raw)
		if err != nil {
			return nil, fmt.Errorf("setting %q: %w", name, err)
		}
		result.Settings[name] = value
	}

	return result, nil
}

// DeleteServiceClickhouseSetting resets a setting to its platform default.
// Resetting a setting that was never configured is a no-op on the server.
func (c *ClientImpl) DeleteServiceClickhouseSetting(ctx context.Context, serviceId string, settingName string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getServicePath(serviceId, "/clickhouseSettings/"+url.PathEscape(settingName)), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
}

// GetServiceClickhouseSettingsSchema returns every configurable ClickHouse
// setting of a service, with its type and constraints.
func (c *ClientImpl) GetServiceClickhouseSettingsSchema(ctx context.Context, serviceId string) ([]ServiceClickhouseSettingSchemaEntry, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/clickhouseSettings/schema"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[serviceClickhouseSettingsSchema]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Result.Settings, nil
}

func clickhouseSettingValueString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}

	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return "", fmt.Errorf("value %s is neither a string nor a number", string(raw))
	}

	return n.String(), nil
}
