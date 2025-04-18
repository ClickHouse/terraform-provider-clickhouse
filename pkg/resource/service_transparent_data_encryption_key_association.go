package resource

import (
	"context"
	_ "embed"
	"errors"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &ServiceTransparentDataEncryptionKeyAssociationResource{}
	_ resource.ResourceWithConfigure   = &ServiceTransparentDataEncryptionKeyAssociationResource{}
	_ resource.ResourceWithImportState = &ServiceTransparentDataEncryptionKeyAssociationResource{}
)

//go:embed descriptions/service_transparent_data_encryption_key_association.md
var serviceTransparentDataEncryptionKeyAssociationResourceDescription string

// NewServiceTransparentDataEncryptionKeyAssociationResource is a helper function to simplify the provider implementation.
func NewServiceTransparentDataEncryptionKeyAssociationResource() resource.Resource {
	return &ServiceTransparentDataEncryptionKeyAssociationResource{}
}

// ServiceTransparentDataEncryptionKeyAssociationResource is the resource implementation.
type ServiceTransparentDataEncryptionKeyAssociationResource struct {
	client api.Client
}

// Metadata returns the resource type name.
func (r *ServiceTransparentDataEncryptionKeyAssociationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_transparent_data_encryption_key_association"
}

// Schema defines the schema for the resource.
func (r *ServiceTransparentDataEncryptionKeyAssociationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Service ID",
				Required:    true,
			},
			"key_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the Encryption key to use for data encryption. Must be an ARN for AWS services or a Key Resource Path for GCP services.",
			},
		},
		MarkdownDescription: serviceResourceDescription,
	}
}

// Configure adds the provider configured client to the resource.
func (r *ServiceTransparentDataEncryptionKeyAssociationResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(api.Client)
}

func (r *ServiceTransparentDataEncryptionKeyAssociationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		// If the entire plan is null, the resource is planned for destruction.
		return
	}

	var plan, state, config models.ServiceTransparentDataEncryptionKeyAssociation
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if !req.State.Raw.IsNull() {
		diags = req.State.Get(ctx, &state)
		resp.Diagnostics.Append(diags...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if !req.Config.Raw.IsNull() {
		diags = req.Config.Get(ctx, &config)
		resp.Diagnostics.Append(diags...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if !req.State.Raw.IsNull() {
		// Validations for updates.

		return
	}

	// Validations for new instances.
}

// Create a new resource
func (r *ServiceTransparentDataEncryptionKeyAssociationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan models.ServiceTransparentDataEncryptionKeyAssociation
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check service has TDE enabled
	svc, err := r.client.GetService(ctx, plan.ServiceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error checking if service has TDE enabled",
			"Could not get service, unexpected error: "+err.Error(),
		)
		return
	}

	if !svc.HasTransparentDataEncryption {
		resp.Diagnostics.AddError(
			"Service has TDE disabled",
			"Service does not have Transparent Data Encryption (TDE) feature enabled.",
		)
		return
	}

	err = r.client.RotateTDEKey(ctx, plan.ServiceID.ValueString(), plan.KeyID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating TDE encryption key association",
			"Could not create TDE encryption key association, unexpected error: "+err.Error(),
		)
		return
	}

	err = r.syncServiceState(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error syncing service state",
			"Could not sync service state, unexpected error: "+err.Error(),
		)
		return
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *ServiceTransparentDataEncryptionKeyAssociationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ServiceTransparentDataEncryptionKeyAssociation
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.syncServiceState(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error syncing service state",
			"Could not sync service state, unexpected error: "+err.Error(),
		)
		return
	}

	if state.ServiceID.IsNull() {
		// Resource was deleted outside terraform
		resp.State.RemoveResource(ctx)
	} else {
		// Set refreshed state
		diags = resp.State.Set(ctx, state)
		resp.Diagnostics.Append(diags...)
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *ServiceTransparentDataEncryptionKeyAssociationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan, state models.ServiceTransparentDataEncryptionKeyAssociation
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// TDE Key rotation
	err := r.client.RotateTDEKey(ctx, plan.ServiceID.ValueString(), plan.KeyID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error rotating TDE encryption key",
			"Could not rotate TDE encryption, unexpected error: "+err.Error(),
		)
		return
	}

	err = r.syncServiceState(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error syncing service state",
			"Could not sync service state, unexpected error: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *ServiceTransparentDataEncryptionKeyAssociationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddError(
		"Deleting Transparent Data Encryption Key association is not supported",
		"Deleting Transparent Data Encryption Key association is not supported. If you want, you can specify a new encryption key. If you need to turn off Transparent Data Encryption, you need to recreate the ClickHouse service.",
	)
	return
}

func (r *ServiceTransparentDataEncryptionKeyAssociationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Retrieve import ID and save to id attribute
	resource.ImportStatePassthroughID(ctx, path.Root("service_id"), req, resp)
}

// syncServiceState fetches the latest state ClickHouse Cloud API and updates the Terraform state.
func (r *ServiceTransparentDataEncryptionKeyAssociationResource) syncServiceState(ctx context.Context, state *models.ServiceTransparentDataEncryptionKeyAssociation) error {
	if state.ServiceID.IsNull() {
		return errors.New("service ID must be set to fetch the service")
	}

	// Get latest service value from ClickHouse OpenAPI
	service, err := r.client.GetService(ctx, state.ServiceID.ValueString())
	if api.IsNotFound(err) {
		// Service was deleted outside terraform.
		state.ServiceID = types.StringNull()

		return nil
	} else if err != nil {
		return err
	}

	state.KeyID = types.StringValue(service.TransparentEncryptionDataKeyID)

	return nil
}
