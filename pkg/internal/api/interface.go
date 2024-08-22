package api

type Client interface {
	GetService(serviceId string) (*Service, error)
	GetOrgPrivateEndpointConfig(cloudProvider string, region string) (*OrgPrivateEndpointConfig, error)
	CreateService(s Service) (*Service, string, error)
	WaitForServiceState(serviceId string, stateChecker func(string) bool, maxWaitSeconds int) error
	UpdateService(serviceId string, s ServiceUpdate) (*Service, error)
	UpdateServiceScaling(serviceId string, s ServiceScalingUpdate) (*Service, error)
	UpdateServicePassword(serviceId string, u ServicePasswordUpdate) (*ServicePasswordUpdateResult, error)
	GetServiceStatusCode(serviceId string) (*int, error)
	DeleteService(serviceId string) (*Service, error)
	GetOrganizationPrivateEndpoints() (*[]PrivateEndpoint, error)
	UpdateOrganizationPrivateEndpoints(orgUpdate OrganizationUpdate) (*[]PrivateEndpoint, error)
}
