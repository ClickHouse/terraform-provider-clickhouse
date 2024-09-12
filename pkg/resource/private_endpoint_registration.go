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
			"private_endpoint_id": schema.StringAttribute{
				Description: "ID of the private endpoint (replaces deprecated attribute `id`)",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "Region of the private endpoint",
				Required:    true,
			},
		},
		MarkdownDescription: `ClickHouse Cloud provides the ability to connect your services to your cloud virtual network through a feature named *Private Link*.

You can use the *private_endpoint_registration* resource to set up the private link feature.

Check the [docs](https://clickhouse.com/docs/en/cloud/security/private-link-overview) for more details on *private link*.`,
		Version: 1,
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

	_, err := r.client.UpdateOrganizationPrivateEndpoints(ctx, orgUpdate)
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

	privateEndpoints, err := r.client.GetOrganizationPrivateEndpoints(ctx)
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

	if privateEndpoint != nil {
		state.Description = types.StringValue(privateEndpoint.Description)
		state.Region = types.StringValue(privateEndpoint.Region)
		state.CloudProvider = types.StringValue(privateEndpoint.CloudProvider)
		state.EndpointId = types.StringValue(privateEndpoint.EndpointId)

		diags = resp.State.Set(ctx, &state)
		resp.Diagnostics.Append(diags...)
	} else {
		resp.State.RemoveResource(ctx)
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

	_, err := r.client.UpdateOrganizationPrivateEndpoints(ctx, orgUpdate)
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

	_, err := r.client.UpdateOrganizationPrivateEndpoints(ctx, orgUpdate)
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

func (r *PrivateEndpointRegistrationResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		// In version 1.0.0 we renamed the `id` field to `private_endpoint_id`.
		0: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"cloud_provider": schema.StringAttribute{
						Required: true,
					},
					"description": schema.StringAttribute{
						Optional: true,
					},
					"id": schema.StringAttribute{
						Required: true,
					},
					"region": schema.StringAttribute{
						Required: true,
					},
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				type oldPrivateEndpointRegistration struct {
					CloudProvider types.String `tfsdk:"cloud_provider"`
					Description   types.String `tfsdk:"description"`
					EndpointId    types.String `tfsdk:"id"`
					Region        types.String `tfsdk:"region"`
				}

				var priorStateData oldPrivateEndpointRegistration

				resp.Diagnostics.Append(req.State.Get(ctx, &priorStateData)...)

				if resp.Diagnostics.HasError() {
					return
				}

				upgradedStateData := models.PrivateEndpointRegistration{
					CloudProvider: priorStateData.CloudProvider,
					Description:   priorStateData.Description,
					EndpointId:    priorStateData.EndpointId,
					Region:        priorStateData.Region,
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, upgradedStateData)...)
			},
		},
	}
}
