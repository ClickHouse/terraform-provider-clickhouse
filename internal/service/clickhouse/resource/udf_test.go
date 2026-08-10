package resource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func TestUDFResourceSchemaMatchesPublicUX(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	resp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid UDF schema implementation: %v", diags)
	}

	wantAttributes := []string{
		"function_name", "runtime", "arguments", "return_type", "type", "pool_size",
		"source_archive_path", "source_archive_hash", "return_name", "command_read_timeout",
		"command_write_timeout", "max_command_execution_time", "send_chunk_header", "format",
		"sandbox_type", "sandbox_version", "fail_on_build_error", "version", "status", "error",
		"created_at", "updated_at",
	}
	if len(resp.Schema.Attributes) != len(wantAttributes) {
		t.Fatalf("attribute count = %d; want %d: %#v", len(resp.Schema.Attributes), len(wantAttributes), resp.Schema.Attributes)
	}
	for _, name := range wantAttributes {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("schema missing %q", name)
		}
	}

	version, ok := resp.Schema.Attributes["version"].(schema.Int64Attribute)
	if !ok || !version.Computed || version.Optional || len(version.PlanModifiers) != 0 {
		t.Errorf("version must be computed without UseStateForUnknown so dependent attachments observe updates: %#v", version)
	}
	poolSize, ok := resp.Schema.Attributes["pool_size"].(schema.Int64Attribute)
	if !ok || !poolSize.Optional || !poolSize.Computed {
		t.Errorf("pool_size must support a conditional computed default: %#v", poolSize)
	}
	returnName, ok := resp.Schema.Attributes["return_name"].(schema.StringAttribute)
	if !ok || !returnName.Optional || !returnName.Computed {
		t.Errorf("return_name must be optional/computed: %#v", returnName)
	}
	maxExecutionTime, ok := resp.Schema.Attributes["max_command_execution_time"].(schema.Int64Attribute)
	if !ok || !maxExecutionTime.Optional || !maxExecutionTime.Computed || maxExecutionTime.Default != nil {
		t.Errorf("max_command_execution_time must support a conditional computed default: %#v", maxExecutionTime)
	}
	failOnBuildError, ok := resp.Schema.Attributes["fail_on_build_error"].(schema.BoolAttribute)
	if !ok || !failOnBuildError.Optional || !failOnBuildError.Computed || failOnBuildError.Default == nil {
		t.Errorf("fail_on_build_error must be optional/computed with default true: %#v", failOnBuildError)
	}
}

func TestUDFResourceValidateConfigRejectsPoolFieldsForExecutable(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	configModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutable)
	configModel.PoolSize = types.Int64Value(2)
	configModel.MaxCommandExecutionTime = types.Int64Value(10)
	configPlan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := configPlan.Set(ctx, &configModel); diags.HasError() {
		t.Fatalf("set config fixture: %v", diags)
	}
	config := tfsdk.Config{Schema: sch, Raw: configPlan.Raw}

	resp := &resource.ValidateConfigResponse{}
	r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: config}, resp)
	if len(resp.Diagnostics.Errors()) != 2 {
		t.Fatalf("ValidateConfig errors = %v; want pool_size and max_command_execution_time errors", resp.Diagnostics.Errors())
	}
	for _, field := range []string{"pool_size", "max_command_execution_time"} {
		found := false
		for _, diagnostic := range resp.Diagnostics.Errors() {
			if strings.Contains(diagnostic.Summary(), field) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ValidateConfig errors = %v; missing %s error", resp.Diagnostics.Errors(), field)
		}
	}
}

func TestUDFResourceCreateUploadsPublishesAndPolls(t *testing.T) {
	ctx := context.Background()
	archive := []byte("zip bytes")
	archivePath := filepath.Join(t.TempDir(), "geocode.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	building := testAPIUDF(api.UDFStatusBuilding, 1)
	ready := testAPIUDF(api.UDFStatusReady, 1)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Expect(ctx).Return(&api.UDFUploadSession{
		UploadID:  "upload-1",
		UploadURL: "https://upload.example/archive",
	}, nil)
	client.UploadUDFArchiveMock.Expect(ctx, "https://upload.example/archive", archive).Return(nil)
	client.CreateUDFMock.Inspect(func(_ context.Context, request api.UDFCreateRequest) {
		if request.FunctionName != "geocode" || request.UploadID != "upload-1" {
			t.Errorf("create request = %+v; missing function/upload identity", request)
		}
		if request.PoolSize == nil || *request.PoolSize != 3 || request.CommandReadTimeout != 10000 {
			t.Errorf("create request = %+v; missing runtime config", request)
		}
	}).Return(building, nil)
	getCalls := 0
	client.GetUDFMock.Set(func(context.Context, string) (*api.UDF, error) {
		getCalls++
		if getCalls == 1 {
			return nil, errors.New(`status: 404, body: {"error":"UDF not found"}`)
		}
		return ready, nil
	})
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read created state: %v", diags)
	}
	if state.Version.ValueInt64() != 1 || state.Status.ValueString() != api.UDFStatusReady {
		t.Fatalf("created state = %+v; want ready version 1", state)
	}
}

func TestUDFResourceCreateRestartsWithFreshUploadAfterGone(t *testing.T) {
	ctx := context.Background()
	archive := []byte("zip bytes")
	archivePath := filepath.Join(t.TempDir(), "geocode.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	building := testAPIUDF(api.UDFStatusBuilding, 1)
	ready := testAPIUDF(api.UDFStatusReady, 1)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)

	var uploadSessions, creates int
	client.CreateUDFUploadSessionMock.Set(func(context.Context) (*api.UDFUploadSession, error) {
		uploadSessions++
		return &api.UDFUploadSession{
			UploadID:  fmt.Sprintf("upload-%d", uploadSessions),
			UploadURL: fmt.Sprintf("https://upload.example/archive-%d", uploadSessions),
		}, nil
	})
	client.UploadUDFArchiveMock.Set(func(_ context.Context, _ string, got []byte) error {
		if string(got) != string(archive) {
			t.Errorf("uploaded archive = %q; want %q", got, archive)
		}
		return nil
	})
	client.CreateUDFMock.Set(func(_ context.Context, request api.UDFCreateRequest) (*api.UDF, error) {
		creates++
		if creates == 1 {
			if request.UploadID != "upload-1" {
				t.Errorf("first create uploadId = %q; want upload-1", request.UploadID)
			}
			return nil, fmt.Errorf(`status: 410, body: {"error":"UDF source archive is unavailable"}`)
		}
		if request.UploadID != "upload-2" {
			t.Errorf("restart create uploadId = %q; want upload-2", request.UploadID)
		}
		return building, nil
	})
	client.GetUDFMock.Set(func(_ context.Context, functionName string) (*api.UDF, error) {
		if functionName != "geocode" {
			t.Errorf("GetUDF functionName = %q; want geocode", functionName)
		}
		if uploadSessions == 0 {
			return nil, errors.New(`status: 404, body: {"error":"UDF not found"}`)
		}
		return ready, nil
	})
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	if uploadSessions != 2 || creates != 2 {
		t.Fatalf("uploadSessions=%d creates=%d; want 2 of each after restart", uploadSessions, creates)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read created state: %v", diags)
	}
	if state.Version.ValueInt64() != 1 || state.Status.ValueString() != api.UDFStatusReady {
		t.Fatalf("created state = %+v; want ready version 1", state)
	}
}

// TestUDFResourceCreateRetriesTransientUploadFailure proves the archive upload
// uses the same restart policy as the write call: a flaky 5xx from the upload
// step is retried with a fresh session instead of failing the whole apply.
func TestUDFResourceCreateRetriesTransientUploadFailure(t *testing.T) {
	ctx := context.Background()
	archive := []byte("zip bytes")
	archivePath := filepath.Join(t.TempDir(), "geocode.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	ready := testAPIUDF(api.UDFStatusReady, 1)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	var uploadSessions, uploads int
	client.CreateUDFUploadSessionMock.Set(func(context.Context) (*api.UDFUploadSession, error) {
		uploadSessions++
		return &api.UDFUploadSession{
			UploadID:  fmt.Sprintf("upload-%d", uploadSessions),
			UploadURL: fmt.Sprintf("https://upload.example/archive-%d", uploadSessions),
		}, nil
	})
	client.UploadUDFArchiveMock.Set(func(context.Context, string, []byte) error {
		uploads++
		if uploads == 1 {
			return fmt.Errorf(`status: 500, body: {"error":"upstream timeout"}`)
		}
		return nil
	})
	client.CreateUDFMock.Inspect(func(_ context.Context, request api.UDFCreateRequest) {
		if request.UploadID != "upload-2" {
			t.Errorf("create uploadId = %q; want the second, successfully uploaded session", request.UploadID)
		}
	}).Return(ready, nil)
	getCalls := 0
	client.GetUDFMock.Set(func(context.Context, string) (*api.UDF, error) {
		getCalls++
		if getCalls == 1 {
			return nil, errors.New(`status: 404, body: {"error":"UDF not found"}`)
		}
		return ready, nil
	})
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	if uploadSessions != 2 || uploads != 2 {
		t.Fatalf("uploadSessions=%d uploads=%d; want a fresh session requested after the first upload failed", uploadSessions, uploads)
	}
	if got := client.CreateUDFAfterCounter(); got != 1 {
		t.Fatalf("CreateUDF calls = %d; want exactly 1, only after the upload succeeded", got)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read created state: %v", diags)
	}
	if state.Version.ValueInt64() != 1 || state.Status.ValueString() != api.UDFStatusReady {
		t.Fatalf("created state = %+v; want ready version 1", state)
	}
}

// TestUDFResourceCreateStopsAfterUnknownPublish proves that an unknown
// non-idempotent POST is reconciled once and never replayed blindly.
func TestUDFResourceCreateStopsAfterUnknownPublish(t *testing.T) {
	ctx := context.Background()
	archive := []byte("zip bytes")
	archivePath := filepath.Join(t.TempDir(), "geocode.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	var uploadSessions, creates int
	client.CreateUDFUploadSessionMock.Set(func(context.Context) (*api.UDFUploadSession, error) {
		uploadSessions++
		return &api.UDFUploadSession{
			UploadID:  fmt.Sprintf("upload-%d", uploadSessions),
			UploadURL: fmt.Sprintf("https://upload.example/archive-%d", uploadSessions),
		}, nil
	})
	client.UploadUDFArchiveMock.Set(func(context.Context, string, []byte) error { return nil })
	client.GetUDFMock.Set(func(context.Context, string) (*api.UDF, error) {
		return nil, errors.New(`status: 404, body: {"error":"UDF not found"}`)
	})
	client.CreateUDFMock.Set(func(context.Context, api.UDFCreateRequest) (*api.UDF, error) {
		creates++
		return nil, api.NewUDFPublishOutcomeUnknownError(fmt.Errorf(`status: 500, body: {"error":"upstream timeout"}`))
	})
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create diagnostics contain no error after an unconfirmed publish")
	}
	if uploadSessions != 1 || creates != 1 {
		t.Fatalf("uploadSessions=%d creates=%d; want one publish attempt before reconciliation", uploadSessions, creates)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("Create must not adopt anything when the write never returns a conflict")
	}
}

func TestUDFPublishUploadTransientThenConflictDoesNotReconcile(t *testing.T) {
	ctx := context.Background()
	plan := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	plan.PoolSize = types.Int64Value(3)
	plan.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	plan.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	var sessions, uploads, creates int
	client.CreateUDFUploadSessionMock.Set(func(context.Context) (*api.UDFUploadSession, error) {
		sessions++
		return &api.UDFUploadSession{UploadID: fmt.Sprintf("upload-%d", sessions), UploadURL: fmt.Sprintf("https://upload.example/%d", sessions)}, nil
	})
	client.UploadUDFArchiveMock.Set(func(context.Context, string, []byte) error {
		uploads++
		if uploads == 1 {
			return errors.New(`status: 500, body: {"error":"temporary upload failure"}`)
		}
		return nil
	})
	client.CreateUDFMock.Set(func(context.Context, api.UDFCreateRequest) (*api.UDF, error) {
		creates++
		return nil, errors.New(`status: 409, body: {"error":"conflict"}`)
	})
	client.GetUDFMock.Optional()

	r := &UDFResource{client: client}
	_, err := r.publishUDFFromArchive(ctx, []byte("zip bytes"), plan, 0, true, &diag.Diagnostics{})
	if err == nil || !api.IsConflict(err) {
		t.Fatalf("publish error = %v; want definitive create conflict", err)
	}
	if sessions != 2 || uploads != 2 || creates != 1 {
		t.Fatalf("sessions=%d uploads=%d creates=%d; want two fresh uploads and one POST", sessions, uploads, creates)
	}
	if got := client.GetUDFAfterCounter(); got != 0 {
		t.Fatalf("GetUDF calls=%d; upload failure must never trigger reconciliation", got)
	}
}

func TestUDFPublishUnknownCreateAdoptsOnlyMatchingSettings(t *testing.T) {
	ctx := context.Background()
	plan := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	plan.PoolSize = types.Int64Value(3)
	plan.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	plan.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	matched := testAPIUDF(api.UDFStatusBuilding, 1)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Return(&api.UDFUploadSession{UploadID: "upload-1", UploadURL: "https://upload.example/1"}, nil)
	client.UploadUDFArchiveMock.Return(nil)
	client.CreateUDFMock.Return(nil, api.NewUDFPublishOutcomeUnknownError(errors.New("connection reset")))
	client.GetUDFMock.Return(matched, nil)

	var diags diag.Diagnostics
	udf, err := (&UDFResource{client: client}).publishUDFFromArchive(ctx, []byte("zip bytes"), plan, 0, true, &diags)
	if err != nil || udf != matched {
		t.Fatalf("publish = %p, %v; want the strongly matching version adopted", udf, err)
	}
	if len(diags.Warnings()) != 1 || !strings.Contains(diags.Warnings()[0].Detail(), "source archive bytes cannot be checked") {
		t.Fatalf("reconciliation warnings = %v; want concise source-verification warning", diags)
	}
}

func TestUDFPublishUnknownRejectsMismatchedVersionAndSettings(t *testing.T) {
	ctx := context.Background()
	plan := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	plan.PoolSize = types.Int64Value(3)
	plan.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	plan.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	observed := testAPIUDF(api.UDFStatusReady, 4)
	observed.ReturnType = "Int64"

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Return(&api.UDFUploadSession{UploadID: "upload-1", UploadURL: "https://upload.example/1"}, nil)
	client.UploadUDFArchiveMock.Return(nil)
	client.CreateUDFVersionMock.Return(nil, api.NewUDFPublishOutcomeUnknownError(errors.New("connection reset")))
	client.GetUDFMock.Return(observed, nil)

	var diags diag.Diagnostics
	_, err := (&UDFResource{client: client}).publishUDFFromArchive(ctx, []byte("zip bytes"), plan, 3, false, &diags)
	var inconclusive *udfPublishOutcomeInconclusiveError
	if !errors.As(err, &inconclusive) {
		t.Fatalf("publish error = %v; want inconclusive reconciliation error", err)
	}
	if inconclusive.expectedVersion != 4 || !strings.Contains(inconclusive.reason, "different settings") {
		t.Fatalf("inconclusive error = %#v; want expected version 4 and settings mismatch", inconclusive)
	}
	if len(diags.Warnings()) != 0 {
		t.Fatalf("reconciliation warnings = %v; mismatched UDF must not be adopted", diags)
	}
}

func TestUDFPublishUnknownUpdateAdoptsExpectedNextVersion(t *testing.T) {
	ctx := context.Background()
	plan := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	plan.PoolSize = types.Int64Value(3)
	plan.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	plan.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	matched := testAPIUDF(api.UDFStatusBuilding, 2)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Return(&api.UDFUploadSession{UploadID: "upload-2", UploadURL: "https://upload.example/2"}, nil)
	client.UploadUDFArchiveMock.Return(nil)
	client.CreateUDFVersionMock.Return(nil, api.NewUDFPublishOutcomeUnknownError(errors.New("connection reset")))
	client.GetUDFMock.Return(matched, nil)

	var diags diag.Diagnostics
	udf, err := (&UDFResource{client: client}).publishUDFFromArchive(ctx, []byte("zip bytes"), plan, 1, false, &diags)
	if err != nil || udf != matched || udf.Version != 2 {
		t.Fatalf("publish = %p, %v; want expected update version 2 adopted", udf, err)
	}
}

func TestUDFResourceCreateConflictDoesNotAdoptPreexistingFunction(t *testing.T) {
	ctx := context.Background()
	archive := []byte("zip bytes")
	archivePath := filepath.Join(t.TempDir(), "geocode.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetUDFMock.Return(testAPIUDF(api.UDFStatusReady, 1), nil)
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create diagnostics contain no error for a preexisting function")
	}
	if summary := resp.Diagnostics.Errors()[0].Summary(); summary != "UDF already exists" {
		t.Fatalf("diagnostic summary = %q; want UDF already exists", summary)
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{"already exists", "terraform import clickhouse_udf.<name> geocode"} {
		if !strings.Contains(detail, want) {
			t.Errorf("diagnostic detail = %q; want to contain %q", detail, want)
		}
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("Create must not adopt preexisting UDF into state")
	}
}

// TestUDFResourceCreateAdoptsAfterUnknownResponse adopts only after the
// non-idempotent publish response is explicitly classified as unknown and a
// read confirms the expected version and settings.
func TestUDFResourceCreateAdoptsAfterUnknownResponse(t *testing.T) {
	ctx := context.Background()
	archive := []byte("zip bytes")
	archivePath := filepath.Join(t.TempDir(), "geocode.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	ready := testAPIUDF(api.UDFStatusReady, 1)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	var uploadSessions, creates int
	client.CreateUDFUploadSessionMock.Set(func(context.Context) (*api.UDFUploadSession, error) {
		uploadSessions++
		return &api.UDFUploadSession{
			UploadID:  fmt.Sprintf("upload-%d", uploadSessions),
			UploadURL: fmt.Sprintf("https://upload.example/archive-%d", uploadSessions),
		}, nil
	})
	client.UploadUDFArchiveMock.Set(func(context.Context, string, []byte) error { return nil })
	client.CreateUDFMock.Set(func(context.Context, api.UDFCreateRequest) (*api.UDF, error) {
		creates++
		return nil, api.NewUDFPublishOutcomeUnknownError(fmt.Errorf(`status: 500, body: {"error":"upstream timeout after commit"}`))
	})
	getCalls := 0
	client.GetUDFMock.Set(func(context.Context, string) (*api.UDF, error) {
		getCalls++
		if getCalls == 1 {
			return nil, errors.New(`status: 404, body: {"error":"UDF not found"}`)
		}
		return ready, nil
	})
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	if uploadSessions != 1 || creates != 1 {
		t.Fatalf("uploadSessions=%d creates=%d; want one publish followed by reconciliation", uploadSessions, creates)
	}
	if len(resp.Diagnostics.Warnings()) != 1 || resp.Diagnostics.Warnings()[0].Summary() != "UDF create was adopted after an unknown publish response" {
		t.Fatalf("Create diagnostics = %v; want one adoption warning", resp.Diagnostics)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read created state: %v", diags)
	}
	if state.Version.ValueInt64() != 1 || state.Status.ValueString() != api.UDFStatusReady {
		t.Fatalf("created state = %+v; want ready version 1", state)
	}
}

func TestUDFResourceCreateRejectsPreexistingFunction(t *testing.T) {
	ctx := context.Background()
	archivePath := filepath.Join(t.TempDir(), "geocode.zip")

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	established := testAPIUDF(api.UDFStatusReady, 5)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetUDFMock.Return(established, nil)
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create diagnostics contain no error when the conflicting function isn't ours")
	}
	if summary := resp.Diagnostics.Errors()[0].Summary(); summary != "UDF already exists" {
		t.Fatalf("diagnostic summary = %q; want UDF already exists", summary)
	}
	if got := client.CreateUDFAfterCounter(); got != 0 {
		t.Fatalf("CreateUDF calls=%d; want no publish after preflight found an existing function", got)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("Create must not adopt an established function that isn't at version 1")
	}
}

func TestUDFResourceCreateFailsAndRetainsFailedBuild(t *testing.T) {
	ctx := context.Background()
	archive := []byte("not a zip archive")
	archivePath := filepath.Join(t.TempDir(), "invalid.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	building := testAPIUDF(api.UDFStatusBuilding, 1)
	failed := testAPIUDF(api.UDFStatusError, 1)
	buildMessage := "uploaded source is not a valid ZIP archive"
	failed.Error = &buildMessage
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Expect(ctx).Return(&api.UDFUploadSession{
		UploadID:  "upload-1",
		UploadURL: "https://upload.example/archive",
	}, nil)
	client.UploadUDFArchiveMock.Expect(ctx, "https://upload.example/archive", archive).Return(nil)
	client.CreateUDFMock.Return(building, nil)
	getCalls := 0
	client.GetUDFMock.Set(func(context.Context, string) (*api.UDF, error) {
		getCalls++
		if getCalls == 1 {
			return nil, errors.New(`status: 404, body: {"error":"UDF not found"}`)
		}
		return failed, nil
	})
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create diagnostics contain no error for a failed build")
	}
	if summary := resp.Diagnostics.Errors()[0].Summary(); summary != "UDF build failed" {
		t.Fatalf("diagnostic summary = %q; want UDF build failed", summary)
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, buildMessage) ||
		!strings.Contains(detail, "was created, but version 1 failed to build") ||
		!strings.Contains(detail, "marked this resource as tainted") ||
		!strings.Contains(detail, "terraform untaint") {
		t.Fatalf("error detail = %q; want retained version guidance and build error", detail)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read failed-build state: %v", diags)
	}
	if state.Version.ValueInt64() != 1 || state.Status.ValueString() != api.UDFStatusError || state.Error.ValueString() != buildMessage {
		t.Fatalf("failed-build state = %+v; want retained errored version 1", state)
	}
	if !state.FailOnBuildError.ValueBool() {
		t.Fatalf("fail_on_build_error = %v; want true", state.FailOnBuildError)
	}
}

func TestUDFResourceCreateRetainsFailedBuildWithWarningWhenFailOnBuildErrorFalse(t *testing.T) {
	ctx := context.Background()
	archive := []byte("not a zip archive")
	archivePath := filepath.Join(t.TempDir(), "invalid.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	planModel.FailOnBuildError = types.BoolValue(false)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	building := testAPIUDF(api.UDFStatusBuilding, 1)
	failed := testAPIUDF(api.UDFStatusError, 1)
	buildMessage := "uploaded source is not a valid ZIP archive"
	failed.Error = &buildMessage
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Expect(ctx).Return(&api.UDFUploadSession{
		UploadID:  "upload-1",
		UploadURL: "https://upload.example/archive",
	}, nil)
	client.UploadUDFArchiveMock.Expect(ctx, "https://upload.example/archive", archive).Return(nil)
	client.CreateUDFMock.Return(building, nil)
	getCalls := 0
	client.GetUDFMock.Set(func(context.Context, string) (*api.UDF, error) {
		getCalls++
		if getCalls == 1 {
			return nil, errors.New(`status: 404, body: {"error":"UDF not found"}`)
		}
		return failed, nil
	})
	r.client = client

	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics contain an error: %v", resp.Diagnostics)
	}
	if len(resp.Diagnostics.Warnings()) != 1 || resp.Diagnostics.Warnings()[0].Summary() != "UDF build failed" {
		t.Fatalf("Create diagnostics = %v; want one UDF build warning", resp.Diagnostics)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read failed-build state: %v", diags)
	}
	if state.Version.ValueInt64() != 1 || state.Status.ValueString() != api.UDFStatusError || state.Error.ValueString() != buildMessage {
		t.Fatalf("failed-build state = %+v; want retained errored version 1", state)
	}
	if state.FailOnBuildError.ValueBool() {
		t.Fatal("fail_on_build_error should remain false")
	}
}

func TestUDFResourceUpdateFailsAndRetainsFailedVersion(t *testing.T) {
	ctx := context.Background()
	archive := []byte("updated invalid archive")
	archivePath := filepath.Join(t.TempDir(), "invalid-v2.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.SourceArchiveHash = types.StringValue("invalid-v2-hash")
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	building := testAPIUDF(api.UDFStatusBuilding, 2)
	failed := testAPIUDF(api.UDFStatusError, 2)
	buildMessage := "syntax error in function.py"
	failed.Error = &buildMessage
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Expect(ctx).Return(&api.UDFUploadSession{
		UploadID:  "upload-2",
		UploadURL: "https://upload.example/archive-v2",
	}, nil)
	client.UploadUDFArchiveMock.Expect(ctx, "https://upload.example/archive-v2", archive).Return(nil)
	client.CreateUDFVersionMock.Inspect(func(_ context.Context, functionName string, request api.UDFVersionCreateRequest) {
		if functionName != "geocode" || request.UploadID != "upload-2" {
			t.Errorf("create version request for %q = %+v; want geocode/upload-2", functionName, request)
		}
	}).Return(building, nil)
	client.GetUDFMock.ExpectFunctionNameParam2("geocode").Return(failed, nil)
	r.client = client

	priorModel := planModel
	priorModel.SourceArchivePath = types.StringValue("function.zip")
	priorModel.SourceArchiveHash = types.StringValue("v1-hash")
	priorModel.Version = types.Int64Value(1)
	priorModel.Status = types.StringValue(api.UDFStatusReady)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &priorModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}
	resp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: priorState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Update diagnostics contain no error for a failed build")
	}
	if summary := resp.Diagnostics.Errors()[0].Summary(); summary != "UDF build failed" {
		t.Fatalf("diagnostic summary = %q; want UDF build failed", summary)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read failed-version state: %v", diags)
	}
	if state.Version.ValueInt64() != 2 || state.Status.ValueString() != api.UDFStatusError || state.Error.ValueString() != buildMessage {
		t.Fatalf("failed-version state = %+v; want retained errored version 2", state)
	}
}

func TestUDFResourceUpdatePolicyOnlyDoesNotPublish(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	stateModel.PoolSize = types.Int64Value(3)
	stateModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	stateModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	stateModel.Version = types.Int64Value(7)
	stateModel.Status = types.StringValue(api.UDFStatusError)
	buildMessage := "previous build failed"
	stateModel.Error = types.StringValue(buildMessage)
	stateModel.CreatedAt = types.StringValue("2026-07-21T10:00:00.000Z")
	stateModel.UpdatedAt = types.StringValue("2026-07-21T10:00:00.000Z")
	stateModel.FailOnBuildError = types.BoolValue(true)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}

	planModel := stateModel
	planModel.FailOnBuildError = types.BoolValue(false)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	r.client = client

	resp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if got := client.CreateUDFVersionAfterCounter(); got != 0 {
		t.Fatalf("CreateUDFVersion calls = %d; want 0 for fail_on_build_error-only update", got)
	}
	if got := client.CreateUDFUploadSessionAfterCounter(); got != 0 {
		t.Fatalf("upload sessions = %d; want 0 for fail_on_build_error-only update", got)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read policy-only state: %v", diags)
	}
	if state.FailOnBuildError.ValueBool() {
		t.Fatal("fail_on_build_error should be false after policy-only update")
	}
	if state.Version.ValueInt64() != 7 || state.Status.ValueString() != api.UDFStatusError || state.Error.ValueString() != buildMessage {
		t.Fatalf("policy-only update changed build state: %+v", state)
	}
}

func TestUDFResourceModifyPlanPreservesBuildAttributesForPolicyOnlyChange(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	stateModel.PoolSize = types.Int64Value(3)
	stateModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	stateModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	stateModel.Version = types.Int64Value(7)
	stateModel.Status = types.StringValue(api.UDFStatusError)
	stateModel.Error = types.StringValue("previous build failed")
	stateModel.CreatedAt = types.StringValue("2026-07-21T10:00:00.000Z")
	stateModel.UpdatedAt = types.StringValue("2026-07-21T11:00:00.000Z")
	stateModel.FailOnBuildError = types.BoolValue(true)
	state := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state fixture: %v", diags)
	}

	configModel := stateModel
	configModel.FailOnBuildError = types.BoolValue(false)
	configModel.Version = types.Int64Unknown()
	configModel.Status = types.StringUnknown()
	configModel.Error = types.StringUnknown()
	configModel.CreatedAt = types.StringUnknown()
	configModel.UpdatedAt = types.StringUnknown()
	configPlan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := configPlan.Set(ctx, &configModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}
	config := tfsdk.Config{Schema: sch, Raw: configPlan.Raw}

	resp := &resource.ModifyPlanResponse{Plan: configPlan}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: config, Plan: configPlan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan diagnostics: %v", resp.Diagnostics)
	}

	var plan models.UDFResourceModel
	if diags := resp.Plan.Get(ctx, &plan); diags.HasError() {
		t.Fatalf("read modified plan: %v", diags)
	}
	if plan.Version.IsUnknown() || plan.Version.ValueInt64() != 7 {
		t.Fatalf("version = %#v; want known 7 for policy-only change", plan.Version)
	}
	if plan.Status.IsUnknown() || plan.Status.ValueString() != api.UDFStatusError {
		t.Fatalf("status = %#v; want known error for policy-only change", plan.Status)
	}
	if plan.FailOnBuildError.ValueBool() {
		t.Fatal("fail_on_build_error should be false in plan")
	}
}

func TestUDFResourceReadRefreshesFailedStatusWithoutError(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	stateModel.PoolSize = types.Int64Value(3)
	stateModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	stateModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	stateModel.Version = types.Int64Value(7)
	stateModel.Status = types.StringValue(api.UDFStatusBuilding)
	stateModel.FailOnBuildError = types.BoolValue(true)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}

	failed := testAPIUDF(api.UDFStatusError, 7)
	buildMessage := "uploaded source is not a valid ZIP archive"
	failed.Error = &buildMessage
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetUDFMock.ExpectFunctionNameParam2("geocode").Return(failed, nil)
	r.client = client

	resp := &resource.ReadResponse{State: priorState}
	r.Read(ctx, resource.ReadRequest{State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read refreshed state: %v", diags)
	}
	if state.Status.ValueString() != api.UDFStatusError || state.Error.ValueString() != buildMessage {
		t.Fatalf("refreshed state = %+v; want failed status/error", state)
	}
	if !state.FailOnBuildError.ValueBool() {
		t.Fatal("fail_on_build_error should be preserved across Read")
	}
}

func TestUDFPublishInputsChangedIgnoresFailOnBuildError(t *testing.T) {
	base := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	base.PoolSize = types.Int64Value(3)
	base.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	base.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)

	otherPolicy := base
	otherPolicy.FailOnBuildError = types.BoolValue(false)
	if udfPublishInputsChanged(base, otherPolicy) {
		t.Fatal("fail_on_build_error-only difference must not count as a publish input change")
	}

	otherHash := base
	otherHash.SourceArchiveHash = types.StringValue("next-hash")
	if !udfPublishInputsChanged(base, otherHash) {
		t.Fatal("source_archive_hash difference must count as a publish input change")
	}
}

func TestUDFResourceUpdateDoesNotAdoptAConcurrentNewerVersion(t *testing.T) {
	ctx := context.Background()
	archive := []byte("updated archive")
	archivePath := filepath.Join(t.TempDir(), "v2.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.SourceArchiveHash = types.StringValue("v2-hash")
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	created := testAPIUDF(api.UDFStatusBuilding, 2)
	concurrent := testAPIUDF(api.UDFStatusReady, 3)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Expect(ctx).Return(&api.UDFUploadSession{
		UploadID:  "upload-2",
		UploadURL: "https://upload.example/archive-v2",
	}, nil)
	client.UploadUDFArchiveMock.Expect(ctx, "https://upload.example/archive-v2", archive).Return(nil)
	client.CreateUDFVersionMock.Return(created, nil)
	client.GetUDFMock.ExpectFunctionNameParam2("geocode").Return(concurrent, nil)
	r.client = client

	priorModel := planModel
	priorModel.SourceArchivePath = types.StringValue("function.zip")
	priorModel.SourceArchiveHash = types.StringValue("v1-hash")
	priorModel.Version = types.Int64Value(1)
	priorModel.Status = types.StringValue(api.UDFStatusReady)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &priorModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}
	resp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: priorState}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Update diagnostics contain no error for a concurrent newer version")
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read update state: %v", diags)
	}
	if state.Version.ValueInt64() != 2 || state.Status.ValueString() != api.UDFStatusBuilding {
		t.Fatalf("state = version %d status %q; want Terraform-created version 2 building", state.Version.ValueInt64(), state.Status.ValueString())
	}
	if state.SourceArchiveHash.ValueString() != "v2-hash" {
		t.Fatalf("source_archive_hash = %q; want v2-hash", state.SourceArchiveHash.ValueString())
	}
}

func TestUDFResourceUpdateAdoptsAfterUnknownResponse(t *testing.T) {
	ctx := context.Background()
	archive := []byte("updated archive")
	archivePath := filepath.Join(t.TempDir(), "v2.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema
	planModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.SourceArchiveHash = types.StringValue("v2-hash")
	planModel.PoolSize = types.Int64Value(3)
	planModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	planModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	nextVersion := testAPIUDF(api.UDFStatusReady, 2)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	var uploadSessions, versions int
	client.CreateUDFUploadSessionMock.Set(func(context.Context) (*api.UDFUploadSession, error) {
		uploadSessions++
		return &api.UDFUploadSession{
			UploadID:  fmt.Sprintf("upload-%d", uploadSessions),
			UploadURL: fmt.Sprintf("https://upload.example/archive-%d", uploadSessions),
		}, nil
	})
	client.UploadUDFArchiveMock.Set(func(context.Context, string, []byte) error { return nil })
	client.CreateUDFVersionMock.Set(func(context.Context, string, api.UDFVersionCreateRequest) (*api.UDF, error) {
		versions++
		return nil, api.NewUDFPublishOutcomeUnknownError(fmt.Errorf(`status: 500, body: {"error":"upstream timeout after commit"}`))
	})
	client.GetUDFMock.ExpectFunctionNameParam2("geocode").Return(nextVersion, nil)
	r.client = client

	priorModel := planModel
	priorModel.SourceArchivePath = types.StringValue("function.zip")
	priorModel.SourceArchiveHash = types.StringValue("v1-hash")
	priorModel.Version = types.Int64Value(1)
	priorModel.Status = types.StringValue(api.UDFStatusReady)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &priorModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}
	resp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if uploadSessions != 1 || versions != 1 {
		t.Fatalf("uploadSessions=%d versions=%d; want one publish followed by reconciliation", uploadSessions, versions)
	}
	if len(resp.Diagnostics.Warnings()) != 1 || resp.Diagnostics.Warnings()[0].Summary() != "UDF version was adopted after an unknown publish response" {
		t.Fatalf("Update diagnostics = %v; want one adoption warning", resp.Diagnostics)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read updated state: %v", diags)
	}
	if state.Version.ValueInt64() != 2 || state.Status.ValueString() != api.UDFStatusReady {
		t.Fatalf("updated state = %+v; want reconciled version 2", state)
	}
}

func TestUDFResourceModifyPlanAppliesTypeAndRuntimeDefaults(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	tests := []struct {
		name               string
		runtime            string
		udfType            string
		wantPoolSize       types.Int64
		wantMaxExecution   types.Int64
		wantSandboxType    string
		wantSandboxVersion string
	}{
		{
			name:               "python executable pool",
			runtime:            api.UDFRuntimePython311,
			udfType:            api.UDFTypeExecutablePool,
			wantPoolSize:       types.Int64Value(3),
			wantMaxExecution:   types.Int64Value(10),
			wantSandboxType:    api.UDFSandboxTypeBasic,
			wantSandboxVersion: api.UDFSandboxVersionV2,
		},
		{
			name:               "native executable",
			runtime:            api.UDFRuntimeNative,
			udfType:            api.UDFTypeExecutable,
			wantPoolSize:       types.Int64Null(),
			wantMaxExecution:   types.Int64Null(),
			wantSandboxType:    api.UDFSandboxTypeBasic,
			wantSandboxVersion: api.UDFSandboxVersionV1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configModel := testUDFResourceModel(t, tc.runtime, tc.udfType)
			configModel.MaxCommandExecutionTime = types.Int64Null()
			configPlan := tfsdk.Plan{
				Schema: sch,
				Raw:    tftypes.NewValue(sch.Type().TerraformType(ctx), nil),
			}
			if diags := configPlan.Set(ctx, &configModel); diags.HasError() {
				t.Fatalf("set config fixture: %v", diags)
			}

			req := resource.ModifyPlanRequest{
				Config: tfsdk.Config{Schema: sch, Raw: configPlan.Raw},
				Plan:   configPlan,
			}
			resp := &resource.ModifyPlanResponse{Plan: configPlan}
			r.ModifyPlan(ctx, req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan diagnostics: %v", resp.Diagnostics)
			}

			var poolSize, maxExecutionTime types.Int64
			var sandboxType, sandboxVersion types.String
			if diags := resp.Plan.GetAttribute(ctx, path.Root("pool_size"), &poolSize); diags.HasError() {
				t.Fatalf("read pool_size: %v", diags)
			}
			if diags := resp.Plan.GetAttribute(ctx, path.Root("max_command_execution_time"), &maxExecutionTime); diags.HasError() {
				t.Fatalf("read max_command_execution_time: %v", diags)
			}
			if diags := resp.Plan.GetAttribute(ctx, path.Root("sandbox_type"), &sandboxType); diags.HasError() {
				t.Fatalf("read sandbox_type: %v", diags)
			}
			if diags := resp.Plan.GetAttribute(ctx, path.Root("sandbox_version"), &sandboxVersion); diags.HasError() {
				t.Fatalf("read sandbox_version: %v", diags)
			}
			if !poolSize.Equal(tc.wantPoolSize) {
				t.Errorf("pool_size = %v; want %v", poolSize, tc.wantPoolSize)
			}
			if !maxExecutionTime.Equal(tc.wantMaxExecution) {
				t.Errorf("max_command_execution_time = %v; want %v", maxExecutionTime, tc.wantMaxExecution)
			}
			if sandboxType.ValueString() != tc.wantSandboxType || sandboxVersion.ValueString() != tc.wantSandboxVersion {
				t.Errorf("sandbox = %s/%s; want %s/%s", sandboxType.ValueString(), sandboxVersion.ValueString(), tc.wantSandboxType, tc.wantSandboxVersion)
			}
		})
	}
}

func TestApplyUDFToStateMapsEveryReadableField(t *testing.T) {
	ctx := context.Background()
	returnName := "result"
	errorMessage := "build failed"
	poolSize := int64(5)
	maxExecutionTime := int64(12)
	state := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	state.SourceArchivePath = types.StringValue("/tmp/function.zip")
	state.SourceArchiveHash = types.StringValue("archive-hash")
	state.FailOnBuildError = types.BoolValue(false)

	udf := &api.UDF{
		FunctionName:            "geocode",
		Version:                 7,
		Status:                  api.UDFStatusError,
		Runtime:                 api.UDFRuntimePython311,
		Type:                    api.UDFTypeExecutablePool,
		Arguments:               []api.UDFArgument{{Name: "lat", Type: "Float64"}},
		ReturnType:              "String",
		ReturnName:              &returnName,
		PoolSize:                &poolSize,
		CommandReadTimeout:      10001,
		CommandWriteTimeout:     10002,
		MaxCommandExecutionTime: &maxExecutionTime,
		SendChunkHeader:         true,
		Format:                  "JSONEachRow",
		SandboxType:             api.UDFSandboxTypeNetEnable,
		SandboxVersion:          api.UDFSandboxVersionV3,
		Error:                   &errorMessage,
		CreatedAt:               "2026-07-21T10:00:00.000Z",
		UpdatedAt:               "2026-07-21T11:00:00.000Z",
	}

	if diags := applyUDFToState(ctx, udf, &state); diags.HasError() {
		t.Fatalf("applyUDFToState: %v", diags)
	}
	if state.Version.ValueInt64() != 7 || state.PoolSize.ValueInt64() != 5 || state.Error.ValueString() != errorMessage {
		t.Fatalf("mapped state is incomplete: %+v", state)
	}
	if state.CommandReadTimeout.ValueInt64() != 10001 || state.Format.ValueString() != "JSONEachRow" || !state.SendChunkHeader.ValueBool() {
		t.Fatalf("writable fields were not refreshed: %+v", state)
	}
	if state.SourceArchivePath.ValueString() != "/tmp/function.zip" || state.SourceArchiveHash.ValueString() != "archive-hash" {
		t.Fatalf("write-only source attributes were not preserved: %+v", state)
	}
	if state.FailOnBuildError.ValueBool() {
		t.Fatalf("fail_on_build_error was not preserved: %+v", state)
	}

	var arguments []models.UDFArgumentModel
	if diags := state.Arguments.ElementsAs(ctx, &arguments, false); diags.HasError() {
		t.Fatalf("decode arguments: %v", diags)
	}
	if len(arguments) != 1 || arguments[0].Name.ValueString() != "lat" {
		t.Fatalf("arguments = %+v; unexpected", arguments)
	}
}

func TestUDFPollingHandlesTransitionsAndTerminalErrors(t *testing.T) {
	t.Run("building to ready", func(t *testing.T) {
		calls := 0
		udf, err := pollUDFVersion(context.Background(), time.Second, time.Millisecond, 2, func(context.Context) (*api.UDF, error) {
			calls++
			status := api.UDFStatusBuilding
			if calls == 2 {
				status = api.UDFStatusReady
			}
			return &api.UDF{FunctionName: "geocode", Version: 2, Status: status}, nil
		})
		if err != nil || udf.Status != api.UDFStatusReady || calls != 2 {
			t.Fatalf("pollUDF = %+v, %v after %d calls", udf, err, calls)
		}
	})

	t.Run("new intermediate status to ready", func(t *testing.T) {
		calls := 0
		udf, err := pollUDFVersion(context.Background(), time.Second, time.Millisecond, 2, func(context.Context) (*api.UDF, error) {
			calls++
			status := "queued"
			if calls == 2 {
				status = api.UDFStatusReady
			}
			return &api.UDF{FunctionName: "geocode", Version: 2, Status: status}, nil
		})
		if err != nil || udf.Status != api.UDFStatusReady || calls != 2 {
			t.Fatalf("pollUDFVersion = %+v, %v after %d calls", udf, err, calls)
		}
	})

	t.Run("update ignores the previous ready version", func(t *testing.T) {
		calls := 0
		udf, err := pollUDFVersion(context.Background(), time.Second, time.Millisecond, 5, func(context.Context) (*api.UDF, error) {
			calls++
			if calls == 1 {
				return &api.UDF{FunctionName: "geocode", Version: 4, Status: api.UDFStatusReady}, nil
			}
			status := api.UDFStatusBuilding
			if calls == 3 {
				status = api.UDFStatusReady
			}
			return &api.UDF{FunctionName: "geocode", Version: 5, Status: status}, nil
		})
		if err != nil || udf.Version != 5 || udf.Status != api.UDFStatusReady || calls != 3 {
			t.Fatalf("pollUDFVersion = %+v, %v after %d calls", udf, err, calls)
		}
	})

	t.Run("build error preserves server state", func(t *testing.T) {
		message := "syntax error"
		udf, err := pollUDFVersion(context.Background(), time.Second, time.Millisecond, 3, func(context.Context) (*api.UDF, error) {
			return &api.UDF{FunctionName: "geocode", Version: 3, Status: api.UDFStatusError, Error: &message}, nil
		})
		if err == nil || udf == nil || udf.Error == nil || *udf.Error != message {
			t.Fatalf("pollUDFVersion = %+v, %v; want terminal state and error", udf, err)
		}
	})

	t.Run("non-transient read error stops immediately", func(t *testing.T) {
		calls := 0
		_, err := pollUDFVersion(context.Background(), time.Second, time.Millisecond, 1, func(context.Context) (*api.UDF, error) {
			calls++
			return nil, errors.New("status: 401, body: unauthorized")
		})
		if err == nil || calls != 1 {
			t.Fatalf("pollUDFVersion err = %v after %d calls; want immediate failure", err, calls)
		}
	})

	t.Run("newer version is not mistaken for the requested version", func(t *testing.T) {
		udf, err := pollUDFVersion(context.Background(), time.Second, time.Millisecond, 5, func(context.Context) (*api.UDF, error) {
			return &api.UDF{FunctionName: "geocode", Version: 6, Status: api.UDFStatusReady}, nil
		})
		if err == nil || udf != nil || !strings.Contains(err.Error(), "waiting for version 5") {
			t.Fatalf("pollUDFVersion = %+v, %v; want a version-race error without adopting version 6", udf, err)
		}
	})
}

func TestRetryUDFMutationHandlesTransientConflicts(t *testing.T) {
	t.Run("conflict then accepted", func(t *testing.T) {
		calls := 0
		err := retryUDFMutation(context.Background(), time.Second, time.Millisecond, "detach UDF", func(context.Context) error {
			calls++
			if calls == 1 {
				return errors.New("status: 409, body: transition in progress")
			}
			return nil
		})
		if err != nil || calls != 2 {
			t.Fatalf("retryUDFMutation = %v after %d calls; want success after 2", err, calls)
		}
	})

	t.Run("permanent error stops immediately", func(t *testing.T) {
		calls := 0
		err := retryUDFMutation(context.Background(), time.Second, time.Millisecond, "delete UDF", func(context.Context) error {
			calls++
			return errors.New("status: 403, body: forbidden")
		})
		if err == nil || calls != 1 {
			t.Fatalf("retryUDFMutation = %v after %d calls; want immediate failure", err, calls)
		}
	})
}

func TestShouldRetryUDFUploadStopsForCanceledContext(t *testing.T) {
	if shouldRetryUDFUpload(context.Canceled) || shouldRetryUDFUpload(context.DeadlineExceeded) {
		t.Fatal("canceled upload requests must not start a fresh session")
	}
}

func TestUDFBuildErrorKeepsOnlyActionableCause(t *testing.T) {
	raw := "Build Failure: failed at ECS build task: Task completed with non-zero exit code: 1\nReason: {\"success\":false,\"error\":\"panic: [X] Failed to extract source zip file: main.py was not found in the zip archive\"}"
	detail := udfBuildFailureDetail("my_fn", 1, true, types.Int64Null(), true, raw)
	if strings.Contains(detail, "Build Failure:") || strings.Contains(detail, "ECS build task") || strings.Contains(detail, `{"success":false`) {
		t.Fatalf("build detail = %q; must not expose internal wrappers or raw JSON", detail)
	}
	if strings.Contains(detail, "panic:") || strings.Contains(detail, "[X]") || !strings.Contains(detail, "Failed to extract source zip file") || !strings.Contains(detail, "main.py was not found") {
		t.Fatalf("build detail = %q; want the actionable inner cause without wrapper markers", detail)
	}
}

func TestAddUDFWriteErrorMessages(t *testing.T) {
	ctx := context.Background()

	t.Run("conflict", func(t *testing.T) {
		var diags diag.Diagnostics
		addUDFWriteError(ctx, &diags, "creating", "geocode", errors.New(`status: 409, body: {"error":"UDF geocode already exists"}`))
		if got := diags.Errors()[0].Summary(); got != "UDF already exists" {
			t.Fatalf("summary = %q; want UDF already exists", got)
		}
		detail := firstDiagnosticDetail(t, diags)
		for _, want := range []string{"geocode", "UDF geocode already exists", "terraform import clickhouse_udf.<name> geocode"} {
			if !strings.Contains(detail, want) {
				t.Errorf("detail = %q; want to contain %q", detail, want)
			}
		}
	})

	t.Run("gone describes the expired upload session", func(t *testing.T) {
		var diags diag.Diagnostics
		addUDFWriteError(ctx, &diags, "creating", "geocode", errors.New(`status: 410, body: {"error":"UDF source archive is unavailable"}`))
		detail := firstDiagnosticDetail(t, diags)
		for _, want := range []string{"UDF source archive is unavailable", "source upload expired", "retried with a new upload"} {
			if !strings.Contains(detail, want) {
				t.Errorf("detail = %q; want to contain %q", detail, want)
			}
		}
	})

	t.Run("forbidden names the gated settings", func(t *testing.T) {
		var diags diag.Diagnostics
		addUDFWriteError(ctx, &diags, "creating", "geocode", errors.New(`status: 403, body: {"error":"The requested UDF runtime or sandbox is not enabled for this organization"}`))
		if got := diags.Errors()[0].Summary(); got != "UDF runtime or sandbox is not enabled for this organization" {
			t.Fatalf("summary = %q", got)
		}
		detail := firstDiagnosticDetail(t, diags)
		for _, want := range []string{"geocode", "The requested UDF runtime or sandbox is not enabled for this organization", "supported runtime or sandbox", "ClickHouse support"} {
			if !strings.Contains(detail, want) {
				t.Errorf("detail = %q; want to contain %q", detail, want)
			}
		}
		if strings.Contains(strings.ToLower(detail), "credential") {
			t.Errorf("detail = %q; must not blame credentials, the default path is never gated", detail)
		}
	})

	t.Run("server error hides service details", func(t *testing.T) {
		var diags diag.Diagnostics
		raw := `status: 500, body: {"error":"upstream timeout"}`
		addUDFWriteError(ctx, &diags, "creating", "geocode", errors.New(raw))
		detail := firstDiagnosticDetail(t, diags)
		if !strings.Contains(detail, "HTTP status 500") || strings.Contains(detail, "upstream") || strings.Contains(detail, "body:") {
			t.Errorf("detail = %q; want a generic server error without service details", detail)
		}
	})
}

func TestUDFWriteDiagnosticsUseGenericGuidanceWithoutDocumentedClassification(t *testing.T) {
	ctx := context.Background()

	t.Run("forbidden", func(t *testing.T) {
		var diags diag.Diagnostics
		addUDFWriteError(ctx, &diags, "creating", "geocode", errors.New(`status: 403, body: {"error":"forbidden"}`))
		if got := diags.Errors()[0].Summary(); got != "Error creating UDF" {
			t.Fatalf("summary = %q; want generic create error", got)
		}
		if strings.Contains(diags.Errors()[0].Detail(), "runtime or sandbox") {
			t.Fatalf("detail = %q; must not claim a runtime gate without documented body/code", diags.Errors()[0].Detail())
		}
	})

	t.Run("unprocessable entity", func(t *testing.T) {
		var diags diag.Diagnostics
		plan := models.UDFAttachmentResourceModel{
			FunctionName: types.StringValue("geocode"),
			ServiceID:    types.StringValue("11111111-1111-1111-1111-111111111111"),
			Version:      types.Int64Null(),
		}
		addUDFAttachmentWriteError(ctx, &diags, "creating", plan, errors.New(`status: 422, body: {"error":"validation failed"}`))
		if got := diags.Errors()[0].Summary(); got != "Error creating UDF attachment" {
			t.Fatalf("summary = %q; want generic attachment error", got)
		}
	})

	t.Run("update conflict does not suggest import", func(t *testing.T) {
		var diags diag.Diagnostics
		addUDFWriteError(ctx, &diags, "updating", "geocode", errors.New(`status: 409, body: {"error":"conflict"}`))
		detail := diags.Errors()[0].Detail()
		if strings.Contains(detail, "terraform import") || !strings.Contains(detail, "refresh") {
			t.Fatalf("detail = %q; want refresh guidance without import", detail)
		}
	})
}

func TestSafeUDFErrorDoesNotExposeUnstructuredTransportDetails(t *testing.T) {
	secret := "dial tcp 10.20.30.40:443: connect: connection refused"
	if got := safeUDFError(errors.New(secret)); got == secret || strings.Contains(got, "10.20.30.40") {
		t.Fatalf("safeUDFError = %q; must not expose unstructured transport details", got)
	}

	serverErr := errors.New(`status: 503, body: {"requestId":"req-123","error":"internal dependency failed"}`)
	got := safeUDFError(serverErr)
	if !strings.Contains(got, "HTTP status 503") || !strings.Contains(got, "req-123") || strings.Contains(got, "dependency") {
		t.Fatalf("safeUDFError = %q; want generic status and request ID without service details", got)
	}
}

func TestAddUDFDeleteErrorMessages(t *testing.T) {
	ctx := context.Background()

	t.Run("timeout after retries names the stuck build", func(t *testing.T) {
		mutationErr := retryUDFMutation(ctx, 20*time.Millisecond, time.Millisecond, "delete UDF", func(context.Context) error {
			return errors.New(`status: 409, body: {"error":"A UDF version is still building and the UDF cannot be deleted"}`)
		})
		if mutationErr == nil {
			t.Fatalf("retryUDFMutation = nil; want a timeout error after exhausting the retry budget")
		}
		var diags diag.Diagnostics
		addUDFDeleteError(ctx, &diags, "geocode", mutationErr)
		detail := firstDiagnosticDetail(t, diags)
		for _, want := range []string{"geocode", "A UDF version is still building and the UDF cannot be deleted"} {
			if !strings.Contains(detail, want) {
				t.Errorf("detail = %q; want to contain %q", detail, want)
			}
		}
	})

	t.Run("other server failure hides service details", func(t *testing.T) {
		var diags diag.Diagnostics
		raw := `status: 500, body: {"error":"boom"}`
		addUDFDeleteError(ctx, &diags, "geocode", errors.New(raw))
		detail := firstDiagnosticDetail(t, diags)
		if !strings.Contains(detail, "HTTP status 500") || strings.Contains(detail, "boom") || strings.Contains(detail, "body:") {
			t.Errorf("detail = %q; want a generic server error", detail)
		}
	})
}

func TestUDFResourceReadErrorHidesServerDetails(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutable)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	raw := `status: 500, body: {"error":"database is unreachable"}`
	client.GetUDFMock.ExpectFunctionNameParam2("geocode").Return(nil, errors.New(raw))
	r.client = client

	resp := &resource.ReadResponse{State: priorState}
	r.Read(ctx, resource.ReadRequest{State: priorState}, resp)

	detail := firstDiagnosticDetail(t, resp.Diagnostics)
	if !strings.Contains(detail, `Could not read UDF "geocode"`) || !strings.Contains(detail, "HTTP status 500") || strings.Contains(detail, "database") || strings.Contains(detail, "body:") {
		t.Errorf("detail = %q; want the resource context and a generic server error", detail)
	}
}

type stubTFState struct{}

func (stubTFState) Set(context.Context, any) diag.Diagnostics { return nil }

func TestFinishUDFWriteWaitErrorSurfacesCauseAndLastStatus(t *testing.T) {
	ctx := context.Background()
	state := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutable)
	state.Version = types.Int64Value(2)
	state.Status = types.StringValue(api.UDFStatusBuilding)

	var diags diag.Diagnostics
	(&UDFResource{}).finishUDFWrite(
		ctx,
		&state,
		&api.UDF{
			FunctionName: "geocode",
			Version:      2,
			Status:       api.UDFStatusBuilding,
		},
		fmt.Errorf("wait for UDF build: %w", context.DeadlineExceeded),
		false,
		types.Int64Value(1),
		&diags,
		stubTFState{},
	)

	if got := diags.Errors()[0].Summary(); got != "Error waiting for UDF build" {
		t.Fatalf("summary = %q", got)
	}
	detail := firstDiagnosticDetail(t, diags)
	for _, want := range []string{
		`Could not finish waiting for UDF "geocode" version 2 to build`,
		`Last known status: "building"`,
		"saved this status in state",
	} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail = %q; want to contain %q", detail, want)
		}
	}
	if strings.Contains(detail, "keep waiting") {
		t.Fatalf("timeout detail = %q; must not claim a second apply keeps polling", detail)
	}
}

func TestUDFBuildFailureDetailCreateVsUpdate(t *testing.T) {
	createTainted := udfBuildFailureDetail("my_fn", 1, true, types.Int64Null(), true, "boom")
	if !strings.Contains(createTainted, `UDF "my_fn" was created, but version 1 failed to build`) ||
		!strings.Contains(createTainted, "marked this resource as tainted") ||
		!strings.Contains(createTainted, "terraform untaint") ||
		!strings.Contains(createTainted, "publish version 2") ||
		!strings.Contains(createTainted, "Build error: boom") {
		t.Fatalf("create/tainted detail = %q", createTainted)
	}

	createWarning := udfBuildFailureDetail("my_fn", 1, true, types.Int64Null(), false, "boom")
	if strings.Contains(createWarning, "marked this resource as tainted") {
		t.Fatalf("create/warning detail should not claim tainting: %q", createWarning)
	}
	if strings.Contains(createWarning, "terraform untaint") {
		t.Fatalf("create/warning detail must not tell the operator to untaint: %q", createWarning)
	}
	if !strings.Contains(createWarning, "update source_archive_hash") {
		t.Fatalf("create/warning detail should point at the in-place recovery path: %q", createWarning)
	}

	updateWithPrior := udfBuildFailureDetail("my_fn", 3, false, types.Int64Value(2), false, "boom")
	if !strings.Contains(updateWithPrior, `UDF "my_fn" version 3 failed to build`) ||
		!strings.Contains(updateWithPrior, "Version 2 is still in use") ||
		!strings.Contains(updateWithPrior, "update source_archive_hash") ||
		!strings.Contains(updateWithPrior, "will not retry the build") {
		t.Fatalf("update-with-prior detail = %q", updateWithPrior)
	}
	if strings.Contains(updateWithPrior, "tainted") {
		t.Fatalf("update detail must not mention taint: %q", updateWithPrior)
	}

	updateWithoutPrior := udfBuildFailureDetail("my_fn", 1, false, types.Int64Null(), false, "boom")
	if strings.Contains(updateWithoutPrior, "is still in use") {
		t.Fatalf("update detail must drop the prior-version clause when it is unknown: %q", updateWithoutPrior)
	}
	if strings.Contains(updateWithoutPrior, "tainted") {
		t.Fatalf("update detail must not mention taint: %q", updateWithoutPrior)
	}
}

func TestUDFResourceUpdatePathOnlyChangeDoesNotPublish(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	stateModel.PoolSize = types.Int64Value(3)
	stateModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	stateModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	stateModel.SourceArchivePath = types.StringValue("old/path/function.zip")
	stateModel.Version = types.Int64Value(4)
	stateModel.Status = types.StringValue(api.UDFStatusReady)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}

	planModel := stateModel
	planModel.SourceArchivePath = types.StringValue("new/path/function.zip")
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	r.client = client

	resp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if got := client.CreateUDFVersionAfterCounter(); got != 0 {
		t.Fatalf("CreateUDFVersion calls = %d; want 0 for a path-only change", got)
	}
	if got := client.CreateUDFUploadSessionAfterCounter(); got != 0 {
		t.Fatalf("upload sessions = %d; want 0 for a path-only change", got)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.SourceArchivePath.ValueString() != "new/path/function.zip" {
		t.Fatalf("source_archive_path = %q; want the plan's new path adopted", state.SourceArchivePath.ValueString())
	}
	if state.Version.ValueInt64() != 4 || state.Status.ValueString() != api.UDFStatusReady {
		t.Fatalf("state = %+v; want version/status unchanged for a path-only change", state)
	}
}

func TestUDFResourceModifyPlanFreezesBuildAttributesForPathOnlyChange(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	stateModel.PoolSize = types.Int64Value(3)
	stateModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	stateModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	stateModel.SourceArchivePath = types.StringValue("old/path.zip")
	stateModel.Version = types.Int64Value(5)
	stateModel.Status = types.StringValue(api.UDFStatusReady)
	stateModel.CreatedAt = types.StringValue("2026-07-21T10:00:00.000Z")
	stateModel.UpdatedAt = types.StringValue("2026-07-21T10:00:00.000Z")
	state := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state fixture: %v", diags)
	}

	configModel := stateModel
	configModel.SourceArchivePath = types.StringValue("new/path.zip")
	configModel.Version = types.Int64Unknown()
	configModel.Status = types.StringUnknown()
	configModel.Error = types.StringUnknown()
	configModel.CreatedAt = types.StringUnknown()
	configModel.UpdatedAt = types.StringUnknown()
	configPlan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := configPlan.Set(ctx, &configModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}
	config := tfsdk.Config{Schema: sch, Raw: configPlan.Raw}

	resp := &resource.ModifyPlanResponse{Plan: configPlan}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: config, Plan: configPlan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan diagnostics: %v", resp.Diagnostics)
	}

	var plan models.UDFResourceModel
	if diags := resp.Plan.Get(ctx, &plan); diags.HasError() {
		t.Fatalf("read modified plan: %v", diags)
	}
	if plan.Version.IsUnknown() || plan.Version.ValueInt64() != 5 {
		t.Fatalf("version = %#v; want known 5 for a path-only change", plan.Version)
	}
	if plan.SourceArchivePath.ValueString() != "new/path.zip" {
		t.Fatalf("source_archive_path = %q; want the new path preserved in the plan", plan.SourceArchivePath.ValueString())
	}
}

func TestUDFReturnNameStaysNullForUnrelatedChange(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	returnName, ok := sch.Attributes["return_name"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("return_name attribute = %#v; want schema.StringAttribute", sch.Attributes["return_name"])
	}
	if len(returnName.PlanModifiers) == 0 {
		t.Fatalf("return_name has no plan modifiers; want UseStateForUnknown so an unrelated change does not publish a new version")
	}

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	state := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state fixture: %v", diags)
	}

	req := planmodifier.StringRequest{
		Path:        path.Root("return_name"),
		State:       state,
		StateValue:  types.StringNull(),
		PlanValue:   types.StringUnknown(),
		ConfigValue: types.StringNull(),
	}
	resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
	for _, modifier := range returnName.PlanModifiers {
		modifier.PlanModifyString(ctx, req, resp)
	}
	if resp.PlanValue.IsUnknown() || !resp.PlanValue.IsNull() {
		t.Fatalf("return_name plan value = %#v; want known null after an unrelated change", resp.PlanValue)
	}

	plan := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	plan.ReturnName = resp.PlanValue
	if udfPublishInputsChanged(plan, stateModel) {
		t.Fatalf("udfPublishInputsChanged = true; want false when return_name resolves to null on both sides")
	}
}

func TestUDFResourceUpdateAdoptsImportedSourceWithoutPublishing(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	stateModel.PoolSize = types.Int64Value(3)
	stateModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	stateModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	stateModel.SourceArchivePath = types.StringNull()
	stateModel.SourceArchiveHash = types.StringNull()
	stateModel.Version = types.Int64Value(1)
	stateModel.Status = types.StringValue(api.UDFStatusReady)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}

	planModel := stateModel
	planModel.SourceArchivePath = types.StringValue("function.zip")
	planModel.SourceArchiveHash = types.StringValue("hash")
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	r.client = client

	resp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if got := client.CreateUDFVersionAfterCounter(); got != 0 {
		t.Fatalf("CreateUDFVersion calls = %d; want 0 when adopting an imported source", got)
	}
	if len(resp.Diagnostics.Warnings()) != 1 || resp.Diagnostics.Warnings()[0].Summary() != "UDF source could not be verified after import" {
		t.Fatalf("Update diagnostics = %v; want one import-adoption warning", resp.Diagnostics)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.SourceArchivePath.ValueString() != "function.zip" || state.SourceArchiveHash.ValueString() != "hash" {
		t.Fatalf("state = %+v; want the plan's path and hash adopted", state)
	}
	if state.Version.ValueInt64() != 1 || state.Status.ValueString() != api.UDFStatusReady {
		t.Fatalf("state = %+v; want version/status unchanged when adopting an imported source", state)
	}
}

func TestUDFResourceUpdatePublishesWhenImportedAndAnotherInputChanged(t *testing.T) {
	ctx := context.Background()
	archive := []byte("zip bytes")
	archivePath := filepath.Join(t.TempDir(), "function.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	stateModel.PoolSize = types.Int64Value(3)
	stateModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	stateModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	stateModel.SourceArchivePath = types.StringNull()
	stateModel.SourceArchiveHash = types.StringNull()
	stateModel.Version = types.Int64Value(1)
	stateModel.Status = types.StringValue(api.UDFStatusReady)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}

	planModel := stateModel
	planModel.SourceArchivePath = types.StringValue(archivePath)
	planModel.SourceArchiveHash = types.StringValue("hash")
	planModel.ReturnType = types.StringValue("Int64")
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	created := testAPIUDF(api.UDFStatusReady, 2)
	created.ReturnType = "Int64"
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Expect(ctx).Return(&api.UDFUploadSession{
		UploadID:  "upload-1",
		UploadURL: "https://upload.example/archive",
	}, nil)
	client.UploadUDFArchiveMock.Expect(ctx, "https://upload.example/archive", archive).Return(nil)
	client.CreateUDFVersionMock.Return(created, nil)
	client.GetUDFMock.ExpectFunctionNameParam2("geocode").Return(created, nil)
	r.client = client

	resp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if got := client.CreateUDFVersionAfterCounter(); got != 1 {
		t.Fatalf("CreateUDFVersion calls = %d; want 1 when a real publish input changed alongside the imported hash", got)
	}

	var state models.UDFResourceModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.Version.ValueInt64() != 2 {
		t.Fatalf("version = %d; want 2 after publish", state.Version.ValueInt64())
	}
}

func TestUDFResourceUpdateHashChangeStillPublishes(t *testing.T) {
	ctx := context.Background()
	archive := []byte("zip bytes v2")
	archivePath := filepath.Join(t.TempDir(), "function.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}

	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutablePool)
	stateModel.PoolSize = types.Int64Value(3)
	stateModel.SandboxType = types.StringValue(api.UDFSandboxTypeBasic)
	stateModel.SandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
	stateModel.SourceArchivePath = types.StringValue(archivePath)
	stateModel.SourceArchiveHash = types.StringValue("v1-hash")
	stateModel.Version = types.Int64Value(1)
	stateModel.Status = types.StringValue(api.UDFStatusReady)
	priorState := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set prior state fixture: %v", diags)
	}

	planModel := stateModel
	planModel.SourceArchiveHash = types.StringValue("v2-hash")
	plan := tfsdk.Plan{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan fixture: %v", diags)
	}

	created := testAPIUDF(api.UDFStatusReady, 2)
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateUDFUploadSessionMock.Expect(ctx).Return(&api.UDFUploadSession{
		UploadID:  "upload-2",
		UploadURL: "https://upload.example/archive-v2",
	}, nil)
	client.UploadUDFArchiveMock.Expect(ctx, "https://upload.example/archive-v2", archive).Return(nil)
	client.CreateUDFVersionMock.Return(created, nil)
	client.GetUDFMock.ExpectFunctionNameParam2("geocode").Return(created, nil)
	r.client = client

	resp := &resource.UpdateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: priorState}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if got := client.CreateUDFVersionAfterCounter(); got != 1 {
		t.Fatalf("CreateUDFVersion calls = %d; want 1 for a real hash change", got)
	}
}

func TestUDFResourceDeleteSucceeds(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutable)
	state := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.DeleteUDFMock.ExpectFunctionNameParam2("geocode").Return(nil)
	r.client = client

	resp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diagnostics: %v", resp.Diagnostics)
	}
}

func TestUDFResourceDeleteReportsUnrecoverableError(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	stateModel := testUDFResourceModel(t, api.UDFRuntimePython311, api.UDFTypeExecutable)
	state := tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state fixture: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.DeleteUDFMock.ExpectFunctionNameParam2("geocode").Return(errors.New(`status: 403, body: {"error":"forbidden"}`))
	r.client = client

	resp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Delete diagnostics contain no error for a non-retryable failure")
	}
	detail := firstDiagnosticDetail(t, resp.Diagnostics)
	if !strings.Contains(detail, "forbidden") {
		t.Errorf("detail = %q; want the API's own cause", detail)
	}
}

func TestUDFResourceImportStateSetsFailOnBuildErrorDefault(t *testing.T) {
	ctx := context.Background()
	r := NewUDFResource().(*UDFResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	sch := schemaResp.Schema

	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "geocode"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diagnostics: %v", resp.Diagnostics)
	}

	var functionName types.String
	if diags := resp.State.GetAttribute(ctx, path.Root("function_name"), &functionName); diags.HasError() {
		t.Fatalf("read function_name: %v", diags)
	}
	if functionName.ValueString() != "geocode" {
		t.Fatalf("function_name = %q; want geocode", functionName.ValueString())
	}

	var failOnBuildError types.Bool
	if diags := resp.State.GetAttribute(ctx, path.Root("fail_on_build_error"), &failOnBuildError); diags.HasError() {
		t.Fatalf("read fail_on_build_error: %v", diags)
	}
	if !failOnBuildError.ValueBool() {
		t.Fatal("fail_on_build_error should default to true after import")
	}
}

func testUDFResourceModel(t *testing.T, runtime, udfType string) models.UDFResourceModel {
	t.Helper()
	argumentType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"name": types.StringType,
		"type": types.StringType,
	}}
	arguments, diags := types.ListValue(argumentType, []attr.Value{})
	if diags.HasError() {
		t.Fatalf("create argument fixture: %v", diags)
	}
	return models.UDFResourceModel{
		FunctionName:            types.StringValue("geocode"),
		Runtime:                 types.StringValue(runtime),
		Arguments:               arguments,
		ReturnType:              types.StringValue("String"),
		Type:                    types.StringValue(udfType),
		PoolSize:                types.Int64Null(),
		SourceArchivePath:       types.StringValue("function.zip"),
		SourceArchiveHash:       types.StringValue("hash"),
		ReturnName:              types.StringNull(),
		CommandReadTimeout:      types.Int64Value(10000),
		CommandWriteTimeout:     types.Int64Value(10000),
		MaxCommandExecutionTime: types.Int64Value(10),
		SendChunkHeader:         types.BoolValue(false),
		Format:                  types.StringValue("TabSeparated"),
		SandboxType:             types.StringNull(),
		SandboxVersion:          types.StringNull(),
		FailOnBuildError:        types.BoolValue(true),
		Version:                 types.Int64Null(),
		Status:                  types.StringNull(),
		Error:                   types.StringNull(),
		CreatedAt:               types.StringNull(),
		UpdatedAt:               types.StringNull(),
	}
}

func testAPIUDF(status string, version int64) *api.UDF {
	poolSize := int64(3)
	maxExecutionTime := int64(10)
	return &api.UDF{
		FunctionName:            "geocode",
		Version:                 version,
		Status:                  status,
		Runtime:                 api.UDFRuntimePython311,
		Type:                    api.UDFTypeExecutablePool,
		Arguments:               []api.UDFArgument{},
		ReturnType:              "String",
		PoolSize:                &poolSize,
		CommandReadTimeout:      10000,
		CommandWriteTimeout:     10000,
		MaxCommandExecutionTime: &maxExecutionTime,
		Format:                  "TabSeparated",
		SandboxType:             api.UDFSandboxTypeBasic,
		SandboxVersion:          api.UDFSandboxVersionV2,
		CreatedAt:               "2026-07-21T10:00:00.000Z",
		UpdatedAt:               "2026-07-21T10:00:00.000Z",
	}
}
