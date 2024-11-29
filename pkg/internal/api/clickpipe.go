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

const (
	ClickPipeRunningState = "Running"
	ClickPipeStoppedState = "Stopped"
)

const (
	ClickPipeJSONEachRowFormat = "JSONEachRow"
	ClickPipesAvroFormat       = "Avro"
)

const (
	ClickPipeKafkaAuthenticationPlain       = "PLAIN"
	ClickPipeKafkaAuthenticationScramSha256 = "SCRAM-SHA-256"
	ClickPipeKafkaAuthenticationScramSha512 = "SCRAM-SHA-512"
)

const (
	ClickPipeKafkaSourceType     = "kafka"
	ClickPipeConfluentSourceType = "confluent"
	ClickPipeMSKSourceType       = "msk"
)

type ClickPipeScaleRequest struct {
	Desired int64 `json:"desired"`
}

func (c *ClientImpl) getClickPipePath(serviceId, clickPipeId, path string) string {
	return c.getServicePath(serviceId, fmt.Sprintf("/clickpipes/%s%s", clickPipeId, path))
}

func (c *ClientImpl) GetClickPipe(ctx context.Context, serviceId string, clickPipeId string) (*ClickPipe, error) {
	req, err := http.NewRequest(http.MethodGet, c.getClickPipePath(serviceId, clickPipeId, ""), nil)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	clickPipeResponse := ResponseWithResult[ClickPipe]{}
	err = json.Unmarshal(body, &clickPipeResponse)
	if err != nil {
		return nil, err
	}

	return &clickPipeResponse.Result, nil
}

func (c *ClientImpl) CreateClickPipe(ctx context.Context, serviceId string, clickPipe ClickPipe) (*ClickPipe, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(clickPipe); err != nil {
		return nil, fmt.Errorf("failed to encode ClickPipe: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.getClickPipePath(serviceId, "", ""), &payload)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	clickPipeResponse := ResponseWithResult[ClickPipe]{}
	if err := json.Unmarshal(body, &clickPipeResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ClickPipe: %w", err)
	}

	return &clickPipeResponse.Result, nil
}

func (c *ClientImpl) waitForClickPipe(ctx context.Context, serviceId string, clickPipeId string, stateChecker func(*ClickPipe) bool, maxWaitSeconds int) (clickPipe *ClickPipe, err error) {
	checkState := func() error {
		clickPipe, err = c.GetClickPipe(ctx, serviceId, clickPipeId)
		if err != nil {
			return err
		}

		if stateChecker(clickPipe) {
			return nil
		}

		return fmt.Errorf("ClickPipe %s is in state %s", clickPipeId, clickPipe.State)
	}

	if maxWaitSeconds < 5 {
		maxWaitSeconds = 5
	}

	err = backoff.Retry(checkState, backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), uint64(maxWaitSeconds/5)))
	return
}

func (c *ClientImpl) WaitForClickPipeState(ctx context.Context, serviceId string, clickPipeId string, checker func(string) bool, maxWaitSeconds int) (clickPipe *ClickPipe, err error) {
	return c.waitForClickPipe(ctx, serviceId, clickPipeId, func(cp *ClickPipe) bool {
		return checker(cp.State)
	}, maxWaitSeconds)
}

func (c *ClientImpl) ScaleClickPipe(ctx context.Context, serviceId string, clickPipeId string, desiredReplicas int64) (*ClickPipe, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(ClickPipeScaleRequest{
		Desired: desiredReplicas,
	}); err != nil {
		return nil, fmt.Errorf("failed to encode ClickPipe: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.getClickPipePath(serviceId, clickPipeId, "/scale"), &payload)
	if err != nil {
		return nil, err
	}
	_, err = c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	return c.waitForClickPipe(ctx, serviceId, clickPipeId, func(clickPipe *ClickPipe) bool {
		return clickPipe.Replicas != nil && clickPipe.Replicas.Desired == desiredReplicas
	}, 300)
}

func (c *ClientImpl) PauseClickPipe(ctx context.Context, serviceId string, clickPipeId string) (*ClickPipe, error) {
	req, err := http.NewRequest(http.MethodPost, c.getClickPipePath(serviceId, clickPipeId, "/pause"), nil)
	if err != nil {
		return nil, err
	}
	_, err = c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	return c.WaitForClickPipeState(ctx, serviceId, clickPipeId, func(state string) bool {
		return state == ClickPipeStoppedState
	}, 300)
}

func (c *ClientImpl) ResumeClickPipe(ctx context.Context, serviceId string, clickPipeId string) (*ClickPipe, error) {
	req, err := http.NewRequest(http.MethodPost, c.getClickPipePath(serviceId, clickPipeId, "/resume"), nil)
	if err != nil {
		return nil, err
	}
	_, err = c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	return c.WaitForClickPipeState(ctx, serviceId, clickPipeId, func(state string) bool {
		return state == ClickPipeRunningState
	}, 300)
}

func (c *ClientImpl) DeleteClickPipe(ctx context.Context, serviceId string, clickPipeId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getClickPipePath(serviceId, clickPipeId, ""), nil)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, req)
	return err
}
