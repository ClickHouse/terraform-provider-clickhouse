package resource

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

var (
	_ resource.Resource              = &PrivateEndpointRegistrationResource{}
	_ resource.ResourceWithConfigure = &PrivateEndpointRegistrationResource{}
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
		DeprecationMessage: "This resource is deprecated since version 3.2.0. Please refer to [the docs](https://github.com/ClickHouse/terraform-provider-clickhouse?tab=readme-ov-file#breaking-changes-and-deprecations) for migration steps.",
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
		MarkdownDescription: `This resource is deprecated since version 3.2.0. Please refer to the docs for migration steps: https://github.com/ClickHouse/terraform-provider-clickhouse?tab=readme-ov-file#breaking-changes-and-deprecations`,
		Version:             1,
	}
}

func (r *PrivateEndpointRegistrationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *PrivateEndpointRegistrationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError(
		"Deprecated resource",
		"The 'clickhouse_private_endpoint_registration' is deprecated and can't be used any more to create new instances.",
	)
}

func (r *PrivateEndpointRegistrationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// This function is a no-op to avoid breaking customer workloads during the deprecation phase.
}

func (r *PrivateEndpointRegistrationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// This function is a no-op to avoid breaking customer workloads during the deprecation phase.
}

func (r *PrivateEndpointRegistrationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.State.RemoveResource(ctx)
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
