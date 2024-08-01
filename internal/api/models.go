package api

/****
	Request and Response models for all API calls.
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
	Id                              string                        `json:"id,omitempty"`
	Name                            string                        `json:"name"`
	Provider                        string                        `json:"provider"`
	Region                          string                        `json:"region"`
	Tier                            string                        `json:"tier"`
	IdleScaling                     bool                          `json:"idleScaling"`
	IpAccessList                    []IpAccess                    `json:"ipAccessList"`
	MinTotalMemoryGb                *int                          `json:"minTotalMemoryGb,omitempty"`
	MaxTotalMemoryGb                *int                          `json:"maxTotalMemoryGb,omitempty"`
	NumReplicas                     *int                          `json:"numReplicas,omitempty"`
	IdleTimeoutMinutes              *int                          `json:"idleTimeoutMinutes,omitempty"`
	State                           string                        `json:"state,omitempty"`
	Endpoints                       []Endpoint                    `json:"endpoints,omitempty"`
	IAMRole                         string                        `json:"iamRole,omitempty"`
	PrivateEndpointConfig           *ServicePrivateEndpointConfig `json:"privateEndpointConfig,omitempty"`
	PrivateEndpointIds              []string                      `json:"privateEndpointIds,omitempty"`
	EncryptionKey                   string                        `json:"encryptionKey,omitempty"`
	EncryptionAssumedRoleIdentifier string                        `json:"encryptionAssumedRoleIdentifier,omitempty"`
}

type ServiceUpdate struct {
	Name               string                    `json:"name,omitempty"`
	IpAccessList       *IpAccessUpdate           `json:"ipAccessList,omitempty"`
	PrivateEndpointIds *PrivateEndpointIdsUpdate `json:"privateEndpointIds,omitempty"`
}

type ServiceScalingUpdate struct {
	IdleScaling        *bool `json:"idleScaling,omitempty"` // bool pointer so that `false`` is not omitted
	MinTotalMemoryGb   *int  `json:"minTotalMemoryGb,omitempty"`
	MaxTotalMemoryGb   *int  `json:"maxTotalMemoryGb,omitempty"`
	NumReplicas        *int  `json:"numReplicas,omitempty"`
	IdleTimeoutMinutes *int  `json:"idleTimeoutMinutes,omitempty"`
}

type ServicePasswordUpdate struct {
	NewPasswordHash   string `json:"newPasswordHash,omitempty"`
	NewDoubleSha1Hash string `json:"newDoubleSha1Hash,omitempty"`
}
