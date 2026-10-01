package api

// ReversePrivateEndpoint represents a ClickPipe reverse private endpoint
type ReversePrivateEndpoint struct {
	CreateReversePrivateEndpoint

	ID                 string              `json:"id,omitempty"`
	ServiceID          string              `json:"serviceId,omitempty"`
	EndpointID         string              `json:"endpointId,omitempty"`
	DNSNames           []string            `json:"dnsNames,omitempty"`
	PrivateDNSNames    []string            `json:"privateDnsNames,omitempty"`
	PrivateDNSMappings []PrivateDNSMapping `json:"privateDnsMappings,omitempty"`
	DNSTargets         []DNSTarget         `json:"dnsTargets,omitempty"`
	Status             string              `json:"status,omitempty"`
}

// CustomPrivateDNSMapping represents a custom DNS name managed by ClickHouse Cloud.
type CustomPrivateDNSMapping struct {
	PrivateDNSName string `json:"privateDnsName,omitempty"`
	TargetID       string `json:"targetId,omitempty"`
}

// PrivateDNSMapping describes a read-only private DNS name and its internal target.
type PrivateDNSMapping struct {
	PrivateDNSName  string `json:"privateDnsName,omitempty"`
	InternalDNSName string `json:"internalDnsName,omitempty"`
}

// DNSTarget describes a read-only DNS target that custom private DNS mappings can reference by ID.
type DNSTarget struct {
	ID              string `json:"id,omitempty"`
	Kind            string `json:"kind,omitempty"`
	InternalDNSName string `json:"internalDnsName,omitempty"`
}

// CreateReversePrivateEndpoint is the request payload for creating a reverse private endpoint
type CreateReversePrivateEndpoint struct {
	Description                string                    `json:"description,omitempty"`
	Type                       string                    `json:"type,omitempty"`
	VPCEndpointServiceName     *string                   `json:"vpcEndpointServiceName,omitempty"`
	VPCResourceConfigurationID *string                   `json:"vpcResourceConfigurationId,omitempty"`
	VPCResourceShareArn        *string                   `json:"vpcResourceShareArn,omitempty"`
	MSKClusterArn              *string                   `json:"mskClusterArn,omitempty"`
	MSKAuthentication          *string                   `json:"mskAuthentication,omitempty"`
	GCPServiceAttachment       *string                   `json:"gcpServiceAttachment,omitempty"`
	CustomPrivateDNSMappings   []CustomPrivateDNSMapping `json:"customPrivateDnsMappings,omitempty"`
}

// UpdateReversePrivateEndpoint is the request payload for updating mutable reverse private endpoint fields.
type UpdateReversePrivateEndpoint struct {
	CustomPrivateDNSMappings *[]CustomPrivateDNSMapping `json:"customPrivateDnsMappings,omitempty"`
}
