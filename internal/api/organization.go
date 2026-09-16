package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type OrganizationUpdate struct {
	PrivateEndpoints *OrgPrivateEndpointsUpdate `json:"privateEndpoints,omitempty"`
	EnableCoreDumps  *bool                      `json:"enableCoreDumps,omitempty"`
}

type OrgResult struct {
	CreatedAt        string                    `json:"createdAt,omitempty"`
	ID               string                    `json:"id,omitempty"`
	Name             string                    `json:"name,omitempty"`
	PrivateEndpoints []PrivateEndpoint         `json:"privateEndpoints,omitempty"`
	EnableCoreDumps  *bool                     `json:"enableCoreDumps,omitempty"`
	Capabilities     *OrganizationCapabilities `json:"capabilities,omitempty"`
}

// OrganizationCapabilities reports per-capability eligibility for gated features. Pointer fields: an API version
// predating this field omits capabilities entirely (nil), and an individual capability may be absent (nil) — callers
// treat nil as "unknown", never as "ineligible".
type OrganizationCapabilities struct {
	Snapshots *bool `json:"snapshots,omitempty"`
}

// GetOrganization retrieves the current organization settings.
func (c *ClientImpl) GetOrganization(ctx context.Context) (*OrgResult, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath(""), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	orgResponse := ResponseWithResult[OrgResult]{}
	err = json.Unmarshal(body, &orgResponse)
	if err != nil {
		return nil, err
	}

	return &orgResponse.Result, nil
}

// UpdateOrganization updates the organization settings.
func (c *ClientImpl) UpdateOrganization(ctx context.Context, orgUpdate OrganizationUpdate) (*OrgResult, error) {
	rb, err := json.Marshal(orgUpdate)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPatch, c.getOrgPath(""), strings.NewReader(string(rb)))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	orgResponse := ResponseWithResult[OrgResult]{}
	err = json.Unmarshal(body, &orgResponse)
	if err != nil {
		return nil, err
	}

	return &orgResponse.Result, nil
}
