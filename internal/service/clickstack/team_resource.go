package clickstack

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = (*teamResource)(nil)
	_ resource.ResourceWithConfigure   = (*teamResource)(nil)
	_ resource.ResourceWithImportState = (*teamResource)(nil)
)

// NewTeamResource is a helper to register the resource with the provider.
func NewTeamResource() resource.Resource {
	return &teamResource{}
}

// teamResource manages team-level settings. The team itself is provisioned
// out-of-band (at signup); this resource adopts the existing team on create
// and manages its settings, currently the default new-user role.
type teamResource struct {
	client *client.Client
}

// teamResourceModel maps the resource schema data.
type teamResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Team            types.String `tfsdk:"team"`
	DefaultUserRole types.String `tfsdk:"default_user_role_id"`
}

func (r *teamResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_team"
}

func (r *teamResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	utils.AlphaWarning("clickhouse_clickstack_team", &resp.Diagnostics)
	resp.Schema = schema.Schema{
		Description: "Manages settings for an existing ClickStack team. The team is provisioned " +
			"out-of-band; this resource adopts it on create and manages its settings. Destroying " +
			"this resource does not delete the team or reset its settings.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:    true,
				Description: "Identifier of the team.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID to manage, sent as the `x-hdx-team` header. Defaults to the " +
					"API key's team. Changing this forces the resource to be replaced.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"default_user_role_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the role assigned to new users who join the team.",
			},
		},
	}
}

func (r *teamResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("expected *service.ProviderData, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}

	if providerData.ClickStack == nil {
		resp.Diagnostics.AddError("ClickStack not configured",
			"This resource requires ClickStack credentials. Set clickstack_api_key on the "+
				"provider (or the CLICKSTACK_API_KEY environment variable), and clickstack_endpoint if not using ClickHouse Cloud.")
		return
	}
	r.client = providerData.ClickStack
}

func (r *teamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.AlphaWarning("clickhouse_clickstack_team", &resp.Diagnostics)
	var plan teamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scoped := r.client.WithTeam(plan.Team.ValueString())

	team, err := scoped.GetTeam(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Team", err.Error())
		return
	}

	roleID, err := scoped.SetDefaultUserRole(ctx, plan.DefaultUserRole.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Setting Default User Role", err.Error())
		return
	}

	plan.ID = types.StringValue(team.ID)
	plan.DefaultUserRole = types.StringPointerValue(roleID)
	tflog.Trace(ctx, "adopted team resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *teamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	team, err := r.client.WithTeam(state.Team.ValueString()).GetTeam(ctx)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Team", err.Error())
		return
	}

	state.ID = types.StringValue(team.ID)
	state.DefaultUserRole = types.StringPointerValue(team.DefaultUserRole)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *teamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan teamResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleID, err := r.client.WithTeam(plan.Team.ValueString()).SetDefaultUserRole(ctx, plan.DefaultUserRole.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Setting Default User Role", err.Error())
		return
	}

	plan.DefaultUserRole = types.StringPointerValue(roleID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *teamResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// The team is provisioned out-of-band and its default role cannot be
	// cleared (the API requires a role ID), so destroying this resource only
	// removes it from state and leaves the team settings untouched.
}

func (r *teamResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by team ID, which is also used as the x-hdx-team header so the
	// import Read resolves the correct team.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
