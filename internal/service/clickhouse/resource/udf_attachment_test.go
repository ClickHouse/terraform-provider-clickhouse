package resource

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func TestUDFAttachmentCreateAttachesRequestedVersionAndPolls(t *testing.T) {
	ctx := context.Background()
	r := NewUDFAttachmentResource().(*UDFAttachmentResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := models.UDFAttachmentResourceModel{
		FunctionName: types.StringValue("geocode"),
		ServiceID:    types.StringValue("11111111-1111-1111-1111-111111111111"),
		Version:      types.Int64Value(2),
		Status:       types.StringNull(),
	}
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	provisioning := &api.UDFAttachment{
		FunctionName: "geocode",
		ServiceID:    "11111111-1111-1111-1111-111111111111",
		Version:      2,
		Status:       api.UDFAttachmentStatusProvisioning,
	}
	deployed := *provisioning
	deployed.Status = api.UDFAttachmentStatusDeployed
	requestedVersion := int64(2)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.AttachUDFMock.Expect(
		ctx,
		"geocode",
		"11111111-1111-1111-1111-111111111111",
		api.UDFAttachRequest{Version: &requestedVersion},
	).Return(provisioning, nil)
	client.GetUDFAttachmentMock.
		ExpectFunctionNameParam2("geocode").
		ExpectServiceIDParam3("11111111-1111-1111-1111-111111111111").
		Return(&deployed, nil)
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}

	var state models.UDFAttachmentResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read created state: %v", diags)
	}
	if state.Version.ValueInt64() != 2 || state.Status.ValueString() != api.UDFAttachmentStatusDeployed {
		t.Fatalf("created state = %+v; want deployed version 2", state)
	}
}

func TestUDFAttachmentCreateExplainsHowToRecoverFromErroredVersion(t *testing.T) {
	ctx := context.Background()
	r := NewUDFAttachmentResource().(*UDFAttachmentResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := models.UDFAttachmentResourceModel{
		FunctionName: types.StringValue("geocode"),
		ServiceID:    types.StringValue("11111111-1111-1111-1111-111111111111"),
		Version:      types.Int64Value(2),
		Status:       types.StringNull(),
	}
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	requestedVersion := int64(2)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.AttachUDFMock.Expect(
		ctx,
		"geocode",
		"11111111-1111-1111-1111-111111111111",
		api.UDFAttachRequest{Version: &requestedVersion},
	).Return(nil, errors.New("status: 409, body: requested UDF version is not ready"))
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() || len(resp.Diagnostics.Errors()) != 1 {
		t.Fatalf("Create diagnostics = %v; want one attachment error", resp.Diagnostics)
	}
	diagnostic := resp.Diagnostics.Errors()[0]
	if diagnostic.Summary() != "Error creating UDF attachment" {
		t.Fatalf("diagnostic summary = %q", diagnostic.Summary())
	}
	for _, want := range []string{"geocode", "version 2", "request failed with HTTP status 409"} {
		if !strings.Contains(diagnostic.Detail(), want) {
			t.Errorf("diagnostic detail = %q; want %q", diagnostic.Detail(), want)
		}
	}
}

func TestUDFAttachmentImportUsesFunctionNameAndServiceID(t *testing.T) {
	ctx := context.Background()
	r := NewUDFAttachmentResource().(*UDFAttachmentResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	if diags := sch.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid UDF attachment schema implementation: %v", diags)
	}
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: sch,
			Raw:    tftypes.NewValue(sch.Type().TerraformType(ctx), nil),
		},
	}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "geocode/11111111-1111-1111-1111-111111111111"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diagnostics: %v", resp.Diagnostics)
	}

	var functionName, serviceID types.String
	if diags := resp.State.GetAttribute(ctx, path.Root("function_name"), &functionName); diags.HasError() {
		t.Fatalf("read function_name: %v", diags)
	}
	if diags := resp.State.GetAttribute(ctx, path.Root("service_id"), &serviceID); diags.HasError() {
		t.Fatalf("read service_id: %v", diags)
	}
	if functionName.ValueString() != "geocode" || serviceID.ValueString() != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("imported function/service = %q/%q", functionName.ValueString(), serviceID.ValueString())
	}
}

func TestUDFAttachmentPollingTreatsStandbyAsTransitional(t *testing.T) {
	calls := 0
	attachment, err := pollUDFAttachment(context.Background(), time.Second, time.Millisecond, 2, func(context.Context) (*api.UDFAttachment, error) {
		calls++
		status := api.UDFAttachmentStatusStandby
		if calls >= 2 {
			status = api.UDFAttachmentStatusDeployed
		}
		return &api.UDFAttachment{
			FunctionName: "geocode",
			ServiceID:    "11111111-1111-1111-1111-111111111111",
			Version:      2,
			Status:       status,
		}, nil
	}, nil)
	if err != nil || attachment.Status != api.UDFAttachmentStatusDeployed || calls != 4 {
		t.Fatalf("pollUDFAttachment = %+v, %v after %d calls", attachment, err, calls)
	}
}

func TestUDFAttachmentPollingWaitsThroughNewStatus(t *testing.T) {
	calls := 0
	attachment, err := pollUDFAttachment(context.Background(), time.Second, time.Millisecond, 2, func(context.Context) (*api.UDFAttachment, error) {
		calls++
		status := "queued"
		if calls >= 2 {
			status = api.UDFAttachmentStatusDeployed
		}
		return &api.UDFAttachment{
			FunctionName: "geocode",
			ServiceID:    "11111111-1111-1111-1111-111111111111",
			Version:      2,
			Status:       status,
		}, nil
	}, nil)
	if err != nil || attachment.Status != api.UDFAttachmentStatusDeployed || calls != 4 {
		t.Fatalf("pollUDFAttachment = %+v, %v after %d calls", attachment, err, calls)
	}
}

func TestUDFAttachmentPollingIgnoresStaleDeployedVersion(t *testing.T) {
	calls := 0
	attachment, err := pollUDFAttachment(context.Background(), time.Second, time.Millisecond, 2, func(context.Context) (*api.UDFAttachment, error) {
		calls++
		version := int64(2)
		if calls == 1 {
			version = 1
		}
		return &api.UDFAttachment{
			FunctionName: "geocode",
			ServiceID:    "11111111-1111-1111-1111-111111111111",
			Version:      version,
			Status:       api.UDFAttachmentStatusDeployed,
		}, nil
	}, nil)
	if err != nil || attachment.Version != 2 || calls != 4 {
		t.Fatalf("pollUDFAttachment = %+v, %v after %d calls; want stable version 2", attachment, err, calls)
	}
}

func TestUDFAttachmentDeleteWaitsForStableAbsence(t *testing.T) {
	calls := 0
	err := pollUDFAttachmentDeleted(context.Background(), time.Second, time.Millisecond, func(context.Context) (*api.UDFAttachment, error) {
		calls++
		if calls == 1 || calls == 3 {
			return &api.UDFAttachment{
				FunctionName: "geocode",
				ServiceID:    "11111111-1111-1111-1111-111111111111",
				Version:      2,
				Status:       api.UDFAttachmentStatusDeprovisioning,
			}, nil
		}
		return nil, errors.New(`status: 404, body: {"error":"not attached"}`)
	})
	if err != nil || calls != 6 {
		t.Fatalf("pollUDFAttachmentDeleted = %v after %d calls; want three stable 404 observations", err, calls)
	}
}

func alwaysProvisioning(functionName, serviceID string) func(context.Context) (*api.UDFAttachment, error) {
	return func(context.Context) (*api.UDFAttachment, error) {
		return &api.UDFAttachment{
			FunctionName: functionName,
			ServiceID:    serviceID,
			Version:      2,
			Status:       api.UDFAttachmentStatusProvisioning,
		}, nil
	}
}

func TestPollUDFAttachmentTimesOutWithLastKnownStatus(t *testing.T) {
	_, err := pollUDFAttachment(context.Background(), 50*time.Millisecond, time.Millisecond, 2, alwaysProvisioning("geocode", "11111111-1111-1111-1111-111111111111"), nil)

	var timeoutErr *udfAttachmentTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("pollUDFAttachment error = %v (%T); want *udfAttachmentTimeoutError", err, err)
	}
	if timeoutErr.stuckAfterWake {
		t.Fatalf("timeoutErr.stuckAfterWake = true; want false for a plain timeout")
	}
	if timeoutErr.lastStatus != api.UDFAttachmentStatusProvisioning {
		t.Fatalf("timeoutErr.lastStatus = %q; want %q", timeoutErr.lastStatus, api.UDFAttachmentStatusProvisioning)
	}
}

func TestPollUDFAttachmentTimeoutWithNoStatusObserved(t *testing.T) {
	_, err := pollUDFAttachment(context.Background(), 20*time.Millisecond, time.Millisecond, 2, func(context.Context) (*api.UDFAttachment, error) {
		return nil, errors.New(`status: 404, body: {"error":"not attached yet"}`)
	}, nil)

	var timeoutErr *udfAttachmentTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("pollUDFAttachment error = %v (%T); want *udfAttachmentTimeoutError", err, err)
	}
	if timeoutErr.lastStatus != "" {
		t.Fatalf("timeoutErr.lastStatus = %q; want empty (never observed)", timeoutErr.lastStatus)
	}
}

func newAttachmentState(functionName, serviceID, status string, version int64) *models.UDFAttachmentResourceModel {
	return &models.UDFAttachmentResourceModel{
		FunctionName: types.StringValue(functionName),
		ServiceID:    types.StringValue(serviceID),
		Version:      types.Int64Value(version),
		Status:       types.StringValue(status),
	}
}

func firstDiagnosticDetail(t *testing.T, diags diag.Diagnostics) string {
	t.Helper()
	if !diags.HasError() || len(diags.Errors()) != 1 {
		t.Fatalf("diagnostics = %v; want exactly one error", diags)
	}
	return diags.Errors()[0].Detail()
}

func TestAddUDFAttachmentWaitErrorTimeoutMessages(t *testing.T) {
	ctx := context.Background()
	serviceID := "11111111-1111-1111-1111-111111111111"

	t.Run("timeout", func(t *testing.T) {
		var diags diag.Diagnostics
		state := newAttachmentState("my_fn", serviceID, api.UDFAttachmentStatusProvisioning, 3)
		addUDFAttachmentWaitError(ctx, &diags, state, &udfAttachmentTimeoutError{lastStatus: api.UDFAttachmentStatusProvisioning})

		if got := diags.Errors()[0].Summary(); got != "Error waiting for UDF attachment" {
			t.Fatalf("summary = %q", got)
		}
		detail := firstDiagnosticDetail(t, diags)
		if !strings.Contains(detail, fmt.Sprintf(`Could not finish attaching UDF "my_fn" to service %s`, serviceID)) {
			t.Errorf("detail = %q; want short attach wait message", detail)
		}
	})

	t.Run("stuck after wake", func(t *testing.T) {
		var diags diag.Diagnostics
		state := newAttachmentState("my_fn", serviceID, api.UDFAttachmentStatusProvisioning, 3)
		addUDFAttachmentWaitError(ctx, &diags, state, &udfAttachmentTimeoutError{
			lastStatus:       api.UDFAttachmentStatusProvisioning,
			lastServiceState: api.StateIdle,
			stuckAfterWake:   true,
		})

		if got := diags.Errors()[0].Summary(); got != "Error waiting for UDF attachment" {
			t.Fatalf("summary = %q", got)
		}
		detail := firstDiagnosticDetail(t, diags)
		if !strings.Contains(detail, fmt.Sprintf(`Could not finish attaching UDF "my_fn" to service %s`, serviceID)) {
			t.Errorf("detail = %q; want short attach wait message", detail)
		}
	})
}

func TestPollUDFAttachmentRecoversIdleServiceExactlyOnce(t *testing.T) {
	calls := 0
	get := func(context.Context) (*api.UDFAttachment, error) {
		calls++
		status := api.UDFAttachmentStatusProvisioning
		if calls > 18 {
			status = api.UDFAttachmentStatusDeployed
		}
		return &api.UDFAttachment{
			FunctionName: "geocode",
			ServiceID:    "11111111-1111-1111-1111-111111111111",
			Version:      2,
			Status:       status,
		}, nil
	}

	getStateCalls := 0
	wakeCalls := 0
	retryCalls := 0
	recovery := &udfAttachmentRecovery{
		GetState: func(context.Context) (string, error) {
			getStateCalls++
			if getStateCalls == 1 {
				return api.StateIdle, nil
			}
			return api.StateRunning, nil
		},
		Wake: func(context.Context) error {
			wakeCalls++
			return nil
		},
		Retry: func(context.Context) error {
			retryCalls++
			return nil
		},
	}

	attachment, err := pollUDFAttachment(context.Background(), time.Second, time.Millisecond, 2, get, recovery)
	if err != nil || attachment == nil || attachment.Status != api.UDFAttachmentStatusDeployed {
		t.Fatalf("pollUDFAttachment = %+v, %v; want a deployed attachment", attachment, err)
	}
	if wakeCalls != 1 {
		t.Fatalf("wakeCalls = %d; want the service woken exactly once", wakeCalls)
	}
	if retryCalls != 1 {
		t.Fatalf("retryCalls = %d; want the attachment retried exactly once", retryCalls)
	}
}

func TestPollUDFAttachmentDoesNotWakeAStoppedService(t *testing.T) {
	get := alwaysProvisioning("geocode", "11111111-1111-1111-1111-111111111111")

	wakeCalls := 0
	retryCalls := 0
	recovery := &udfAttachmentRecovery{
		GetState: func(context.Context) (string, error) {
			return api.StateStopped, nil
		},
		Wake: func(context.Context) error {
			wakeCalls++
			return nil
		},
		Retry: func(context.Context) error {
			retryCalls++
			return nil
		},
	}

	_, err := pollUDFAttachment(context.Background(), 40*time.Millisecond, time.Millisecond, 2, get, recovery)
	var timeoutErr *udfAttachmentTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("pollUDFAttachment error = %v (%T); want *udfAttachmentTimeoutError", err, err)
	}
	if timeoutErr.stuckAfterWake {
		t.Fatalf("timeoutErr.stuckAfterWake = true; want false since the service was never woken")
	}
	if wakeCalls != 0 {
		t.Fatalf("wakeCalls = %d; want the poller to never wake a stopped service", wakeCalls)
	}
	if retryCalls != 0 {
		t.Fatalf("retryCalls = %d; want the poller to never retry an attachment on a stopped service", retryCalls)
	}
}

func TestPollUDFAttachmentStillStuckAfterRecoveryUsesFullTimeout(t *testing.T) {
	get := alwaysProvisioning("geocode", "11111111-1111-1111-1111-111111111111")

	getStateCalls := 0
	wakeCalls := 0
	retryCalls := 0
	recovery := &udfAttachmentRecovery{
		GetState: func(context.Context) (string, error) {
			getStateCalls++
			if getStateCalls == 1 {
				return api.StateIdle, nil
			}
			return api.StateRunning, nil
		},
		Wake: func(context.Context) error {
			wakeCalls++
			return nil
		},
		Retry: func(context.Context) error {
			retryCalls++
			return nil
		},
	}

	started := time.Now()
	last, err := pollUDFAttachment(context.Background(), 50*time.Millisecond, time.Millisecond, 2, get, recovery)
	elapsed := time.Since(started)

	var timeoutErr *udfAttachmentTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("pollUDFAttachment error = %v (%T); want *udfAttachmentTimeoutError", err, err)
	}
	if !timeoutErr.stuckAfterWake {
		t.Fatalf("timeoutErr.stuckAfterWake = false; want the timeout to record the successful wake")
	}
	if last == nil || last.Status != api.UDFAttachmentStatusProvisioning {
		t.Fatalf("last = %+v; want the last observed provisioning attachment", last)
	}
	if elapsed < 40*time.Millisecond {
		t.Fatalf("polling stopped after %s; want it to use the full timeout", elapsed)
	}
	if wakeCalls != 1 {
		t.Fatalf("wakeCalls = %d; want the service woken exactly once even though it never recovered", wakeCalls)
	}
	if retryCalls != 1 {
		t.Fatalf("retryCalls = %d; want the attachment retried exactly once", retryCalls)
	}
}

func TestAddUDFAttachmentWaitErrorHidesServerDetails(t *testing.T) {
	ctx := context.Background()
	serviceID := "11111111-1111-1111-1111-111111111111"
	state := newAttachmentState("my_fn", serviceID, api.UDFAttachmentStatusProvisioning, 3)

	var diags diag.Diagnostics
	raw := `status: 500, body: {"error":"the deployment queue is backed up"}`
	addUDFAttachmentWaitError(ctx, &diags, state, errors.New(raw))

	if got := diags.Errors()[0].Summary(); got != "Error waiting for UDF attachment" {
		t.Fatalf("summary = %q", got)
	}
	detail := firstDiagnosticDetail(t, diags)
	if !strings.Contains(detail, "HTTP status 500") || strings.Contains(detail, "deployment queue") || strings.Contains(detail, "body:") {
		t.Errorf("detail = %q; want a generic server error", detail)
	}
}

func TestUDFAttachmentReadErrorHidesServerDetails(t *testing.T) {
	ctx := context.Background()
	r := NewUDFAttachmentResource().(*UDFAttachmentResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := models.UDFAttachmentResourceModel{
		FunctionName: types.StringValue("geocode"),
		ServiceID:    types.StringValue("11111111-1111-1111-1111-111111111111"),
		Version:      types.Int64Value(1),
		Status:       types.StringValue(api.UDFAttachmentStatusDeployed),
	}
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	raw := `status: 500, body: {"error":"database is unreachable"}`
	client.GetUDFAttachmentMock.
		ExpectFunctionNameParam2("geocode").
		ExpectServiceIDParam3("11111111-1111-1111-1111-111111111111").
		Return(nil, errors.New(raw))
	r.client = client

	resp := &resource.ReadResponse{State: priorState}
	r.Read(ctx, resource.ReadRequest{State: priorState}, resp)

	detail := firstDiagnosticDetail(t, resp.Diagnostics)
	if !strings.Contains(detail, "HTTP status 500") || strings.Contains(detail, "database") || strings.Contains(detail, "body:") {
		t.Errorf("detail = %q; want a generic server error", detail)
	}
}

func TestAddUDFDetachWaitErrorMessages(t *testing.T) {
	ctx := context.Background()
	serviceID := "11111111-1111-1111-1111-111111111111"

	t.Run("timeout", func(t *testing.T) {
		state := newAttachmentState("my_fn", serviceID, api.UDFAttachmentStatusDeployed, 1)
		var diags diag.Diagnostics
		addUDFDetachWaitError(ctx, &diags, state, fmt.Errorf("wait for UDF detachment: %w", context.DeadlineExceeded))

		if got := diags.Errors()[0].Summary(); got != "Error waiting for UDF detachment" {
			t.Fatalf("summary = %q", got)
		}
		detail := firstDiagnosticDetail(t, diags)
		if !strings.Contains(detail, fmt.Sprintf(`Could not finish detaching UDF "my_fn" from service %s`, serviceID)) {
			t.Errorf("detail = %q; want short detach wait message", detail)
		}
	})

	t.Run("other server failure hides service details", func(t *testing.T) {
		state := newAttachmentState("my_fn", serviceID, api.UDFAttachmentStatusDeployed, 1)
		var diags diag.Diagnostics
		raw := `status: 500, body: {"error":"get UDF attachment returned an empty response"}`
		addUDFDetachWaitError(ctx, &diags, state, errors.New(raw))

		if got := diags.Errors()[0].Summary(); got != "Error waiting for UDF detachment" {
			t.Fatalf("summary = %q", got)
		}
		detail := firstDiagnosticDetail(t, diags)
		if !strings.Contains(detail, "HTTP status 500") || strings.Contains(detail, "empty response") || strings.Contains(detail, "body:") {
			t.Errorf("detail = %q; want a generic server error", detail)
		}
	})
}

func TestAddUDFAttachmentDetachErrorExplainsStuckTransition(t *testing.T) {
	ctx := context.Background()
	state := newAttachmentState("my_fn", "11111111-1111-1111-1111-111111111111", api.UDFAttachmentStatusProvisioning, 2)

	mutationErr := retryUDFMutation(ctx, 20*time.Millisecond, time.Millisecond, "detach UDF", func(context.Context) error {
		return errors.New(`status: 409, body: {"error":"A UDF attachment transition is already in progress for this service"}`)
	})
	if mutationErr == nil {
		t.Fatalf("retryUDFMutation = nil; want a timeout error after exhausting the retry budget")
	}

	var diags diag.Diagnostics
	addUDFAttachmentDetachError(ctx, &diags, state, mutationErr)

	if got := diags.Errors()[0].Summary(); got != "Error detaching UDF" {
		t.Fatalf("summary = %q", got)
	}
	detail := firstDiagnosticDetail(t, diags)
	for _, want := range []string{"my_fn", "11111111-1111-1111-1111-111111111111", "A UDF attachment transition is already in progress"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail = %q; want to contain %q", detail, want)
		}
	}
}

func TestAddUDFAttachmentDetachErrorSurfacesFullAPIError(t *testing.T) {
	ctx := context.Background()
	state := newAttachmentState("my_fn", "11111111-1111-1111-1111-111111111111", api.UDFAttachmentStatusDeployed, 2)

	var diags diag.Diagnostics
	raw := `status: 400, body: {"error":"bad request"}`
	addUDFAttachmentDetachError(ctx, &diags, state, errors.New(raw))

	detail := firstDiagnosticDetail(t, diags)
	if !strings.Contains(detail, "bad request") || strings.Contains(detail, "status:") || strings.Contains(detail, "body:") {
		t.Errorf("detail = %q; want the safe API cause without raw status/body", detail)
	}
}

// TestUDFAttachmentResourceDeleteDetachFails covers the detach-fails branch of
// Delete: a non-retryable detach error is reported without ever polling for
// deletion, so the test completes without any real waiting.
func TestUDFAttachmentResourceDeleteDetachFails(t *testing.T) {
	ctx := context.Background()
	r := NewUDFAttachmentResource().(*UDFAttachmentResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := models.UDFAttachmentResourceModel{
		FunctionName: types.StringValue("geocode"),
		ServiceID:    types.StringValue("11111111-1111-1111-1111-111111111111"),
		Version:      types.Int64Value(1),
		Status:       types.StringValue(api.UDFAttachmentStatusDeployed),
	}
	state := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.DetachUDFMock.
		ExpectFunctionNameParam2("geocode").
		ExpectServiceIDParam3("11111111-1111-1111-1111-111111111111").
		Return(errors.New(`status: 403, body: {"error":"forbidden"}`))
	r.client = client

	resp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Delete diagnostics contain no error for a non-retryable detach failure")
	}
	detail := firstDiagnosticDetail(t, resp.Diagnostics)
	if !strings.Contains(detail, "forbidden") {
		t.Errorf("detail = %q; want the API's own cause", detail)
	}
	if got := client.GetUDFAttachmentAfterCounter(); got != 0 {
		t.Fatalf("GetUDFAttachment calls = %d; want 0, Delete must not poll after a failed detach", got)
	}
}

// TestUDFAttachmentResourceDeleteWaitFails covers waitForUDFAttachmentDeleted
// via the real Delete path: the detach succeeds but the follow-up GetUDFAttachment
// returns a hard, non-retryable error, so pollUDFAttachmentDeleted returns
// immediately instead of sleeping through the real 15-minute timeout.
func TestUDFAttachmentResourceDeleteWaitFails(t *testing.T) {
	ctx := context.Background()
	r := NewUDFAttachmentResource().(*UDFAttachmentResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := models.UDFAttachmentResourceModel{
		FunctionName: types.StringValue("geocode"),
		ServiceID:    types.StringValue("11111111-1111-1111-1111-111111111111"),
		Version:      types.Int64Value(1),
		Status:       types.StringValue(api.UDFAttachmentStatusDeployed),
	}
	state := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.DetachUDFMock.
		ExpectFunctionNameParam2("geocode").
		ExpectServiceIDParam3("11111111-1111-1111-1111-111111111111").
		Return(nil)
	client.GetUDFAttachmentMock.
		ExpectFunctionNameParam2("geocode").
		ExpectServiceIDParam3("11111111-1111-1111-1111-111111111111").
		Return(nil, errors.New(`status: 403, body: {"error":"forbidden"}`))
	r.client = client

	resp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Delete diagnostics contain no error when confirming detachment fails")
	}
	detail := firstDiagnosticDetail(t, resp.Diagnostics)
	if !strings.Contains(detail, "forbidden") {
		t.Errorf("detail = %q; want the API's own cause", detail)
	}
}

// TestUDFAttachmentResourceUpdateReportsWaitError exercises Update, which was
// at 0% coverage. AttachUDF succeeds immediately; the follow-up GetUDFAttachment
// returns a hard error so waitForUDFAttachmentDeployed fails fast instead of
// polling through the real interval.
func TestUDFAttachmentResourceUpdateReportsWaitError(t *testing.T) {
	ctx := context.Background()
	r := NewUDFAttachmentResource().(*UDFAttachmentResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := models.UDFAttachmentResourceModel{
		FunctionName: types.StringValue("geocode"),
		ServiceID:    types.StringValue("11111111-1111-1111-1111-111111111111"),
		Version:      types.Int64Value(3),
		Status:       types.StringNull(),
	}
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	provisioning := &api.UDFAttachment{
		FunctionName: "geocode",
		ServiceID:    "11111111-1111-1111-1111-111111111111",
		Version:      3,
		Status:       api.UDFAttachmentStatusProvisioning,
	}
	requestedVersion := int64(3)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.AttachUDFMock.Expect(
		ctx,
		"geocode",
		"11111111-1111-1111-1111-111111111111",
		api.UDFAttachRequest{Version: &requestedVersion},
	).Return(provisioning, nil)
	client.GetUDFAttachmentMock.
		ExpectFunctionNameParam2("geocode").
		ExpectServiceIDParam3("11111111-1111-1111-1111-111111111111").
		Return(nil, errors.New(`status: 403, body: {"error":"forbidden"}`))
	r.client = client

	resp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Update diagnostics contain no error when confirming deployment fails")
	}
	detail := firstDiagnosticDetail(t, resp.Diagnostics)
	if !strings.Contains(detail, "forbidden") {
		t.Errorf("detail = %q; want the API's own cause", detail)
	}
}

func TestAddUDFAttachmentWriteErrorAttachmentLimitIncludesAPIMessage(t *testing.T) {
	ctx := context.Background()
	plan := models.UDFAttachmentResourceModel{
		FunctionName: types.StringValue("geocode"),
		ServiceID:    types.StringValue("11111111-1111-1111-1111-111111111111"),
		Version:      types.Int64Null(),
	}

	var diags diag.Diagnostics
	addUDFAttachmentWriteError(ctx, &diags, "creating", plan, errors.New(`status: 422, body: {"error":"Service cannot have more than 5 UDFs attached"}`))

	if got := diags.Errors()[0].Summary(); got != "Service has reached its UDF attachment limit" {
		t.Fatalf("summary = %q", got)
	}
	detail := firstDiagnosticDetail(t, diags)
	if !strings.Contains(detail, "Service cannot have more than 5 UDFs attached") {
		t.Errorf("detail = %q; want the API's own message with the real limit", detail)
	}
}

func TestAddUDFAttachmentWriteErrorServiceNotSupported(t *testing.T) {
	ctx := context.Background()
	plan := models.UDFAttachmentResourceModel{
		FunctionName: types.StringValue("geocode"),
		ServiceID:    types.StringValue("11111111-1111-1111-1111-111111111111"),
		Version:      types.Int64Null(),
	}

	var diags diag.Diagnostics
	addUDFAttachmentWriteError(ctx, &diags, "creating", plan, errors.New(`status: 400, body: {"error":"UDFs are not supported on this service"}`))

	if got := diags.Errors()[0].Summary(); got != "UDFs are not supported on this service" {
		t.Fatalf("summary = %q", got)
	}
	detail := firstDiagnosticDetail(t, diags)
	for _, want := range []string{"UDFs are not supported on this service", "retrying will not help"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail = %q; want to contain %q", detail, want)
		}
	}
}
