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
	PrivateEndpoints   []PrivateEndpointModel `tfsdk:"private_endpoints"`
	PrivateEndpointIds types.List             `tfsdk:"private_endpoint_ids"`
}

type PrivateEndpointModel struct {
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
			"private_endpoints": schema.ListNestedAttribute{
				Description:  "List of private endpoint ids to register",
				Required:     true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"cloud_provider": schema.StringAttribute{
							Description: "Cloud provider of the private endpoint ID",
							Required: true,
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
				},
			},
			"private_endpoint_ids": schema.ListAttribute{
				Description: "List of private endpoint IDs",
				ElementType: types.StringType,
				Computed:    true,
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
	for _, item := range plan.PrivateEndpoints {
		add = append(add, PrivateEndpoint{
			CloudProvider: item.CloudProvider.ValueString(),
			Description:   item.Description.ValueString(),
			EndpointId:    item.EndpointId.ValueString(),
			Region:        item.Region.ValueString(),
		})
	}

	if len(add) > 0 {
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
	}

	privateEndpoints, err := r.client.GetOrganizationPrivateEndpoints()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching ClickHouse Organization Private Endpoint IDs",
			"Could not fetch organization private endpoint IDs, unexpected error: "+err.Error(),
		)
		return
	}

	privateEndpointIds := []string{}
	for index, privateEndpoint := range *privateEndpoints {
		planPrivateEndpoint := PrivateEndpointModel{
			CloudProvider: types.StringValue(privateEndpoint.CloudProvider),
			EndpointId:    types.StringValue(privateEndpoint.EndpointId),
			Region:        types.StringValue(privateEndpoint.Region),
		}

		if (!plan.PrivateEndpoints[index].Description.IsNull()) {
			planPrivateEndpoint.Description = types.StringValue(privateEndpoint.Description)
		}

		plan.PrivateEndpoints[index] = planPrivateEndpoint
		privateEndpointIds = append(privateEndpointIds, privateEndpoint.EndpointId)
	}
	plan.PrivateEndpointIds, _ = types.ListValueFrom(ctx, types.StringType, privateEndpointIds)

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

	newPrivateEndpoints := []PrivateEndpointModel{}
	privateEndpointIds := []string{}
	for index, privateEndpoint := range *privateEndpoints {
		statePrivateEndpoint := PrivateEndpointModel{
			CloudProvider: types.StringValue(privateEndpoint.CloudProvider),
			EndpointId:    types.StringValue(privateEndpoint.EndpointId),
			Region:        types.StringValue(privateEndpoint.Region),
		}

		if (!(privateEndpoint.Description == "" && state.PrivateEndpoints[index].Description.IsNull())) {
			statePrivateEndpoint.Description = types.StringValue(privateEndpoint.Description)
		}

		newPrivateEndpoints = append(newPrivateEndpoints, statePrivateEndpoint)
		privateEndpointIds = append(privateEndpointIds, privateEndpoint.EndpointId)
	}
	state.PrivateEndpoints = newPrivateEndpoints
	state.PrivateEndpointIds, _ = types.ListValueFrom(ctx, types.StringType, privateEndpointIds)

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

	orgUpdate := OrganizationUpdate{}

	if !equal(plan.PrivateEndpoints, state.PrivateEndpoints) {
		privateEndpointsRawOld := state.PrivateEndpoints
		privateEndpointsRawNew := plan.PrivateEndpoints

		privateEndpointsOld := []PrivateEndpoint{}
		privateEndpointsNew := []PrivateEndpoint{}

		for _, item := range privateEndpointsRawOld {
			privateEndpoint := PrivateEndpoint{
				CloudProvider: item.CloudProvider.ValueString(),
				Description:   item.Description.ValueString(),
				EndpointId:    item.EndpointId.ValueString(),
				Region:        item.Region.ValueString(),
			}

			privateEndpointsOld = append(privateEndpointsOld, privateEndpoint)
		}

		for _, item := range privateEndpointsRawNew {
			privateEndpoint := PrivateEndpoint{
				CloudProvider: item.CloudProvider.ValueString(),
				Description:   item.Description.ValueString(),
				EndpointId:    item.EndpointId.ValueString(),
				Region:        item.Region.ValueString(),
			}

			privateEndpointsNew = append(privateEndpointsNew, privateEndpoint)
		}

		orgUpdate.PrivateEndpoints = &OrgPrivateEndpointsUpdate{
			Add:    privateEndpointsNew,
			Remove: privateEndpointsOld,
		}
	}

	_, err := r.client.UpdateOrganizationPrivateEndpoints(orgUpdate)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Registering ClickHouse Organization Private Endpoint IDs",
			"Could not update organization private endpoint IDs, unexpected error: "+err.Error(),
		)
		return
	}

	privateEndpoints, err := r.client.GetOrganizationPrivateEndpoints()
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching ClickHouse Organization Private Endpoint IDs",
			"Could not fetch organization private endpoint IDs, unexpected error: "+err.Error(),
		)
		return
	}

	privateEndpointIds := []string{}
	for index, privateEndpoint := range *privateEndpoints {
		planPrivateEndpoint := PrivateEndpointModel{
			CloudProvider: types.StringValue(privateEndpoint.CloudProvider),
			EndpointId:    types.StringValue(privateEndpoint.EndpointId),
			Region:        types.StringValue(privateEndpoint.Region),
		}

		if (!plan.PrivateEndpoints[index].Description.IsNull()) {
			planPrivateEndpoint.Description = types.StringValue(privateEndpoint.Description)
		}

		plan.PrivateEndpoints[index] = planPrivateEndpoint
		privateEndpointIds = append(privateEndpointIds, privateEndpoint.EndpointId)
	}
	plan.PrivateEndpointIds, _ = types.ListValueFrom(ctx, types.StringType, privateEndpointIds)

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

	remove := []PrivateEndpoint{}
	for _, item := range state.PrivateEndpoints {
		remove = append(remove, PrivateEndpoint{
			CloudProvider: item.CloudProvider.ValueString(),
			EndpointId:    item.EndpointId.ValueString(),
			Region:        item.Region.ValueString(),
		})
	}

	if len(remove) > 0 {
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
}

func (r *PrivateEndpointRegistrationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
