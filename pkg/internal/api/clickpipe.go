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
	ClickPipeDegradedState      = "Degraded"
	ClickPipeRunningState       = "Running"
	ClickPipeStoppingState      = "Stopping"
	ClickPipeStoppedState       = "Stopped"
	ClickPipeFailedState        = "Failed"
	ClickPipeCompletedState     = "Completed"
	ClickPipeSnapShotState      = "Snapshot"
	ClickPipeInternalErrorState = "InternalError"
)

const (
	ClickPipeStateStart  = "start"
	ClickPipeStateStop   = "stop"
	ClickPipeStateResync = "resync"
)

const (
	ClickPipeJSONEachRowFormat   = "JSONEachRow"
	ClickPipeAvroFormat          = "Avro"
	ClickPipeAvroConfluentFormat = "AvroConfluent"
	ClickPipeCSVFormat           = "CSV"
	ClickPipeCSVWithNamesFormat  = "CSVWithNames"
	ClickPipeParquetFormat       = "Parquet"
)

var ClickPipeStreamingFormats = []string{
	ClickPipeJSONEachRowFormat,
	ClickPipeAvroFormat,
	ClickPipeAvroConfluentFormat,
}

var (
	ClickPipeKafkaFormats   = ClickPipeStreamingFormats
	ClickPipeKinesisFormats = ClickPipeKafkaFormats
)

const (
	ClickPipeAuthenticationIAMRole          = "IAM_ROLE"
	ClickPipeAuthenticationIAMUser          = "IAM_USER"
	ClickPipeAuthenticationConnectionString = "CONNECTION_STRING"

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
	ClickPipeAuthenticationConnectionString,
}

var ClickPipeKinesisAuthenticationMethods = []string{
	ClickPipeAuthenticationIAMRole,
	ClickPipeAuthenticationIAMUser,
}

const (
	ClickPipeKinesisTrimHorizonIteratorType = "TRIM_HORIZON"
	ClickPipeKinesisLatestIteratorType      = "LATEST"
	ClickPipeKinesisAtTimestampIteratorType = "AT_TIMESTAMP"
)

var ClickPipeKinesisIteratorTypes = []string{
	ClickPipeKinesisTrimHorizonIteratorType,
	ClickPipeKinesisLatestIteratorType,
	ClickPipeKinesisAtTimestampIteratorType,
}

var ClickPipeObjectStorageFormats = []string{
	ClickPipeJSONEachRowFormat,
	ClickPipeCSVFormat,
	ClickPipeCSVWithNamesFormat,
	ClickPipeParquetFormat,
}

const (
	ClickPipeObjectStorageS3Type        = "s3"
	ClickPipeObjectStorageGCSType       = "gcs"
	ClickPipeObjectStorageAzureBlobType = "azureblobstorage"
)

var ClickPipeObjectStorageTypes = []string{
	ClickPipeObjectStorageS3Type,
	ClickPipeObjectStorageGCSType,
	ClickPipeObjectStorageAzureBlobType,
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
	// Postgres replication modes
	ClickPipeReplicationModeCDC      = "cdc"
	ClickPipeReplicationModeSnapshot = "snapshot"
	ClickPipeReplicationModeCDCOnly  = "cdc_only"
)

const (
	ClickPipeTableEngineMergeTree          = "MergeTree"
	ClickPipeTableEngineReplacingMergeTree = "ReplacingMergeTree"
	ClickPipeTableEngineNull               = "Null"
)

var ClickPipePostgresReplicationModes = []string{
	ClickPipeReplicationModeCDC,
	ClickPipeReplicationModeSnapshot,
	ClickPipeReplicationModeCDCOnly,
}

var ClickPipeBigQueryReplicationModes = []string{
	ClickPipeReplicationModeSnapshot,
}

var ClickPipePostgresTableEngines = []string{
	ClickPipeTableEngineMergeTree,
	ClickPipeTableEngineReplacingMergeTree,
	ClickPipeTableEngineNull,
}

var ClickPipeBigQueryTableEngines = []string{
	ClickPipeTableEngineMergeTree,
	ClickPipeTableEngineReplacingMergeTree,
	ClickPipeTableEngineNull,
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
	Replicas             *int64   `json:"replicas,omitempty"`
	ReplicaCpuMillicores *int64   `json:"replicaCpuMillicores,omitempty"`
	ReplicaMemoryGb      *float64 `json:"replicaMemoryGb,omitempty"`
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

func (c *ClientImpl) waitForClickPipe(ctx context.Context, serviceId string, clickPipeId string, stateChecker func(*ClickPipe) bool, maxElapsedTime time.Duration) (clickPipe *ClickPipe, err error) {
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

	if maxElapsedTime < 5*time.Second {
		maxElapsedTime = 5
	}

	err = backoff.Retry(checkState, backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(maxElapsedTime), backoff.WithMaxInterval(maxElapsedTime/5)))
	return
}

func (c *ClientImpl) WaitForClickPipeState(ctx context.Context, serviceId string, clickPipeId string, checker func(string) bool, maxWait time.Duration) (clickPipe *ClickPipe, err error) {
	return c.waitForClickPipe(ctx, serviceId, clickPipeId, func(cp *ClickPipe) bool {
		return checker(cp.State)
	}, maxWait)
}

func (c *ClientImpl) ScalingClickPipe(ctx context.Context, serviceId string, clickPipeId string, request ClickPipeScalingRequest) (*ClickPipe, error) {
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

func (c *ClientImpl) GetClickPipeSettings(ctx context.Context, serviceId string, clickPipeId string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, c.getClickPipePath(serviceId, clickPipeId, "/settings"), nil)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	settingsResponse := ResponseWithResult[map[string]any]{}
	if err := json.Unmarshal(body, &settingsResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ClickPipe settings: %w", err)
	}

	return settingsResponse.Result, nil
}

func (c *ClientImpl) UpdateClickPipeSettings(ctx context.Context, serviceId string, clickPipeId string, settings map[string]any) (map[string]any, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(settings); err != nil {
		return nil, fmt.Errorf("failed to encode ClickPipe settings: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, c.getClickPipePath(serviceId, clickPipeId, "/settings"), &payload)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	settingsResponse := ResponseWithResult[map[string]any]{}
	if err := json.Unmarshal(body, &settingsResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ClickPipe settings: %w", err)
	}

	return settingsResponse.Result, nil
}

type ClickPipeCdcScaling struct {
	ReplicaCpuMillicores int64   `json:"replicaCpuMillicores"`
	ReplicaMemoryGb      float64 `json:"replicaMemoryGb"`
}

type ClickPipeCdcScalingRequest struct {
	ReplicaCpuMillicores int64   `json:"replicaCpuMillicores"`
	ReplicaMemoryGb      float64 `json:"replicaMemoryGb"`
}

func (c *ClientImpl) GetClickPipeCdcScaling(ctx context.Context, serviceId string) (*ClickPipeCdcScaling, error) {
	req, err := http.NewRequest(http.MethodGet, c.getServicePath(serviceId, "/clickpipesCdcScaling"), nil)
	if err != nil {
		return nil, err
	}
	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	scalingResponse := ResponseWithResult[ClickPipeCdcScaling]{}
	if err := json.Unmarshal(body, &scalingResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CDC scaling: %w", err)
	}

	return &scalingResponse.Result, nil
}

func (c *ClientImpl) UpdateClickPipeCdcScaling(ctx context.Context, serviceId string, request ClickPipeCdcScalingRequest) (*ClickPipeCdcScaling, error) {
	var payload bytes.Buffer
	if err := json.NewEncoder(&payload).Encode(request); err != nil {
		return nil, fmt.Errorf("failed to encode CDC scaling: %w", err)
	}

	req, err := http.NewRequest(http.MethodPatch, c.getServicePath(serviceId, "/clickpipesCdcScaling"), &payload)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	scalingResponse := ResponseWithResult[ClickPipeCdcScaling]{}
	if err := json.Unmarshal(body, &scalingResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CDC scaling: %w", err)
	}

	return &scalingResponse.Result, nil
}

func (c *ClientImpl) WaitForClickPipeCdcScaling(ctx context.Context, serviceId string, expectedCpuMillicores int64, expectedMemoryGb float64, maxElapsedTime time.Duration) (scaling *ClickPipeCdcScaling, err error) {
	checkScaling := func() error {
		scaling, err = c.GetClickPipeCdcScaling(ctx, serviceId)
		if err != nil {
			return err
		}

		// Check if the scaling values match the expected values
		if scaling.ReplicaCpuMillicores == expectedCpuMillicores && scaling.ReplicaMemoryGb == expectedMemoryGb {
			return nil
		}

		return fmt.Errorf("CDC scaling not yet applied: current cpu=%d (expected %d), memory=%.1f (expected %.1f)",
			scaling.ReplicaCpuMillicores, expectedCpuMillicores, scaling.ReplicaMemoryGb, expectedMemoryGb)
	}

	if maxElapsedTime < 5*time.Second {
		maxElapsedTime = 5 * time.Second
	}

	err = backoff.Retry(checkScaling, backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(maxElapsedTime), backoff.WithMaxInterval(maxElapsedTime/5)))
	return
}
