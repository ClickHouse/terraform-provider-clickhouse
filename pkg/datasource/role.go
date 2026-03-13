package datasource

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

//go:embed descriptions/role.md
var roleDataSourceDescription string

var _ datasource.DataSource = &roleDataSource{}

func NewRoleDataSource() datasource.DataSource {
	return &roleDataSource{}
}

type roleDataSource struct {
	client api.Client
}

type roleDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	TenantID  types.String `tfsdk:"tenant_id"`
	OwnerID   types.String `tfsdk:"owner_id"`
	Type      types.String `tfsdk:"type"`
	Actors    types.List   `tfsdk:"actors"`
	Policies  types.List   `tfsdk:"policies"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (d *roleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *roleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (d *roleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: roleDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier of the role. Exactly one of id or name must be set.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the role to look up. Exactly one of id or name must be set.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name")),
				},
			},
			"tenant_id": schema.StringAttribute{
				Description: "Tenant ID that owns this role.",
				Computed:    true,
			},
			"owner_id": schema.StringAttribute{
				Description: "Owner ID of this role.",
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
	}
}

func (d *roleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data roleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var role *api.RBACRole

	if !data.ID.IsNull() && !data.ID.IsUnknown() {
		r, err := d.client.GetRole(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading role", "Could not read role: "+err.Error())
			return
		}
		role = r
	} else {
		roles, err := d.client.ListRoles(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error listing roles", "Could not list roles: "+err.Error())
			return
		}
		name := data.Name.ValueString()
		for i := range roles {
			if roles[i].Name == name {
				role = &roles[i]
				break
			}
		}
		if role == nil {
			resp.Diagnostics.AddError(
				"Role not found",
				fmt.Sprintf("No role with name %q found in the organization.", name),
			)
			return
		}
	}

	data.ID = types.StringValue(role.ID)
	data.Name = types.StringValue(role.Name)
	data.TenantID = types.StringValue(role.TenantID)
	data.OwnerID = types.StringValue(role.OwnerID)
	data.Type = types.StringValue(role.Type)
	data.CreatedAt = types.StringValue(role.CreatedAt)
	data.UpdatedAt = types.StringValue(role.UpdatedAt)

	actorValues := make([]attr.Value, len(role.Actors))
	for i, a := range role.Actors {
		actorValues[i] = types.StringValue(a)
	}
	actorsList, diags := types.ListValue(types.StringType, actorValues)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Actors = actorsList

	policyValues := make([]attr.Value, len(role.Policies))
	for i, p := range role.Policies {
		policyModel, diags := models.APIToRolePolicyModel(p)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		policyValues[i] = policyModel.ObjectValue()
	}
	policiesList, diags := types.ListValue(models.RolePolicyModel{}.ObjectType(), policyValues)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Policies = policiesList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
