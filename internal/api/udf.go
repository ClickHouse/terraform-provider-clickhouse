package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const logFieldServiceID = "serviceId"

const (
	UDFRuntimePython311 = "python3.11"
	UDFRuntimeNative    = "native"

	UDFTypeExecutable     = "executable"
	UDFTypeExecutablePool = "executable_pool"

	UDFSandboxTypeBasic     = "basic"
	UDFSandboxTypeNetEnable = "netenable"

	UDFSandboxVersionV1 = "v1"
	UDFSandboxVersionV2 = "v2"
	UDFSandboxVersionV3 = "v3"

	UDFStatusBuilding = "building"
	UDFStatusError    = "error"
	UDFStatusReady    = "ready"

	UDFAttachmentStatusDeployed       = "deployed"
	UDFAttachmentStatusDeprovisioning = "deprovisioning"
	UDFAttachmentStatusError          = "error"
	UDFAttachmentStatusProvisioning   = "provisioning"
	UDFAttachmentStatusStandby        = "standby"
)

var (
	UDFRuntimes        = []string{UDFRuntimePython311, UDFRuntimeNative}
	UDFTypes           = []string{UDFTypeExecutable, UDFTypeExecutablePool}
	UDFSandboxTypes    = []string{UDFSandboxTypeBasic, UDFSandboxTypeNetEnable}
	UDFSandboxVersions = []string{UDFSandboxVersionV1, UDFSandboxVersionV2, UDFSandboxVersionV3}
)

type UDFArgument struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type UDF struct {
	FunctionName            string        `json:"functionName"`
	Version                 int64         `json:"version"`
	Status                  string        `json:"status"`
	Runtime                 string        `json:"runtime"`
	Type                    string        `json:"type"`
	Arguments               []UDFArgument `json:"arguments"`
	ReturnType              string        `json:"returnType"`
	ReturnName              *string       `json:"returnName"`
	PoolSize                *int64        `json:"poolSize"`
	CommandReadTimeout      int64         `json:"commandReadTimeout"`
	CommandWriteTimeout     int64         `json:"commandWriteTimeout"`
	MaxCommandExecutionTime *int64        `json:"maxCommandExecutionTime"`
	SendChunkHeader         bool          `json:"sendChunkHeader"`
	Format                  string        `json:"format"`
	SandboxType             string        `json:"sandboxType"`
	SandboxVersion          string        `json:"sandboxVersion"`
	Error                   *string       `json:"error"`
	CreatedAt               string        `json:"createdAt"`
	UpdatedAt               string        `json:"updatedAt"`
}

type UDFAttachment struct {
	FunctionName string `json:"functionName"`
	ServiceID    string `json:"serviceId"`
	Status       string `json:"status"`
	Version      int64  `json:"version"`
}

type UDFUploadSession struct {
	UploadID  string `json:"uploadId"`
	UploadURL string `json:"uploadUrl"`
	ExpiresAt string `json:"expiresAt"`
}

type udfPublishOutcomeUnknownError struct {
	err error
}

func (e *udfPublishOutcomeUnknownError) Error() string {
	if message := e.err.Error(); len(message) >= len("status: 000") && strings.HasPrefix(message, "status: ") {
		return fmt.Sprintf("UDF publish may have succeeded, but the provider did not receive a usable response (status: %s)", message[len("status: "):len("status: 000")])
	}
	return "UDF publish may have succeeded, but the provider did not receive a usable response"
}

func (e *udfPublishOutcomeUnknownError) Unwrap() error {
	return e.err
}

func markUDFPublishOutcomeUnknown(err error) error {
	if err == nil {
		return nil
	}
	var unknown *udfPublishOutcomeUnknownError
	if errors.As(err, &unknown) {
		return err
	}
	return &udfPublishOutcomeUnknownError{err: err}
}

func NewUDFPublishOutcomeUnknownError(err error) error {
	return markUDFPublishOutcomeUnknown(err)
}

func IsUDFPublishOutcomeUnknown(err error) bool {
	var unknown *udfPublishOutcomeUnknownError
	return errors.As(err, &unknown)
}

type UDFVersionCreateRequest struct {
	UploadID                string        `json:"uploadId"`
	Runtime                 string        `json:"runtime"`
	Arguments               []UDFArgument `json:"arguments"`
	ReturnType              string        `json:"returnType"`
	ReturnName              *string       `json:"returnName"`
	Type                    string        `json:"type"`
	PoolSize                *int64        `json:"poolSize"`
	CommandReadTimeout      int64         `json:"commandReadTimeout"`
	CommandWriteTimeout     int64         `json:"commandWriteTimeout"`
	MaxCommandExecutionTime *int64        `json:"maxCommandExecutionTime"`
	SendChunkHeader         bool          `json:"sendChunkHeader"`
	Format                  string        `json:"format"`
	SandboxType             string        `json:"sandboxType"`
	SandboxVersion          string        `json:"sandboxVersion"`
}

type UDFCreateRequest struct {
	FunctionName string `json:"functionName"`
	UDFVersionCreateRequest
}

type UDFAttachRequest struct {
	Version *int64 `json:"version,omitempty"`
}

func (c *ClientImpl) udfPath(functionName string) string {
	path := "/udfs"
	if functionName != "" {
		path += "/" + url.PathEscape(functionName)
	}
	return c.getOrgPath(path)
}

func (c *ClientImpl) udfAttachmentPath(functionName, serviceID string) string {
	return c.udfPath(functionName) + "/attachments/" + url.PathEscape(serviceID)
}

func (c *ClientImpl) CreateUDFUploadSession(ctx context.Context) (*UDFUploadSession, error) {
	req, err := http.NewRequest(http.MethodPost, c.getOrgPath("/udfUploads/url"), nil)
	if err != nil {
		return nil, fmt.Errorf("create UDF upload-session request: %w", err)
	}

	// Upload-session create returns 201 Created.
	body, err := c.doRequestWithAcceptedStatus(ctx, req, http.StatusOK, http.StatusCreated)
	if err != nil {
		return nil, err
	}

	var session ResponseWithResult[UDFUploadSession]
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("decode UDF upload session: %w", err)
	}
	if session.Result.UploadID == "" || session.Result.UploadURL == "" {
		return nil, fmt.Errorf("decode UDF upload session: invalid or incomplete response")
	}
	return &session.Result, nil
}

func (c *ClientImpl) UploadUDFArchive(ctx context.Context, uploadURL string, archive []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(archive))
	if err != nil {
		// Do not return the parser error: net/url includes the complete
		// presigned URL in its message for malformed inputs.
		return fmt.Errorf("create UDF archive upload request: invalid upload URL")
	}
	req.Header.Set("Content-Type", "application/zip")
	req.ContentLength = int64(len(archive))

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		// http.Client errors normally include the request URL. Keep this
		// object-storage error concise and safe for Terraform diagnostics.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("upload UDF archive: %w", ctxErr)
		}
		return fmt.Errorf("upload UDF archive: request failed")
	}
	defer resp.Body.Close()

	if _, readErr := io.Copy(io.Discard, resp.Body); readErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("upload UDF archive: %w", ctxErr)
		}
		return fmt.Errorf("upload UDF archive: response read failed")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("status: %d, UDF archive upload failed", resp.StatusCode)
	}
	return nil
}

func (c *ClientImpl) CreateUDF(ctx context.Context, request UDFCreateRequest) (*UDF, error) {
	return c.writeUDF(ctx, c.udfPath(""), request, "create UDF", request.FunctionName)
}

func (c *ClientImpl) GetUDF(ctx context.Context, functionName string) (*UDF, error) {
	req, err := http.NewRequest(http.MethodGet, c.udfPath(functionName), nil)
	if err != nil {
		return nil, fmt.Errorf("create get UDF request: %w", err)
	}
	body, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	var udf ResponseWithResult[UDF]
	if err := json.Unmarshal(body, &udf); err != nil {
		return nil, fmt.Errorf("decode UDF %q: %w", functionName, err)
	}
	if !isUsableUDFResponse(udf.Result) || udf.Result.FunctionName != functionName {
		return nil, fmt.Errorf("decode UDF %q: invalid or incomplete response", functionName)
	}
	return &udf.Result, nil
}

func (c *ClientImpl) CreateUDFVersion(ctx context.Context, functionName string, request UDFVersionCreateRequest) (*UDF, error) {
	return c.writeUDF(ctx, c.udfPath(functionName)+"/versions", request, "create UDF version", functionName)
}

func (c *ClientImpl) writeUDF(ctx context.Context, path string, request any, operation, functionName string) (*UDF, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", operation, err)
	}
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create %s request: %w", operation, err)
	}

	// A UDF publish consumes the upload ID. The API has no idempotency key, so
	// ambiguous failures must not replay this POST.
	responseBody, err := c.doUDFPublishRequest(ctx, req)
	if err != nil {
		if isAmbiguousUDFPublishError(err) {
			return nil, markUDFPublishOutcomeUnknown(err)
		}
		return nil, err
	}

	var udf ResponseWithResult[UDF]
	if err := json.Unmarshal(responseBody, &udf); err != nil {
		return nil, markUDFPublishOutcomeUnknown(fmt.Errorf("decode %s response: %w", operation, err))
	}
	if !isUsableUDFResponse(udf.Result) || udf.Result.FunctionName != functionName {
		return nil, markUDFPublishOutcomeUnknown(fmt.Errorf("decode %s response: invalid or incomplete UDF", operation))
	}
	return &udf.Result, nil
}

// doUDFPublishRequest makes one publish request. Replaying a POST would consume
// an upload ID again, while a lost response is reconciled by the resource.
func (c *ClientImpl) doUDFPublishRequest(ctx context.Context, req *http.Request) ([]byte, error) {
	return c.doRequestWithStatus(ctx, req, false, http.StatusOK, http.StatusCreated)
}

func isAmbiguousUDFPublishError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	if !strings.HasPrefix(message, "status: ") {
		return true
	}
	return strings.HasPrefix(message, "status: 5")
}

func isUsableUDFResponse(udf UDF) bool {
	return udf.FunctionName != "" &&
		udf.Version > 0 &&
		udf.Status != "" &&
		udf.Runtime != "" &&
		udf.Type != "" &&
		udf.ReturnType != "" &&
		udf.CommandReadTimeout > 0 &&
		udf.CommandWriteTimeout > 0 &&
		udf.Format != "" &&
		udf.SandboxType != "" &&
		udf.SandboxVersion != ""
}

func (c *ClientImpl) DeleteUDF(ctx context.Context, functionName string) error {
	req, err := http.NewRequest(http.MethodDelete, c.udfPath(functionName), nil)
	if err != nil {
		return fmt.Errorf("create delete UDF request: %w", err)
	}
	_, err = c.doRequest(ctx, req)
	return err
}

const udfServiceWakeMaxWaitSeconds = 10 * 60

type udfServiceDependencyError struct {
	Code         string `json:"code"`
	ServiceState string `json:"serviceState"`
	CanWake      bool   `json:"canWake"`
}

func isUDFServiceIdle(err error) bool {
	response, ok := parseUDFServiceDependencyError(err)
	return ok && response.Code == "SERVICE_IDLE" && response.ServiceState == StateIdle && response.CanWake
}

func isUDFServiceAwaking(err error) bool {
	response, ok := parseUDFServiceDependencyError(err)
	return ok && response.Code == "SERVICE_NOT_RUNNING" && response.ServiceState == StateAwaking && !response.CanWake
}

func parseUDFServiceDependencyError(err error) (udfServiceDependencyError, bool) {
	var response udfServiceDependencyError
	if err == nil {
		return response, false
	}

	message := err.Error()
	if !strings.HasPrefix(message, "status: 424") {
		return response, false
	}

	const marker = "body: "
	markerIndex := strings.Index(message, marker)
	if markerIndex < 0 {
		return response, false
	}
	if json.Unmarshal([]byte(message[markerIndex+len(marker):]), &response) != nil || response.Code == "" {
		return response, false
	}
	return response, true
}

func (c *ClientImpl) doUDFAttachmentRequest(ctx context.Context, serviceID string, req *http.Request) ([]byte, error) {
	body, err := c.doRequest(ctx, req)
	serviceIdle := isUDFServiceIdle(err)
	serviceAwaking := isUDFServiceAwaking(err)
	if !serviceIdle && !serviceAwaking {
		return body, err
	}

	if serviceIdle {
		tflog.Info(ctx, "ClickHouse service is idle; waking it up before retrying the UDF attachment", map[string]any{
			logFieldServiceID: serviceID,
		})

		if wakeErr := c.wakeService(ctx, serviceID); wakeErr != nil {
			state, stateErr := c.getServiceState(ctx, serviceID)
			if stateErr != nil || (state != StateAwaking && state != StateRunning) {
				return nil, fmt.Errorf("service %s is idle and waking it up failed: %w", serviceID, wakeErr)
			}
			tflog.Info(ctx, "ClickHouse service was already woken up by another request; continuing", map[string]any{
				logFieldServiceID: serviceID,
			})
		}
	} else {
		tflog.Info(ctx, "ClickHouse service is already awaking; waiting before retrying the UDF attachment", map[string]any{
			logFieldServiceID: serviceID,
		})
	}

	if waitErr := c.waitForServiceRunning(ctx, serviceID, udfServiceWakeMaxWaitSeconds); waitErr != nil {
		return nil, fmt.Errorf("service %s did not reach the running state before retrying UDF attachment: %w", serviceID, waitErr)
	}

	return c.doRequest(ctx, req)
}

func (c *ClientImpl) AttachUDF(ctx context.Context, functionName, serviceID string, request UDFAttachRequest) (*UDFAttachment, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode UDF attachment request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, c.udfAttachmentPath(functionName, serviceID), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create UDF attachment request: %w", err)
	}
	responseBody, err := c.doUDFAttachmentRequest(ctx, serviceID, req)
	if err != nil {
		return nil, err
	}

	var attachment ResponseWithResult[UDFAttachment]
	if err := json.Unmarshal(responseBody, &attachment); err != nil {
		return nil, fmt.Errorf("decode UDF attachment response: %w", err)
	}
	if !isUsableUDFAttachmentResponse(attachment.Result) ||
		attachment.Result.FunctionName != functionName ||
		attachment.Result.ServiceID != serviceID {
		return nil, fmt.Errorf("decode UDF attachment response: invalid or incomplete response")
	}
	return &attachment.Result, nil
}

func (c *ClientImpl) GetUDFAttachment(ctx context.Context, functionName, serviceID string) (*UDFAttachment, error) {
	req, err := http.NewRequest(http.MethodGet, c.udfAttachmentPath(functionName, serviceID), nil)
	if err != nil {
		return nil, fmt.Errorf("create get UDF attachment request: %w", err)
	}
	responseBody, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	var attachment ResponseWithResult[UDFAttachment]
	if err := json.Unmarshal(responseBody, &attachment); err != nil {
		return nil, fmt.Errorf("decode UDF attachment response: %w", err)
	}
	if !isUsableUDFAttachmentResponse(attachment.Result) ||
		attachment.Result.FunctionName != functionName ||
		attachment.Result.ServiceID != serviceID {
		return nil, fmt.Errorf("decode UDF attachment response: invalid or incomplete response")
	}
	return &attachment.Result, nil
}

func isUsableUDFAttachmentResponse(attachment UDFAttachment) bool {
	return attachment.FunctionName != "" && attachment.ServiceID != "" && attachment.Version > 0 && attachment.Status != ""
}

func (c *ClientImpl) DetachUDF(ctx context.Context, functionName, serviceID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.udfAttachmentPath(functionName, serviceID), nil)
	if err != nil {
		return fmt.Errorf("create detach UDF request: %w", err)
	}
	_, err = c.doRequest(ctx, req)
	return err
}

// WakeService sends the awake command to an idle service.
func (c *ClientImpl) WakeService(ctx context.Context, serviceID string) error {
	return c.wakeService(ctx, serviceID)
}
