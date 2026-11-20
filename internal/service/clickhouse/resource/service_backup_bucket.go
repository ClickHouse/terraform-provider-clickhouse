package resource

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

var (
	_ resource.Resource                   = &ServiceBackupBucketResource{}
	_ resource.ResourceWithConfigure      = &ServiceBackupBucketResource{}
	_ resource.ResourceWithImportState    = &ServiceBackupBucketResource{}
	_ resource.ResourceWithValidateConfig = &ServiceBackupBucketResource{}
)

//go:embed descriptions/service_backup_bucket.md
var serviceBackupBucketResourceDescription string

func NewServiceBackupBucketResource() resource.Resource {
	return &ServiceBackupBucketResource{}
}

type ServiceBackupBucketResource struct {
	client api.Client
}

func (r *ServiceBackupBucketResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_backup_bucket"
}

func (r *ServiceBackupBucketResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: serviceBackupBucketResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Backup bucket ID, as returned by the ClickHouse Cloud API.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Cloud service ID the external backup bucket is attached to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bucket_provider": schema.StringAttribute{
				Description: "Cloud provider of the bucket. One of `AWS`, `GCP` or `AZURE`. Changing it replaces the resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(api.BackupBucketProviderAWS, api.BackupBucketProviderGCP, api.BackupBucketProviderAzure),
				},
			},
			"bucket_path": schema.StringAttribute{
				Description: "Path (URL) of the bucket. Required for `AWS` and `GCP`, not allowed for `AZURE`.",
				Optional:    true,
			},
			"iam_role_arn": schema.StringAttribute{
				Description: "ARN of the IAM role ClickHouse Cloud assumes to write backups. Required for `AWS`, not allowed otherwise.",
				Optional:    true,
			},
			"iam_role_session_name": schema.StringAttribute{
				Description: "Session name used when assuming the IAM role. Only allowed for `AWS`. Computed if not set.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"access_key_id": schema.StringAttribute{
				Description: "HMAC access key ID. Required for `GCP`, not allowed otherwise.",
				Optional:    true,
			},
			"secret_access_key": schema.StringAttribute{
				Description: "HMAC secret access key. Required for `GCP`, not allowed otherwise. The API never returns it, so drift cannot be detected.",
				Optional:    true,
				Sensitive:   true,
			},
			"container_name": schema.StringAttribute{
				Description: "Azure Blob Storage container name. Required for `AZURE`, not allowed otherwise.",
				Optional:    true,
			},
			"connection_string": schema.StringAttribute{
				Description: "Azure Storage connection string. Required for `AZURE`, not allowed otherwise. The API never returns it, so drift cannot be detected.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

// backupBucketProviderFields lists which attributes each provider requires
// and which it forbids. Attributes in neither list do not apply to the
// provider and must be unset.
var backupBucketProviderFields = map[string][]string{
	api.BackupBucketProviderAWS:   {"bucket_path", "iam_role_arn"},
	api.BackupBucketProviderGCP:   {"bucket_path", "access_key_id", "secret_access_key"},
	api.BackupBucketProviderAzure: {"container_name", "connection_string"},
}

var backupBucketOptionalProviderFields = map[string][]string{
	api.BackupBucketProviderAWS: {"iam_role_session_name"},
}

var backupBucketAllProviderFields = []string{
	"bucket_path", "iam_role_arn", "iam_role_session_name",
	"access_key_id", "secret_access_key", "container_name", "connection_string",
}

func (r *ServiceBackupBucketResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var provider types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("bucket_provider"), &provider)...)
	if resp.Diagnostics.HasError() || provider.IsNull() || provider.IsUnknown() {
		return
	}

	allowed := map[string]bool{}
	for _, f := range backupBucketProviderFields[provider.ValueString()] {
		allowed[f] = true
	}
	for _, f := range backupBucketOptionalProviderFields[provider.ValueString()] {
		allowed[f] = true
	}

	for _, f := range backupBucketAllProviderFields {
		var v types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(f), &v)...)
		if resp.Diagnostics.HasError() || v.IsUnknown() {
			continue
		}
		if !v.IsNull() && !allowed[f] {
			resp.Diagnostics.AddAttributeError(path.Root(f),
				"Attribute not valid for bucket provider",
				fmt.Sprintf("%s is not supported when bucket_provider is %q.", f, provider.ValueString()))
		}
	}
	for _, f := range backupBucketProviderFields[provider.ValueString()] {
		var v types.String
		resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(f), &v)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if v.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root(f),
				"Missing required attribute for bucket provider",
				fmt.Sprintf("%s is required when bucket_provider is %q.", f, provider.ValueString()))
		}
	}
}

func (r *ServiceBackupBucketResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError("ClickHouse Cloud API not configured",
			"This resource requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).")
		return
	}
	r.client = providerData.API
}

func (r *ServiceBackupBucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning("clickhouse_service_backup_bucket", &resp.Diagnostics)

	var plan models.ServiceBackupBucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()

	// Refuse to clobber an existing bucket. The user should import it.
	existing, err := r.client.GetBackupBucket(ctx, serviceID)
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error checking for existing backup bucket", err.Error())
		return
	}
	if existing != nil && existing.Id != "" {
		resp.Diagnostics.AddError(
			"Backup bucket already exists for this service",
			fmt.Sprintf("Service %s already has an external backup bucket. Import it into Terraform with: terraform import clickhouse_service_backup_bucket.<name> %s", serviceID, serviceID),
		)
		return
	}

	bucket, err := r.client.CreateBackupBucket(ctx, serviceID, backupBucketRequestFromPlan(plan))
	if err != nil {
		addBackupBucketWriteErrorDiagnostic(&resp.Diagnostics, "creating", serviceID, err)
		return
	}

	applyBackupBucketToState(bucket, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceBackupBucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ServiceBackupBucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bucket, err := r.client.GetBackupBucket(ctx, state.ServiceID.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading backup bucket", err.Error())
		return
	}
	// The API may answer 200 with an empty result when no bucket is configured.
	if bucket.Id == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	applyBackupBucketToState(bucket, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceBackupBucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning("clickhouse_service_backup_bucket", &resp.Diagnostics)

	var plan models.ServiceBackupBucketResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()
	bucket, err := r.client.UpdateBackupBucket(ctx, serviceID, backupBucketRequestFromPlan(plan))
	if err != nil {
		addBackupBucketWriteErrorDiagnostic(&resp.Diagnostics, "updating", serviceID, err)
		return
	}

	applyBackupBucketToState(bucket, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceBackupBucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ServiceBackupBucketResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteBackupBucket(ctx, state.ServiceID.ValueString())
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting backup bucket", err.Error())
	}
}

func (r *ServiceBackupBucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning("clickhouse_service_backup_bucket", &resp.Diagnostics)

	// The bucket ID is only known after Read, so seed service_id and let Read
	// fill the rest. `id` is set by Read.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), req.ID)...)
}

func backupBucketRequestFromPlan(plan models.ServiceBackupBucketResourceModel) api.BackupBucketRequest {
	str := func(v types.String) *string {
		if v.IsNull() || v.IsUnknown() {
			return nil
		}
		return v.ValueStringPointer()
	}

	return api.BackupBucketRequest{
		BucketProvider:     plan.BucketProvider.ValueString(),
		BucketPath:         str(plan.BucketPath),
		IamRoleArn:         str(plan.IamRoleArn),
		IamRoleSessionName: str(plan.IamRoleSessionName),
		AccessKeyId:        str(plan.AccessKeyID),
		SecretAccessKey:    str(plan.SecretAccessKey),
		ContainerName:      str(plan.ContainerName),
		ConnectionString:   str(plan.ConnectionString),
	}
}

// applyBackupBucketToState maps an API BackupBucket into the state model.
// Secrets are write-only and never returned by the API, so they are left
// untouched (the plan or prior state value is kept).
func applyBackupBucketToState(bucket *api.BackupBucket, state *models.ServiceBackupBucketResourceModel) {
	opt := func(s string) types.String {
		if s == "" {
			return types.StringNull()
		}
		return types.StringValue(s)
	}

	state.ID = types.StringValue(bucket.Id)
	state.BucketProvider = types.StringValue(bucket.BucketProvider)
	state.BucketPath = opt(bucket.BucketPath)
	state.IamRoleArn = opt(bucket.IamRoleArn)
	state.IamRoleSessionName = opt(bucket.IamRoleSessionName)
	state.AccessKeyID = opt(bucket.AccessKeyId)
	state.ContainerName = opt(bucket.ContainerName)
}

func addBackupBucketWriteErrorDiagnostic(diags *diag.Diagnostics, verb, serviceID string, err error) {
	switch {
	case api.IsNotFound(err):
		diags.AddError(
			"Service not found",
			fmt.Sprintf("Service %s does not exist or is not visible to the caller. Confirm clickhouse_service.<name>.id is correct and the API key has access.", serviceID),
		)
	case api.IsForbidden(err):
		diags.AddError(
			"Not allowed to manage the backup bucket",
			"The API key needs the ADMIN role and the `control-plane:service:manage-backups` permission to manage external backup buckets.",
		)
	default:
		diags.AddError(fmt.Sprintf("Error %s backup bucket", verb), err.Error())
	}
}
