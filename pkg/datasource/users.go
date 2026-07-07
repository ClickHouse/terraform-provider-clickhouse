package datasource

import (
	"context"
	_ "embed"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

//go:embed descriptions/users.md
var usersDataSourceDescription string

var _ datasource.DataSource = &usersDataSource{}

func NewUsersDataSource() datasource.DataSource {
	return &usersDataSource{}
}

type usersDataSource struct {
	client api.Client
}

type usersDataSourceModel struct {
	Users types.List `tfsdk:"users"`
}

func (d *usersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *usersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *usersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: usersDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"users": schema.ListNestedAttribute{
				Description: "List of all members in the organization. Empty if the organization has no members. A user who has only been invited (not yet accepted) is not a member and will not appear here.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "User ID.",
							Computed:    true,
						},
						"email": schema.StringAttribute{
							Description: "Email address of the user.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "Display name of the user.",
							Computed:    true,
						},
						"joined_at": schema.StringAttribute{
							Description: "Timestamp when the user joined the organization.",
							Computed:    true,
						},
						"assigned_roles": schema.ListNestedAttribute{
							Description: "Roles assigned to the user.",
							Computed:    true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"id": schema.StringAttribute{
										Description: "The ID of the assigned role.",
										Computed:    true,
									},
									"name": schema.StringAttribute{
										Description: "The name of the assigned role.",
										Computed:    true,
									},
									"type": schema.StringAttribute{
										Description: "The type of the assigned role (system or custom).",
										Computed:    true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *usersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data usersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	members, err := d.client.ListMembers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing users", "Could not list users: "+err.Error())
		return
	}

	userValues := make([]attr.Value, len(members))
	for i := range members {
		obj, diags := memberToUserObject(members[i])
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		userValues[i] = obj
	}

	usersList, diags := types.ListValue(userObjectType(), userValues)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Users = usersList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func userObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":             types.StringType,
			"email":          types.StringType,
			"name":           types.StringType,
			"joined_at":      types.StringType,
			"assigned_roles": types.ListType{ElemType: types.ObjectType{AttrTypes: assignedRoleAttrTypes}},
		},
	}
}

func memberToUserObject(member api.Member) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	roleValues := make([]attr.Value, len(member.AssignedRoles))
	for i, ar := range member.AssignedRoles {
		obj, d := types.ObjectValue(assignedRoleAttrTypes, map[string]attr.Value{
			"id":   types.StringValue(ar.RoleID),
			"name": types.StringValue(ar.RoleName),
			"type": types.StringValue(ar.RoleType),
		})
		diags.Append(d...)
		if diags.HasError() {
			return types.ObjectNull(userObjectType().AttrTypes), diags
		}
		roleValues[i] = obj
	}
	assignedRoles, d := types.ListValue(types.ObjectType{AttrTypes: assignedRoleAttrTypes}, roleValues)
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(userObjectType().AttrTypes), diags
	}

	obj, d := types.ObjectValue(userObjectType().AttrTypes, map[string]attr.Value{
		"id":             types.StringValue(member.UserID),
		"email":          types.StringValue(member.Email),
		"name":           types.StringValue(member.Name),
		"joined_at":      types.StringValue(member.JoinedAt),
		"assigned_roles": assignedRoles,
	})
	diags.Append(d...)

	return obj, diags
}
