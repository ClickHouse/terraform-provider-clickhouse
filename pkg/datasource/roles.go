package datasource

import (
	"context"
	_ "embed"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

//go:embed descriptions/roles.md
var rolesDataSourceDescription string

var _ datasource.DataSource = &rolesDataSource{}

func NewRolesDataSource() datasource.DataSource {
	return &rolesDataSource{}
}

type rolesDataSource struct {
	client api.Client
}

type rolesDataSourceModel struct {
	Roles types.List `tfsdk:"roles"`
}

func (d *rolesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			"Expected api.Client, got something else. Please report this issue to the provider developers.",
		)
		return
	}
	d.client = client
}

func (d *rolesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_roles"
}

func (d *rolesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: rolesDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"roles": schema.ListNestedAttribute{
				Description: "List of all roles in the organization.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "Unique identifier for the role.",
							Computed:    true,
						},
						"tenant_id": schema.StringAttribute{
							Description: "Tenant ID that owns this role.",
							Computed:    true,
						},
						"owner_id": schema.StringAttribute{
							Description: "Owner ID of this role.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "Name of the role.",
							Computed:    true,
						},
						"type": schema.StringAttribute{
							Description: "Type of the role: 'system' or 'custom'.",
							Computed:    true,
						},
						"actors": schema.ListAttribute{
							Description: "List of actors assigned to this role.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"policies": rolePoliciesSchemaAttribute(),
						"created_at": schema.StringAttribute{
							Description: "Timestamp when the role was created.",
							Computed:    true,
						},
						"updated_at": schema.StringAttribute{
							Description: "Timestamp when the role was last updated.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *rolesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data rolesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roles, err := d.client.ListRoles(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error listing roles",
			"Could not list roles: "+err.Error(),
		)
		return
	}

	roleValues := make([]attr.Value, len(roles))
	for i, role := range roles {
		roleModel, diags := apiRoleToDataSourceModel(role)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		roleValues[i] = roleModel
	}

	rolesList, diags := types.ListValue(roleObjectType(), roleValues)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Roles = rolesList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// rolePoliciesSchemaAttribute returns the shared schema definition for the
// policies field used by both the clickhouse_role and clickhouse_roles data sources.
func rolePoliciesSchemaAttribute() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description: "List of policies attached to this role.",
		Computed:    true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"id": schema.StringAttribute{
					Description: "Server-assigned policy ID.",
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
					Computed:    true,
				},
				"permissions": schema.SetAttribute{
					Description: "List of permission strings.",
					Computed:    true,
					ElementType: types.StringType,
				},
				"resources": schema.SetAttribute{
					Description: "List of resources this policy applies to.",
					Computed:    true,
					ElementType: types.StringType,
				},
				"tags": schema.SingleNestedAttribute{
					Description: "Optional tags for additional policy metadata.",
					Computed:    true,
					Attributes: map[string]schema.Attribute{
						"role": schema.StringAttribute{
							Description: "SQL console role level for passwordless DB access. One of: sql-console-admin (full access), sql-console-readonly (read-only).",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func roleObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":         types.StringType,
			"tenant_id":  types.StringType,
			"owner_id":   types.StringType,
			"name":       types.StringType,
			"type":       types.StringType,
			"actors":     types.ListType{ElemType: types.StringType},
			"policies":   types.ListType{ElemType: models.RolePolicyModel{}.ObjectType()},
			"created_at": types.StringType,
			"updated_at": types.StringType,
		},
	}
}

func apiRoleToDataSourceModel(role api.RBACRole) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	actorValues := make([]attr.Value, len(role.Actors))
	for i, a := range role.Actors {
		actorValues[i] = types.StringValue(a)
	}
	actorsList, d := types.ListValue(types.StringType, actorValues)
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(roleObjectType().AttrTypes), diags
	}

	policyValues := make([]attr.Value, len(role.Policies))
	for i, p := range role.Policies {
		policyModel, d := models.APIToRolePolicyModel(p)
		diags.Append(d...)
		if diags.HasError() {
			return types.ObjectNull(roleObjectType().AttrTypes), diags
		}
		policyValues[i] = policyModel.ObjectValue()
	}
	policiesList, d := types.ListValue(models.RolePolicyModel{}.ObjectType(), policyValues)
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(roleObjectType().AttrTypes), diags
	}

	obj, d := types.ObjectValue(roleObjectType().AttrTypes, map[string]attr.Value{
		"id":         types.StringValue(role.ID),
		"tenant_id":  types.StringValue(role.TenantID),
		"owner_id":   types.StringValue(role.OwnerID),
		"name":       types.StringValue(role.Name),
		"type":       types.StringValue(role.Type),
		"actors":     actorsList,
		"policies":   policiesList,
		"created_at": types.StringValue(role.CreatedAt),
		"updated_at": types.StringValue(role.UpdatedAt),
	})
	diags.Append(d...)

	return obj, diags
}
