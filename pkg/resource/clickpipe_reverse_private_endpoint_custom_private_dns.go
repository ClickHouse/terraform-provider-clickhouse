package resource

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

var (
	_ resource.Resource                = &ClickPipeReversePrivateEndpointCustomPrivateDNSResource{}
	_ resource.ResourceWithConfigure   = &ClickPipeReversePrivateEndpointCustomPrivateDNSResource{}
	_ resource.ResourceWithImportState = &ClickPipeReversePrivateEndpointCustomPrivateDNSResource{}
)

//go:embed descriptions/clickpipes_reverse_private_endpoint_custom_private_dns.md
var clickPipeReversePrivateEndpointCustomPrivateDNSResourceDescription string

func NewClickPipeReversePrivateEndpointCustomPrivateDNSResource() resource.Resource {
	return &ClickPipeReversePrivateEndpointCustomPrivateDNSResource{}
}

// ClickPipeReversePrivateEndpointCustomPrivateDNSResource manages custom private DNS mappings for a reverse private endpoint.
type ClickPipeReversePrivateEndpointCustomPrivateDNSResource struct {
	client *api.ClientImpl
}

func (r *ClickPipeReversePrivateEndpointCustomPrivateDNSResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickpipes_reverse_private_endpoint_custom_private_dns"
}

func (r *ClickPipeReversePrivateEndpointCustomPrivateDNSResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: clickPipeReversePrivateEndpointCustomPrivateDNSResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Resource identifier in service_id:reverse_private_endpoint_id format.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the ClickHouse service that owns the reverse private endpoint.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"reverse_private_endpoint_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The ID of the ClickPipes reverse private endpoint to manage custom private DNS mappings for.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mapping": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "Full replacement list of custom private DNS mappings. Use an empty list to clear mappings.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"private_dns_name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Custom private DNS name managed by ClickHouse Cloud.",
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
					},
				},
			},
		},
	}
}

func (r *ClickPipeReversePrivateEndpointCustomPrivateDNSResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func customPrivateDNSResourceID(serviceID, reversePrivateEndpointID string) string {
	return serviceID + ":" + reversePrivateEndpointID
}

func customPrivateDNSMappingsFromPlan(ctx context.Context, mappings types.List) ([]api.CustomPrivateDNSMapping, diag.Diagnostics) {
	var diags diag.Diagnostics

	if mappings.IsNull() {
		return []api.CustomPrivateDNSMapping{}, diags
	}
	if mappings.IsUnknown() {
		diags.AddError("Unknown mapping", "The mapping value must be known before custom private DNS mappings can be applied.")
		return nil, diags
	}

	var mappingModels []models.CustomPrivateDNSMappingModel
	diags.Append(mappings.ElementsAs(ctx, &mappingModels, false)...)
	if diags.HasError() {
		return nil, diags
	}

	result := make([]api.CustomPrivateDNSMapping, len(mappingModels))
	for i, mapping := range mappingModels {
		result[i] = api.CustomPrivateDNSMapping{
			PrivateDNSName: mapping.PrivateDNSName.ValueString(),
		}
	}

	return result, diags
}

func customPrivateDNSMappingsToModel(mappings []api.CustomPrivateDNSMapping) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics

	mappingValues := make([]attr.Value, len(mappings))
	for i, mapping := range mappings {
		mappingValues[i] = models.CustomPrivateDNSMappingModel{
			PrivateDNSName: types.StringValue(mapping.PrivateDNSName),
		}.ObjectValue()
	}

	mappingList, d := types.ListValue(models.CustomPrivateDNSMappingModel{}.ObjectType(), mappingValues)
	diags.Append(d...)

	return mappingList, diags
}

func applyReversePrivateEndpointCustomPrivateDNSToModel(endpoint *api.ReversePrivateEndpoint, data *models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	serviceID := data.ServiceID.ValueString()
	reversePrivateEndpointID := data.ReversePrivateEndpointID.ValueString()
	data.ID = types.StringValue(customPrivateDNSResourceID(serviceID, reversePrivateEndpointID))

	mapping, d := customPrivateDNSMappingsToModel(endpoint.CustomPrivateDNSMappings)
	diags.Append(d...)
	data.Mapping = mapping

	return diags
}

func (r *ClickPipeReversePrivateEndpointCustomPrivateDNSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, ok := r.updateCustomPrivateDNSMappings(ctx, &data, &resp.Diagnostics)
	if !ok {
		return
	}

	resp.Diagnostics.Append(applyReversePrivateEndpointCustomPrivateDNSToModel(endpoint, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClickPipeReversePrivateEndpointCustomPrivateDNSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := data.ServiceID.ValueString()
	reversePrivateEndpointID := data.ReversePrivateEndpointID.ValueString()

	tflog.Debug(ctx, "Reading ClickPipe reverse private endpoint custom private DNS mappings", map[string]interface{}{
		"service_id":                  serviceID,
		"reverse_private_endpoint_id": reversePrivateEndpointID,
	})

	endpoint, err := r.client.GetReversePrivateEndpoint(ctx, serviceID, reversePrivateEndpointID)
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading ClickPipe reverse private endpoint", err.Error())
		return
	}

	resp.Diagnostics.Append(applyReversePrivateEndpointCustomPrivateDNSToModel(endpoint, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClickPipeReversePrivateEndpointCustomPrivateDNSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, ok := r.updateCustomPrivateDNSMappings(ctx, &data, &resp.Diagnostics)
	if !ok {
		return
	}

	resp.Diagnostics.Append(applyReversePrivateEndpointCustomPrivateDNSToModel(endpoint, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ClickPipeReversePrivateEndpointCustomPrivateDNSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := data.ServiceID.ValueString()
	reversePrivateEndpointID := data.ReversePrivateEndpointID.ValueString()
	mappings := []api.CustomPrivateDNSMapping{}

	tflog.Debug(ctx, "Clearing ClickPipe reverse private endpoint custom private DNS mappings", map[string]interface{}{
		"service_id":                  serviceID,
		"reverse_private_endpoint_id": reversePrivateEndpointID,
	})

	_, err := r.client.UpdateReversePrivateEndpoint(ctx, serviceID, reversePrivateEndpointID, api.UpdateReversePrivateEndpoint{
		CustomPrivateDNSMappings: &mappings,
	})
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error clearing ClickPipe reverse private endpoint custom private DNS mappings", err.Error())
	}
}

func (r *ClickPipeReversePrivateEndpointCustomPrivateDNSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	idParts := strings.Split(req.ID, ":")
	if len(idParts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected import identifier with format: service_id:reverse_private_endpoint_id. Got: %q", req.ID),
		)
		return
	}

	mapping, diags := customPrivateDNSMappingsToModel(nil)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel{
		ID:                       types.StringValue(req.ID),
		ServiceID:                types.StringValue(idParts[0]),
		ReversePrivateEndpointID: types.StringValue(idParts[1]),
		Mapping:                  mapping,
	})...)
}

func (r *ClickPipeReversePrivateEndpointCustomPrivateDNSResource) updateCustomPrivateDNSMappings(ctx context.Context, data *models.ClickPipeReversePrivateEndpointCustomPrivateDNSResourceModel, diags *diag.Diagnostics) (*api.ReversePrivateEndpoint, bool) {
	serviceID := data.ServiceID.ValueString()
	reversePrivateEndpointID := data.ReversePrivateEndpointID.ValueString()
	mappings, d := customPrivateDNSMappingsFromPlan(ctx, data.Mapping)
	diags.Append(d...)
	if diags.HasError() {
		return nil, false
	}

	tflog.Debug(ctx, "Updating ClickPipe reverse private endpoint custom private DNS mappings", map[string]interface{}{
		"service_id":                  serviceID,
		"reverse_private_endpoint_id": reversePrivateEndpointID,
	})

	endpoint, err := r.client.UpdateReversePrivateEndpoint(ctx, serviceID, reversePrivateEndpointID, api.UpdateReversePrivateEndpoint{
		CustomPrivateDNSMappings: &mappings,
	})
	if err != nil {
		diags.AddError("Error updating ClickPipe reverse private endpoint custom private DNS mappings", err.Error())
		return nil, false
	}

	return endpoint, true
}
