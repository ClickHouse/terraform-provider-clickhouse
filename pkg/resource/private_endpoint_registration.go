package resource

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
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
	client api.Client
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

	r.client = req.ProviderData.(api.Client)
}

func (r *PrivateEndpointRegistrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.PrivateEndpointRegistration
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	add := []api.PrivateEndpoint{}
	add = append(add, api.PrivateEndpoint{
		CloudProvider: plan.CloudProvider.ValueString(),
		Description:   plan.Description.ValueString(),
		EndpointId:    plan.EndpointId.ValueString(),
		Region:        plan.Region.ValueString(),
	})

	orgUpdate := api.OrganizationUpdate{
		PrivateEndpoints: &api.OrgPrivateEndpointsUpdate{
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
	var state models.PrivateEndpointRegistration
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

	var privateEndpoint *api.PrivateEndpoint
	for _, pe := range *privateEndpoints {
		// openapi validator guarantees uniqueness by ID
		if pe.EndpointId == state.EndpointId.ValueString() {
			clone := pe
			privateEndpoint = &clone
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
	var config, plan, state models.PrivateEndpointRegistration
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	diags = req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)

	orgUpdate := api.OrganizationUpdate{
		PrivateEndpoints: &api.OrgPrivateEndpointsUpdate{
			Add: []api.PrivateEndpoint{
				{
					CloudProvider: plan.CloudProvider.ValueString(),
					Description:   plan.Description.ValueString(),
					EndpointId:    plan.EndpointId.ValueString(),
					Region:        plan.Region.ValueString(),
				},
			},
			Remove: []api.PrivateEndpoint{
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
	var state models.PrivateEndpointRegistration
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	remove := []api.PrivateEndpoint{
		{
			CloudProvider: state.CloudProvider.ValueString(),
			EndpointId:    state.EndpointId.ValueString(),
			Region:        state.Region.ValueString(),
		},
	}

	orgUpdate := api.OrganizationUpdate{
		PrivateEndpoints: &api.OrgPrivateEndpointsUpdate{
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
