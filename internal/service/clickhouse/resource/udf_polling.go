package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

const (
	udfPollInterval                 = 5 * time.Second
	udfAttachmentStableObservations = 3
	udfServiceStateCheckInterval    = 30 * time.Second
)

// udfAttachmentRecovery wakes an idle service and retries the same attachment
// once the service is running again.
type udfAttachmentRecovery struct {
	GetState func(context.Context) (string, error)
	Wake     func(context.Context) error
	Retry    func(context.Context) error
}

// udfAttachmentTimeoutError is returned when attach polling times out.
// stuckAfterWake means we woke the idle service and it still made no progress.
type udfAttachmentTimeoutError struct {
	lastStatus       string
	lastServiceState string
	stuckAfterWake   bool
}

func (e *udfAttachmentTimeoutError) Error() string {
	if e.stuckAfterWake {
		return fmt.Sprintf("wait for UDF attachment: still %q after waking the service", e.lastStatus)
	}
	return fmt.Sprintf("wait for UDF attachment: timed out (last status=%q, service state=%q)", e.lastStatus, e.lastServiceState)
}

func waitForUDFReady(
	ctx context.Context,
	maxWait time.Duration,
	expectedVersion int64,
	get func(context.Context) (*api.UDF, error),
) (*api.UDF, error) {
	return pollUDFVersion(ctx, maxWait, udfPollInterval, expectedVersion, get)
}

func pollUDFVersion(
	ctx context.Context,
	maxWait, interval time.Duration,
	expectedVersion int64,
	get func(context.Context) (*api.UDF, error),
) (*api.UDF, error) {
	ctx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	var last *api.UDF
	for {
		udf, err := get(ctx)
		if err == nil {
			if udf == nil {
				return last, fmt.Errorf("get UDF returned an empty response")
			}
			switch {
			case udf.Version < expectedVersion:
			case udf.Version > expectedVersion:
				return last, fmt.Errorf(
					"UDF %q advanced to version %d while waiting for version %d",
					udf.FunctionName,
					udf.Version,
					expectedVersion,
				)
			default:
				last = udf
				switch udf.Status {
				case api.UDFStatusReady:
					return udf, nil
				case api.UDFStatusError:
					message := "the build did not provide an error message"
					if udf.Error != nil && *udf.Error != "" {
						message = *udf.Error
					}
					return udf, fmt.Errorf("UDF %q version %d build failed: %s", udf.FunctionName, udf.Version, message)
				case api.UDFStatusBuilding:
				default:
					// A newly added intermediate state is safe to wait through. The
					// outer timeout still prevents an unknown terminal state hanging.
				}
			}
		} else if !isUDFNotFound(err) && !isUDFServerError(err) {
			return last, err
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, fmt.Errorf("wait for UDF build: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func waitForUDFAttachmentDeployed(
	ctx context.Context,
	maxWait time.Duration,
	expectedVersion int64,
	get func(context.Context) (*api.UDFAttachment, error),
	recovery *udfAttachmentRecovery,
) (*api.UDFAttachment, error) {
	return pollUDFAttachment(ctx, maxWait, udfPollInterval, expectedVersion, get, recovery)
}

func pollUDFAttachment(
	ctx context.Context,
	maxWait, interval time.Duration,
	expectedVersion int64,
	get func(context.Context) (*api.UDFAttachment, error),
	recovery *udfAttachmentRecovery,
) (*api.UDFAttachment, error) {
	ctx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	var last *api.UDFAttachment
	stableObservations := 0
	lastServiceState := ""
	wakeAttempted := false
	wakeSucceeded := false
	retryAttempted := false
	serviceStateCheckEveryPolls := int(udfServiceStateCheckInterval / udfPollInterval)
	pollCount := 0
	for {
		pollCount++
		attachment, err := get(ctx)
		if err == nil {
			if attachment == nil {
				return last, fmt.Errorf("get UDF attachment returned an empty response")
			}
			if attachment.Version != expectedVersion {
				stableObservations = 0
			} else {
				last = attachment
				switch attachment.Status {
				case api.UDFAttachmentStatusDeployed:
					stableObservations++
					if stableObservations >= udfAttachmentStableObservations {
						return attachment, nil
					}
				case api.UDFAttachmentStatusError:
					return attachment, fmt.Errorf(
						"UDF %q version %d attachment to service %q failed",
						attachment.FunctionName,
						attachment.Version,
						attachment.ServiceID,
					)
				case api.UDFAttachmentStatusProvisioning, api.UDFAttachmentStatusDeprovisioning, api.UDFAttachmentStatusStandby:
					stableObservations = 0
				default:
					stableObservations = 0
				}
			}
		} else if !isUDFNotFound(err) && !isUDFServerError(err) {
			return last, err
		} else {
			stableObservations = 0
		}

		stuck := last != nil && last.Status == api.UDFAttachmentStatusProvisioning
		if stuck && recovery != nil && recovery.GetState != nil && pollCount%serviceStateCheckEveryPolls == 0 {
			state, stateErr := recovery.GetState(ctx)
			if stateErr == nil {
				lastServiceState = state
				switch {
				case state == api.StateIdle && !wakeAttempted && recovery.Wake != nil:
					wakeAttempted = true
					tflog.Info(ctx, "UDF attachment is provisioning while the service is idle; waking the service", map[string]any{
						"functionName": last.FunctionName,
						"serviceId":    last.ServiceID,
					})
					if wakeErr := recovery.Wake(ctx); wakeErr != nil {
						tflog.Warn(ctx, "failed to wake idle service for a stuck UDF attachment", map[string]any{
							"serviceId": last.ServiceID,
							"error":     safeUDFError(wakeErr),
						})
					} else {
						wakeSucceeded = true
					}
				case state == api.StateRunning && wakeAttempted && !retryAttempted && recovery.Retry != nil:
					retryAttempted = true
					tflog.Info(ctx, "ClickHouse service is running; retrying the stuck UDF attachment", map[string]any{
						"functionName": last.FunctionName,
						"serviceId":    last.ServiceID,
					})
					if retryErr := recovery.Retry(ctx); retryErr != nil {
						tflog.Warn(ctx, "failed to retry the stuck UDF attachment", map[string]any{
							"serviceId": last.ServiceID,
							"error":     safeUDFError(retryErr),
						})
					}
				}
			}
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			status := ""
			if last != nil {
				status = last.Status
			}
			return last, &udfAttachmentTimeoutError{
				lastStatus:       status,
				lastServiceState: lastServiceState,
				stuckAfterWake:   wakeSucceeded && status == api.UDFAttachmentStatusProvisioning,
			}
		case <-timer.C:
		}
	}
}

func waitForUDFAttachmentDeleted(
	ctx context.Context,
	maxWait time.Duration,
	get func(context.Context) (*api.UDFAttachment, error),
) error {
	return pollUDFAttachmentDeleted(ctx, maxWait, udfPollInterval, get)
}

func pollUDFAttachmentDeleted(
	ctx context.Context,
	maxWait, interval time.Duration,
	get func(context.Context) (*api.UDFAttachment, error),
) error {
	ctx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	stableObservations := 0
	for {
		attachment, err := get(ctx)
		switch {
		case isUDFNotFound(err):
			stableObservations++
			if stableObservations >= udfAttachmentStableObservations {
				return nil
			}
		case err != nil && !isUDFServerError(err):
			return err
		case err != nil:
			stableObservations = 0
		case attachment == nil:
			return fmt.Errorf("get UDF attachment returned an empty response")
		case attachment.Status == api.UDFAttachmentStatusError:
			return fmt.Errorf(
				"UDF %q version %d detachment from service %q failed",
				attachment.FunctionName,
				attachment.Version,
				attachment.ServiceID,
			)
		default:
			stableObservations = 0
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("wait for UDF detachment: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func retryUDFDelete(
	ctx context.Context,
	maxWait time.Duration,
	remove func(context.Context) error,
) error {
	return retryUDFMutation(ctx, maxWait, udfPollInterval, "delete UDF", remove)
}

func retryUDFAttachmentDetach(
	ctx context.Context,
	maxWait time.Duration,
	detach func(context.Context) error,
) error {
	return retryUDFMutation(ctx, maxWait, udfPollInterval, "detach UDF", detach)
}

// udfMutationTimeoutError means a delete/detach kept conflicting until timeout.
type udfMutationTimeoutError struct {
	operation string
	ctxErr    error
	lastErr   error
}

func (e *udfMutationTimeoutError) Error() string {
	return fmt.Sprintf("%s: %v (last API error: %s)", e.operation, e.ctxErr, safeUDFError(e.lastErr))
}

func (e *udfMutationTimeoutError) Unwrap() error {
	return e.ctxErr
}

// retryUDFMutation retries delete/detach while another change is still in progress.
func retryUDFMutation(
	ctx context.Context,
	maxWait, interval time.Duration,
	operation string,
	mutate func(context.Context) error,
) error {
	ctx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	var lastErr error
	for {
		err := mutate(ctx)
		switch {
		case err == nil, isUDFNotFound(err):
			return nil
		case !api.IsConflict(err) && !isUDFServerError(err):
			return err
		default:
			lastErr = err
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return &udfMutationTimeoutError{operation: operation, ctxErr: ctx.Err(), lastErr: lastErr}
		case <-timer.C:
		}
	}
}
