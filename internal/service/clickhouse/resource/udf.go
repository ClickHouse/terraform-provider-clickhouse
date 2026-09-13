package resource

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

var (
	_ resource.Resource                   = (*UDFResource)(nil)
	_ resource.ResourceWithConfigure      = (*UDFResource)(nil)
	_ resource.ResourceWithImportState    = (*UDFResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*UDFResource)(nil)
	_ resource.ResourceWithValidateConfig = (*UDFResource)(nil)
)

//go:embed descriptions/udf.md
var udfResourceDescription string

const udfBuildTimeout = 30 * time.Minute

const udfWriteMaxAttempts = 3

const udfReconcileTimeout = 10 * time.Second

const udfSafeDetailMaxLength = 2000

var udfNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

func NewUDFResource() resource.Resource {
	return &UDFResource{}
}

type UDFResource struct {
	client api.Client
}

func (r *UDFResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_udf"
}

func (r *UDFResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: udfResourceDescription,
		Attributes: map[string]schema.Attribute{
			"function_name": schema.StringAttribute{
				Description: "Name of the UDF. Unique within the organization.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(udfNamePattern, "must start with a letter and contain only letters, numbers, or underscores"),
				},
			},
			"runtime": schema.StringAttribute{
				Description: "Runtime used to execute the UDF command.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(api.UDFRuntimes...),
				},
			},
			"arguments": schema.ListNestedAttribute{
				Description: "Arguments passed to the UDF command.",
				Required:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Name of the argument. Required for Native and JSONEachRow formats.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.RegexMatches(udfNamePattern, "must start with a letter and contain only letters, numbers, or underscores"),
							},
						},
						"type": schema.StringAttribute{
							Description: "ClickHouse data type of the argument.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
					},
				},
			},
			"return_type": schema.StringAttribute{
				Description: "ClickHouse data type of the returned value.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"type": schema.StringAttribute{
				Description: "Executable UDF type.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(api.UDFTypes...),
				},
			},
			"pool_size": schema.Int64Attribute{
				Description: "Command pool size for executable_pool UDFs.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"source_archive_path": schema.StringAttribute{
				Description: "Local path to the ZIP archive uploaded by the provider.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"source_archive_hash": schema.StringAttribute{
				Description: "Base64-encoded SHA-256 of the ZIP archive. A change publishes a new UDF version.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"return_name": schema.StringAttribute{
				Description: "Name of the returned value, or null when unnamed.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(udfNamePattern, "must start with a letter and contain only letters, numbers, or underscores"),
				},
			},
			"command_read_timeout": schema.Int64Attribute{
				Description: "Command stdout read timeout in milliseconds.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(10000),
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"command_write_timeout": schema.Int64Attribute{
				Description: "Command stdin write timeout in milliseconds.",
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(10000),
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"max_command_execution_time": schema.Int64Attribute{
				Description: "Maximum command execution time in seconds for executable_pool UDFs.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"send_chunk_header": schema.BoolAttribute{
				Description: "Whether ClickHouse sends a row-count chunk header.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"format": schema.StringAttribute{
				Description: "Input and output format used by the UDF command.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("TabSeparated"),
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"sandbox_type": schema.StringAttribute{
				Description: "Sandbox isolation level.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(api.UDFSandboxTypes...),
				},
			},
			"sandbox_version": schema.StringAttribute{
				Description: "Sandbox runtime version.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(api.UDFSandboxVersions...),
				},
			},
			"fail_on_build_error": schema.BoolAttribute{
				Description: "When true (the default), a failed build fails the apply. The failed version stays in state. Changing this setting does not create a new UDF version.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"version": schema.Int64Attribute{
				Description: "Version number of the UDF.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "Build state of this UDF version.",
				Computed:    true,
			},
			"error": schema.StringAttribute{
				Description: "Build error, or null when no build error is present.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "Creation timestamp.",
				Computed:    true,
			},
			"updated_at": schema.StringAttribute{
				Description: "Last-update timestamp.",
				Computed:    true,
			},
		},
	}
}

func (r *UDFResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError(
			"ClickHouse Cloud API not configured",
			"This resource requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).",
		)
		return
	}
	r.client = providerData.API
}

func (r *UDFResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.BetaWarning("clickhouse_udf", &resp.Diagnostics)

	var config models.UDFResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.Type.IsUnknown() || config.Runtime.IsUnknown() {
		return
	}

	if config.Type.ValueString() == api.UDFTypeExecutable && !config.PoolSize.IsNull() && !config.PoolSize.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("pool_size"),
			"pool_size is not valid for executable UDFs",
			"Remove pool_size or change type to executable_pool.",
		)
	}
	if config.Type.ValueString() == api.UDFTypeExecutable &&
		!config.MaxCommandExecutionTime.IsNull() &&
		!config.MaxCommandExecutionTime.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("max_command_execution_time"),
			"max_command_execution_time is not valid for executable UDFs",
			"Remove max_command_execution_time or change type to executable_pool.",
		)
	}

	if config.Runtime.ValueString() == api.UDFRuntimeNative {
		if !config.SandboxType.IsNull() && !config.SandboxType.IsUnknown() && config.SandboxType.ValueString() != api.UDFSandboxTypeBasic {
			resp.Diagnostics.AddAttributeError(
				path.Root("sandbox_type"),
				"Native UDFs use the basic sandbox",
				"Set sandbox_type to basic or omit it. The API always uses the basic sandbox for native UDFs.",
			)
		}
		if !config.SandboxVersion.IsNull() && !config.SandboxVersion.IsUnknown() && config.SandboxVersion.ValueString() != api.UDFSandboxVersionV1 {
			resp.Diagnostics.AddAttributeError(
				path.Root("sandbox_version"),
				"Native UDFs use sandbox version v1",
				"Set sandbox_version to v1 or omit it. The API always uses v1 for native UDFs.",
			)
		}
	} else if !config.SandboxVersion.IsNull() && !config.SandboxVersion.IsUnknown() && config.SandboxVersion.ValueString() == api.UDFSandboxVersionV1 {
		resp.Diagnostics.AddAttributeError(
			path.Root("sandbox_version"),
			"Sandbox version v1 requires the native runtime",
			"Use sandbox version v2 or v3 with the python3.11 runtime.",
		)
	}
}

func (r *UDFResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var config models.UDFResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !config.Type.IsUnknown() {
		poolSize := types.Int64Null()
		maxCommandExecutionTime := types.Int64Null()
		if config.Type.ValueString() == api.UDFTypeExecutablePool {
			switch {
			case config.PoolSize.IsUnknown():
				poolSize = types.Int64Unknown()
			case config.PoolSize.IsNull():
				poolSize = types.Int64Value(3)
			default:
				poolSize = config.PoolSize
			}
			switch {
			case config.MaxCommandExecutionTime.IsUnknown():
				maxCommandExecutionTime = types.Int64Unknown()
			case config.MaxCommandExecutionTime.IsNull():
				maxCommandExecutionTime = types.Int64Value(10)
			default:
				maxCommandExecutionTime = config.MaxCommandExecutionTime
			}
		}
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("pool_size"), poolSize)...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("max_command_execution_time"), maxCommandExecutionTime)...)
	}

	if !config.Runtime.IsUnknown() {
		if config.Runtime.ValueString() == api.UDFRuntimeNative {
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("sandbox_type"), api.UDFSandboxTypeBasic)...)
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("sandbox_version"), api.UDFSandboxVersionV1)...)
		} else {
			sandboxType := config.SandboxType
			if sandboxType.IsNull() {
				sandboxType = types.StringValue(api.UDFSandboxTypeBasic)
			}
			sandboxVersion := config.SandboxVersion
			if sandboxVersion.IsNull() {
				sandboxVersion = types.StringValue(api.UDFSandboxVersionV2)
			}
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("sandbox_type"), sandboxType)...)
			resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("sandbox_version"), sandboxVersion)...)
		}
	}

	if req.State.Raw.IsNull() || resp.Diagnostics.HasError() {
		return
	}

	var plan, state models.UDFResourceModel
	resp.Diagnostics.Append(resp.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if udfPublishInputsChanged(plan, state) {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("version"), state.Version)...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("status"), state.Status)...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("error"), state.Error)...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("created_at"), state.CreatedAt)...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("updated_at"), state.UpdatedAt)...)
}

func (r *UDFResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.UDFResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.preflightUDFCreate(ctx, plan.FunctionName.ValueString()); err != nil {
		addUDFWriteError(ctx, &resp.Diagnostics, "creating", plan.FunctionName.ValueString(), err)
		return
	}

	archive, ok := readUDFArchive(plan.SourceArchivePath.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}
	udf, err := r.publishUDFFromArchive(ctx, archive, plan, 0, true, &resp.Diagnostics)
	if err != nil {
		addUDFWriteError(ctx, &resp.Diagnostics, "creating", plan.FunctionName.ValueString(), err)
		return
	}

	resp.Diagnostics.Append(applyUDFToState(ctx, udf, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ready, waitErr := waitForUDFReady(ctx, udfBuildTimeout, udf.Version, func(ctx context.Context) (*api.UDF, error) {
		return r.client.GetUDF(ctx, plan.FunctionName.ValueString())
	})
	r.finishUDFWrite(ctx, &plan, ready, waitErr, true, types.Int64Null(), &resp.Diagnostics, &resp.State)
}

func (r *UDFResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.UDFResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	udf, err := r.client.GetUDF(ctx, state.FunctionName.ValueString())
	if err != nil {
		if isUDFNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tflog.Error(ctx, "reading UDF failed", map[string]any{
			"functionName": state.FunctionName.ValueString(),
			"error":        safeUDFError(err),
		})
		resp.Diagnostics.AddError(
			"Error reading UDF",
			fmt.Sprintf("Could not read UDF %q: %s", state.FunctionName.ValueString(), safeUDFError(err)),
		)
		return
	}
	resp.Diagnostics.Append(applyUDFToState(ctx, udf, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UDFResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.UDFResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state models.UDFResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	comparisonState := state
	imported := comparisonState.SourceArchiveHash.IsNull()
	if imported {
		comparisonState.SourceArchiveHash = plan.SourceArchiveHash
	}

	if !udfPublishInputsChanged(plan, comparisonState) {
		state.FailOnBuildError = plan.FailOnBuildError
		state.SourceArchivePath = plan.SourceArchivePath
		if imported {
			state.SourceArchiveHash = plan.SourceArchiveHash
			resp.Diagnostics.AddWarning(
				"UDF source could not be verified after import",
				fmt.Sprintf(
					"Terraform cannot tell whether %q matches the deployed version of UDF %q, so it accepted the configured source as-is. Make sure source_archive_path points at the source that was actually deployed.",
					plan.SourceArchivePath.ValueString(),
					state.FunctionName.ValueString(),
				),
			)
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	archive, ok := readUDFArchive(plan.SourceArchivePath.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}
	created, err := r.publishUDFFromArchive(ctx, archive, plan, state.Version.ValueInt64(), false, &resp.Diagnostics)
	if err != nil {
		addUDFWriteError(ctx, &resp.Diagnostics, "updating", plan.FunctionName.ValueString(), err)
		return
	}

	resp.Diagnostics.Append(applyUDFToState(ctx, created, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ready, waitErr := waitForUDFReady(ctx, udfBuildTimeout, created.Version, func(ctx context.Context) (*api.UDF, error) {
		return r.client.GetUDF(ctx, plan.FunctionName.ValueString())
	})
	r.finishUDFWrite(ctx, &plan, ready, waitErr, false, state.Version, &resp.Diagnostics, &resp.State)
}

func (r *UDFResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.UDFResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := retryUDFDelete(ctx, udfBuildTimeout, func(ctx context.Context) error {
		return r.client.DeleteUDF(ctx, state.FunctionName.ValueString())
	}); err != nil {
		addUDFDeleteError(ctx, &resp.Diagnostics, state.FunctionName.ValueString(), err)
	}
}

func (r *UDFResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !udfNamePattern.MatchString(req.ID) {
		resp.Diagnostics.AddError(
			"Invalid UDF import ID",
			fmt.Sprintf("Expected a function name matching %s, got %q.", udfNamePattern, req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("function_name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("fail_on_build_error"), true)...)
}

func (r *UDFResource) createAndUploadUDFArchive(ctx context.Context, archive []byte) (*api.UDFUploadSession, error) {
	session, err := r.client.CreateUDFUploadSession(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.client.UploadUDFArchive(ctx, session.UploadURL, archive); err != nil {
		return nil, err
	}
	return session, nil
}

type udfExistingFunctionError struct {
	functionName string
}

func (e *udfExistingFunctionError) Error() string {
	return fmt.Sprintf("UDF %q already exists", e.functionName)
}

// preflightUDFCreate prevents a create reconciliation from mistaking a
// pre-existing function for a publish whose response was lost.
func (r *UDFResource) preflightUDFCreate(ctx context.Context, functionName string) error {
	existing, err := r.client.GetUDF(ctx, functionName)
	if isUDFNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check whether UDF %q already exists: %w", functionName, err)
	}
	if existing == nil {
		return fmt.Errorf("check whether UDF %q already exists: API returned an empty response", functionName)
	}
	return &udfExistingFunctionError{functionName: functionName}
}

func (r *UDFResource) publishUDFFromArchive(
	ctx context.Context,
	archive []byte,
	plan models.UDFResourceModel,
	priorVersion int64,
	isCreate bool,
	diags *diag.Diagnostics,
) (*api.UDF, error) {
	functionName := plan.FunctionName.ValueString()
	var lastErr error

	for attempt := 1; attempt <= udfWriteMaxAttempts; attempt++ {
		session, err := r.createAndUploadUDFArchive(ctx, archive)
		if err != nil {
			lastErr = err
			if !shouldRetryUDFUpload(err) || attempt == udfWriteMaxAttempts {
				return nil, fmt.Errorf("upload UDF source archive: %w", lastErr)
			}
			tflog.Warn(ctx, "UDF source archive upload failed; retrying", map[string]any{
				"functionName": functionName,
				"attempt":      attempt,
				"error":        safeUDFError(err),
			})
			continue
		}

		versionRequest, versionDiags := udfVersionRequest(ctx, plan, session.UploadID)
		if versionDiags.HasError() {
			return nil, fmt.Errorf("%s", versionDiags.Errors()[0].Detail())
		}

		var udf *api.UDF
		if isCreate {
			udf, err = r.client.CreateUDF(ctx, api.UDFCreateRequest{
				FunctionName:            functionName,
				UDFVersionCreateRequest: versionRequest,
			})
		} else {
			udf, err = r.client.CreateUDFVersion(ctx, functionName, versionRequest)
		}
		if err == nil {
			return udf, nil
		}
		lastErr = err

		if api.IsUDFPublishOutcomeUnknown(err) {
			reconciled, ok, reason := r.reconcileUDFWrite(ctx, plan, priorVersion, isCreate)
			if ok {
				tflog.Info(ctx, "Confirmed UDF publish after an unknown response", map[string]any{
					"functionName": functionName,
					"version":      reconciled.Version,
					"attempt":      attempt,
				})
				addUDFReconcileWarning(diags, functionName, reconciled.Version, isCreate)
				return reconciled, nil
			}
			return nil, &udfPublishOutcomeInconclusiveError{
				functionName:    functionName,
				expectedVersion: expectedUDFVersion(priorVersion, isCreate),
				reason:          reason,
				cause:           err,
			}
		}

		if !shouldRetryUDFPublish(err, isCreate) || attempt == udfWriteMaxAttempts {
			return nil, lastErr
		}

		tflog.Warn(ctx, "UDF write failed; retrying with a new upload session", map[string]any{
			"functionName": functionName,
			"attempt":      attempt,
			"error":        safeUDFError(err),
		})
	}

	return nil, lastErr
}

func (r *UDFResource) reconcileUDFWrite(
	ctx context.Context,
	plan models.UDFResourceModel,
	priorVersion int64,
	isCreate bool,
) (*api.UDF, bool, string) {
	functionName := plan.FunctionName.ValueString()
	reconcileCtx, cancel := context.WithTimeout(ctx, udfReconcileTimeout)
	defer cancel()

	existing, err := r.client.GetUDF(reconcileCtx, functionName)
	if err != nil {
		return nil, false, fmt.Sprintf("could not read the UDF after the publish response was lost: %s", safeUDFError(err))
	}
	if existing == nil {
		return nil, false, "the read-after-write check returned an empty UDF response"
	}

	expectedVersion := expectedUDFVersion(priorVersion, isCreate)
	if existing.Version != expectedVersion {
		return nil, false, fmt.Sprintf("the read-after-write check found version %d, but version %d was expected", existing.Version, expectedVersion)
	}
	if matches, reason := udfMatchesPublishPlan(ctx, existing, plan); !matches {
		return nil, false, reason
	}

	return existing, true, ""
}

func expectedUDFVersion(priorVersion int64, isCreate bool) int64 {
	if isCreate {
		return 1
	}
	return priorVersion + 1
}

func udfMatchesPublishPlan(ctx context.Context, udf *api.UDF, plan models.UDFResourceModel) (bool, string) {
	if udf == nil {
		return false, "the read-after-write check returned an empty UDF response"
	}
	if udf.FunctionName != plan.FunctionName.ValueString() {
		return false, "the read-after-write check returned a different UDF function"
	}
	request, requestDiags := udfVersionRequest(ctx, plan, "")
	if requestDiags.HasError() {
		return false, "Terraform could not reconstruct the requested UDF settings for reconciliation"
	}

	if udf.Runtime != request.Runtime ||
		udf.ReturnType != request.ReturnType ||
		udf.Type != request.Type ||
		udf.CommandReadTimeout != request.CommandReadTimeout ||
		udf.CommandWriteTimeout != request.CommandWriteTimeout ||
		udf.SendChunkHeader != request.SendChunkHeader ||
		udf.Format != request.Format ||
		udf.SandboxType != request.SandboxType ||
		udf.SandboxVersion != request.SandboxVersion ||
		!equalUDFArguments(udf.Arguments, request.Arguments) ||
		!equalUDFStringPointer(udf.ReturnName, request.ReturnName) ||
		!equalUDFInt64Pointer(udf.PoolSize, request.PoolSize) ||
		!equalUDFInt64Pointer(udf.MaxCommandExecutionTime, request.MaxCommandExecutionTime) {
		return false, "the read-after-write check found a UDF with different settings"
	}

	return true, ""
}

func addUDFReconcileWarning(diags *diag.Diagnostics, functionName string, version int64, isCreate bool) {
	if isCreate {
		diags.AddWarning(
			"UDF create was adopted after an unknown publish response",
			fmt.Sprintf(
				"Terraform adopted UDF %q at version %d after the publish response was lost because its observable settings matched the request. The source archive bytes cannot be checked; verify source_archive_path and source_archive_hash.",
				functionName, version,
			),
		)
		return
	}
	diags.AddWarning(
		"UDF version was adopted after an unknown publish response",
		fmt.Sprintf(
			"Terraform adopted version %d of UDF %q after the publish response was lost because its observable settings matched the request. The source archive bytes cannot be checked; verify source_archive_path and source_archive_hash.",
			version, functionName,
		),
	)
}

type udfPublishOutcomeInconclusiveError struct {
	functionName    string
	expectedVersion int64
	reason          string
	cause           error
}

func (e *udfPublishOutcomeInconclusiveError) Error() string {
	return fmt.Sprintf("could not confirm whether publishing UDF %q version %d succeeded: %s", e.functionName, e.expectedVersion, e.reason)
}

func (e *udfPublishOutcomeInconclusiveError) Unwrap() error {
	return e.cause
}

func shouldRetryUDFUpload(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return isUDFHTTPStatus(err, 410) || isUDFServerError(err) || isUDFTransportError(err)
}

func shouldRetryUDFPublish(err error, isCreate bool) bool {
	if isUDFHTTPStatus(err, 410) {
		return true
	}

	return !isCreate && api.IsConflict(err)
}

func (r *UDFResource) finishUDFWrite(
	ctx context.Context,
	state *models.UDFResourceModel,
	udf *api.UDF,
	waitErr error,
	isCreate bool,
	priorVersion types.Int64,
	diags *diag.Diagnostics,
	tfState interface {
		Set(context.Context, any) diag.Diagnostics
	},
) {
	if udf != nil {
		diags.Append(applyUDFToState(ctx, udf, state)...)
		diags.Append(tfState.Set(ctx, state)...)
	}
	if waitErr == nil {
		return
	}

	if udf != nil && udf.Status == api.UDFStatusError {
		rawError := safeUDFError(waitErr)
		if udf.Error != nil && *udf.Error != "" {
			rawError = safeUDFBuildCause(*udf.Error)
		}
		failOnBuildError := udfFailOnBuildError(state)
		detail := udfBuildFailureDetail(udf.FunctionName, udf.Version, isCreate, priorVersion, isCreate && failOnBuildError, rawError)
		if failOnBuildError {
			diags.AddError("UDF build failed", detail)
			return
		}
		diags.AddWarning("UDF build failed", detail)
		return
	}

	detail := fmt.Sprintf(
		"Could not finish waiting for UDF %q version %d to build within %s. Last known status: %q.",
		state.FunctionName.ValueString(),
		state.Version.ValueInt64(),
		udfBuildTimeout,
		state.Status.ValueString(),
	)

	if errors.Is(waitErr, context.DeadlineExceeded) || errors.Is(waitErr, context.Canceled) {
		detail += " Terraform saved this status in state. Refresh or run plan to check whether the build completed before applying again."
	} else {
		detail += " " + safeUDFError(waitErr)
	}

	diags.AddError("Error waiting for UDF build", detail)
}

func udfBuildFailureDetail(functionName string, version int64, isCreate bool, priorVersion types.Int64, tainted bool, buildError string) string {
	buildError = safeUDFBuildCause(buildError)
	var detail string
	switch {
	case isCreate && tainted:
		detail = fmt.Sprintf(
			"UDF %q was created, but version %d failed to build. Terraform marked this resource as tainted. Fix the source and run apply to replace it, or run terraform untaint then apply to publish version %d. Build error: %s",
			functionName, version, version+1, buildError,
		)
	case isCreate:
		detail = fmt.Sprintf(
			"UDF %q was created, but version %d failed to build. Fix the source, update source_archive_hash, then apply. Applying again without changing the source will not retry the build. Build error: %s",
			functionName, version, buildError,
		)
	case !priorVersion.IsNull() && !priorVersion.IsUnknown():
		detail = fmt.Sprintf(
			"UDF %q version %d failed to build. Version %d is still in use. Fix the source, update source_archive_hash, then apply. Applying again without changing the source will not retry the build. Build error: %s",
			functionName, version, priorVersion.ValueInt64(), buildError,
		)
	default:
		detail = fmt.Sprintf(
			"UDF %q version %d failed to build. Fix the source, update source_archive_hash, then apply. Applying again without changing the source will not retry the build. Build error: %s",
			functionName, version, buildError,
		)
	}
	return detail
}

func udfPublishInputsChanged(plan, state models.UDFResourceModel) bool {
	return !plan.Runtime.Equal(state.Runtime) ||
		!plan.Arguments.Equal(state.Arguments) ||
		!plan.ReturnType.Equal(state.ReturnType) ||
		!plan.Type.Equal(state.Type) ||
		!plan.PoolSize.Equal(state.PoolSize) ||
		!plan.SourceArchiveHash.Equal(state.SourceArchiveHash) ||
		!plan.ReturnName.Equal(state.ReturnName) ||
		!plan.CommandReadTimeout.Equal(state.CommandReadTimeout) ||
		!plan.CommandWriteTimeout.Equal(state.CommandWriteTimeout) ||
		!plan.MaxCommandExecutionTime.Equal(state.MaxCommandExecutionTime) ||
		!plan.SendChunkHeader.Equal(state.SendChunkHeader) ||
		!plan.Format.Equal(state.Format) ||
		!plan.SandboxType.Equal(state.SandboxType) ||
		!plan.SandboxVersion.Equal(state.SandboxVersion)
}

func udfFailOnBuildError(state *models.UDFResourceModel) bool {
	if state == nil || state.FailOnBuildError.IsNull() || state.FailOnBuildError.IsUnknown() {
		return true
	}
	return state.FailOnBuildError.ValueBool()
}

func readUDFArchive(archivePath string, diags *diag.Diagnostics) ([]byte, bool) {
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		diags.AddAttributeError(
			path.Root("source_archive_path"),
			"Error reading UDF source archive",
			fmt.Sprintf("Could not read %q: %s", archivePath, err),
		)
		return nil, false
	}
	return archive, true
}

func udfVersionRequest(ctx context.Context, plan models.UDFResourceModel, uploadID string) (api.UDFVersionCreateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	var argumentModels []models.UDFArgumentModel
	diags.Append(plan.Arguments.ElementsAs(ctx, &argumentModels, false)...)
	if diags.HasError() {
		return api.UDFVersionCreateRequest{}, diags
	}
	arguments := make([]api.UDFArgument, len(argumentModels))
	for index, argument := range argumentModels {
		arguments[index] = api.UDFArgument{Name: argument.Name.ValueString(), Type: argument.Type.ValueString()}
	}

	return api.UDFVersionCreateRequest{
		UploadID:                uploadID,
		Runtime:                 plan.Runtime.ValueString(),
		Arguments:               arguments,
		ReturnType:              plan.ReturnType.ValueString(),
		ReturnName:              optionalStringPointer(plan.ReturnName),
		Type:                    plan.Type.ValueString(),
		PoolSize:                optionalInt64Pointer(plan.PoolSize),
		CommandReadTimeout:      plan.CommandReadTimeout.ValueInt64(),
		CommandWriteTimeout:     plan.CommandWriteTimeout.ValueInt64(),
		MaxCommandExecutionTime: optionalInt64Pointer(plan.MaxCommandExecutionTime),
		SendChunkHeader:         plan.SendChunkHeader.ValueBool(),
		Format:                  plan.Format.ValueString(),
		SandboxType:             plan.SandboxType.ValueString(),
		SandboxVersion:          plan.SandboxVersion.ValueString(),
	}, diags
}

func applyUDFToState(ctx context.Context, udf *api.UDF, state *models.UDFResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	state.FunctionName = types.StringValue(udf.FunctionName)
	state.Runtime = types.StringValue(udf.Runtime)
	state.ReturnType = types.StringValue(udf.ReturnType)
	state.Type = types.StringValue(udf.Type)
	state.CommandReadTimeout = types.Int64Value(udf.CommandReadTimeout)
	state.CommandWriteTimeout = types.Int64Value(udf.CommandWriteTimeout)
	state.SendChunkHeader = types.BoolValue(udf.SendChunkHeader)
	state.Format = types.StringValue(udf.Format)
	state.SandboxType = types.StringValue(udf.SandboxType)
	state.SandboxVersion = types.StringValue(udf.SandboxVersion)
	state.Version = types.Int64Value(udf.Version)
	state.Status = types.StringValue(udf.Status)
	state.CreatedAt = types.StringValue(udf.CreatedAt)
	state.UpdatedAt = types.StringValue(udf.UpdatedAt)
	state.ReturnName = stringPointerValue(udf.ReturnName)
	state.PoolSize = int64PointerValue(udf.PoolSize)
	state.MaxCommandExecutionTime = int64PointerValue(udf.MaxCommandExecutionTime)
	if udf.Error == nil || strings.TrimSpace(*udf.Error) == "" {
		state.Error = types.StringNull()
	} else {
		state.Error = types.StringValue(safeUDFBuildCause(*udf.Error))
	}

	argumentType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"name": types.StringType,
		"type": types.StringType,
	}}
	argumentValues := make([]attr.Value, 0, len(udf.Arguments))
	for _, argument := range udf.Arguments {
		value, valueDiags := types.ObjectValue(argumentType.AttrTypes, map[string]attr.Value{
			"name": types.StringValue(argument.Name),
			"type": types.StringValue(argument.Type),
		})
		diags.Append(valueDiags...)
		argumentValues = append(argumentValues, value)
	}
	arguments, listDiags := types.ListValue(argumentType, argumentValues)
	diags.Append(listDiags...)
	state.Arguments = arguments

	return diags
}

func addUDFWriteError(ctx context.Context, diags *diag.Diagnostics, operation, functionName string, err error) {
	tflog.Error(ctx, fmt.Sprintf("%s UDF failed", operation), map[string]any{
		"functionName": functionName,
		"error":        safeUDFError(err),
	})

	var inconclusiveErr *udfPublishOutcomeInconclusiveError
	if errors.As(err, &inconclusiveErr) {
		detail := fmt.Sprintf(
			"Terraform could not confirm whether publishing UDF %q version %d succeeded after the API response was lost. It did not retry to avoid creating a duplicate version. %s Check the UDF's latest version and settings, then refresh before retrying.",
			functionName,
			inconclusiveErr.expectedVersion,
			inconclusiveErr.reason,
		)
		if requestID := udfRequestID(err); requestID != "" {
			detail += fmt.Sprintf(" Request ID: %s.", requestID)
		}
		diags.AddError("UDF publish outcome could not be confirmed", detail)
		return
	}

	var existingErr *udfExistingFunctionError
	if errors.As(err, &existingErr) {
		diags.AddError(
			"UDF already exists",
			fmt.Sprintf(
				"Could not create UDF %q because it already exists. If it should be managed by this resource, import it: terraform import clickhouse_udf.<name> %s.",
				functionName, functionName,
			),
		)
		return
	}

	status, code, message, requestID := udfErrorInfo(err)
	safeMessage := safeUDFError(err)
	if requestID != "" && !strings.Contains(safeMessage, requestID) {
		safeMessage += fmt.Sprintf(" (request ID: %s)", requestID)
	}

	switch {
	case api.IsConflict(err) && operation == "creating" && udfErrorIndicatesExistingFunction(status, code, message):
		diags.AddError(
			"UDF already exists",
			fmt.Sprintf(
				"Could not create UDF %q because it already exists: %s. If it should be managed by this resource, import it: terraform import clickhouse_udf.<name> %s.",
				functionName, safeMessage, functionName,
			),
		)
	case api.IsConflict(err):
		detail := fmt.Sprintf("Could not finish %s UDF %q: %s.", operation, functionName, safeMessage)
		if operation == "updating" {
			detail += " Another UDF update may be in progress; refresh the resource and retry after it settles."
		}
		diags.AddError(fmt.Sprintf("Error %s UDF", operation), detail)
	case isUDFHTTPStatus(err, 410):
		diags.AddError(
			fmt.Sprintf("Error %s UDF", operation),
			fmt.Sprintf(
				"Could not finish %s UDF %q: %s. The source upload expired before it could be used, and the provider retried with a new upload. Run apply again if the problem persists.",
				operation, functionName, safeMessage,
			),
		)
	case api.IsForbidden(err) && udfErrorIndicatesRuntimeGate(status, code, message):
		diags.AddError(
			"UDF runtime or sandbox is not enabled for this organization",
			fmt.Sprintf(
				"Could not finish %s UDF %q: %s. Choose a supported runtime or sandbox setting, or contact ClickHouse support.",
				operation, functionName, safeMessage,
			),
		)
	default:
		diags.AddError(
			fmt.Sprintf("Error %s UDF", operation),
			fmt.Sprintf("Could not finish %s UDF %q: %s", operation, functionName, safeMessage),
		)
	}
}

func addUDFDeleteError(ctx context.Context, diags *diag.Diagnostics, functionName string, err error) {
	tflog.Error(ctx, "deleting UDF failed", map[string]any{
		"functionName": functionName,
		"error":        safeUDFError(err),
	})

	var timeoutErr *udfMutationTimeoutError
	if errors.As(err, &timeoutErr) {
		diags.AddError(
			"Error deleting UDF",
			fmt.Sprintf(
				"Could not delete UDF %q: %s. Terraform will check the saved state before another apply; refresh first if the transition may have finished.",
				functionName, safeUDFError(timeoutErr.lastErr),
			),
		)
		return
	}

	diags.AddError(
		"Error deleting UDF",
		fmt.Sprintf("Could not delete UDF %q: %s", functionName, safeUDFError(err)),
	)
}

func optionalStringPointer(value types.String) *string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueString()
	return &result
}

func optionalInt64Pointer(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueInt64()
	return &result
}

func stringPointerValue(value *string) types.String {
	if value == nil {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

func int64PointerValue(value *int64) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*value)
}

func equalUDFStringPointer(left, right *string) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func equalUDFInt64Pointer(left, right *int64) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func equalUDFArguments(left, right []api.UDFArgument) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type udfErrorMetadata struct {
	status    int
	code      string
	message   string
	requestID string
}

var udfRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)

func udfErrorInfo(err error) (status int, code, message, requestID string) {
	var metadataChain []udfErrorMetadata
	for current := err; current != nil; current = errors.Unwrap(current) {
		metadataChain = append(metadataChain, parseUDFErrorText(current.Error()))
	}
	for index := len(metadataChain) - 1; index >= 0; index-- {
		metadata := metadataChain[index]
		if status == 0 && metadata.status != 0 {
			status = metadata.status
		}
		if code == "" && metadata.code != "" {
			code = metadata.code
		}
		if message == "" && metadata.message != "" {
			message = metadata.message
		}
		if requestID == "" && metadata.requestID != "" {
			requestID = metadata.requestID
		}
	}
	return status, code, message, requestID
}

func isUDFHTTPStatus(err error, want int) bool {
	status, _, _, _ := udfErrorInfo(err)
	return status == want
}

func isUDFNotFound(err error) bool {
	if !isUDFHTTPStatus(err, 404) {
		return false
	}
	message := strings.ToLower(err.Error())
	return !strings.Contains(message, "cannot get ") && !strings.Contains(message, "cannot delete ")
}

func isUDFServerError(err error) bool {
	status, _, _, _ := udfErrorInfo(err)
	return status >= 500 && status <= 599
}

func isUDFTransportError(err error) bool {
	status, _, _, _ := udfErrorInfo(err)
	return err != nil && status == 0
}

func udfRequestID(err error) string {
	_, _, _, requestID := udfErrorInfo(err)
	return requestID
}

func safeUDFError(err error) string {
	if err == nil {
		return ""
	}
	status, code, message, requestID := udfErrorInfo(err)
	if status >= 500 && status <= 599 {
		message = fmt.Sprintf("request failed with HTTP status %d", status)
	}
	if status == 0 && code == "" && requestID == "" {
		lower := strings.ToLower(message)
		switch {
		case strings.Contains(lower, "context deadline exceeded"):
			message = "context deadline exceeded"
		case strings.Contains(lower, "context canceled"), strings.Contains(lower, "context cancelled"):
			message = "context canceled"
		default:
			return "request failed"
		}
	}
	if message == "" {
		message = "request failed"
	}
	if requestID != "" && !strings.Contains(message, requestID) {
		message += fmt.Sprintf(" (request ID: %s)", requestID)
	}
	return limitUDFDetail(message)
}

func safeUDFBuildCause(raw string) string {
	metadata := parseUDFErrorText(raw)
	if metadata.message == "" {
		return "the build did not provide an error message"
	}
	return limitUDFDetail(metadata.message)
}

func limitUDFDetail(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if len(message) <= udfSafeDetailMaxLength {
		return message
	}
	return message[:udfSafeDetailMaxLength-3] + "..."
}

func parseUDFErrorText(raw string) udfErrorMetadata {
	metadata := udfErrorMetadata{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return metadata
	}
	metadata.status = api.StatusFromMessage(trimmed)

	if value, ok := parseEmbeddedUDFJSON(trimmed); ok {
		metadata.code = jsonStringField(value, "code")
		metadata.requestID = safeRequestID(jsonStringField(value, "requestId"))
		if metadata.requestID == "" {
			metadata.requestID = safeRequestID(jsonStringField(value, "requestID"))
		}
		if metadata.requestID == "" {
			metadata.requestID = safeRequestID(jsonStringField(value, "request_id"))
		}
		metadata.message = jsonErrorMessage(value)
	}

	if metadata.message == "" {
		message := trimmed
		if marker := indexFold(message, "body:"); marker >= 0 {
			message = strings.TrimSpace(message[marker+len("body:"):])
			if value, ok := parseEmbeddedUDFJSON(message); ok {
				metadata.message = jsonErrorMessage(value)
			}
		}
	}
	if metadata.message == "" {
		if metadata.status != 0 {
			metadata.message = fmt.Sprintf("request failed with HTTP status %d", metadata.status)
		} else {
			metadata.message = cleanUDFText(trimmed)
		}
	}
	return metadata
}

func parseEmbeddedUDFJSON(text string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(text)))
	var value any
	if err := decoder.Decode(&value); err == nil {
		return value, true
	}
	for index := strings.IndexByte(text, '{'); index >= 0; {
		var candidate any
		if err := json.Unmarshal([]byte(text[index:]), &candidate); err == nil {
			return candidate, true
		}
		next := strings.IndexByte(text[index+1:], '{')
		if next < 0 {
			break
		}
		index += next + 1
	}
	return nil, false
}

func jsonStringField(value any, key string) string {
	switch typed := value.(type) {
	case map[string]any:
		if field, ok := typed[key].(string); ok {
			return strings.TrimSpace(field)
		}
		for _, child := range typed {
			if nested := jsonStringField(child, key); nested != "" {
				return nested
			}
		}
	case []any:
		for _, child := range typed {
			if nested := jsonStringField(child, key); nested != "" {
				return nested
			}
		}
	}
	return ""
}

func jsonErrorMessage(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"error", "message", "detail", "reason", "cause", "description"} {
			if child, exists := typed[key]; exists {
				switch childValue := child.(type) {
				case string:
					if nested, ok := parseEmbeddedUDFJSON(childValue); ok {
						if message := jsonErrorMessage(nested); message != "" {
							return message
						}
					}
					if message := cleanUDFText(childValue); message != "" {
						return message
					}
				default:
					if message := jsonErrorMessage(childValue); message != "" {
						return message
					}
				}
			}
		}
		for _, child := range typed {
			if message := jsonErrorMessage(child); message != "" {
				return message
			}
		}
	case []any:
		for _, child := range typed {
			if message := jsonErrorMessage(child); message != "" {
				return message
			}
		}
	}
	return ""
}

func cleanUDFText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if value, ok := parseEmbeddedUDFJSON(text); ok {
		if message := jsonErrorMessage(value); message != "" {
			return message
		}
	}
	for _, markerName := range []string{"reason:", "cause:", "detail:", "message:"} {
		if marker := indexFold(text, markerName); marker >= 0 {
			text = strings.TrimSpace(text[marker+len(markerName):])
			if value, ok := parseEmbeddedUDFJSON(text); ok {
				if message := jsonErrorMessage(value); message != "" {
					return message
				}
			}
			break
		}
	}
	for _, prefix := range []string{"build failure:", "build failed:", "error:"} {
		if strings.HasPrefix(strings.ToLower(text), prefix) {
			text = strings.TrimSpace(text[len(prefix):])
		}
	}
	for {
		before := text
		for _, prefix := range []string{"panic:", "exception:", "fatal:"} {
			if strings.HasPrefix(strings.ToLower(text), prefix) {
				text = strings.TrimSpace(text[len(prefix):])
			}
		}
		if strings.HasPrefix(text, "[") {
			if end := strings.IndexByte(text, ']'); end > 0 && end <= 16 {
				text = strings.TrimSpace(text[end+1:])
			}
		}
		if text == before {
			break
		}
	}
	return limitUDFDetail(text)
}

func indexFold(text, needle string) int {
	return strings.Index(strings.ToLower(text), strings.ToLower(needle))
}

func safeRequestID(value string) string {
	if udfRequestIDPattern.MatchString(value) {
		return value
	}
	return ""
}

func udfErrorIndicatesExistingFunction(status int, code, message string) bool {
	if status != 409 {
		return false
	}
	value := strings.ToLower(code + " " + message)
	return strings.Contains(value, "already_exist") || strings.Contains(value, "already exists") || strings.Contains(value, "function exists")
}

func udfErrorIndicatesRuntimeGate(status int, code, message string) bool {
	if status != 403 {
		return false
	}
	value := strings.ToLower(code + " " + message)
	return strings.Contains(value, "runtime") && (strings.Contains(value, "sandbox") || strings.Contains(value, "enabled") || strings.Contains(value, "allow"))
}
