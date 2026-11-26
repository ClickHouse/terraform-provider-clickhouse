package resource

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// The connector installs the platform layer before the infrastructure
// reports ready; budget like a standard provisioning.
const byocReadyWaitSeconds = 90 * 60

var (
	_ resource.Resource              = (*ByocInfrastructureReadyResource)(nil)
	_ resource.ResourceWithConfigure = (*ByocInfrastructureReadyResource)(nil)
)

//go:embed descriptions/byoc_infrastructure_ready.md
var byocInfrastructureReadyResourceDescription string

func NewByocInfrastructureReadyResource() resource.Resource {
	return &ByocInfrastructureReadyResource{}
}

type ByocInfrastructureReadyResource struct {
	client api.Client
}

type byocInfrastructureReadyModel struct {
	ID     types.String `tfsdk:"id"`
	ByocID types.String `tfsdk:"byoc_id"`
	State  types.String `tfsdk:"state"`
}

func (r *ByocInfrastructureReadyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_byoc_infrastructure_ready"
}

func (r *ByocInfrastructureReadyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: byocInfrastructureReadyResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the BYOC infrastructure.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"byoc_id": schema.StringAttribute{
				Description: "ID of the BYOC infrastructure to wait for.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"state": schema.StringAttribute{
				Description: "State of the infrastructure when readiness was confirmed. Not refreshed afterwards; read the clickhouse_byoc_infrastructure data source for the current state.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ByocInfrastructureReadyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError(
			"ClickHouse Cloud API not configured",
			"This resource requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).",
		)
		return
	}
	r.client = providerData.API
}

// byocReadyWaitDone stops the wait on readiness and on the states readiness
// cannot follow from without someone acting.
func byocReadyWaitDone(state string) bool {
	switch state {
	case api.ByocStateReady, api.ByocStateDegraded, api.ByocStateTerminating, api.ByocStateTerminated:
		return true
	}
	return false
}

func (r *ByocInfrastructureReadyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning("clickhouse_byoc_infrastructure_ready", &resp.Diagnostics)

	var plan byocInfrastructureReadyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	byocId := plan.ByocID.ValueString()

	err := r.client.WaitForByocInfrastructureState(ctx, byocId, byocReadyWaitDone, byocReadyWaitSeconds)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for BYOC infrastructure to become ready",
			fmt.Sprintf("BYOC infrastructure %s did not reach the %q state: %s\n\nFor a restricted-access infrastructure, check that the connector was installed with a valid enrollment token and is running (the clickhouse_byoc_infrastructure data source reports its liveness in 'connector'), then run terraform apply again to resume waiting.", byocId, api.ByocStateReady, err.Error()),
		)
		return
	}

	details, err := r.client.GetByocInfrastructure(ctx, byocId)
	if err != nil {
		resp.Diagnostics.AddError("Error reading BYOC infrastructure", err.Error())
		return
	}
	if details.State != api.ByocStateReady {
		detail := fmt.Sprintf("BYOC infrastructure %s entered state %q instead of %q.", byocId, details.State, api.ByocStateReady)
		if details.Connector != nil && details.Connector.PlatformDetail != nil {
			detail += "\n\nConnector: " + *details.Connector.PlatformDetail
		}
		resp.Diagnostics.AddError("BYOC infrastructure did not become ready", detail)
		return
	}

	plan.ID = plan.ByocID
	plan.State = types.StringValue(details.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read keeps the recorded readiness: it is a one-time gate for dependent
// resources, and a later degradation must not plan a new wait.
func (r *ByocInfrastructureReadyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state byocInfrastructureReadyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable in practice: byoc_id requires replacement.
func (r *ByocInfrastructureReadyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan byocInfrastructureReadyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ByocInfrastructureReadyResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}
