package clickhouse

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &PrivateEndpointRegistrationResource{}
	_ resource.ResourceWithConfigure   = &PrivateEndpointRegistrationResource{}
	_ resource.ResourceWithImportState = &PrivateEndpointRegistrationResource{}
)

func NewPrivateEndpointRegistrationResource() resource.Resource {
	return &PrivateEndpointRegistrationResource{}
}

type PrivateEndpointRegistrationResource struct {
	client *Client
}

type PrivateEndpointRegistrationResourceModel struct {
	CloudProvider types.String `tfsdk:"cloud_provider"`
	Description   types.String `tfsdk:"description"`
	EndpointId    types.String `tfsdk:"id"`
	Region        types.String `tfsdk:"region"`
}

func (r *PrivateEndpointRegistrationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_private_endpoint_registration"
}

func (r *PrivateEndpointRegistrationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider of the private endpoint ID",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Description of the private endpoint",
				Optional:    true,
			},
			"id": schema.StringAttribute{
				Description: "ID of the private endpoint",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "Region of the private endpoint",
				Required:    true,
			},
		},
	}
}

func (r *PrivateEndpointRegistrationResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(*Client)
}

func (r *PrivateEndpointRegistrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PrivateEndpointRegistrationResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	add := []PrivateEndpoint{}
	add = append(add, PrivateEndpoint{
		CloudProvider: plan.CloudProvider.ValueString(),
		Description:   plan.Description.ValueString(),
		EndpointId:    plan.EndpointId.ValueString(),
		Region:        plan.Region.ValueString(),
	})

	orgUpdate := OrganizationUpdate{
		PrivateEndpoints: &OrgPrivateEndpointsUpdate{
			Add: add,
		},
	}

	_, err := r.client.UpdateOrganizationPrivateEndpoints(orgUpdate)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Registering ClickHouse Organization Private Endpoint IDs",
			"Could not update organization private endpoint IDs, unexpected error: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *PrivateEndpointRegistrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PrivateEndpointRegistrationResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	privateEndpoints, err := r.client.GetOrganizationPrivateEndpoints()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading ClickHouse Organization Private Endpoint IDs",
			"Could not read organization private endpoint IDs, unexpected error: "+err.Error(),
		)
		return
	}

	var privateEndpoint *PrivateEndpoint
	for _, pe := range *privateEndpoints {
		// openapi validator guarantees uniqueness by ID
		if pe.EndpointId == state.EndpointId.ValueString() {
			privateEndpoint = &PrivateEndpoint{
				CloudProvider: pe.CloudProvider,
				Description:   pe.Description,
				EndpointId:    pe.EndpointId,
				Region:        pe.Region,
			}
			break
		}
	}

	if privateEndpoint == nil {
		resp.Diagnostics.AddError("Private endpoint not found", "Could not find private endpoint in org registration")
		return
	}

	state.Description = types.StringValue(privateEndpoint.Description)
	state.Region = types.StringValue(privateEndpoint.Region)
	state.CloudProvider = types.StringValue(privateEndpoint.CloudProvider)

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *PrivateEndpointRegistrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config, plan, state PrivateEndpointRegistrationResourceModel
	diags := req.Plan.Get(ctx, &plan)
	diags = req.State.Get(ctx, &state)
	diags = req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)

	orgUpdate := OrganizationUpdate{
		PrivateEndpoints: &OrgPrivateEndpointsUpdate{
			Add: []PrivateEndpoint{
				{
					CloudProvider: plan.CloudProvider.ValueString(),
					Description:   plan.Description.ValueString(),
					EndpointId:    plan.EndpointId.ValueString(),
					Region:        plan.Region.ValueString(),
				},
			},
			Remove: []PrivateEndpoint{
				{
					CloudProvider: state.CloudProvider.ValueString(),
					Description:   state.Description.ValueString(),
					EndpointId:    state.EndpointId.ValueString(),
					Region:        state.Region.ValueString(),
				},
			},
		},
	}

	_, err := r.client.UpdateOrganizationPrivateEndpoints(orgUpdate)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Registering ClickHouse Organization Private Endpoint IDs",
			"Could not update organization private endpoint IDs, unexpected error: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *PrivateEndpointRegistrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PrivateEndpointRegistrationResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	remove := []PrivateEndpoint{
		{
			CloudProvider: state.CloudProvider.ValueString(),
			EndpointId:    state.EndpointId.ValueString(),
			Region:        state.Region.ValueString(),
		},
	}

	orgUpdate := OrganizationUpdate{
		PrivateEndpoints: &OrgPrivateEndpointsUpdate{
			Remove: remove,
		},
	}

	_, err := r.client.UpdateOrganizationPrivateEndpoints(orgUpdate)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Registering ClickHouse Organization Private Endpoint IDs",
			"Could not update organization private endpoint IDs, unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *PrivateEndpointRegistrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
