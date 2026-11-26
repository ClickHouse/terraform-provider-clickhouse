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

// BYOC infrastructure states as reported by the ClickHouse Cloud API. The
// API documents state as an open string, so callers must tolerate others.
const (
	ByocStateProvisioning = "infra-provisioning"
	ByocStateReady        = "infra-ready"
	ByocStateDegraded     = "infra-degraded"
	ByocStateUpgrading    = "infra-upgrading"
	ByocStateTerminating  = "infra-terminating"
	ByocStateTerminated   = "infra-terminated"
	// ByocStateAwaitingConnector: a restricted-access infrastructure is
	// registered and waits for the first heartbeat of the connector the
	// customer installs.
	ByocStateAwaitingConnector = "infra-awaiting-connector"
)

// BYOC access models. Standard infrastructures are provisioned by ClickHouse
// through a management role; restricted-access ones register a cluster the
// customer owns, reached only through the connector they install.
const (
	ByocAccessModelStandard   = "standard"
	ByocAccessModelRestricted = "restricted"
)

// Image modes of a restricted-access infrastructure: direct pulls platform
// images from the ClickHouse registry, bundle from the customer's registry.
const (
	ByocImageModeDirect = "direct"
	ByocImageModeBundle = "bundle"
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
	AccessModel   string `json:"accessModel,omitempty"`
}

// IsRestrictedAccess reports whether the infrastructure uses the restricted
// access model. An absent access model is standard.
func IsRestrictedAccess(accessModel string) bool {
	return accessModel == ByocAccessModelRestricted
}

// ByocRestrictedFacts is the registration of a restricted-access
// infrastructure: the customer cluster, the identities ClickHouse trusts, the
// image source and the storage services use. Optional fields are pointers so
// an omitted value is not sent and the API applies its default.
type ByocRestrictedFacts struct {
	ClusterName              string   `json:"clusterName"`
	PlatformNodeGroup        *string  `json:"platformNodeGroup,omitempty"`
	AvailabilityZones        []string `json:"availabilityZones,omitempty"`
	NamespacePrefix          *string  `json:"namespacePrefix,omitempty"`
	ExecutorRoleArn          *string  `json:"executorRoleArn,omitempty"`
	NodeRoleArns             []string `json:"nodeRoleArns,omitempty"`
	ImageMode                string   `json:"imageMode"`
	ImageRegistry            *string  `json:"imageRegistry,omitempty"`
	DataBucket               string   `json:"dataBucket"`
	BackupBucket             string   `json:"backupBucket"`
	StorageRoleArn           string   `json:"storageRoleArn"`
	ScraperEndpoint          *string  `json:"scraperEndpoint,omitempty"`
	EnableMonitoring         *bool    `json:"enableMonitoring,omitempty"`
	EnableSnapshotController *bool    `json:"enableSnapshotController,omitempty"`
	EnableAutoscaler         *bool    `json:"enableAutoscaler,omitempty"`
}

// ByocConnector is the liveness of the connector of a restricted-access
// infrastructure. PlatformDetail is set only when the customer has to act.
type ByocConnector struct {
	LastHeartbeatAt *string                  `json:"lastHeartbeatAt,omitempty"`
	Components      []ByocConnectorComponent `json:"components"`
	PlatformState   *string                  `json:"platformState,omitempty"`
	PlatformDetail  *string                  `json:"platformDetail,omitempty"`
}

// ByocConnectorComponent is one connector component (executor, scraper,
// troubleshooter, ...) as it last reported.
type ByocConnectorComponent struct {
	Name            string  `json:"name"`
	State           string  `json:"state"`
	Version         *string `json:"version,omitempty"`
	LastHeartbeatAt *string `json:"lastHeartbeatAt,omitempty"`
}

// ByocEnrollmentTokenRequest asks for a token with a lifetime of TtlSeconds
// (whole minutes between one hour and one day); nil takes the API default.
type ByocEnrollmentTokenRequest struct {
	TtlSeconds *int64 `json:"ttlSeconds,omitempty"`
}

// ByocEnrollmentToken is a token the connector of a restricted-access
// infrastructure enrolls with. The API returns it once and never again.
type ByocEnrollmentToken struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

// ByocInfrastructureDetails is the full read-back of a BYOC infrastructure,
// including connectivity toggles and BYO-VPC settings read from the data
// plane. Write-only create parameters (externalId, Azure credentials, public
// subnet IDs, availability zone suffixes) are never returned.
type ByocInfrastructureDetails struct {
	Id                           string               `json:"id"`
	State                        string               `json:"state"`
	AccountId                    string               `json:"accountId"`
	RegionId                     string               `json:"regionId"`
	CloudProvider                string               `json:"cloudProvider"`
	DisplayName                  string               `json:"displayName"`
	EnablePrivateLink            *bool                `json:"enablePrivateLink,omitempty"`
	EnablePrivateLoadBalancer    *bool                `json:"enablePrivateLoadBalancer,omitempty"`
	EnablePublicLoadBalancer     *bool                `json:"enablePublicLoadBalancer,omitempty"`
	GcpPscSubnetId               *string              `json:"gcpPscSubnetId,omitempty"`
	VpcCidrRange                 *string              `json:"vpcCidrRange,omitempty"`
	VpcAvailabilityZoneList      []string             `json:"vpcAvailabilityZoneList,omitempty"`
	IsByoVpc                     *bool                `json:"isByoVpc,omitempty"`
	ByoVpcId                     *string              `json:"byoVpcId,omitempty"`
	ByoVpcPrivateSubnetIds       []string             `json:"byoVpcPrivateSubnetIds,omitempty"`
	ByoVpcPodCidrRangeNames      []string             `json:"byoVpcPodCidrRangeNames,omitempty"`
	ByoVpcSharedVpcHostProjectId *string              `json:"byoVpcSharedVpcHostProjectId,omitempty"`
	AccessModel                  string               `json:"accessModel,omitempty"`
	RestrictedFacts              *ByocRestrictedFacts `json:"restrictedFacts,omitempty"`
	Connector                    *ByocConnector       `json:"connector,omitempty"`
}

// ByocInfrastructureCreateRequest is the payload for creating a BYOC
// infrastructure. RegionId and AccountId are required; the rest are
// per-cloud/per-topology options.
type ByocInfrastructureCreateRequest struct {
	RegionId                  string               `json:"regionId"`
	AccountId                 string               `json:"accountId"`
	DisplayName               *string              `json:"displayName,omitempty"`
	ExternalId                *string              `json:"externalId,omitempty"`
	TenantId                  *string              `json:"tenantId,omitempty"`
	ServicePrincipalClientId  *string              `json:"servicePrincipalClientId,omitempty"`
	VpcCidrRange              *string              `json:"vpcCidrRange,omitempty"`
	AvailabilityZoneSuffixes  []string             `json:"availabilityZoneSuffixes,omitempty"`
	VpcId                     *string              `json:"vpcId,omitempty"`
	PrivateSubnetIds          []string             `json:"privateSubnetIds,omitempty"`
	PublicSubnetIds           []string             `json:"publicSubnetIds,omitempty"`
	GcpPodCidrRangeNames      []string             `json:"gcpPodCidrRangeNames,omitempty"`
	GcpSharedVpcHostProjectId *string              `json:"gcpSharedVpcHostProjectId,omitempty"`
	Tags                      map[string]string    `json:"tags,omitempty"`
	AccessModel               *string              `json:"accessModel,omitempty"`
	RestrictedFacts           *ByocRestrictedFacts `json:"restrictedFacts,omitempty"`
}

// ByocInfrastructureValidateRequest is the payload for the preflight
// validation endpoint. It takes the same parameters as creation except
// displayName, so a payload that passes validation is not rejected by create
// for the same inputs.
type ByocInfrastructureValidateRequest struct {
	RegionId                  string               `json:"regionId"`
	AccountId                 string               `json:"accountId"`
	ExternalId                *string              `json:"externalId,omitempty"`
	TenantId                  *string              `json:"tenantId,omitempty"`
	ServicePrincipalClientId  *string              `json:"servicePrincipalClientId,omitempty"`
	VpcCidrRange              *string              `json:"vpcCidrRange,omitempty"`
	AvailabilityZoneSuffixes  []string             `json:"availabilityZoneSuffixes,omitempty"`
	VpcId                     *string              `json:"vpcId,omitempty"`
	PrivateSubnetIds          []string             `json:"privateSubnetIds,omitempty"`
	PublicSubnetIds           []string             `json:"publicSubnetIds,omitempty"`
	GcpPodCidrRangeNames      []string             `json:"gcpPodCidrRangeNames,omitempty"`
	GcpSharedVpcHostProjectId *string              `json:"gcpSharedVpcHostProjectId,omitempty"`
	Tags                      map[string]string    `json:"tags,omitempty"`
	AccessModel               *string              `json:"accessModel,omitempty"`
	RestrictedFacts           *ByocRestrictedFacts `json:"restrictedFacts,omitempty"`
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

// ByocInfrastructureCustomerAuthorizedAccess is the customer setting that
// controls when ClickHouse personnel may access a BYOC infrastructure, plus
// the data plane's verdict at the time of the read. Timestamps are RFC3339.
// A never-configured setting reads back as Enabled = true, DurationSeconds = 0.
type ByocInfrastructureCustomerAuthorizedAccess struct {
	Enabled            bool    `json:"enabled"`
	DurationSeconds    int64   `json:"durationSeconds"`
	StartTimestamp     *string `json:"startTimestamp,omitempty"`
	Allowed            bool    `json:"allowed"`
	WindowEndTimestamp *string `json:"windowEndTimestamp,omitempty"`
}

// ByocInfrastructureCustomerAuthorizedAccessUpdateRequest replaces the whole
// setting: Enabled = false denies, DurationSeconds = 0 allows without a
// deadline, otherwise access is allowed for DurationSeconds from
// StartTimestamp (RFC3339, required when DurationSeconds > 0).
type ByocInfrastructureCustomerAuthorizedAccessUpdateRequest struct {
	Enabled         bool    `json:"enabled"`
	DurationSeconds int64   `json:"durationSeconds"`
	StartTimestamp  *string `json:"startTimestamp,omitempty"`
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
	// Unwrapped so callers can classify the status (IsConflict matches the
	// "status: 409" prefix).
	body, err := c.doRequestWithStatus(ctx, req, false)
	if err != nil {
		return nil, err
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

// CreateByocInfrastructureEnrollmentToken mints an enrollment token for a
// restricted-access infrastructure. The API answers 409 until the cluster
// trust of the infrastructure is admitted; callers decide whether to wait.
func (c *ClientImpl) CreateByocInfrastructureEnrollmentToken(ctx context.Context, byocId string, r ByocEnrollmentTokenRequest) (*ByocEnrollmentToken, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal create BYOC enrollment token request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.getByocInfrastructurePath(byocId, "/enrollmentToken"), bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build create BYOC enrollment token request: %w", err)
	}

	// No retries: a replayed mint whose response was lost would issue a
	// second live token, and its 409 would read as trust not yet admitted.
	body, err := c.doRequestWithStatus(ctx, req, false)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[ByocEnrollmentToken]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BYOC enrollment token: %w", err)
	}
	return &resp.Result, nil
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

func (c *ClientImpl) GetByocInfrastructureCustomerAuthorizedAccess(ctx context.Context, byocId string) (*ByocInfrastructureCustomerAuthorizedAccess, error) {
	req, err := http.NewRequest(http.MethodGet, c.getByocInfrastructurePath(byocId, "/customerAuthorizedAccess"), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build get BYOC infrastructure customer-authorized access request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[ByocInfrastructureCustomerAuthorizedAccess]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BYOC infrastructure customer-authorized access: %w", err)
	}
	return &resp.Result, nil
}

func (c *ClientImpl) UpdateByocInfrastructureCustomerAuthorizedAccess(ctx context.Context, byocId string, r ByocInfrastructureCustomerAuthorizedAccessUpdateRequest) (*ByocInfrastructureCustomerAuthorizedAccess, error) {
	payload, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal update BYOC infrastructure customer-authorized access request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.getByocInfrastructurePath(byocId, "/customerAuthorizedAccess"), bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build update BYOC infrastructure customer-authorized access request: %w", err)
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to update BYOC infrastructure customer-authorized access: %w", err)
	}

	resp := ResponseWithResult[ByocInfrastructureCustomerAuthorizedAccess]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal update BYOC infrastructure customer-authorized access response: %w", err)
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
		if is5xx(err) || Is4xxPermanent(err) {
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

	// Deadline on the context, not just a retry count: each GET can itself
	// retry transport failures for about a minute, which would otherwise
	// stretch the budget to a multiple of maxWaitSeconds.
	ctx, cancel := context.WithTimeout(ctx, time.Duration(maxWaitSeconds)*time.Second)
	defer cancel()

	return backoff.Retry(checkState, backoff.WithContext(backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), uint64(maxWaitSeconds/5)), ctx)) //nolint:gosec
}
