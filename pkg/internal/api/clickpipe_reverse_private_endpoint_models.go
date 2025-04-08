package api

// ReversePrivateEndpoint represents a ClickPipe reverse private endpoint
type ReversePrivateEndpoint struct {
	CreateReversePrivateEndpoint

	ID              string   `json:"id,omitempty"`
	ServiceID       string   `json:"serviceId,omitempty"`
	EndpointID      string   `json:"endpointId,omitempty"`
	DNSNames        []string `json:"dnsNames,omitempty"`
	PrivateDNSNames []string `json:"privateDnsNames,omitempty"`
	Status          string   `json:"status,omitempty"`
}

// CreateReversePrivateEndpoint is the request payload for creating a reverse private endpoint
type CreateReversePrivateEndpoint struct {
	Description                string  `json:"description,omitempty"`
	Type                       string  `json:"type,omitempty"`
	VPCEndpointServiceName     *string `json:"vpcEndpointServiceName,omitempty"`
	VPCResourceConfigurationID *string `json:"vpcResourceConfigurationId,omitempty"`
	VPCResourceShareArn        *string `json:"vpcResourceShareArn,omitempty"`
	MSKClusterArn              *string `json:"mskClusterArn,omitempty"`
	MSKAuthentication          *string `json:"mskAuthentication,omitempty"`
}
