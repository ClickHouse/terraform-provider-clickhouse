package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ServiceProfile is a custom instance profile available to the organization
// in a given region, e.g. a dynamic BYOC profile like "v1-standard-byoc-4".
type ServiceProfile struct {
	Profile  string  `json:"profile"`
	CpuCores float64 `json:"cpuCores"`
	MemoryGi float64 `json:"memoryGi"`
}

// ListServiceProfiles returns the custom instance profiles available to the
// organization in the given region. BYOC profiles are only included when
// byocId is set to the BYOC infrastructure id they are configured for.
func (c *ClientImpl) ListServiceProfiles(ctx context.Context, regionId string, byocId string) ([]ServiceProfile, error) {
	req, err := http.NewRequest(http.MethodGet, c.getOrgPath("/serviceProfiles"), nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("region_id", regionId)
	if byocId != "" {
		q.Set("byoc_id", byocId)
	}
	req.URL.RawQuery = q.Encode()

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp := ResponseWithResult[[]ServiceProfile]{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal service profiles list: %w", err)
	}
	return resp.Result, nil
}
