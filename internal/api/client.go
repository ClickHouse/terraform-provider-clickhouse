package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ClientImpl struct {
	BaseUrl        string
	HttpClient     *http.Client
	OrganizationId string
	TokenKey       string
	TokenSecret    string
}

func NewClient(apiUrl string, organizationId string, tokenKey string, tokenSecret string) (*ClientImpl, error) {
	client := &ClientImpl{
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

type ServicePasswordUpdateResult struct {
	Password string `json:"password,omitempty"`
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

type OrgPrivateEndpointConfig struct {
	EndpointServiceId string `json:"endpointServiceId,omitempty"`
}

type OrgPrivateEndpointConfigGetResponse struct {
	Result OrgPrivateEndpointConfig `json:"result"`
}

type ServiceBody struct {
	Service Service `json:"service"`
}

type ServicePrivateEndpointConfigResponse struct {
	Result ServicePrivateEndpointConfig `json:"result"`
}

func (c *ClientImpl) getOrgPath(path string) string {
	return fmt.Sprintf("%s/organizations/%s%s", c.BaseUrl, c.OrganizationId, path)
}

func (c *ClientImpl) getServicePath(serviceId string, path string) string {
	if serviceId == "" {
		return c.getOrgPath("/services")
	}
	return c.getOrgPath(fmt.Sprintf("/services/%s%s", serviceId, path))
}

func (c *ClientImpl) getPrivateEndpointConfigPath(cloudProvider string, region string) string {
	return c.getOrgPath(fmt.Sprintf("/privateEndpointConfig?cloud_provider=%s&region_id=%s", cloudProvider, region))
}

func (c *ClientImpl) doRequest(req *http.Request) ([]byte, error) {
	credentials := fmt.Sprintf("%s:%s", c.TokenKey, c.TokenSecret)
	base64Credentials := base64.StdEncoding.EncodeToString([]byte(credentials))
	authHeader := fmt.Sprintf("Basic %s", base64Credentials)
	req.Header.Set("Authorization", authHeader)

	res, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %d, body: %s", res.StatusCode, body)
	}

	return body, err
}

func (c *ClientImpl) checkStatusCode(req *http.Request) (*int, error) {
	credentials := fmt.Sprintf("%s:%s", c.TokenKey, c.TokenSecret)
	base64Credentials := base64.StdEncoding.EncodeToString([]byte(credentials))
	authHeader := fmt.Sprintf("Basic %s", base64Credentials)
	req.Header.Set("Authorization", authHeader)

	res, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	return &res.StatusCode, err
}

// GetService - Returns a specifc order
func (c *ClientImpl) GetService(serviceId string) (*Service, error) {
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

	service := serviceResponse.Result

	req, err = http.NewRequest("GET", c.getServicePath(serviceId, "/privateEndpointConfig"), nil)
	if err != nil {
		return nil, err
	}

	body, err = c.doRequest(req)
	if err != nil {
		return nil, err
	}

	endpointConfigResponse := ServicePrivateEndpointConfigResponse{}
	err = json.Unmarshal(body, &endpointConfigResponse)
	if err != nil {
		return nil, err
	}

	service.PrivateEndpointConfig = &endpointConfigResponse.Result

	return &service, nil
}

func (c *ClientImpl) GetOrgPrivateEndpointConfig(cloudProvider string, region string) (*OrgPrivateEndpointConfig, error) {
	privateEndpointConfigPath := c.getPrivateEndpointConfigPath(cloudProvider, region)

	req, err := http.NewRequest("GET", privateEndpointConfigPath, nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	privateEndpointConfigResponse := OrgPrivateEndpointConfigGetResponse{}
	if err = json.Unmarshal(body, &privateEndpointConfigResponse); err != nil {
		return nil, err
	}

	return &privateEndpointConfigResponse.Result, nil
}

func (c *ClientImpl) CreateService(s Service) (*Service, string, error) {
	rb, err := json.Marshal(s)
	if err != nil {
		return nil, "", err
	}

	req, err := http.NewRequest("POST", c.getServicePath("", ""), strings.NewReader(string(rb)))
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, "", err
	}

	serviceResponse := ServicePostResponse{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, "", err
	}

	return &serviceResponse.Result.Service, serviceResponse.Result.Password, nil
}

func (c *ClientImpl) UpdateService(serviceId string, s ServiceUpdate) (*Service, error) {
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

func (c *ClientImpl) UpdateServiceScaling(serviceId string, s ServiceScalingUpdate) (*Service, error) {
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

func (c *ClientImpl) UpdateServicePassword(serviceId string, u ServicePasswordUpdate) (*ServicePasswordUpdateResult, error) {
	rb, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PATCH", c.getServicePath(serviceId, "/password"), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	serviceResponse := ServicePasswordUpdateResult{}
	err = json.Unmarshal(body, &serviceResponse)
	if err != nil {
		return nil, err
	}

	return &serviceResponse, nil
}

func (c *ClientImpl) GetServiceStatusCode(serviceId string) (*int, error) {
	req, err := http.NewRequest("GET", c.getServicePath(serviceId, ""), nil)
	if err != nil {
		return nil, err
	}

	statusCode, err := c.checkStatusCode(req)
	if err != nil {
		return nil, err
	}

	return statusCode, nil
}

func (c *ClientImpl) DeleteService(serviceId string) (*Service, error) {
	service, err := c.GetService(serviceId)
	if err != nil {
		return nil, err
	}

	if service.State != "stopped" && service.State != "stopping" {
		rb, _ := json.Marshal(ServiceStateUpdate{
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
	}

	numErrors := 0
	for {
		service, err := c.GetService(serviceId)
		if err != nil {
			numErrors++
			if numErrors > MaxRetry {
				return nil, err
			}
			time.Sleep(5 * time.Second)
			continue
		}

		if service.State == "stopped" {
			break
		}
		time.Sleep(5 * time.Second)
	}

	req, err := http.NewRequest("DELETE", c.getServicePath(serviceId, ""), nil)
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

	for {
		statusCode, _ := c.GetServiceStatusCode(serviceId)

		if *statusCode == 404 {
			break
		}

		time.Sleep(5 * time.Second)
	}

	return &serviceResponse.Result.Service, nil
}

/****
	Organization
****/

type PrivateEndpoint struct {
	CloudProvider string `json:"cloudProvider,omitempty"`
	Description   string `json:"description,omitempty"`
	EndpointId    string `json:"id,omitempty"`
	Region        string `json:"region,omitempty"`
}

type OrgPrivateEndpointsUpdate struct {
	Add    []PrivateEndpoint `json:"add,omitempty"`
	Remove []PrivateEndpoint `json:"remove,omitempty"`
}

type OrganizationUpdate struct {
	PrivateEndpoints *OrgPrivateEndpointsUpdate `json:"privateEndpoints"`
}

type OrgResult struct {
	CreatedAt        string            `json:"createdAt,omitempty"`
	ID               string            `json:"id,omitempty"`
	Name             string            `json:"name,omitempty"`
	PrivateEndpoints []PrivateEndpoint `json:"privateEndpoints,omitempty"`
}

type OrganizationGetResponse struct {
	Result OrgResult `json:"result,omitempty"`
}

type OrganizationUpdateResponse struct {
	Result OrgResult `json:"result,omitempty"`
}

func (c *ClientImpl) GetOrganizationPrivateEndpoints() (*[]PrivateEndpoint, error) {
	req, err := http.NewRequest("GET", c.getOrgPath(""), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	orgResponse := OrganizationGetResponse{}
	err = json.Unmarshal(body, &orgResponse)
	if err != nil {
		return nil, err
	}

	return &orgResponse.Result.PrivateEndpoints, nil
}

func (c *ClientImpl) UpdateOrganizationPrivateEndpoints(orgUpdate OrganizationUpdate) (*[]PrivateEndpoint, error) {
	rb, err := json.Marshal(orgUpdate)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("PATCH", c.getOrgPath(""), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	orgResponse := OrganizationUpdateResponse{}
	err = json.Unmarshal(body, &orgResponse)
	if err != nil {
		return nil, err
	}

	return &orgResponse.Result.PrivateEndpoints, nil
}
