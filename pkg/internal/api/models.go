package api

const (
	EndpointProtocolNativeSecure = "nativesecure"
	EndpointProtocolHTTPS        = "https"
	EndpointProtocolMysql        = "mysql"
)

type IpAccess struct {
	Source      string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`
}

type Endpoint struct {
	Protocol string `json:"protocol,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Enabled  bool   `json:"enabled"`
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
	BYOCId                          *string                       `json:"byocId,omitempty"`
	DataWarehouseId                 *string                       `json:"dataWarehouseId,omitempty"`
	IsPrimary                       *bool                         `json:"isPrimary,omitempty"`
	ReadOnly                        bool                          `json:"isReadonly"`
	Name                            string                        `json:"name"`
	Provider                        string                        `json:"provider"`
	Region                          string                        `json:"region"`
	Tier                            string                        `json:"tier,omitempty"`
	IdleScaling                     bool                          `json:"idleScaling"`
	IpAccessList                    []IpAccess                    `json:"ipAccessList"`
	MinTotalMemoryGb                *int                          `json:"minTotalMemoryGb,omitempty"`
	MaxTotalMemoryGb                *int                          `json:"maxTotalMemoryGb,omitempty"`
	MinReplicaMemoryGb              *int                          `json:"minReplicaMemoryGb,omitempty"`
	MaxReplicaMemoryGb              *int                          `json:"maxReplicaMemoryGb,omitempty"`
	NumReplicas                     *int                          `json:"numReplicas,omitempty"`
	IdleTimeoutMinutes              *int                          `json:"idleTimeoutMinutes,omitempty"`
	State                           string                        `json:"state,omitempty"`
	Endpoints                       []Endpoint                    `json:"endpoints,omitempty"`
	IAMRole                         string                        `json:"iamRole,omitempty"`
	PrivateEndpointConfig           *ServicePrivateEndpointConfig `json:"privateEndpointConfig,omitempty"`
	PrivateEndpointIds              []string                      `json:"privateEndpointIds,omitempty"`
	EncryptionKey                   string                        `json:"encryptionKey,omitempty"`
	EncryptionAssumedRoleIdentifier string                        `json:"encryptionAssumedRoleIdentifier,omitempty"`
	HasTransparentDataEncryption    bool                          `json:"hasTransparentDataEncryption,omitempty"`
	TransparentEncryptionDataKeyID  string                        `json:"transparentDataEncryptionKeyId,omitempty"`
	EncryptionRoleID                string                        `json:"encryptionRoleId,omitempty"`
	BackupConfiguration             *BackupConfiguration          `json:"backupConfiguration,omitempty"`
	ReleaseChannel                  string                        `json:"releaseChannel,omitempty"`
	QueryAPIEndpoints               *ServiceQueryEndpoint         `json:"-"`
}

type ServiceUpdate struct {
	Name               string                    `json:"name,omitempty"`
	IpAccessList       *IpAccessUpdate           `json:"ipAccessList,omitempty"`
	PrivateEndpointIds *PrivateEndpointIdsUpdate `json:"privateEndpointIds,omitempty"`
	ReleaseChannel     string                    `json:"releaseChannel,omitempty"`
	Endpoints          []Endpoint                `json:"endpoints,omitempty"`
}

type ServiceKeyRotation struct {
	TransparentDataEncryptionKeyId string `json:"transparentDataEncryptionKeyId"`
}

// FixMemoryBounds ensures the MinTotalMemoryGb and MaxTotalMemoryGb fields are set before doing an API call to create the service
// This is needed because there is a different interface between the /replicaScaling and the service creation API calls.
func (s *Service) FixMemoryBounds() {
	if s.MinReplicaMemoryGb == nil && s.MinTotalMemoryGb != nil {
		// Due to a bug on the API, we always assumed the MinTotalMemoryGb value was always related to 3 replicas.
		// Now we use a per-replica API to set the min total memory so we need to divide by 3 to get the same
		// behaviour as before.
		minReplicaMemory := *s.MinTotalMemoryGb / 3
		s.MinReplicaMemoryGb = &minReplicaMemory
		s.MinTotalMemoryGb = nil
	}

	if s.MaxReplicaMemoryGb == nil && s.MaxTotalMemoryGb != nil {
		// Due to a bug on the API, we always assumed the MaxTotalMemoryGb value was always related to 3 replicas.
		// Now we use a per-replica API to set the min total memory so we need to divide by 3 to get the same
		// behaviour as before.
		maxReplicaMemory := *s.MaxTotalMemoryGb / 3
		s.MaxReplicaMemoryGb = &maxReplicaMemory
		s.MaxTotalMemoryGb = nil
	}
}
