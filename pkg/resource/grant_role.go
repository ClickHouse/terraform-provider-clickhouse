//go:build alpha

package resource

import (
	"context"
	_ "embed"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

//go:embed descriptions/grant_role.md
var grantResourceDescription string

var (
	_ resource.Resource              = &GrantRoleResource{}
	_ resource.ResourceWithConfigure = &GrantRoleResource{}
)

func NewGrantRoleResource() resource.Resource {
	return &GrantRoleResource{}
}

type GrantRoleResource struct {
	client api.Client
}

func (r *GrantRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_grant_role"
}

func (r *GrantRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Service ID",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role_name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the role to be granted",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"grantee_user_name": schema.StringAttribute{
				Optional:    true,
				Description: "Name of the `user` to grant `role_name` to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.Expressions{path.MatchRoot("grantee_role_name")}...),
					stringvalidator.AtLeastOneOf(path.Expressions{
						path.MatchRoot("grantee_user_name"),
						path.MatchRoot("grantee_role_name"),
					}...),
				},
			},
			"grantee_role_name": schema.StringAttribute{
				Optional:    true,
				Description: "Name of the `role` to grant `role_name` to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.Expressions{path.MatchRoot("grantee_user_name")}...),
					stringvalidator.AtLeastOneOf(path.Expressions{
						path.MatchRoot("grantee_user_name"),
						path.MatchRoot("grantee_role_name"),
					}...),
				},
			},
			"admin_option": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "If true, the grantee will be able to grant `role_name` to other `users` or `roles`.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
		},
		MarkdownDescription: grantResourceDescription,
	}
}

func (r *GrantRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(api.Client)
}

func (r *GrantRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.GrantRole
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	grant := api.GrantRole{
		RoleName:        plan.RoleName.ValueString(),
		GranteeUserName: plan.GranteeUserName.ValueStringPointer(),
		GranteeRoleName: plan.GranteeRoleName.ValueStringPointer(),
		AdminOption:     plan.AdminOption.ValueBool(),
	}

	createdGrant, err := r.client.CreateGrant(ctx, plan.ServiceID.ValueString(), grant)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating ClickHouse Role Grant",
			"Could not create role grant, unexpected error: "+err.Error(),
		)
		return
	}

	state := models.GrantRole{
		ServiceID:       plan.ServiceID,
		RoleName:        types.StringValue(createdGrant.RoleName),
		GranteeUserName: types.StringPointerValue(createdGrant.GranteeUserName),
		GranteeRoleName: types.StringPointerValue(createdGrant.GranteeRoleName),
		AdminOption:     types.BoolValue(createdGrant.AdminOption),
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *GrantRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.GrantRole
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	grant, err := r.client.GetGrant(ctx, state.ServiceID.ValueString(), state.RoleName.ValueString(), state.GranteeUserName.ValueStringPointer(), state.GranteeRoleName.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading ClickHouse Role Grant",
			"Could not read role grant, unexpected error: "+err.Error(),
		)
		return
	}

	if grant != nil {
		state.RoleName = types.StringValue(grant.RoleName)
		state.GranteeUserName = types.StringPointerValue(grant.GranteeUserName)
		state.GranteeRoleName = types.StringPointerValue(grant.GranteeRoleName)
		state.AdminOption = types.BoolValue(grant.AdminOption)

		diags = resp.State.Set(ctx, &state)
		resp.Diagnostics.Append(diags...)
	} else {
		resp.State.RemoveResource(ctx)
	}
}

func (r *GrantRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	panic("Update of grant resource is not supported")
}

func (r *GrantRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.GrantRole
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteGrant(ctx, state.ServiceID.ValueString(), state.RoleName.ValueString(), state.GranteeUserName.ValueStringPointer(), state.GranteeRoleName.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting ClickHouse Role Grant",
			"Could not delete role grant, unexpected error: "+err.Error(),
		)
		return
	}
}
