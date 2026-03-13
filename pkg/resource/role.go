//go:build alpha

package resource

import (
	"context"
	_ "embed"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource                = &RoleResource{}
	_ resource.ResourceWithConfigure   = &RoleResource{}
	_ resource.ResourceWithImportState = &RoleResource{}
)

//go:embed descriptions/role.md
var roleResourceDescription string

func NewRoleResource() resource.Resource {
	return &RoleResource{}
}

type RoleResource struct {
	client api.Client
}

func (r *RoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *RoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: roleResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier for the role.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_id": schema.StringAttribute{
				Description: "Tenant ID that owns this role.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"owner_id": schema.StringAttribute{
				Description: "Owner ID of this role.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the custom role.",
				Required:    true,
			},
			"type": schema.StringAttribute{
				Description: "Type of the role. Always 'custom' for managed roles.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"policies": schema.ListNestedAttribute{
				Description: "List of policies attached to this role.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "Server-assigned policy ID. Changes on every update since the server replaces all policies on PATCH.",
							Computed:    true,
						},
						"role_id": schema.StringAttribute{
							Description: "ID of the role this policy belongs to.",
							Computed:    true,
						},
						"tenant_id": schema.StringAttribute{
							Description: "Tenant ID that owns this policy.",
							Computed:    true,
						},
						"effect": schema.StringAttribute{
							Description: "Whether this policy allows or denies the specified permissions.",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.OneOf(api.RBACAllowDenyValues...),
							},
						},
						"permissions": schema.SetAttribute{
							Description: "List of permission strings granted or denied by this policy.",
							Required:    true,
							ElementType: types.StringType,
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
							},
						},
						"resources": schema.SetAttribute{
							Description: "List of resources this policy applies to. Format: 'instance/<uuid>' or 'instance/*'.",
							Required:    true,
							ElementType: types.StringType,
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
							},
						},
						"tags": schema.SingleNestedAttribute{
							Description: "Optional tags for additional policy metadata.",
							Optional:    true,
							Attributes: map[string]schema.Attribute{
								"role": schema.StringAttribute{
									Description: "SQL console role level for passwordless DB access. One of: sql-console-admin (full access), sql-console-readonly (read-only).",
									Required:    true,
									Validators: []validator.String{
										stringvalidator.OneOf(api.RBACPolicyRoleV2Values...),
									},
								},
							},
						},
					},
				},
			},
			"created_at": schema.StringAttribute{
				Description: "Timestamp when the role was created.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Description: "Timestamp when the role was last updated.",
				Computed:    true,
			},
		},
	}
}

func (r *RoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.RoleResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, d := planPoliciesToAPICreate(ctx, plan.Policies)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq := api.RoleCreateRequest{
		Name:     plan.Name.ValueString(),
		Actors:   []string{},
		Policies: policies,
	}

	role, err := r.client.CreateRole(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating role",
			"Could not create role: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(applyRoleToState(role, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *RoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.RoleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	syncDiags, err := r.syncRoleState(ctx, &state)
	resp.Diagnostics.Append(syncDiags...)
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading role",
			"Could not read role: "+err.Error(),
		)
		return
	}

	if state.Type.ValueString() != api.RBACRoleTypeCustom {
		resp.Diagnostics.AddError(
			"Cannot manage system role",
			"Role "+state.Name.ValueString()+" is a system role and cannot be managed by this resource. Only custom roles are supported.",
		)
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *RoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.RoleResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state models.RoleResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	policies, d := planPoliciesToAPICreate(ctx, plan.Policies)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq := api.RoleUpdateRequest{
		Name:     plan.Name.ValueString(),
		Policies: &policies,
	}

	role, err := r.client.UpdateRole(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating role",
			"Could not update role: "+err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(applyRoleToState(role, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *RoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.RoleResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteRole(ctx, state.ID.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error deleting role",
			"Could not delete role: "+err.Error(),
		)
	}
}

func (r *RoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// syncRoleState fetches the role from the API and updates the state model.
// Returns diagnostics from state mapping and a separate error for API failures;
// callers should check api.IsNotFound on the error.
func (r *RoleResource) syncRoleState(ctx context.Context, state *models.RoleResourceModel) (diag.Diagnostics, error) {
	role, err := r.client.GetRole(ctx, state.ID.ValueString())
	if err != nil {
		return nil, err
	}
	return applyRoleToState(role, state), nil
}

// applyRoleToState maps an API RBACRole response into the Terraform state model.
func applyRoleToState(role *api.RBACRole, state *models.RoleResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(role.ID)
	state.TenantID = types.StringValue(role.TenantID)
	state.OwnerID = types.StringValue(role.OwnerID)
	state.Name = types.StringValue(role.Name)
	state.Type = types.StringValue(role.Type)
	state.CreatedAt = types.StringValue(role.CreatedAt)
	state.UpdatedAt = types.StringValue(role.UpdatedAt)

	// Policies — preserve null vs empty-list distinction from the plan/prior state.
	// If the plan omitted policies (null), keep null. If it was set to [], keep [].
	if len(role.Policies) == 0 {
		if state.Policies.IsNull() || state.Policies.IsUnknown() {
			state.Policies = types.ListNull(models.RolePolicyModel{}.ObjectType())
		} else {
			emptyList, d := types.ListValue(models.RolePolicyModel{}.ObjectType(), []attr.Value{})
			diags.Append(d...)
			state.Policies = emptyList
		}
	} else {
		policyValues := make([]attr.Value, len(role.Policies))
		for i, p := range role.Policies {
			policyModel, d := models.APIToRolePolicyModel(p)
			diags.Append(d...)
			if diags.HasError() {
				return diags
			}
			policyValues[i] = policyModel.ObjectValue()
		}
		policiesList, d := types.ListValue(models.RolePolicyModel{}.ObjectType(), policyValues)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		state.Policies = policiesList
	}

	return diags
}

func planPoliciesToAPICreate(ctx context.Context, policiesList types.List) ([]api.RBACPolicyCreateRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	if policiesList.IsNull() || policiesList.IsUnknown() {
		return []api.RBACPolicyCreateRequest{}, diags
	}

	var policyModels []models.RolePolicyModel
	diags.Append(policiesList.ElementsAs(ctx, &policyModels, false)...)
	if diags.HasError() {
		return nil, diags
	}

	result := make([]api.RBACPolicyCreateRequest, len(policyModels))
	for i, pm := range policyModels {
		var permissions []string
		diags.Append(pm.Permissions.ElementsAs(ctx, &permissions, false)...)
		if diags.HasError() {
			return nil, diags
		}

		var resources []string
		diags.Append(pm.Resources.ElementsAs(ctx, &resources, false)...)
		if diags.HasError() {
			return nil, diags
		}

		var tags *api.RBACPolicyTags
		if !pm.Tags.IsNull() && !pm.Tags.IsUnknown() {
			var tagsModel models.RolePolicyTagsModel
			diags.Append(pm.Tags.As(ctx, &tagsModel, basetypes.ObjectAsOptions{})...)
			if diags.HasError() {
				return nil, diags
			}

			tags = &api.RBACPolicyTags{}
			if !tagsModel.RoleV2.IsNull() && !tagsModel.RoleV2.IsUnknown() {
				tags.RoleV2 = tagsModel.RoleV2.ValueString()
			}
		}

		result[i] = api.RBACPolicyCreateRequest{
			AllowDeny:   api.RBACAllowDeny(pm.Effect.ValueString()),
			Permissions: permissions,
			Resources:   resources,
			Tags:        tags,
		}
	}

	return result, diags
}
