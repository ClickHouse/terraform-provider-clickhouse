package resource

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/tfutils"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

var (
	_ resource.Resource                = &ServicePrivateEndpointsAttachmentResource{}
	_ resource.ResourceWithConfigure   = &ServicePrivateEndpointsAttachmentResource{}
	_ resource.ResourceWithImportState = &ServicePrivateEndpointsAttachmentResource{}
)

func NewServicePrivateEndpointsAttachmentResource() resource.Resource {
	return &ServicePrivateEndpointsAttachmentResource{}
}

type ServicePrivateEndpointsAttachmentResource struct {
	client api.Client
}

func (r *ServicePrivateEndpointsAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_private_endpoints_attachment"
}

func (r *ServicePrivateEndpointsAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"private_endpoint_ids": schema.ListAttribute{
				Description: "List of private endpoint IDs",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Default:     listdefault.StaticValue(tfutils.CreateEmptyList(types.StringType)),
			},
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Servie ID",
				Optional:    true,
			},
		},
	}
}

func (r *ServicePrivateEndpointsAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(api.Client)
}

func (r *ServicePrivateEndpointsAttachmentResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		// If the entire plan is null, the resource is planned for destruction.
		return
	}

	var plan, state models.ServicePrivateEndpointsAttachment
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if !req.State.Raw.IsNull() {
		diags = req.State.Get(ctx, &state)
		resp.Diagnostics.Append(diags...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.ServiceId.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("service_id"),
			"clickhouse_service_private_endpoints_attachment is invalid",
			"service_id must be set",
		)
	}

	if len(plan.PrivateEndpointIds.Elements()) == 0 {
		resp.Diagnostics.AddAttributeError(
			path.Root("private_endpoint_ids"),
			"clickhouse_service_private_endpoints_attachment is invalid",
			"private_endpoint_ids must be set",
		)
	}
}

func (r *ServicePrivateEndpointsAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.ServicePrivateEndpointsAttachment
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	service := api.ServiceUpdate{
		PrivateEndpointIds: &api.PrivateEndpointIdsUpdate{
			Add: []string{},
		},
	}
	servicePrivateEndpointIds := make([]types.String, 0, len(plan.PrivateEndpointIds.Elements()))
	plan.PrivateEndpointIds.ElementsAs(ctx, &servicePrivateEndpointIds, false)
	for _, item := range servicePrivateEndpointIds {
		service.PrivateEndpointIds.Add = append(service.PrivateEndpointIds.Add, item.ValueString())
	}

	_, err := r.client.UpdateService(plan.ServiceId.ValueString(), service)
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

func (r *ServicePrivateEndpointsAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ServicePrivateEndpointsAttachment
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get latest service value from ClickHouse OpenAPI
	service, err := r.client.GetService(state.ServiceId.ValueString())
	if api.IsNotFound(err) {
		// Service not found, hence attachment cannot exist as well.
		resp.State.RemoveResource(ctx)
		return
	} else if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading ClickHouse Service",
			"Could not read ClickHouse service private endpoints service id"+state.ServiceId.ValueString()+": "+err.Error(),
		)
		return
	}

	// Overwrite items with refreshed state
	if len(service.PrivateEndpointIds) == 0 {
		resp.State.RemoveResource(ctx)
		return
	} else {
		state.PrivateEndpointIds, _ = types.ListValueFrom(ctx, types.StringType, service.PrivateEndpointIds)
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *ServicePrivateEndpointsAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config, plan, state models.ServicePrivateEndpointsAttachment
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	diags = req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)

	service := api.ServiceUpdate{
		PrivateEndpointIds: &api.PrivateEndpointIdsUpdate{
			Add:    []string{},
			Remove: []string{},
		},
	}
	servicePrivateEndpointIds := make([]types.String, 0, len(plan.PrivateEndpointIds.Elements()))
	plan.PrivateEndpointIds.ElementsAs(ctx, &servicePrivateEndpointIds, false)
	for _, item := range servicePrivateEndpointIds {
		service.PrivateEndpointIds.Add = append(service.PrivateEndpointIds.Add, item.ValueString())
	}

	servicePrivateEndpointIds = make([]types.String, 0, len(state.PrivateEndpointIds.Elements()))
	state.PrivateEndpointIds.ElementsAs(ctx, &servicePrivateEndpointIds, false)
	for _, item := range servicePrivateEndpointIds {
		service.PrivateEndpointIds.Remove = append(service.PrivateEndpointIds.Add, item.ValueString())
	}

	_, err := r.client.UpdateService(plan.ServiceId.ValueString(), service)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Registering ClickHouse Organization Private Endpoint IDs",
			"Could not update organization private endpoint IDs, service id"+state.ServiceId.ValueString()+": "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ServicePrivateEndpointsAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ServicePrivateEndpointsAttachment
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	service := api.ServiceUpdate{
		PrivateEndpointIds: &api.PrivateEndpointIdsUpdate{
			Remove: []string{},
		},
	}

	servicePrivateEndpointIds := make([]types.String, 0, len(state.PrivateEndpointIds.Elements()))
	state.PrivateEndpointIds.ElementsAs(ctx, &servicePrivateEndpointIds, false)
	for _, item := range servicePrivateEndpointIds {
		service.PrivateEndpointIds.Remove = append(service.PrivateEndpointIds.Add, item.ValueString())
	}

	_, err := r.client.UpdateService(state.ServiceId.ValueString(), service)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Registering ClickHouse Organization Private Endpoint IDs",
			"Could not update organization private endpoint IDs, service id"+state.ServiceId.ValueString()+": "+err.Error(),
		)
		return
	}
}

func (r *ServicePrivateEndpointsAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("service_id"), req, resp)
}
