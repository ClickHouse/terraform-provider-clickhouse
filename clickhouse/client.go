package clickhouse

import (
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseUrl        string
	HttpClient     *http.Client
	OrganizationId string
	TokenKey       string
	TokenSecret    string
}

type IpAccess struct {
	Source      string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`
}

type IpAccessUpdate struct {
	Add    []IpAccess `json:"add,omitempty"`
	Remove []IpAccess `json:"remove,omitempty"`
}

type Service struct {
	Id                 string     `json:"id,omitempty"`
	Name               string     `json:"name,omitempty"`
	Provider           string     `json:"provider,omitempty"`
	Region             string     `json:"region,omitempty"`
	Tier               string     `json:"tier,omitempty"`
	IdleScaling        bool       `json:"idleScaling,omitempty"`
	IpAccessList       []IpAccess `json:"ipAccessList,omitempty"`
	MinTotalMemoryGb   int        `json:"minTotalMemoryGb,omitempty"`
	MaxTotalMemoryGb   int        `json:"maxTotalMemoryGb,omitempty"`
	IdleTimeoutMinutes int        `json:"idleTimeoutMinutes,omitempty"`
	State              string     `json:"state,omitempty"`
}

type ServiceUpdate struct {
	Name         string          `json:"name,omitempty"`
	IpAccessList *IpAccessUpdate `json:"ipAccessList,omitempty"`
}

type ServiceScalingUpdate struct {
	IdleScaling        *bool `json:"idleScaling,omitempty"` // bool pointer so that `false`` is not omitted
	MinTotalMemoryGb   int   `json:"minTotalMemoryGb,omitempty"`
	MaxTotalMemoryGb   int   `json:"maxTotalMemoryGb,omitempty"`
	IdleTimeoutMinutes int   `json:"idleTimeoutMinutes,omitempty"`
}

type ServiceStateUpdate struct {
	Command string `json:"command"`
}

type ServiceResponseResult struct {
	Service  Service `json:"service"`
	Password string  `json:"password"`
}

type ServiceDeleteResponse struct {
	Result ServiceResponseResult `json:"result"`
}

type ServicePostResponse struct {
	Result ServiceResponseResult `json:"result"`
}

type ServicePatchResponse struct {
	Result Service `json:"result"`
}

type ServiceGetResponse struct {
	Result Service `json:"result"`
}

type ServiceBody struct {
	Service Service `json:"service"`
}

func NewClient(env string, organizationId string, tokenKey string, tokenSecret string) (*Client, error) {
	envMap := map[string]string{
		"local":      "http://localhost:2023/v1",
		"qa":         "https://api.control-plane.clickhouse-dev.com/v1",
		"staging":    "https://api.control-plane.clickhouse-staging.com/v1",
		"production": "https://api.clickhouse.cloud/v1",
	}

	apiUrl, hasApiUrl := envMap[env]
	if !hasApiUrl {
		return nil, errors.New(fmt.Sprintf("Invalid environment: \"%s\". Only \"production\", \"staging\", \"qa\", or \"local\" is allowed.", env))
	}

	client := &Client{
		BaseUrl: apiUrl,
		HttpClient: &http.Client{
			Timeout: time.Second * 30,
		},
		OrganizationId: organizationId,
		TokenKey:       tokenKey,
		TokenSecret:    tokenSecret,
	}

	return client, nil
}

func (c *Client) getOrgPath(path string) string {
	return fmt.Sprintf("%s/organizations/%s%s", c.BaseUrl, c.OrganizationId, path)
}

func (c *Client) getServicePath(serviceId string, path string) string {
	if serviceId == "" {
		return c.getOrgPath("/services")
	} else {
		return c.getOrgPath(fmt.Sprintf("/services/%s%s", serviceId, path))
	}
}

func (c *Client) doRequest(req *http.Request) ([]byte, error) {
	credentials := fmt.Sprintf("%s:%s", c.TokenKey, c.TokenSecret)
	base64Credentials := b64.StdEncoding.EncodeToString([]byte(credentials))
	authHeader := fmt.Sprintf("Basic %s", base64Credentials)
	req.Header.Set("Authorization", authHeader)

	res, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %d, body: %s", res.StatusCode, body)
	}

	return body, err
}

// GetOrder - Returns a specifc order
func (c *Client) GetService(serviceId string) (*Service, error) {
	req, err := http.NewRequest("GET", c.getServicePath(serviceId, ""), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ServiceGetResponse{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	return &serviceResponse.Result, nil
}

func (c *Client) CreateService(s Service) (*Service, error) {
	rb, err := json.Marshal(s)

	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.getServicePath("", ""), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ServicePostResponse{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	return &serviceResponse.Result.Service, nil
}

func (c *Client) UpdateService(serviceId string, s ServiceUpdate) (*Service, error) {
	rb, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PATCH", c.getServicePath(serviceId, ""), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ServicePatchResponse{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	return &serviceResponse.Result, nil
}

func (c *Client) UpdateServiceScaling(serviceId string, s ServiceScalingUpdate) (*Service, error) {
	rb, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PATCH", c.getServicePath(serviceId, "/scaling"), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ServicePatchResponse{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	return &serviceResponse.Result, nil
}

func (c *Client) DeleteService(serviceId string) (*Service, error) {
	rb, err := json.Marshal(ServiceStateUpdate{
		Command: "stop",
	})
	req, err := http.NewRequest("PATCH", c.getServicePath(serviceId, "/state"), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	_, err = c.doRequest(req)
	if err != nil {
		return nil, err
	}

	for {
		service, err := c.GetService(serviceId)
		if err != nil {
			return nil, err
		}
		stopped := service.State == "stopped"
		if stopped {
			break
		}
		time.Sleep(5 * time.Second)
	}

	req, err = http.NewRequest("DELETE", c.getServicePath(serviceId, ""), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ServiceDeleteResponse{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	return &serviceResponse.Result.Service, nil
}
