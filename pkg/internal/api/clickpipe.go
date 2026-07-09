package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	ClickPipeProvisioningState  = "Provisioning"
	ClickPipeDegradedState      = "Degraded"
	ClickPipeRunningState       = "Running"
	ClickPipeStoppingState      = "Stopping"
	ClickPipeStoppedState       = "Stopped"
	ClickPipePausingState       = "Pausing"
	ClickPipePausedState        = "Paused"
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
	ClickPipeProtobufFormat      = "Protobuf"
	ClickPipeCSVFormat           = "CSV"
	ClickPipeCSVWithNamesFormat  = "CSVWithNames"
	ClickPipeParquetFormat       = "Parquet"
)

var ClickPipeStreamingFormats = []string{
	ClickPipeJSONEachRowFormat,
	ClickPipeAvroFormat,
	ClickPipeAvroConfluentFormat,
}

var ClickPipeKafkaFormats = []string{
	ClickPipeJSONEachRowFormat,
	ClickPipeAvroFormat,
	ClickPipeAvroConfluentFormat,
	ClickPipeProtobufFormat,
}

var ClickPipeKinesisFormats = ClickPipeStreamingFormats

const (
	ClickPipeAuthenticationIAMRole          = "IAM_ROLE"
	ClickPipeAuthenticationIAMUser          = "IAM_USER"
	ClickPipeAuthenticationConnectionString = "CONNECTION_STRING"
	ClickPipeAuthenticationServiceAccount   = "SERVICE_ACCOUNT"

	ClickPipeKafkaAuthenticationPlain       = "PLAIN"
	ClickPipeKafkaAuthenticationScramSha256 = "SCRAM-SHA-256"
	ClickPipeKafkaAuthenticationScramSha512 = "SCRAM-SHA-512"
	ClickPipeKafkaAuthenticationMutualTLS   = "MUTUAL_TLS"
)

var ClickPipeKafkaAuthenticationMethods = []string{
	ClickPipeKafkaAuthenticationPlain,
	ClickPipeKafkaAuthenticationScramSha256,
	ClickPipeKafkaAuthenticationScramSha512,
	ClickPipeAuthenticationIAMRole,
	ClickPipeAuthenticationIAMUser,
	ClickPipeKafkaAuthenticationMutualTLS,
}

const (
	ClickPipeKafkaSourceType              = "kafka"
	ClickPipeKafkaRedpandaSourceType      = "redpanda"
	ClickPipeKafkaConfluentSourceType     = "confluent"
	ClickPipeKafkaMSKSourceType           = "msk"
	ClickPipeKafkaWarpStreamSourceType    = "warpstream"
	ClickPipeKafkaAzureEventHubSourceType = "azureeventhub"
	ClickPipeKafkaGCMKSourceType          = "gcmk"
)

var ClickPipeKafkaSourceTypes = []string{
	ClickPipeKafkaSourceType,
	ClickPipeKafkaRedpandaSourceType,
	ClickPipeKafkaConfluentSourceType,
	ClickPipeKafkaMSKSourceType,
	ClickPipeKafkaWarpStreamSourceType,
	ClickPipeKafkaAzureEventHubSourceType,
	ClickPipeKafkaGCMKSourceType,
}

const (
	ClickPipePostgresSourceType            = "postgres"
	ClickPipePostgresSupabaseSourceType    = "supabase"
	ClickPipePostgresNeonSourceType        = "neon"
	ClickPipePostgresAlloyDBSourceType     = "alloydb"
	ClickPipePostgresPlanetScaleSourceType = "planetscale"
	ClickPipePostgresRDSSourceType         = "rdspostgres"
	ClickPipePostgresAuroraSourceType      = "aurorapostgres"
	ClickPipePostgresCloudSQLSourceType    = "cloudsqlpostgres"
	ClickPipePostgresAzureSourceType       = "azurepostgres"
	ClickPipePostgresCrunchySourceType     = "crunchybridge"
	ClickPipePostgresTigerDataSourceType   = "tigerdata"
)

var ClickPipePostgresSourceTypes = []string{
	ClickPipePostgresSourceType,
	ClickPipePostgresSupabaseSourceType,
	ClickPipePostgresNeonSourceType,
	ClickPipePostgresAlloyDBSourceType,
	ClickPipePostgresPlanetScaleSourceType,
	ClickPipePostgresRDSSourceType,
	ClickPipePostgresAuroraSourceType,
	ClickPipePostgresCloudSQLSourceType,
	ClickPipePostgresAzureSourceType,
	ClickPipePostgresCrunchySourceType,
	ClickPipePostgresTigerDataSourceType,
}

// ClickPipePostgresAcceptedSourceTypes lists the source types accepted as input. The
// provider-flavored variants above are deprecated: users must collapse them to the base
// `postgres` type. ClickPipePostgresSourceTypes is retained to normalize legacy state.
var ClickPipePostgresAcceptedSourceTypes = []string{
	ClickPipePostgresSourceType,
}

// CollapsePostgresSourceType maps any legacy provider-flavored Postgres type to its base type.
func CollapsePostgresSourceType(sourceType string) string {
	if slices.Contains(ClickPipePostgresSourceTypes, sourceType) {
		return ClickPipePostgresSourceType
	}
	return sourceType
}

var ClickPipeObjectStorageAuthenticationMethods = []string{
	ClickPipeAuthenticationIAMRole,
	ClickPipeAuthenticationIAMUser,
	ClickPipeAuthenticationConnectionString,
	ClickPipeAuthenticationServiceAccount,
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

const (
	ClickPipePubSubSourceType = "pubsub"
)

var ClickPipePubSubFormats = []string{
	ClickPipeJSONEachRowFormat,
	ClickPipeAvroFormat,
	ClickPipeProtobufFormat,
}

const (
	ClickPipePubSubSeekTypeLatest    = "latest"
	ClickPipePubSubSeekTypeEarliest  = "earliest"
	ClickPipePubSubSeekTypeTimestamp = "timestamp"
)

var ClickPipePubSubSeekTypes = []string{
	ClickPipePubSubSeekTypeLatest,
	ClickPipePubSubSeekTypeEarliest,
	ClickPipePubSubSeekTypeTimestamp,
}

var ClickPipePubSubAuthenticationMethods = []string{
	ClickPipeAuthenticationServiceAccount,
}

var ClickPipeObjectStorageFormats = []string{
	ClickPipeJSONEachRowFormat,
	ClickPipeCSVFormat,
	ClickPipeCSVWithNamesFormat,
	ClickPipeParquetFormat,
	ClickPipeAvroFormat,
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
	ClickPipeObjectStorageCompressionNone   = "none"
	ClickPipeObjectStorageCompressionAuto   = "auto"
	ClickPipeObjectStorageCompressionGZIP   = "gzip"
	ClickPipeObjectStorageCompressionBrotli = "brotli"
	ClickPipeObjectStorageCompressionBr     = "br"
	ClickPipeObjectStorageCompressionXZ     = "xz"
	ClickPipeObjectStorageCompressionLZMA   = "LZMA"
	ClickPipeObjectStorageCompressionZstd   = "zstd"
)

var ClickPipeObjectStorageCompressions = []string{
	ClickPipeObjectStorageCompressionNone,
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
	ClickPipeMySQLReplicationMechanismGTID    = "GTID"
	ClickPipeMySQLReplicationMechanismFilePos = "FILE_POS"
)

var ClickPipeMySQLReplicationModes = []string{
	ClickPipeReplicationModeCDC,
	ClickPipeReplicationModeSnapshot,
	ClickPipeReplicationModeCDCOnly,
}

var ClickPipeMySQLReplicationMechanisms = []string{
	ClickPipeMySQLReplicationMechanismGTID,
	ClickPipeMySQLReplicationMechanismFilePos,
}

var ClickPipeMySQLTableEngines = []string{
	ClickPipeTableEngineMergeTree,
	ClickPipeTableEngineReplacingMergeTree,
	ClickPipeTableEngineNull,
}

var ClickPipeMySQLAuthenticationMethods = []string{
	"basic",
	"IAM_ROLE",
}

const (
	ClickPipeMySQLSourceTypeMySQL            = "mysql"
	ClickPipeMySQLSourceTypeRDSMySQL         = "rdsmysql"
	ClickPipeMySQLSourceTypeAuroraMySQL      = "auroramysql"
	ClickPipeMySQLSourceTypePlanetScaleVites = "planetscalevitess"
	ClickPipeMySQLSourceTypeMariaDB          = "mariadb"
	ClickPipeMySQLSourceTypeRDSMariaDB       = "rdsmariadb"
)

var ClickPipeMySQLSourceTypes = []string{
	ClickPipeMySQLSourceTypeMySQL,
	ClickPipeMySQLSourceTypeRDSMySQL,
	ClickPipeMySQLSourceTypeAuroraMySQL,
	ClickPipeMySQLSourceTypePlanetScaleVites,
	ClickPipeMySQLSourceTypeMariaDB,
	ClickPipeMySQLSourceTypeRDSMariaDB,
}

// ClickPipeMySQLMariaDBSourceTypes lists the MariaDB-flavored MySQL source types.
var ClickPipeMySQLMariaDBSourceTypes = []string{
	ClickPipeMySQLSourceTypeMariaDB,
	ClickPipeMySQLSourceTypeRDSMariaDB,
}

func IsClickPipeMySQLMariaDBSourceType(sourceType string) bool {
	return slices.Contains(ClickPipeMySQLMariaDBSourceTypes, sourceType)
}

// ClickPipeMySQLAcceptedSourceTypes lists the source types accepted as input. Provider
// prefixes are collapsed to a base engine type, but the MariaDB engine stays distinct
// from MySQL. ClickPipeMySQLSourceTypes is retained to normalize legacy state.
var ClickPipeMySQLAcceptedSourceTypes = []string{
	ClickPipeMySQLSourceTypeMySQL,
	ClickPipeMySQLSourceTypeMariaDB,
}

// CollapseMySQLSourceType maps any legacy provider-flavored MySQL type to its base engine
// type, preserving the MySQL/MariaDB distinction.
func CollapseMySQLSourceType(sourceType string) string {
	if IsClickPipeMySQLMariaDBSourceType(sourceType) {
		return ClickPipeMySQLSourceTypeMariaDB
	}
	if slices.Contains(ClickPipeMySQLSourceTypes, sourceType) {
		return ClickPipeMySQLSourceTypeMySQL
	}
	return sourceType
}

// MongoDB constants
const (
	ClickPipeMongoDBReadPreferencePrimary            = "primary"
	ClickPipeMongoDBReadPreferencePrimaryPreferred   = "primaryPreferred"
	ClickPipeMongoDBReadPreferenceSecondary          = "secondary"
	ClickPipeMongoDBReadPreferenceSecondaryPreferred = "secondaryPreferred"
	ClickPipeMongoDBReadPreferenceNearest            = "nearest"
)

var ClickPipeMongoDBReadPreferences = []string{
	ClickPipeMongoDBReadPreferencePrimary,
	ClickPipeMongoDBReadPreferencePrimaryPreferred,
	ClickPipeMongoDBReadPreferenceSecondary,
	ClickPipeMongoDBReadPreferenceSecondaryPreferred,
	ClickPipeMongoDBReadPreferenceNearest,
}

var ClickPipeMongoDBReplicationModes = []string{
	ClickPipeReplicationModeCDC,
	ClickPipeReplicationModeSnapshot,
	ClickPipeReplicationModeCDCOnly,
}

var ClickPipeMongoDBTableEngines = []string{
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

// serviceWakeMaxWaitSeconds caps how long doClickPipeRequest waits for an idle
// service to reach the running state after sending the awake command.
const serviceWakeMaxWaitSeconds = 10 * 60

// doClickPipeRequest sends a ClickPipes API request through doRequest.
// The ClickPipes API rejects requests with 424 FAILED_DEPENDENCY while the
// target service is idle; in that case the service is woken up, we wait for it
// to reach the running state and the original request is retried once.
// https://github.com/ClickHouse/terraform-provider-clickhouse/issues/376
func (c *ClientImpl) doClickPipeRequest(ctx context.Context, serviceId string, req *http.Request) ([]byte, error) {
	body, err := c.doRequest(ctx, req)
	if !IsServiceIdle(err) {
		return body, err
	}

	tflog.Info(ctx, "ClickHouse service is idle; waking it up before retrying the ClickPipes request", map[string]any{
		"serviceId": serviceId,
	})

	if wakeErr := c.wakeService(ctx, serviceId); wakeErr != nil {
		return nil, fmt.Errorf("service %s is idle and waking it up failed: %w", serviceId, wakeErr)
	}

	if waitErr := c.waitForServiceRunning(ctx, serviceId, serviceWakeMaxWaitSeconds); waitErr != nil {
		return nil, fmt.Errorf("service %s is idle and did not reach the running state after waking it up: %w", serviceId, waitErr)
	}

	return c.doRequest(ctx, req)
}

func (c *ClientImpl) GetClickPipe(ctx context.Context, serviceId string, clickPipeId string) (*ClickPipe, error) {
	req, err := http.NewRequest(http.MethodGet, c.getClickPipePath(serviceId, clickPipeId, ""), nil)
	if err != nil {
		return nil, err
	}
	body, err := c.doClickPipeRequest(ctx, serviceId, req)
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

	body, err := c.doClickPipeRequest(ctx, serviceId, req)
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

	body, err := c.doClickPipeRequest(ctx, serviceId, req)
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

	body, err := c.doClickPipeRequest(ctx, serviceId, req)
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
	body, err := c.doClickPipeRequest(ctx, serviceId, req)
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
	_, err = c.doClickPipeRequest(ctx, serviceId, req)
	return err
}

func (c *ClientImpl) GetClickPipeSettings(ctx context.Context, serviceId string, clickPipeId string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, c.getClickPipePath(serviceId, clickPipeId, "/settings"), nil)
	if err != nil {
		return nil, err
	}
	body, err := c.doClickPipeRequest(ctx, serviceId, req)
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

	body, err := c.doClickPipeRequest(ctx, serviceId, req)
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
	body, err := c.doClickPipeRequest(ctx, serviceId, req)
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

	body, err := c.doClickPipeRequest(ctx, serviceId, req)
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
