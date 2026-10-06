package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// ClickHouseSettingValue is a setting value in its native JSON type. The API
// documents a string or an integer; Text holds the value without quotes and
// IsNumber records that it is sent, or was received, as a JSON number.
type ClickHouseSettingValue struct {
	Text     string
	IsNumber bool
}

func (v ClickHouseSettingValue) MarshalJSON() ([]byte, error) {
	if v.IsNumber {
		return json.Marshal(json.Number(v.Text))
	}
	return json.Marshal(v.Text)
}

func (v *ClickHouseSettingValue) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	switch val := raw.(type) {
	case nil:
	case string:
		*v = ClickHouseSettingValue{Text: val}
	case json.Number:
		*v = ClickHouseSettingValue{Text: val.String(), IsNumber: true}
	case bool:
		*v = ClickHouseSettingValue{Text: strconv.FormatBool(val)}
	default:
		var compact bytes.Buffer
		if err := json.Compact(&compact, data); err != nil {
			return err
		}
		*v = ClickHouseSettingValue{Text: compact.String()}
	}
	return nil
}

// ClickHouseSetting mirrors the OpenAPI ServiceClickhouseSetting shape.
type ClickHouseSetting struct {
	Name  string                 `json:"name"`
	Value ClickHouseSettingValue `json:"value"`
}

type clickHouseSettingsList struct {
	Settings []ClickHouseSetting `json:"settings"`
}

// ClickHouseSettingSchema follows the OpenAPI ServiceClickhouseSettingSchemaEntry
// shape, plus a Default the document does not list. Enum, Default and Example
// are decoded without a fixed type because the API may send them as strings or
// as numbers; only Name and Type are read.
type ClickHouseSettingSchema struct {
	Name              string `json:"name"`
	Type              string `json:"type"`
	Description       string `json:"description,omitempty"`
	Enum              []any  `json:"enum,omitempty"`
	Default           any    `json:"default,omitempty"`
	Warning           string `json:"warning,omitempty"`
	DeprecationNotice string `json:"deprecationNotice,omitempty"`
	Example           any    `json:"example,omitempty"`
}

type clickHouseSettingsSchemaList struct {
	Settings []ClickHouseSettingSchema `json:"settings"`
}

// ClickHouseSettingsUpdate is the PATCH request body. Settings not named are
// left as they are; resetting one is a separate DELETE.
type ClickHouseSettingsUpdate struct {
	Settings map[string]ClickHouseSettingValue `json:"settings"`
}

// ClickHouseSettingWarning is a warning the API returns for a setting whose
// change may be disruptive.
type ClickHouseSettingWarning struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

// ClickHouseSettingsUpdateResult mirrors the OpenAPI
// ServiceClickhouseSettingsPatchResponse shape.
type ClickHouseSettingsUpdateResult struct {
	Settings map[string]ClickHouseSettingValue `json:"settings"`
	Warnings []ClickHouseSettingWarning        `json:"warnings"`
}

// clickHouseSettingsResponseError marks a settings update the API accepted
// whose response could not be read. The settings were written; only the
// warnings the response carried are lost.
type clickHouseSettingsResponseError struct {
	err error
}

func (e *clickHouseSettingsResponseError) Error() string {
	return "decode ClickHouse settings update response: " + e.err.Error()
}

func (e *clickHouseSettingsResponseError) Unwrap() error {
	return e.err
}

func NewClickHouseSettingsResponseError(err error) error {
	return &clickHouseSettingsResponseError{err: err}
}

func IsClickHouseSettingsResponseUnreadable(err error) bool {
	var unreadable *clickHouseSettingsResponseError
	return errors.As(err, &unreadable)
}

// ListClickHouseSettings returns the settings explicitly set on the service.
func (c *ClientImpl) ListClickHouseSettings(ctx context.Context, serviceId string) ([]ClickHouseSetting, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/clickhouseSettings"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[clickHouseSettingsList]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Result.Settings, nil
}

// GetClickHouseSettingsSchema returns every setting the service lets callers
// configure, with its type.
func (c *ClientImpl) GetClickHouseSettingsSchema(ctx context.Context, serviceId string) ([]ClickHouseSettingSchema, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/clickhouseSettings/schema"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[clickHouseSettingsSchemaList]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Result.Settings, nil
}

// UpdateClickHouseSettings sets the given settings in one request and returns
// the API's warnings.
func (c *ClientImpl) UpdateClickHouseSettings(ctx context.Context, serviceId string, u ClickHouseSettingsUpdate) (*ClickHouseSettingsUpdateResult, error) {
	rb, err := json.Marshal(u)
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

	response := ResponseWithResult[ClickHouseSettingsUpdateResult]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, NewClickHouseSettingsResponseError(err)
	}

	return &response.Result, nil
}

// ResetClickHouseSetting returns a setting to its platform default.
func (c *ClientImpl) ResetClickHouseSetting(ctx context.Context, serviceId string, settingName string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getServicePath(serviceId, "/clickhouseSettings/"+url.PathEscape(settingName)), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
}
