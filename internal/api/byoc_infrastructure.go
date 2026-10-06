package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// BYOC infrastructure states as reported by the ClickHouse Cloud API.
const (
	ByocStateProvisioning = "infra-provisioning"
	ByocStateReady        = "infra-ready"
	ByocStateDegraded     = "infra-degraded"
	ByocStateUpgrading    = "infra-upgrading"
	ByocStateTerminating  = "infra-terminating"
	ByocStateTerminated   = "infra-terminated"
)

// ByocInfrastructure is the BYOC infrastructure summary returned by the
// create and update endpoints.
type ByocInfrastructure struct {
	Id            string `json:"id"`
	State         string `json:"state"`
	AccountId     string `json:"accountId"`
	RegionId      string `json:"regionId"`
	CloudProvider string `json:"cloudProvider"`
	DisplayName   string `json:"displayName"`
}

// ByocInfrastructureDetails is the full read-back of a BYOC infrastructure,
// including connectivity toggles and BYO-VPC settings read from the data
// plane. Write-only create parameters (externalId, Azure credentials, public
// subnet IDs, availability zone suffixes) are never returned.
type ByocInfrastructureDetails struct {
	Id                           string   `json:"id"`
	State                        string   `json:"state"`
	AccountId                    string   `json:"accountId"`
	RegionId                     string   `json:"regionId"`
	CloudProvider                string   `json:"cloudProvider"`
	DisplayName                  string   `json:"displayName"`
	EnablePrivateLink            *bool    `json:"enablePrivateLink,omitempty"`
	EnablePrivateLoadBalancer    *bool    `json:"enablePrivateLoadBalancer,omitempty"`
	EnablePublicLoadBalancer     *bool    `json:"enablePublicLoadBalancer,omitempty"`
	GcpPscSubnetId               *string  `json:"gcpPscSubnetId,omitempty"`
	VpcCidrRange                 *string  `json:"vpcCidrRange,omitempty"`
	VpcAvailabilityZoneList      []string `json:"vpcAvailabilityZoneList,omitempty"`
	IsByoVpc                     *bool    `json:"isByoVpc,omitempty"`
	ByoVpcId                     *string  `json:"byoVpcId,omitempty"`
	ByoVpcPrivateSubnetIds       []string `json:"byoVpcPrivateSubnetIds,omitempty"`
	ByoVpcPodCidrRangeNames      []string `json:"byoVpcPodCidrRangeNames,omitempty"`
	ByoVpcSharedVpcHostProjectId *string  `json:"byoVpcSharedVpcHostProjectId,omitempty"`
}

// ByocInfrastructureCreateRequest is the payload for creating a BYOC
// infrastructure. RegionId and AccountId are required; the rest are
// per-cloud/per-topology options.
type ByocInfrastructureCreateRequest struct {
	RegionId                  string            `json:"regionId"`
	AccountId                 string            `json:"accountId"`
	DisplayName               *string           `json:"displayName,omitempty"`
	ExternalId                *string           `json:"externalId,omitempty"`
	TenantId                  *string           `json:"tenantId,omitempty"`
	ServicePrincipalClientId  *string           `json:"servicePrincipalClientId,omitempty"`
	VpcCidrRange              *string           `json:"vpcCidrRange,omitempty"`
	AvailabilityZoneSuffixes  []string          `json:"availabilityZoneSuffixes,omitempty"`
	VpcId                     *string           `json:"vpcId,omitempty"`
	PrivateSubnetIds          []string          `json:"privateSubnetIds,omitempty"`
	PublicSubnetIds           []string          `json:"publicSubnetIds,omitempty"`
	GcpPodCidrRangeNames      []string          `json:"gcpPodCidrRangeNames,omitempty"`
	GcpSharedVpcHostProjectId *string           `json:"gcpSharedVpcHostProjectId,omitempty"`
	Tags                      map[string]string `json:"tags,omitempty"`
}

// ByocInfrastructureValidateRequest is the payload for the preflight
// validation endpoint. It takes the same parameters as creation except
// displayName, so a payload that passes validation is not rejected by create
// for the same inputs.
type ByocInfrastructureValidateRequest struct {
	RegionId                  string            `json:"regionId"`
	AccountId                 string            `json:"accountId"`
	ExternalId                *string           `json:"externalId,omitempty"`
	TenantId                  *string           `json:"tenantId,omitempty"`
	ServicePrincipalClientId  *string           `json:"servicePrincipalClientId,omitempty"`
	VpcCidrRange              *string           `json:"vpcCidrRange,omitempty"`
	AvailabilityZoneSuffixes  []string          `json:"availabilityZoneSuffixes,omitempty"`
	VpcId                     *string           `json:"vpcId,omitempty"`
	PrivateSubnetIds          []string          `json:"privateSubnetIds,omitempty"`
	PublicSubnetIds           []string          `json:"publicSubnetIds,omitempty"`
	GcpPodCidrRangeNames      []string          `json:"gcpPodCidrRangeNames,omitempty"`
	GcpSharedVpcHostProjectId *string           `json:"gcpSharedVpcHostProjectId,omitempty"`
	Tags                      map[string]string `json:"tags,omitempty"`
}

// ByocInfrastructureUpdateRequest is the payload for updating a BYOC
// infrastructure. Only set fields are sent; Tags replaces the full tag map.
// GcpPscSubnetId is only accepted together with EnablePrivateLink = true.
// Tags is a pointer so an explicit empty map serializes as "tags":{} and
// clears the server-side tags instead of being dropped by omitempty.
type ByocInfrastructureUpdateRequest struct {
	DisplayName               *string            `json:"displayName,omitempty"`
	EnablePrivateLink         *bool              `json:"enablePrivateLink,omitempty"`
	EnablePrivateLoadBalancer *bool              `json:"enablePrivateLoadBalancer,omitempty"`
	EnablePublicLoadBalancer  *bool              `json:"enablePublicLoadBalancer,omitempty"`
	GcpPscSubnetId            *string            `json:"gcpPscSubnetId,omitempty"`
	Tags                      *map[string]string `json:"tags,omitempty"`
}

// ByocInfrastructureValidation is the outcome of the preflight validation.
// When Supported is false an empty check list means nothing was verified,
// not that everything passed.
type ByocInfrastructureValidation struct {
	CloudProvider string                              `json:"cloudProvider"`
	Supported     bool                                `json:"supported"`
	AllPassed     bool                                `json:"allPassed"`
	AnyPassed     bool                                `json:"anyPassed"`
	Checks        []ByocInfrastructureValidationCheck `json:"checks"`
}

// ByocInfrastructureValidationCheck is one simulated cloud permission check.
// The check list mirrors data plane internals and may change; pin logic to
// the aggregate flags, not to specific check names.
type ByocInfrastructureValidationCheck struct {
	Name    string `json:"name"`
	Action  string `json:"action"`
	Group   string `json:"group"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

// ByocInfrastructurePrivateEndpointConfig is the private link endpoint
// service of a BYOC infrastructure.
type ByocInfrastructurePrivateEndpointConfig struct {
	EndpointName       string `json:"endpointName"`
	PrivateDnsHostname string `json:"privateDnsHostname"`
}

func (c *ClientImpl) getByocInfrastructurePath(byocId string, path string) string {
	if byocId == "" {
		return c.getOrgPath("/byocInfrastructure")
	}
	return c.getOrgPath(fmt.Sprintf("/byocInfrastructure/%s%s", byocId, path))
}

func (c *ClientImpl) CreateByocInfrastructure(ctx context.Context, r ByocInfrastructureCreateRequest) (*ByocInfrastructure, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal create BYOC infrastructure request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.getByocInfrastructurePath("", ""), bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build create BYOC infrastructure request: %w", err)
	}

	// No retries: the API has no idempotency key, and replaying a create whose
	// response was lost could start a duplicate hour-long provisioning.
	body, err := c.doRequestWithStatus(ctx, req, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create BYOC infrastructure: %w", err)
	}

	resp := ResponseWithResult[ByocInfrastructure]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal create BYOC infrastructure response: %w", err)
	}
	return &resp.Result, nil
}

func (c *ClientImpl) ValidateByocInfrastructure(ctx context.Context, r ByocInfrastructureValidateRequest) (*ByocInfrastructureValidation, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal validate BYOC infrastructure request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.getByocInfrastructurePath("", "")+"/validate", bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build validate BYOC infrastructure request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to validate BYOC infrastructure: %w", err)
	}

	resp := ResponseWithResult[ByocInfrastructureValidation]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal validate BYOC infrastructure response: %w", err)
	}
	return &resp.Result, nil
}

func (c *ClientImpl) GetByocInfrastructure(ctx context.Context, byocId string) (*ByocInfrastructureDetails, error) {
	req, err := http.NewRequest(http.MethodGet, c.getByocInfrastructurePath(byocId, ""), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get BYOC infrastructure request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[ByocInfrastructureDetails]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BYOC infrastructure details: %w", err)
	}
	return &resp.Result, nil
}

func (c *ClientImpl) UpdateByocInfrastructure(ctx context.Context, byocId string, r ByocInfrastructureUpdateRequest) (*ByocInfrastructure, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal update BYOC infrastructure request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.getByocInfrastructurePath(byocId, ""), bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build update BYOC infrastructure request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update BYOC infrastructure: %w", err)
	}

	resp := ResponseWithResult[ByocInfrastructure]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal update BYOC infrastructure response: %w", err)
	}
	return &resp.Result, nil
}

func (c *ClientImpl) DeleteByocInfrastructure(ctx context.Context, byocId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getByocInfrastructurePath(byocId, ""), nil)
	if err != nil {
		return fmt.Errorf("failed to build delete BYOC infrastructure request: %w", err)
	}

	if _, err := c.doRequest(ctx, req); err != nil {
		return err
	}
	return nil
}

func (c *ClientImpl) GetByocInfrastructureTags(ctx context.Context, byocId string) (map[string]string, error) {
	req, err := http.NewRequest(http.MethodGet, c.getByocInfrastructurePath(byocId, "/tags"), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get BYOC infrastructure tags request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[struct {
		Tags map[string]string `json:"tags"`
	}]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BYOC infrastructure tags: %w", err)
	}
	if resp.Result.Tags == nil {
		return map[string]string{}, nil
	}
	return resp.Result.Tags, nil
}

func (c *ClientImpl) GetByocInfrastructurePrivateEndpointConfig(ctx context.Context, byocId string) (*ByocInfrastructurePrivateEndpointConfig, error) {
	req, err := http.NewRequest(http.MethodGet, c.getByocInfrastructurePath(byocId, "/privateEndpointConfig"), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get BYOC infrastructure private endpoint config request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[ByocInfrastructurePrivateEndpointConfig]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BYOC infrastructure private endpoint config: %w", err)
	}
	return &resp.Result, nil
}

// WaitForByocInfrastructureState polls the BYOC infrastructure details until
// stateChecker returns true for the reported state, mirroring
// WaitForServiceState. Permanent 4xx errors (including 404 after termination
// completes) fail immediately; the caller decides whether a 404 is terminal
// success (deletion) or failure.
func (c *ClientImpl) WaitForByocInfrastructureState(ctx context.Context, byocId string, stateChecker func(string) bool, maxWaitSeconds int) error {
	checkState := func() error {
		details, err := c.GetByocInfrastructure(ctx, byocId)
		if is5xx(err) || is4xxPermanent(err) {
			return backoff.Permanent(err)
		} else if err != nil {
			return err
		}

		if stateChecker(details.State) {
			return nil
		}

		return fmt.Errorf("BYOC infrastructure %s is in state %s", byocId, details.State)
	}

	if maxWaitSeconds < 5 {
		maxWaitSeconds = 5
	}

	return backoff.Retry(checkState, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), uint64(maxWaitSeconds/5)), ctx)) //nolint:gosec
}
