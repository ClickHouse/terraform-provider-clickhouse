package api

import (
	"context"
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

	GetQueryEndpoint(ctx context.Context, serviceID string) (*ServiceQueryEndpoint, error)
	CreateQueryEndpoint(ctx context.Context, serviceID string, endpoint ServiceQueryEndpoint) (*ServiceQueryEndpoint, error)
	DeleteQueryEndpoint(ctx context.Context, serviceID string) error

	GetClickPipe(ctx context.Context, serviceId string, clickPipeId string) (*ClickPipe, error)
	CreateClickPipe(ctx context.Context, serviceId string, clickPipe ClickPipe) (*ClickPipe, error)
	WaitForClickPipeState(ctx context.Context, serviceId string, clickPipeId string, stateChecker func(string) bool, maxWaitSeconds uint64) (*ClickPipe, error)
	ScalingClickPipe(ctx context.Context, serviceId string, clickPipeId string, request ClickPipeScaling) (*ClickPipe, error)
	ChangeClickPipeState(ctx context.Context, serviceId string, clickPipeId string, command string) (*ClickPipe, error)
	DeleteClickPipe(ctx context.Context, serviceId string, clickPipeId string) error
}
