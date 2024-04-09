package clickhouse

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

func NewClient(apiUrl string, organizationId string, tokenKey string, tokenSecret string) (*Client, error) {
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

/****
	Service
****/

type IpAccess struct {
	Source      string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`
}

type Endpoint struct {
	Protocol string `json:"protocol,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
}

type IpAccessUpdate struct {
	Add    []IpAccess `json:"add,omitempty"`
	Remove []IpAccess `json:"remove,omitempty"`
}

type PrivateEndpointIdsUpdate struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

type ServicePrivateEndpointConfig struct {
	EndpointServiceId  string `json:"endpointServiceId,omitempty"`
	PrivateDnsHostname string `json:"privateDnsHostname,omitempty"`
}
type ServiceManagedEncryption struct {
	KeyArn        string `json:"keyArn,omitempty"`
	AssumeRoleArn string `json:"assumeRoleArn,omitempty"`
}

type Service struct {
	Id                    						string                        `json:"id,omitempty"`
	Name                  						string                        `json:"name"`
	Provider              						string                        `json:"provider"`
	Region                						string                        `json:"region"`
	Tier                  						string                        `json:"tier"`
	IdleScaling           						bool                          `json:"idleScaling"`
	IpAccessList          						[]IpAccess                    `json:"ipAccessList"`
	MinTotalMemoryGb      						*int                          `json:"minTotalMemoryGb,omitempty"`
	MaxTotalMemoryGb      						*int                          `json:"maxTotalMemoryGb,omitempty"`
	IdleTimeoutMinutes    						*int                          `json:"idleTimeoutMinutes,omitempty"`
	State                 						string                        `json:"state,omitempty"`
	Endpoints             						[]Endpoint                    `json:"endpoints,omitempty"`
	IAMRole						    						string                        `json:"iamRole,omitempty"`
	PrivateEndpointConfig 						*ServicePrivateEndpointConfig `json:"privateEndpointConfig,omitempty"`
	PrivateEndpointIds    						[]string                      `json:"privateEndpointIds,omitempty"`
	EncryptionKey    									string   											`json:"encryptionKey,omitempty"`
	EncryptionAssumedRoleIdentifier		string												`json:"encryptionAssumedRoleIdentifier,omitempty"`
}

type ServiceUpdate struct {
	Name               string                    `json:"name,omitempty"`
	IpAccessList       *IpAccessUpdate           `json:"ipAccessList,omitempty"`
	PrivateEndpointIds *PrivateEndpointIdsUpdate `json:"privateEndpointIds,omitempty"`
}

type ServiceScalingUpdate struct {
	IdleScaling        *bool `json:"idleScaling,omitempty"` // bool pointer so that `false`` is not omitted
	MinTotalMemoryGb   *int   `json:"minTotalMemoryGb,omitempty"`
	MaxTotalMemoryGb   *int   `json:"maxTotalMemoryGb,omitempty"`
	IdleTimeoutMinutes *int   `json:"idleTimeoutMinutes,omitempty"`
}

type ServicePasswordUpdate struct {
	NewPasswordHash   string `json:"newPasswordHash,omitempty"`
	NewDoubleSha1Hash string `json:"newDoubleSha1Hash,omitempty"`
}

func ServicePasswordUpdateFromPlainPassword(password string) ServicePasswordUpdate {
	hash := sha256.Sum256([]byte(password))

	singleSha1Hash := sha1.Sum([]byte(password))
	doubleSha1Hash := sha1.Sum(singleSha1Hash[:])

	return ServicePasswordUpdate{
		NewPasswordHash:   base64.StdEncoding.EncodeToString(hash[:]),
		NewDoubleSha1Hash: hex.EncodeToString(doubleSha1Hash[:]),
	}
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
func (c *Client) getPrivateEndpointConfigPath(cloudProvider string, region string) string {
	return c.getOrgPath(fmt.Sprintf("/privateEndpointConfig?cloud_provider=%s&region_id=%s", cloudProvider, region))
}

func (c *Client) doRequest(req *http.Request) ([]byte, error) {
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

func (c *Client) checkStatusCode(req *http.Request) (*int, error) {
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

func (c *Client) GetOrgPrivateEndpointConfig(cloudProvider string, region string) (*OrgPrivateEndpointConfig, error) {
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

func (c *Client) CreateService(s Service) (*Service, string, error) {
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

func (c *Client) UpdateServicePassword(serviceId string, u ServicePasswordUpdate) (*ServicePasswordUpdateResult, error) {
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

func (c *Client) GetServiceStatusCode(serviceId string) (*int, error) {
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

func (c *Client) DeleteService(serviceId string) (*Service, error) {
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
			if (numErrors > MAX_RETRY) {
				return nil, err
			} else {
				time.Sleep(5 * time.Second)
				continue
			}
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

	numErrors = 0
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
	CreatedAt				 string            `json:"createdAt,omitempty"`
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

func (c *Client) GetOrganizationPrivateEndpoints() (*[]PrivateEndpoint, error) {
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

func (c *Client) UpdateOrganizationPrivateEndpoints(orgUpdate OrganizationUpdate) (*[]PrivateEndpoint, error) {
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
