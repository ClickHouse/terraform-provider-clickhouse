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
	ClickPipeProvisioningState  = "Provisioning"
	ClickPipeRunningState       = "Running"
	ClickPipeStoppedState       = "Stopped"
	ClickPipeFailedState        = "Failed"
	ClickPipeCompletedState     = "Completed"
	ClickPipeInternalErrorState = "InternalError"
)

const (
	ClickPipeStateStart = "start"
	ClickPipeStateStop  = "stop"
)

const (
	ClickPipeJSONEachRowFormat   = "JSONEachRow"
	ClickPipeAvroFormat          = "Avro"
	ClickPipeAvroConfluentFormat = "AvroConfluent"
	ClickPipeCSVFormat           = "CSV"
	ClickPipeCSVWithNamesFormat  = "CSVWithNames"
	ClickPipeParquetFormat       = "Parquet"
)

var ClickPipeKafkaFormats = []string{
	ClickPipeJSONEachRowFormat,
	ClickPipeAvroFormat,
	ClickPipeAvroConfluentFormat,
}

const (
	ClickPipeAuthenticationIAMRole = "IAM_ROLE"
	ClickPipeAuthenticationIAMUser = "IAM_USER"

	ClickPipeKafkaAuthenticationPlain       = "PLAIN"
	ClickPipeKafkaAuthenticationScramSha256 = "SCRAM-SHA-256"
	ClickPipeKafkaAuthenticationScramSha512 = "SCRAM-SHA-512"
)

var ClickPipeKafkaAuthenticationMethods = []string{
	ClickPipeKafkaAuthenticationPlain,
	ClickPipeKafkaAuthenticationScramSha256,
	ClickPipeKafkaAuthenticationScramSha512,
	ClickPipeAuthenticationIAMRole,
	ClickPipeAuthenticationIAMUser,
}

const (
	ClickPipeKafkaSourceType              = "kafka"
	ClickPipeKafkaRedpandaSourceType      = "redpanda"
	ClickPipeKafkaConfluentSourceType     = "confluent"
	ClickPipeKafkaMSKSourceType           = "msk"
	ClickPipeKafkaWarpStreamSourceType    = "warpstream"
	ClickPipeKafkaAzureEventHubSourceType = "azureeventhub"
)

var ClickPipeKafkaSourceTypes = []string{
	ClickPipeKafkaSourceType,
	ClickPipeKafkaRedpandaSourceType,
	ClickPipeKafkaConfluentSourceType,
	ClickPipeKafkaMSKSourceType,
	ClickPipeKafkaWarpStreamSourceType,
	ClickPipeKafkaAzureEventHubSourceType,
}

var ClickPipeObjectStorageAuthenticationMethods = []string{
	ClickPipeAuthenticationIAMRole,
	ClickPipeAuthenticationIAMUser,
}

var ClickPipeObjectStorageFormats = []string{
	ClickPipeJSONEachRowFormat,
	ClickPipeCSVFormat,
	ClickPipeCSVWithNamesFormat,
	ClickPipeParquetFormat,
}

const (
	ClickPipeObjectStorageS3Type  = "s3"
	ClickPipeObjectStorageGCSType = "gcs"
)

var ClickPipeObjectStorageTypes = []string{
	ClickPipeObjectStorageS3Type,
	ClickPipeObjectStorageGCSType,
}

const (
	ClickPipeObjectStorageCompressionAuto   = "auto"
	ClickPipeObjectStorageCompressionGZIP   = "gzip"
	ClickPipeObjectStorageCompressionBrotli = "brotli"
	ClickPipeObjectStorageCompressionBr     = "br"
	ClickPipeObjectStorageCompressionXZ     = "xz"
	ClickPipeObjectStorageCompressionLZMA   = "LZMA"
	ClickPipeObjectStorageCompressionZstd   = "zstd"
)

var ClickPipeObjectStorageCompressions = []string{
	ClickPipeObjectStorageCompressionAuto,
	ClickPipeObjectStorageCompressionGZIP,
	ClickPipeObjectStorageCompressionBrotli,
	ClickPipeObjectStorageCompressionBr,
	ClickPipeObjectStorageCompressionXZ,
	ClickPipeObjectStorageCompressionLZMA,
	ClickPipeObjectStorageCompressionZstd,
}

const (
	ClickPipeKafkaOffsetFromBeginningStrategy = "from_beginning"
	ClickPipeKafkaOffsetFromLatestStrategy    = "from_latest"
	ClickPipeKafkaOffsetFromTimestampStrategy = "from_timestamp"
)

var ClickPipeKafkaOffsetStrategies = []string{
	ClickPipeKafkaOffsetFromBeginningStrategy,
	ClickPipeKafkaOffsetFromLatestStrategy,
	ClickPipeKafkaOffsetFromTimestampStrategy,
}

type ClickPipeScalingRequest struct {
	Replicas    *int64 `json:"replicas,omitempty"`
	Concurrency *int64 `json:"concurrency,omitempty"`
}

type ClickPipeStateRequest struct {
	Command string `json:"command"`
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

func (c *ClientImpl) UpdateClickPipe(ctx context.Context, serviceId string, clickPipeId string, request ClickPipeUpdate) (*ClickPipe, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(request); err != nil {
		return nil, fmt.Errorf("failed to encode ClickPipe: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, c.getClickPipePath(serviceId, clickPipeId, ""), &payload)
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

func (c *ClientImpl) waitForClickPipe(ctx context.Context, serviceId string, clickPipeId string, stateChecker func(*ClickPipe) bool, maxWaitSeconds uint64) (clickPipe *ClickPipe, err error) {
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

	err = backoff.Retry(checkState, backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), maxWaitSeconds/5))
	return
}

func (c *ClientImpl) WaitForClickPipeState(ctx context.Context, serviceId string, clickPipeId string, checker func(string) bool, maxWaitSeconds uint64) (clickPipe *ClickPipe, err error) {
	return c.waitForClickPipe(ctx, serviceId, clickPipeId, func(cp *ClickPipe) bool {
		return checker(cp.State)
	}, maxWaitSeconds)
}

func (c *ClientImpl) ScalingClickPipe(ctx context.Context, serviceId string, clickPipeId string, request ClickPipeScaling) (*ClickPipe, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(request); err != nil {
		return nil, fmt.Errorf("failed to encode ClickPipe: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, c.getClickPipePath(serviceId, clickPipeId, "/scaling"), &payload)
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

func (c *ClientImpl) ChangeClickPipeState(ctx context.Context, serviceId string, clickPipeId string, command string) (*ClickPipe, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(ClickPipeStateRequest{
		Command: command,
	}); err != nil {
		return nil, fmt.Errorf("failed to encode ClickPipe: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, c.getClickPipePath(serviceId, clickPipeId, "/state"), &payload)
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

func (c *ClientImpl) DeleteClickPipe(ctx context.Context, serviceId string, clickPipeId string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getClickPipePath(serviceId, clickPipeId, ""), nil)
	if err != nil {
		return err
	}
	_, err = c.doRequest(ctx, req)
	return err
}
