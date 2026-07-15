package resource

import (
	"context"
	_ "embed"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &InvitationResource{}
	_ resource.ResourceWithConfigure   = &InvitationResource{}
	_ resource.ResourceWithImportState = &InvitationResource{}
)

//go:embed descriptions/invitation.md
var invitationResourceDescription string

func NewInvitationResource() resource.Resource {
	return &InvitationResource{}
}

type InvitationResource struct {
	client api.Client
}

type InvitationModel struct {
	ID              types.String `tfsdk:"id"`
	Email           types.String `tfsdk:"email"`
	AssignedRoleIDs types.Set    `tfsdk:"assigned_role_ids"`
	Role            types.String `tfsdk:"role"`
	CreatedAt       types.String `tfsdk:"created_at"`
	ExpireAt        types.String `tfsdk:"expire_at"`
}

func (r *InvitationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_invitation"
}

func (r *InvitationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: invitationResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier of the invitation.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				Description: "Email address of the invited user.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"assigned_role_ids": schema.SetAttribute{
				Description: "Set of role IDs to assign to the invited user when they accept the invitation. Look up system role IDs with the clickhouse_role data source, or use the ID of a clickhouse_role resource for custom roles. Conflicts with role.",
				Optional:    true,
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description:        "Deprecated legacy organization role for the invited user (\"admin\" or \"developer\"). Use assigned_role_ids instead. Conflicts with assigned_role_ids.",
				DeprecationMessage: "Use assigned_role_ids instead.",
				Optional:           true,
				Validators: []validator.String{
					stringvalidator.OneOf("admin", "developer"),
					stringvalidator.ConflictsWith(path.MatchRoot("assigned_role_ids")),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"created_at": schema.StringAttribute{
				Description: "ISO-8601 timestamp of when the invitation was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"expire_at": schema.StringAttribute{
				Description: "ISO-8601 timestamp of when the invitation expires.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *InvitationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			"Expected api.Client, got something else. Please report this issue to the provider developers.",
		)
		return
	}
	r.client = client
}

func (r *InvitationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan InvitationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var roleIDs []string
	if !plan.AssignedRoleIDs.IsNull() && !plan.AssignedRoleIDs.IsUnknown() {
		resp.Diagnostics.Append(plan.AssignedRoleIDs.ElementsAs(ctx, &roleIDs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	invitation, err := r.client.CreateInvitation(ctx, api.CreateInvitationRequest{
		Email:           plan.Email.ValueString(),
		AssignedRoleIds: roleIDs,
		Role:            plan.Role.ValueString(),
	})
	if err != nil {
		// If the email already belongs to a member the API returns 409. Treat
		// that as a fulfilled onboarding rather than a hard error: the user is
		// already in the org, so re-inviting is unnecessary. There is no
		// invitation to reference, so key the resource off the member's user ID;
		// a later Read will 404 on GetInvitation and reconcile via membership.
		if api.IsConflict(err) {
			member, memberErr := r.findMemberByEmail(ctx, plan.Email.ValueString())
			if memberErr != nil {
				resp.Diagnostics.AddError("Error creating invitation", "Invitation returned a conflict and membership could not be verified: "+memberErr.Error())
				return
			}
			if member != nil {
				tflog.Info(ctx, "Invitation target is already a member; treating as fulfilled", map[string]any{"email": plan.Email.ValueString()})
				plan.ID = types.StringValue(member.UserID)
				plan.CreatedAt = types.StringValue("")
				plan.ExpireAt = types.StringValue("")
				resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
				return
			}
		}
		resp.Diagnostics.AddError("Error creating invitation", "Could not create invitation: "+err.Error())
		return
	}

	// Only the computed fields come from the server. email/role/assigned_role_ids
	// are immutable inputs and are left exactly as configured to satisfy
	// Terraform's post-apply consistency check (the server may expand a legacy
	// `role` into assignedRoles, which must not leak back into assigned_role_ids).
	plan.ID = types.StringValue(invitation.ID)
	plan.CreatedAt = types.StringValue(invitation.CreatedAt)
	plan.ExpireAt = types.StringValue(invitation.ExpireAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *InvitationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state InvitationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	invitation, err := r.client.GetInvitation(ctx, state.ID.ValueString())
	if err != nil {
		if !api.IsNotFound(err) {
			resp.Diagnostics.AddError("Error reading invitation", "Could not read invitation: "+err.Error())
			return
		}

		// The invitation is gone. This is ambiguous: it may have been accepted
		// (consumed server-side) or revoked/expired. Cross-check membership by
		// email so an accepted invite is NOT re-created on the next apply.
		member, memberErr := r.findMemberByEmail(ctx, state.Email.ValueString())
		if memberErr != nil {
			resp.Diagnostics.AddError("Error reconciling invitation", "Invitation was not found and membership could not be verified: "+memberErr.Error())
			return
		}
		if member == nil {
			// Genuinely revoked/expired externally: drop from state so it is
			// re-created on the next apply.
			resp.State.RemoveResource(ctx)
			return
		}

		// Accepted: the user is now a member. Keep the resource in state unchanged
		// so onboarding is not repeated. The member's current roles are managed by
		// clickhouse_role_assignment and deliberately not reconciled here.
		tflog.Info(ctx, "Invitation was accepted; user is now a member", map[string]any{"email": state.Email.ValueString()})
		resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
		return
	}

	// Pending invitation: refresh computed fields only. On import (email still
	// null because ImportState only set the id) also populate the immutable
	// inputs from the server so the imported state is usable.
	state.ID = types.StringValue(invitation.ID)
	state.CreatedAt = types.StringValue(invitation.CreatedAt)
	state.ExpireAt = types.StringValue(invitation.ExpireAt)

	if state.Email.IsNull() {
		state.Email = types.StringValue(invitation.Email)
		if invitation.Role != "" {
			state.Role = types.StringValue(invitation.Role)
		}
		roleIDs := make([]string, 0, len(invitation.AssignedRoles))
		for _, ar := range invitation.AssignedRoles {
			roleIDs = append(roleIDs, ar.RoleID)
		}
		roleSet, d := invitationRoleSet(roleIDs, true)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		state.AssignedRoleIDs = roleSet
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is required by the resource.Resource interface but is unreachable in
// practice: email, assigned_role_ids and role all force replacement, and the
// remaining attributes are computed. If it is ever invoked, persist the plan
// (which equals prior state) so no drift is introduced.
func (r *InvitationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan InvitationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *InvitationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state InvitationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteInvitation(ctx, state.ID.ValueString())
	if err != nil {
		// Already consumed (accepted) or revoked: nothing to do.
		if api.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error deleting invitation", "Could not revoke invitation: "+err.Error())
	}
}

func (r *InvitationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// findMemberByEmail returns the member whose email matches, or nil if none is
// found. There is no email-based member lookup at the API layer, so this mirrors
// the client-side scan the clickhouse_user data source performs.
func (r *InvitationResource) findMemberByEmail(ctx context.Context, email string) (*api.Member, error) {
	members, err := r.client.ListMembers(ctx)
	if err != nil {
		return nil, err
	}
	for i := range members {
		if members[i].Email == email {
			return &members[i], nil
		}
	}
	return nil, nil
}

// invitationRoleSet builds a types.Set of role IDs. When the source list is
// empty and keepNull is true it returns a null set to preserve the distinction
// between "not configured" (null) and "configured but empty" (set([])).
func invitationRoleSet(ids []string, keepNull bool) (types.Set, diag.Diagnostics) {
	if len(ids) == 0 && keepNull {
		return types.SetNull(types.StringType), nil
	}
	values := make([]attr.Value, len(ids))
	for i, id := range ids {
		values[i] = types.StringValue(id)
	}
	return types.SetValue(types.StringType, values)
}
