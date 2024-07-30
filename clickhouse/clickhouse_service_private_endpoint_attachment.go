package clickhouse

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ClickhouseServicePrivateEndpointAttachmentResource{}
	_ resource.ResourceWithConfigure   = &ClickhouseServicePrivateEndpointAttachmentResource{}
	_ resource.ResourceWithImportState = &ClickhouseServicePrivateEndpointAttachmentResource{}
)

func NewClickhouseServicePrivateEndpointAttachmentResource() resource.Resource {
	return &ClickhouseServicePrivateEndpointAttachmentResource{}
}

type ClickhouseServicePrivateEndpointAttachmentResource struct {
	client *Client
}

type ClickhouseServicePrivateEndpointAttachmentModel struct {
	PrivateEndpointIds types.List   `tfsdk:"private_endpoint_ids"`
	ServiceId          types.String `tfsdk:"service_id"`
}

func (r *ClickhouseServicePrivateEndpointAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_private_endpoint_attachment"
}

func (r *ClickhouseServicePrivateEndpointAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"private_endpoint_ids": schema.ListAttribute{
				Description: "List of private endpoint IDs",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Default:     listdefault.StaticValue(createEmptyList(types.StringType)),
			},
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Servie ID",
				Required:    true,
			},
		},
	}
}

func (r *ClickhouseServicePrivateEndpointAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(*Client)
}

func (r *ClickhouseServicePrivateEndpointAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ClickhouseServicePrivateEndpointAttachmentModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	service := ServiceUpdate{
		Name:         "",
		IpAccessList: nil,
		PrivateEndpointIds: &PrivateEndpointIdsUpdate{
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

func (r *ClickhouseServicePrivateEndpointAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ClickhouseServicePrivateEndpointAttachmentModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get latest service value from ClickHouse OpenAPI
	service, err := r.client.GetService(state.ServiceId.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading ClickHouse Service",
			"Could not read ClickHouse service edpoints service id"+state.ServiceId.ValueString()+": "+err.Error(),
		)
		return
	}

	// Overwrite items with refreshed state
	if len(service.PrivateEndpointIds) == 0 {
		state.PrivateEndpointIds = createEmptyList(types.StringType)
	} else {
		state.PrivateEndpointIds, _ = types.ListValueFrom(ctx, types.StringType, service.PrivateEndpointIds)
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *ClickhouseServicePrivateEndpointAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config, plan, state ClickhouseServicePrivateEndpointAttachmentModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	diags = req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)

	service := ServiceUpdate{
		Name:         "",
		IpAccessList: nil,
		PrivateEndpointIds: &PrivateEndpointIdsUpdate{
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

func (r *ClickhouseServicePrivateEndpointAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ClickhouseServicePrivateEndpointAttachmentModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	service := ServiceUpdate{
		Name:         "",
		IpAccessList: nil,
		PrivateEndpointIds: &PrivateEndpointIdsUpdate{
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

func (r *ClickhouseServicePrivateEndpointAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("service_id"), req, resp)
}
