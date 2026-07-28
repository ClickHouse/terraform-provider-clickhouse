package clickstack

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// Member status values tracked in state.
const (
	memberStatusActive  = "active"
	memberStatusPending = "pending"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = (*teamMemberResource)(nil)
	_ resource.ResourceWithConfigure   = (*teamMemberResource)(nil)
	_ resource.ResourceWithImportState = (*teamMemberResource)(nil)
)

// NewTeamMemberResource is a helper to register the resource with the provider.
func NewTeamMemberResource() resource.Resource {
	return &teamMemberResource{}
}

// teamMemberResource manages a team member's role assignment. Creating the
// resource invites the user: if they already have an account the role is
// assigned immediately (status "active"); otherwise a pending invitation is
// created (status "pending") and an invite URL is returned.
type teamMemberResource struct {
	client *client.Client
}

// teamMemberResourceModel maps the resource schema data.
type teamMemberResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Team      types.String `tfsdk:"team"`
	Email     types.String `tfsdk:"email"`
	Name      types.String `tfsdk:"name"`
	RoleID    types.String `tfsdk:"role_id"`
	Status    types.String `tfsdk:"status"`
	InviteURL types.String `tfsdk:"invite_url"`
}

func (r *teamMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_team_member"
}

func (r *teamMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a team member and their RBAC role. On create the member is invited: " +
			"existing accounts are assigned the role immediately (`status` = `active`), otherwise a " +
			"pending invitation is created (`status` = `pending`) and `invite_url` is populated. " +
			"**Note:** on ClickHouse Cloud, team membership is managed through ClickHouse Cloud (the " +
			"`clickhouse_role_assignment` resource), not the ClickStack API; this resource is for self-hosted ClickStack.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed: true,
				Description: "Identifier of the member. For an active member this is their user ID; " +
					"for a pending invitation this is the invitation ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID to manage this member under, sent as the `x-hdx-team` header. " +
					"Defaults to the API key's team. Changing this forces the member to be replaced.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			emailAttr: schema.StringAttribute{
				Required:    true,
				Description: "Email address of the member. Changing this forces the member to be replaced.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			nameAttr: schema.StringAttribute{
				Optional: true,
				Description: "Display name used when creating a pending invitation. Changing this forces " +
					"the member to be replaced.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			roleIDAttr: schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "ID of the role to assign to the member. Use the `clickstack_role` resource or data source to obtain it. " +
					"Omit on OSS deployments, which have no RBAC and ignore the role. When omitted, the role assigned by the " +
					"server (e.g. the team's default role on RBAC deployments) is tracked in state.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			statusAttr: schema.StringAttribute{
				Computed:    true,
				Description: "Membership status: `active` (account assigned the role) or `pending` (invitation outstanding).",
			},
			inviteURLAttr: schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Join URL for a pending invitation. Empty for active members.",
			},
		},
	}
}

func (r *teamMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
		addNotConfiguredError(&resp.Diagnostics, "resource")
		return
	}
	r.client = providerData.ClickStack
}

func (r *teamMemberResource) ValidateConfig(_ context.Context, _ resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_clickstack_team_member", &resp.Diagnostics)
}

func (r *teamMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan teamMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.WithTeam(plan.Team.ValueString()).InviteTeamMember(ctx, client.InviteTeamMemberInput{
		Email:  plan.Email.ValueString(),
		RoleID: plan.RoleID.ValueString(),
		Name:   plan.Name.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Inviting Team Member", err.Error())
		return
	}

	applyInviteResult(&plan, result)
	tflog.Trace(ctx, "created team member resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *teamMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state teamMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scoped := r.client.WithTeam(state.Team.ValueString())
	email := state.Email.ValueString()

	// Prefer an active membership; an accepted invite transitions here.
	members, err := scoped.ListTeamMembers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Team Members", err.Error())
		return
	}
	for _, m := range members {
		if strings.EqualFold(m.Email, email) {
			state.ID = types.StringValue(m.ID)
			state.RoleID = resolveRoleID(state.RoleID, m.RoleID)
			state.Status = types.StringValue(memberStatusActive)
			state.InviteURL = types.StringValue("")
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	// Otherwise look for a still-pending invitation.
	invitations, err := scoped.ListInvitations(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Team Invitations", err.Error())
		return
	}
	for _, inv := range invitations {
		if strings.EqualFold(inv.Email, email) {
			state.ID = types.StringValue(inv.ID)
			state.RoleID = resolveRoleID(state.RoleID, inv.RoleID)
			state.Status = types.StringValue(memberStatusPending)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	// Neither a member nor a pending invite: the resource is gone.
	resp.State.RemoveResource(ctx)
}

func (r *teamMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state teamMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scoped := r.client.WithTeam(plan.Team.ValueString())

	if state.Status.ValueString() == memberStatusActive {
		// Active members have their role updated in place.
		if err := scoped.UpdateMemberRole(ctx, state.ID.ValueString(), plan.RoleID.ValueString()); err != nil {
			resp.Diagnostics.AddError("Error Updating Team Member Role", err.Error())
			return
		}
		plan.ID = state.ID
		plan.Status = types.StringValue(memberStatusActive)
		plan.InviteURL = types.StringValue("")
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	// Pending invitations carry their role on the invite itself, so changing
	// the role means deleting the old invite and re-issuing it.
	if err := scoped.DeleteInvitation(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Error Replacing Team Invitation", err.Error())
		return
	}
	result, err := scoped.InviteTeamMember(ctx, client.InviteTeamMemberInput{
		Email:  plan.Email.ValueString(),
		RoleID: plan.RoleID.ValueString(),
		Name:   plan.Name.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Re-inviting Team Member", err.Error())
		return
	}

	applyInviteResult(&plan, result)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *teamMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state teamMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scoped := r.client.WithTeam(state.Team.ValueString())

	var err error
	if state.Status.ValueString() == memberStatusActive {
		err = scoped.RemoveMember(ctx, state.ID.ValueString())
	} else {
		err = scoped.DeleteInvitation(ctx, state.ID.ValueString())
	}
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Error Deleting Team Member", err.Error())
	}
}

func (r *teamMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Accept "<email>" or "<team>/<email>". The remaining attributes are
	// resolved during the import Read by matching on email.
	if team, email, ok := strings.Cut(req.ID, "/"); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team"), team)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(emailAttr), email)...)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root(emailAttr), req, resp)
}

// resolveRoleID returns the role_id to persist in state: the server's value
// when present, otherwise the prior state value. OSS deployments have no RBAC
// and return an empty role, so preserving the prior (including a null) avoids
// resetting a configured role and creating perpetual drift.
func resolveRoleID(prior types.String, fromServer string) types.String {
	if fromServer != "" {
		return types.StringValue(fromServer)
	}
	return prior
}

// applyInviteResult copies an invite result into the model.
func applyInviteResult(m *teamMemberResourceModel, result *client.InviteResult) {
	// The invite API returns no role back, so a role_id left unknown by the
	// plan (Computed attribute omitted from config on create) can't be
	// resolved here. Persist null instead of an unknown value; the next Read
	// fills in the server-assigned role via resolveRoleID.
	if m.RoleID.IsUnknown() {
		m.RoleID = types.StringNull()
	}
	m.Status = types.StringValue(result.Status)
	if result.Status == memberStatusActive && result.UserID != nil {
		m.ID = types.StringValue(*result.UserID)
		m.InviteURL = types.StringValue("")
		return
	}
	if result.InvitationID != nil {
		m.ID = types.StringValue(*result.InvitationID)
	}
	m.InviteURL = types.StringValue(result.URL)
}
