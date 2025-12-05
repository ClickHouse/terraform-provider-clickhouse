package api

import (
	"context"
	"time"
)

type Client interface {
	GetApiKeyID(ctx context.Context, name *string) (*ApiKey, error)

	GetService(ctx context.Context, serviceId string) (*Service, error)
	GetOrgPrivateEndpointConfig(ctx context.Context, cloudProvider string, region string) (*OrgPrivateEndpointConfig, error)
	CreateService(ctx context.Context, s Service) (*Service, string, error)
	WaitForServiceState(ctx context.Context, serviceId string, stateChecker func(string) bool, maxWaitSeconds int) error
	UpdateService(ctx context.Context, serviceId string, s ServiceUpdate) (*Service, error)
	UpdateReplicaScaling(ctx context.Context, serviceId string, s ReplicaScalingUpdate) (*Service, error)
	UpdateServicePassword(ctx context.Context, serviceId string, u ServicePasswordUpdate) (*ServicePasswordUpdateResult, error)
	DeleteService(ctx context.Context, serviceId string) (*Service, error)
	GetOrganizationPrivateEndpoints(ctx context.Context) (*[]PrivateEndpoint, error)
	UpdateOrganizationPrivateEndpoints(ctx context.Context, orgUpdate OrganizationUpdate) (*[]PrivateEndpoint, error)
	GetBackupConfiguration(ctx context.Context, serviceId string) (*BackupConfiguration, error)
	UpdateBackupConfiguration(ctx context.Context, serviceId string, b BackupConfiguration) (*BackupConfiguration, error)
	RotateTDEKey(ctx context.Context, serviceId string, keyId string) error

	GetQueryEndpoint(ctx context.Context, serviceID string) (*ServiceQueryEndpoint, error)
	CreateQueryEndpoint(ctx context.Context, serviceID string, endpoint ServiceQueryEndpoint) (*ServiceQueryEndpoint, error)
	DeleteQueryEndpoint(ctx context.Context, serviceID string) error

	GetClickPipe(ctx context.Context, serviceId string, clickPipeId string) (*ClickPipe, error)
	CreateClickPipe(ctx context.Context, serviceId string, clickPipe ClickPipe) (*ClickPipe, error)
	UpdateClickPipe(ctx context.Context, serviceId string, clickPipeId string, request ClickPipeUpdate) (*ClickPipe, error)
	WaitForClickPipeState(ctx context.Context, serviceId string, clickPipeId string, stateChecker func(string) bool, maxWait time.Duration) (*ClickPipe, error)
	ScalingClickPipe(ctx context.Context, serviceId string, clickPipeId string, request ClickPipeScalingRequest) (*ClickPipe, error)
	ChangeClickPipeState(ctx context.Context, serviceId string, clickPipeId string, command string) (*ClickPipe, error)
	DeleteClickPipe(ctx context.Context, serviceId string, clickPipeId string) error
	GetClickPipeSettings(ctx context.Context, serviceId string, clickPipeId string) (map[string]any, error)
	UpdateClickPipeSettings(ctx context.Context, serviceId string, clickPipeId string, settings map[string]any) (map[string]any, error)
	GetClickPipeCdcScaling(ctx context.Context, serviceId string) (*ClickPipeCdcScaling, error)
	UpdateClickPipeCdcScaling(ctx context.Context, serviceId string, request ClickPipeCdcScalingRequest) (*ClickPipeCdcScaling, error)

	GetReversePrivateEndpointPath(serviceId, reversePrivateEndpointId string) string
	ListReversePrivateEndpoints(ctx context.Context, serviceId string) ([]*ReversePrivateEndpoint, error)
	GetReversePrivateEndpoint(ctx context.Context, serviceId, reversePrivateEndpointId string) (*ReversePrivateEndpoint, error)
	CreateReversePrivateEndpoint(ctx context.Context, serviceId string, request CreateReversePrivateEndpoint) (*ReversePrivateEndpoint, error)
	DeleteReversePrivateEndpoint(ctx context.Context, serviceId, reversePrivateEndpointId string) error
	WaitForReversePrivateEndpointState(ctx context.Context, serviceId string, reversePrivateEndpointId string, stateChecker func(string) bool, maxWaitSeconds uint64) (*ReversePrivateEndpoint, error)
}
