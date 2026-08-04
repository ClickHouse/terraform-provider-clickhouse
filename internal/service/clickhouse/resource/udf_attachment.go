package resource

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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
	_ resource.Resource                   = (*UDFAttachmentResource)(nil)
	_ resource.ResourceWithConfigure      = (*UDFAttachmentResource)(nil)
	_ resource.ResourceWithImportState    = (*UDFAttachmentResource)(nil)
	_ resource.ResourceWithValidateConfig = (*UDFAttachmentResource)(nil)
)

//go:embed descriptions/udf_attachment.md
var udfAttachmentResourceDescription string

const udfAttachmentTimeout = 15 * time.Minute

var uuidPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func NewUDFAttachmentResource() resource.Resource {
	return &UDFAttachmentResource{}
}

type UDFAttachmentResource struct {
	client api.Client
}

func (r *UDFAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_udf_attachment"
}

func (r *UDFAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: udfAttachmentResourceDescription,
		Attributes: map[string]schema.Attribute{
			"function_name": schema.StringAttribute{
				Description: "Name of the UDF.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(udfNamePattern, "must start with a letter and contain only letters, numbers, or underscores"),
				},
			},
			"service_id": schema.StringAttribute{
				Description: "ID of the attached service.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(uuidPattern, "must be a UUID"),
				},
			},
			"version": schema.Int64Attribute{
				Description: "Version to attach.",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"status": schema.StringAttribute{
				Description: "Current attachment lifecycle state.",
				Computed:    true,
			},
		},
	}
}

func (r *UDFAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *UDFAttachmentResource) ValidateConfig(_ context.Context, _ resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_udf_attachment", &resp.Diagnostics)
}

func (r *UDFAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.UDFAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.writeAttachment(ctx, &plan, "creating", &resp.Diagnostics, &resp.State)
}

func (r *UDFAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.UDFAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	attachment, err := r.client.GetUDFAttachment(ctx, state.FunctionName.ValueString(), state.ServiceID.ValueString())
	if err != nil {
		if isUDFNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		tflog.Error(ctx, "reading UDF attachment failed", map[string]any{
			"functionName": state.FunctionName.ValueString(),
			"serviceId":    state.ServiceID.ValueString(),
			"error":        safeUDFError(err),
		})
		resp.Diagnostics.AddError(
			"Error reading UDF attachment",
			fmt.Sprintf(
				"Could not read the attachment of UDF %q to service %s: %s",
				state.FunctionName.ValueString(),
				state.ServiceID.ValueString(),
				safeUDFError(err),
			),
		)
		return
	}
	applyUDFAttachmentToState(attachment, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *UDFAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.UDFAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.writeAttachment(ctx, &plan, "updating", &resp.Diagnostics, &resp.State)
}

func (r *UDFAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.UDFAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := retryUDFAttachmentDetach(ctx, udfAttachmentTimeout, func(ctx context.Context) error {
		return r.client.DetachUDF(ctx, state.FunctionName.ValueString(), state.ServiceID.ValueString())
	}); err != nil {
		addUDFAttachmentDetachError(ctx, &resp.Diagnostics, &state, err)
		return
	}
	if err := waitForUDFAttachmentDeleted(ctx, udfAttachmentTimeout, func(ctx context.Context) (*api.UDFAttachment, error) {
		return r.client.GetUDFAttachment(ctx, state.FunctionName.ValueString(), state.ServiceID.ValueString())
	}); err != nil {
		addUDFDetachWaitError(ctx, &resp.Diagnostics, &state, err)
	}
}

func (r *UDFAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || !udfNamePattern.MatchString(parts[0]) || !uuidPattern.MatchString(parts[1]) {
		resp.Diagnostics.AddError(
			"Invalid UDF attachment import ID",
			fmt.Sprintf("Expected function_name/service_id with a valid function name and UUID service ID, got %q.", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("function_name"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), parts[1])...)
}

func (r *UDFAttachmentResource) attach(ctx context.Context, plan models.UDFAttachmentResourceModel) (*api.UDFAttachment, error) {
	return r.client.AttachUDF(
		ctx,
		plan.FunctionName.ValueString(),
		plan.ServiceID.ValueString(),
		api.UDFAttachRequest{Version: optionalInt64Pointer(plan.Version)},
	)
}

func (r *UDFAttachmentResource) writeAttachment(
	ctx context.Context,
	plan *models.UDFAttachmentResourceModel,
	operation string,
	diags *diag.Diagnostics,
	tfState interface {
		Set(context.Context, any) diag.Diagnostics
	},
) {
	attachment, err := r.attach(ctx, *plan)
	if err != nil {
		addUDFAttachmentWriteError(ctx, diags, operation, *plan, err)
		return
	}
	applyUDFAttachmentToState(attachment, plan)
	diags.Append(tfState.Set(ctx, plan)...)
	if diags.HasError() {
		return
	}

	deployed, waitErr := waitForUDFAttachmentDeployed(ctx, udfAttachmentTimeout, attachment.Version, func(ctx context.Context) (*api.UDFAttachment, error) {
		return r.client.GetUDFAttachment(ctx, plan.FunctionName.ValueString(), plan.ServiceID.ValueString())
	}, &udfAttachmentRecovery{
		GetState: func(ctx context.Context) (string, error) {
			service, err := r.client.GetServiceBase(ctx, plan.ServiceID.ValueString())
			if err != nil {
				return "", err
			}
			return service.State, nil
		},
		Wake: func(ctx context.Context) error {
			return r.client.WakeService(ctx, plan.ServiceID.ValueString())
		},
		Retry: func(ctx context.Context) error {
			_, err := r.attach(ctx, *plan)
			return err
		},
	})
	finishUDFAttachmentWrite(ctx, plan, deployed, waitErr, diags, tfState)
}

func finishUDFAttachmentWrite(
	ctx context.Context,
	state *models.UDFAttachmentResourceModel,
	attachment *api.UDFAttachment,
	waitErr error,
	diags *diag.Diagnostics,
	tfState interface {
		Set(context.Context, any) diag.Diagnostics
	},
) {
	if attachment != nil {
		applyUDFAttachmentToState(attachment, state)
		diags.Append(tfState.Set(ctx, state)...)
	}
	if waitErr != nil {
		addUDFAttachmentWaitError(ctx, diags, state, waitErr)
	}
}

func addUDFAttachmentWaitError(ctx context.Context, diags *diag.Diagnostics, state *models.UDFAttachmentResourceModel, waitErr error) {
	functionName := state.FunctionName.ValueString()
	serviceID := state.ServiceID.ValueString()
	tflog.Error(ctx, "UDF attachment did not reach the deployed state", map[string]any{
		"functionName": functionName,
		"serviceId":    serviceID,
		"error":        safeUDFError(waitErr),
	})

	var timeoutErr *udfAttachmentTimeoutError
	if errors.As(waitErr, &timeoutErr) {
		detail := fmt.Sprintf(
			"Could not finish attaching UDF %q to service %s within %s.",
			functionName, serviceID, udfAttachmentTimeout,
		)
		if timeoutErr.lastStatus != "" {
			detail += fmt.Sprintf(" Last observed attachment status: %q.", timeoutErr.lastStatus)
		}
		switch {
		case timeoutErr.stuckAfterWake:
			detail += " The service was idle; Terraform woke it, but the attachment made no further progress."
		case timeoutErr.lastServiceState != "":
			detail += fmt.Sprintf(" Last observed service state: %q.", timeoutErr.lastServiceState)
		}
		detail += " Terraform saved this status in state. Refresh or run plan to check whether the attachment completed before applying again."
		diags.AddError("Error waiting for UDF attachment", detail)
		return
	}

	diags.AddError(
		"Error waiting for UDF attachment",
		fmt.Sprintf(
			"Could not finish attaching UDF %q to service %s: %s",
			functionName, serviceID, safeUDFError(waitErr),
		),
	)
}

func addUDFDetachWaitError(ctx context.Context, diags *diag.Diagnostics, state *models.UDFAttachmentResourceModel, err error) {
	functionName := state.FunctionName.ValueString()
	serviceID := state.ServiceID.ValueString()
	tflog.Error(ctx, "waiting for UDF detachment failed", map[string]any{
		"functionName": functionName,
		"serviceId":    serviceID,
		"error":        safeUDFError(err),
	})

	if errors.Is(err, context.DeadlineExceeded) {
		diags.AddError(
			"Error waiting for UDF detachment",
			fmt.Sprintf(
				"Could not finish detaching UDF %q from service %s within %s. Terraform saved the last observed state; refresh or run plan to check whether the detach completed before applying again.",
				functionName, serviceID, udfAttachmentTimeout,
			),
		)
		return
	}

	diags.AddError(
		"Error waiting for UDF detachment",
		fmt.Sprintf("Could not finish detaching UDF %q from service %s: %s", functionName, serviceID, safeUDFError(err)),
	)
}

func addUDFAttachmentDetachError(ctx context.Context, diags *diag.Diagnostics, state *models.UDFAttachmentResourceModel, err error) {
	functionName := state.FunctionName.ValueString()
	serviceID := state.ServiceID.ValueString()
	tflog.Error(ctx, "detaching UDF failed", map[string]any{
		"functionName": functionName,
		"serviceId":    serviceID,
		"error":        safeUDFError(err),
	})

	var timeoutErr *udfMutationTimeoutError
	if errors.As(err, &timeoutErr) {
		diags.AddError(
			"Error detaching UDF",
			fmt.Sprintf(
				"Could not detach UDF %q from service %s: %s. Refresh first to check whether the transition finished before applying again.",
				functionName, serviceID, safeUDFError(timeoutErr.lastErr),
			),
		)
		return
	}

	diags.AddError(
		"Error detaching UDF",
		fmt.Sprintf("Could not detach UDF %q from service %s: %s", functionName, serviceID, safeUDFError(err)),
	)
}

func applyUDFAttachmentToState(attachment *api.UDFAttachment, state *models.UDFAttachmentResourceModel) {
	state.FunctionName = types.StringValue(attachment.FunctionName)
	state.ServiceID = types.StringValue(attachment.ServiceID)
	state.Version = types.Int64Value(attachment.Version)
	state.Status = types.StringValue(attachment.Status)
}

func addUDFAttachmentWriteError(ctx context.Context, diags *diag.Diagnostics, operation string, plan models.UDFAttachmentResourceModel, err error) {
	functionName := plan.FunctionName.ValueString()
	serviceID := plan.ServiceID.ValueString()
	tflog.Error(ctx, fmt.Sprintf("%s UDF attachment failed", operation), map[string]any{
		"functionName": functionName,
		"serviceId":    serviceID,
		"error":        safeUDFError(err),
	})
	status, code, message, requestID := udfErrorInfo(err)
	safeMessage := safeUDFError(err)
	if requestID != "" && !strings.Contains(safeMessage, requestID) {
		safeMessage += fmt.Sprintf(" (request ID: %s)", requestID)
	}

	switch {
	case isUDFHTTPStatus(err, 422) && udfErrorIndicatesAttachmentLimit(status, code, message):
		diags.AddError(
			"Service has reached its UDF attachment limit",
			fmt.Sprintf(
				"Could not attach UDF %q to service %s: %s",
				functionName, serviceID, safeMessage,
			),
		)
	case api.IsBadRequestWith(err, "not supported on this service"):
		diags.AddError(
			"UDFs are not supported on this service",
			fmt.Sprintf(
				"Could not attach UDF %q to service %s: %s. This is a property of the service itself; retrying will not help. Attach the UDF to a different service instead.",
				functionName, serviceID, safeMessage,
			),
		)
	case api.IsConflict(err):
		diags.AddError(
			fmt.Sprintf("Error %s UDF attachment", operation),
			fmt.Sprintf(
				"Could not attach UDF %q (version %d) to service %s: %s",
				functionName, plan.Version.ValueInt64(), serviceID, safeMessage,
			),
		)
	default:
		diags.AddError(
			fmt.Sprintf("Error %s UDF attachment", operation),
			fmt.Sprintf("Could not attach UDF %q to service %s: %s", functionName, serviceID, safeMessage),
		)
	}
}

func udfErrorIndicatesAttachmentLimit(status int, code, message string) bool {
	if status != 422 {
		return false
	}
	value := strings.ToLower(code + " " + message)
	return strings.Contains(value, "udf") &&
		(strings.Contains(value, "attach") || strings.Contains(value, "service")) &&
		(strings.Contains(value, "limit") || strings.Contains(value, "maximum") || strings.Contains(value, "max") || strings.Contains(value, "more than"))
}
