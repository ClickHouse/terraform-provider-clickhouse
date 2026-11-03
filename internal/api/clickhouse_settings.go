package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type ServiceClickhouseSettingWarning struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// ServiceClickhouseSettingsUpdateResult holds setting values as strings: the
// API returns either JSON strings or JSON integers.
type ServiceClickhouseSettingsUpdateResult struct {
	Settings map[string]string
	Warnings []ServiceClickhouseSettingWarning
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

// ListServiceClickhouseSettings returns only settings that were explicitly set.
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
func (c *ClientImpl) DeleteServiceClickhouseSetting(ctx context.Context, serviceId string, settingName string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getServicePath(serviceId, "/clickhouseSettings/"+url.PathEscape(settingName)), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
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
