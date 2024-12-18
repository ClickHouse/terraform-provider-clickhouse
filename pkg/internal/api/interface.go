package api

import (
	"context"
)

type Client interface {
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

	CreateDatabase(ctx context.Context, serviceID string, db Database) error
	GetDatabase(ctx context.Context, serviceID string, name string) (*Database, error)
	DeleteDatabase(ctx context.Context, serviceID string, name string) error
	SyncDatabase(ctx context.Context, serviceID string, db Database) error
}
