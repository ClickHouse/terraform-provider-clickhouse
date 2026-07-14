package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// MaxAutoScalingScheduleEntries is the server-side cap on entries per schedule
// (MAX_AUTOSCALING_SCHEDULE_ENTRIES). Exposed so the provider's schema validator
// stays in sync with the server.
const MaxAutoScalingScheduleEntries = 10

// AutoScalingScheduleEntry mirrors the OpenAPI public AutoScalingScheduleEntryV1
// shape: a single weekly recurring scaling window.
type AutoScalingScheduleEntry struct {
	// ID is server-generated; empty when sent in a POST request.
	ID                 string `json:"id,omitempty"`
	Name               string `json:"name"`
	Weekdays           []int  `json:"weekdays"`
	StartHourUtc       int    `json:"startHourUtc"`
	EndHourUtc         int    `json:"endHourUtc"`
	MinReplicaMemoryGb *int   `json:"minReplicaMemoryGb,omitempty"`
	MaxReplicaMemoryGb *int   `json:"maxReplicaMemoryGb,omitempty"`
	MinReplicas        *int   `json:"minReplicas,omitempty"`
	MaxReplicas        *int   `json:"maxReplicas,omitempty"`
	IdleScaling        *bool  `json:"idleScaling,omitempty"`
	IdleTimeoutMinutes *int   `json:"idleTimeoutMinutes,omitempty"`
	// IsActiveNow is server-computed and only present in GET responses.
	IsActiveNow bool `json:"isActiveNow,omitempty"`
}

// AutoScalingScheduleBaseConfig is the fallback configuration applied when no
// entry is currently active. Returned in GET responses and accepted in updates.
type AutoScalingScheduleBaseConfig struct {
	MinReplicaMemoryGb *int  `json:"minReplicaMemoryGb,omitempty"`
	MaxReplicaMemoryGb *int  `json:"maxReplicaMemoryGb,omitempty"`
	MinReplicas        *int  `json:"minReplicas,omitempty"`
	MaxReplicas        *int  `json:"maxReplicas,omitempty"`
	IdleScaling        *bool `json:"idleScaling,omitempty"`
	IdleTimeoutMinutes *int  `json:"idleTimeoutMinutes,omitempty"`
}

// AutoScalingSchedule is the response payload returned by GET and POST.
type AutoScalingSchedule struct {
	Entries       []AutoScalingScheduleEntry     `json:"entries"`
	BaseConfig    *AutoScalingScheduleBaseConfig `json:"baseConfig,omitempty"`
	ActiveEntryID string                         `json:"activeEntryId,omitempty"`
}

// AutoScalingScheduleUpdate is the POST request body. It replaces the full
// schedule for the service.
type AutoScalingScheduleUpdate struct {
	Entries []AutoScalingScheduleEntry `json:"entries"`
}

func (c *ClientImpl) GetScheduledScaling(ctx context.Context, serviceId string) (*AutoScalingSchedule, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/scalingSchedule"), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[AutoScalingSchedule]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Result, nil
}

func (c *ClientImpl) UpdateScheduledScaling(ctx context.Context, serviceId string, s AutoScalingScheduleUpdate) (*AutoScalingSchedule, error) {
	rb, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.getServicePath(serviceId, "/scalingSchedule"), bytes.NewReader(rb))
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	response := ResponseWithResult[AutoScalingSchedule]{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return &response.Result, nil
}

func (c *ClientImpl) DeleteScheduledScaling(ctx context.Context, serviceId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getServicePath(serviceId, "/scalingSchedule"), nil)
	if err != nil {
		return err
	}

	_, err = c.doRequest(ctx, req)
	return err
}
