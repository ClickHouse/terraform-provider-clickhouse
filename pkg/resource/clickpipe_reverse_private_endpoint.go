//go:build alpha

package resource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &ClickPipeReversePrivateEndpointResource{}
var _ resource.ResourceWithImportState = &ClickPipeReversePrivateEndpointResource{}

const clickPipeReversePrivateEndpointResourceDescription = `
This experimental resource allows you to create and manage ClickPipes reverse private endpoints for a secure data source connections in ClickHouse Cloud.

**Resource is early access and may change in future releases. Feature coverage might not fully cover all ClickPipe capabilities.**
`

func NewClickPipeReversePrivateEndpointResource() resource.Resource {
	return &ClickPipeReversePrivateEndpointResource{}
}

// ClickPipeReversePrivateEndpointResource defines the resource implementation.
type ClickPipeReversePrivateEndpointResource struct {
	client *api.ClientImpl
}

func (r *ClickPipeReversePrivateEndpointResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickpipes_reverse_private_endpoint"
}

func (r *ClickPipeReversePrivateEndpointResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: clickPipeReversePrivateEndpointResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique identifier for the reverse private endpoint",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"service_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the ClickHouse service to associate with this reverse private endpoint",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Description of the reverse private endpoint",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Type of the reverse private endpoint (VPC_ENDPOINT_SERVICE, VPC_RESOURCE, or MSK_MULTI_VPC)",
				Validators: []validator.String{
					stringvalidator.OneOf(api.ReversePrivateEndpointTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vpc_endpoint_service_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "VPC endpoint service name, required for VPC_ENDPOINT_SERVICE type",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vpc_resource_configuration_id": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "VPC resource configuration ID, required for VPC_RESOURCE type",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vpc_resource_share_arn": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "VPC resource share ARN, required for VPC_RESOURCE type",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"msk_cluster_arn": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "MSK cluster ARN, required for MSK_MULTI_VPC type",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"msk_authentication": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "MSK cluster authentication type (SASL_IAM or SASL_SCRAM), required for MSK_MULTI_VPC type",
				Validators: []validator.String{
					stringvalidator.OneOf(api.MSKAuthenticationTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"endpoint_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Reverse private endpoint endpoint ID",
			},
			"dns_names": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Reverse private endpoint internal DNS names",
			},
			"private_dns_names": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Reverse private endpoint private DNS names",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Status of the reverse private endpoint",
			},
		},
	}
}

func (r *ClickPipeReversePrivateEndpointResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*api.ClientImpl)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *api.ClientImpl, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *ClickPipeReversePrivateEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.ClickPipeReversePrivateEndpointResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := data.ServiceID.ValueString()

	// Validate fields based on type
	switch data.Type.ValueString() {
	case api.ReversePrivateEndpointTypeVPCEndpointService:
		if data.VPCEndpointServiceName.IsNull() {
			resp.Diagnostics.AddError(
				"Missing required field",
				"vpc_endpoint_service_name is required when type is VPC_ENDPOINT_SERVICE",
			)
			return
		}
	case api.ReversePrivateEndpointTypeVPCResource:
		if data.VPCResourceConfigurationID.IsNull() || data.VPCResourceShareArn.IsNull() {
			resp.Diagnostics.AddError(
				"Missing required fields",
				"vpc_resource_configuration_id and vpc_resource_share_arn are required when type is VPC_RESOURCE",
			)
			return
		}
	case api.ReversePrivateEndpointTypeMSKMultiVPC:
		if data.MSKClusterArn.IsNull() || data.MSKAuthentication.IsNull() {
			resp.Diagnostics.AddError(
				"Missing required fields",
				"msk_cluster_arn and msk_authentication are required when type is MSK_MULTI_VPC",
			)
			return
		}
	}

	createReq := api.CreateReversePrivateEndpoint{
		Description: data.Description.ValueString(),
		Type:        data.Type.ValueString(),
	}

	// Set optional fields if provided
	if !data.VPCEndpointServiceName.IsNull() {
		value := data.VPCEndpointServiceName.ValueString()
		createReq.VPCEndpointServiceName = &value
	}
	if !data.VPCResourceConfigurationID.IsNull() {
		value := data.VPCResourceConfigurationID.ValueString()
		createReq.VPCResourceConfigurationID = &value
	}
	if !data.VPCResourceShareArn.IsNull() {
		value := data.VPCResourceShareArn.ValueString()
		createReq.VPCResourceShareArn = &value
	}
	if !data.MSKClusterArn.IsNull() {
		value := data.MSKClusterArn.ValueString()
		createReq.MSKClusterArn = &value
	}
	if !data.MSKAuthentication.IsNull() {
		value := data.MSKAuthentication.ValueString()
		createReq.MSKAuthentication = &value
	}

	// Create new reverse private endpoint
	tflog.Debug(ctx, "Creating ClickPipe reverse private endpoint", map[string]interface{}{
		"service_id": serviceID,
	})

	endpoint, err := r.client.CreateReversePrivateEndpoint(ctx, serviceID, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating ClickPipe reverse private endpoint", err.Error())
		return
	}

	// Wait for the reverse private endpoint status change from provisioning,
	// We expect the endpoint to be in a ready state, however, it can also be in
	// pending acceptance or failed. This will be handled by the provider user.
	endpoint, err = r.client.WaitForReversePrivateEndpointState(ctx, serviceID, endpoint.ID, func(status string) bool {
		return status != api.ReversePrivateEndpointStatusProvisioning
	}, 60*10)

	if err != nil {
		resp.Diagnostics.AddError("Error waiting for ClickPipe reverse private endpoint to be ready", err.Error())
		return
	}

	// Map response body to model
	data.ID = types.StringValue(endpoint.ID)
	data.ServiceID = types.StringValue(serviceID) // Ensure we set the service ID
	data.Description = types.StringValue(endpoint.Description)
	data.Type = types.StringValue(endpoint.Type)
	data.EndpointID = types.StringValue(endpoint.EndpointID)
	data.Status = types.StringValue(endpoint.Status)

	if endpoint.VPCEndpointServiceName != nil {
		data.VPCEndpointServiceName = types.StringValue(*endpoint.VPCEndpointServiceName)
	} else {
		data.VPCEndpointServiceName = types.StringNull()
	}

	if endpoint.VPCResourceConfigurationID != nil {
		data.VPCResourceConfigurationID = types.StringValue(*endpoint.VPCResourceConfigurationID)
	} else {
		data.VPCResourceConfigurationID = types.StringNull()
	}

	if endpoint.VPCResourceShareArn != nil {
		data.VPCResourceShareArn = types.StringValue(*endpoint.VPCResourceShareArn)
	} else {
		data.VPCResourceShareArn = types.StringNull()
	}

	if endpoint.MSKClusterArn != nil {
		data.MSKClusterArn = types.StringValue(*endpoint.MSKClusterArn)
	} else {
		data.MSKClusterArn = types.StringNull()
	}

	if endpoint.MSKAuthentication != nil {
		data.MSKAuthentication = types.StringValue(*endpoint.MSKAuthentication)
	} else {
		data.MSKAuthentication = types.StringNull()
	}

	// Convert string slices to Terraform list values
	dnsNames, diags := types.ListValueFrom(ctx, types.StringType, endpoint.DNSNames)
	resp.Diagnostics.Append(diags...)
	data.DNSNames = dnsNames

	privateDNSNames, diags := types.ListValueFrom(ctx, types.StringType, endpoint.PrivateDNSNames)
	resp.Diagnostics.Append(diags...)
	data.PrivateDNSNames = privateDNSNames

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClickPipeReversePrivateEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.ClickPipeReversePrivateEndpointResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := data.ServiceID.ValueString()
	endpointID := data.ID.ValueString()

	tflog.Debug(ctx, "Reading ClickPipe reverse private endpoint", map[string]interface{}{
		"service_id":  serviceID,
		"endpoint_id": endpointID,
	})

	endpoint, err := r.client.GetReversePrivateEndpoint(ctx, serviceID, endpointID)
	if err != nil {
		if api.IsNotFound(err) {
			// If the resource doesn't exist, remove it from state
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading ClickPipe reverse private endpoint", err.Error())
		return
	}

	// Map response body to model
	data.ID = types.StringValue(endpoint.ID)
	data.ServiceID = types.StringValue(serviceID) // Ensure we keep the service ID
	data.Description = types.StringValue(endpoint.Description)
	data.Type = types.StringValue(endpoint.Type)
	data.EndpointID = types.StringValue(endpoint.EndpointID)
	data.Status = types.StringValue(endpoint.Status)

	if endpoint.VPCEndpointServiceName != nil {
		data.VPCEndpointServiceName = types.StringValue(*endpoint.VPCEndpointServiceName)
	} else {
		data.VPCEndpointServiceName = types.StringNull()
	}

	if endpoint.VPCResourceConfigurationID != nil {
		data.VPCResourceConfigurationID = types.StringValue(*endpoint.VPCResourceConfigurationID)
	} else {
		data.VPCResourceConfigurationID = types.StringNull()
	}

	if endpoint.VPCResourceShareArn != nil {
		data.VPCResourceShareArn = types.StringValue(*endpoint.VPCResourceShareArn)
	} else {
		data.VPCResourceShareArn = types.StringNull()
	}

	if endpoint.MSKClusterArn != nil {
		data.MSKClusterArn = types.StringValue(*endpoint.MSKClusterArn)
	} else {
		data.MSKClusterArn = types.StringNull()
	}

	if endpoint.MSKAuthentication != nil {
		data.MSKAuthentication = types.StringValue(*endpoint.MSKAuthentication)
	} else {
		data.MSKAuthentication = types.StringNull()
	}

	// Convert string slices to Terraform list values
	dnsNames, diags := types.ListValueFrom(ctx, types.StringType, endpoint.DNSNames)
	resp.Diagnostics.Append(diags...)
	data.DNSNames = dnsNames

	privateDNSNames, diags := types.ListValueFrom(ctx, types.StringType, endpoint.PrivateDNSNames)
	resp.Diagnostics.Append(diags...)
	data.PrivateDNSNames = privateDNSNames

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClickPipeReversePrivateEndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// According to the API, reverse private endpoints don't support updates, so we'll return an error
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"ClickPipe reverse private endpoints do not support updates. To change configuration, please delete and recreate the resource.",
	)
}

func (r *ClickPipeReversePrivateEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.ClickPipeReversePrivateEndpointResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := data.ServiceID.ValueString()
	endpointID := data.ID.ValueString()

	tflog.Debug(ctx, "Deleting ClickPipe reverse private endpoint", map[string]interface{}{
		"service_id":  serviceID,
		"endpoint_id": endpointID,
	})

	err := r.client.DeleteReversePrivateEndpoint(ctx, serviceID, endpointID)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting ClickPipe reverse private endpoint", err.Error())
		return
	}

	err = backoff.Retry(func() error {
		rpe, err := r.client.GetReversePrivateEndpoint(ctx, serviceID, endpointID)

		if err != nil {
			if api.IsNotFound(err) {
				return nil // Successfully deleted
			}
			return err
		}

		return fmt.Errorf("ClickPipe reverse private endpoint %s is still present", rpe.ID)
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(5*time.Second), 60*10))

	if err != nil {
		resp.Diagnostics.AddError("Error waiting for ClickPipe reverse private endpoint to be deleted", err.Error())
		return
	}
}

// ImportState imports a ClickPipe reverse private endpoint into the state.
func (r *ClickPipeReversePrivateEndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, ":")

	if len(idParts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import identifier with format: service_id:id. Got: %q", req.ID),
		)
		return
	}

	id := idParts[0]
	endpointID := idParts[1]

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), endpointID)...)
}
