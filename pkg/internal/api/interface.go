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
	RotateTDEKey(ctx context.Context, serviceId string, keyId string) error

	GetQueryEndpoint(ctx context.Context, serviceID string) (*ServiceQueryEndpoint, error)
	CreateQueryEndpoint(ctx context.Context, serviceID string, endpoint ServiceQueryEndpoint) (*ServiceQueryEndpoint, error)
	DeleteQueryEndpoint(ctx context.Context, serviceID string) error

	GetClickPipe(ctx context.Context, serviceId string, clickPipeId string) (*ClickPipe, error)
	CreateClickPipe(ctx context.Context, serviceId string, clickPipe ClickPipe) (*ClickPipe, error)
	UpdateClickPipe(ctx context.Context, serviceId string, clickPipeId string, request ClickPipeUpdate) (*ClickPipe, error)
	WaitForClickPipeState(ctx context.Context, serviceId string, clickPipeId string, stateChecker func(string) bool, maxWaitSeconds uint64) (*ClickPipe, error)
	ScalingClickPipe(ctx context.Context, serviceId string, clickPipeId string, request ClickPipeScaling) (*ClickPipe, error)
	ChangeClickPipeState(ctx context.Context, serviceId string, clickPipeId string, command string) (*ClickPipe, error)
	DeleteClickPipe(ctx context.Context, serviceId string, clickPipeId string) error

	GetReversePrivateEndpointPath(serviceId, reversePrivateEndpointId string) string
	ListReversePrivateEndpoints(ctx context.Context, serviceId string) ([]*ReversePrivateEndpoint, error)
	GetReversePrivateEndpoint(ctx context.Context, serviceId, reversePrivateEndpointId string) (*ReversePrivateEndpoint, error)
	CreateReversePrivateEndpoint(ctx context.Context, serviceId string, request CreateReversePrivateEndpoint) (*ReversePrivateEndpoint, error)
	DeleteReversePrivateEndpoint(ctx context.Context, serviceId, reversePrivateEndpointId string) error
	WaitForReversePrivateEndpointState(ctx context.Context, serviceId string, reversePrivateEndpointId string, stateChecker func(string) bool, maxWaitSeconds uint64) (*ReversePrivateEndpoint, error)

	CreateUser(ctx context.Context, serviceId string, user User) (*User, error)
	GetUser(ctx context.Context, serviceID string, name string) (*User, error)
	DeleteUser(ctx context.Context, serviceID string, name string) error

	CreateRole(ctx context.Context, serviceId string, role Role) (*Role, error)
	GetRole(ctx context.Context, serviceID string, name string) (*Role, error)
	DeleteRole(ctx context.Context, serviceID string, name string) error

	GrantRole(ctx context.Context, serviceId string, grantRole GrantRole) (*GrantRole, error)
	GetGrantRole(ctx context.Context, serviceID string, grantedRoleName string, granteeUserName *string, granteeRoleName *string) (*GrantRole, error)
	RevokeGrantRole(ctx context.Context, serviceID string, grantedRoleName string, granteeUserName *string, granteeRoleName *string) error

	GrantPrivilege(ctx context.Context, serviceId string, grantPrivilege GrantPrivilege) (*GrantPrivilege, error)
	GetGrantPrivilege(ctx context.Context, serviceID string, accessType string, database *string, table *string, column *string, granteeUserName *string, granteeRoleName *string) (*GrantPrivilege, error)
	RevokeGrantPrivilege(ctx context.Context, serviceID string, accessType string, database *string, table *string, column *string, granteeUserName *string, granteeRoleName *string) error

	CreateDatabase(ctx context.Context, serviceID string, db Database) error
	GetDatabase(ctx context.Context, serviceID string, name string) (*Database, error)
	DeleteDatabase(ctx context.Context, serviceID string, name string) error
	SyncDatabase(ctx context.Context, serviceID string, db Database) error
}
