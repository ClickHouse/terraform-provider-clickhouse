package datasource

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

//go:embed descriptions/user.md
var userDataSourceDescription string

var _ datasource.DataSource = &userDataSource{}

func NewUserDataSource() datasource.DataSource {
	return &userDataSource{}
}

type userDataSource struct {
	client api.Client
}

var assignedRoleAttrTypes = map[string]attr.Type{
	"id":   types.StringType,
	"name": types.StringType,
	"type": types.StringType,
}

type userDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Email         types.String `tfsdk:"email"`
	Name          types.String `tfsdk:"name"`
	JoinedAt      types.String `tfsdk:"joined_at"`
	AssignedRoles types.List   `tfsdk:"assigned_roles"`
}

func (d *userDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: userDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "User ID. Exactly one of id or email must be set.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("email")),
				},
			},
			"email": schema.StringAttribute{
				Description: "Email address of the user. Exactly one of id or email must be set.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("email")),
				},
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
	}
}

func (d *userDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data userDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var member *api.Member

	if !data.ID.IsNull() && !data.ID.IsUnknown() {
		m, err := d.client.GetMember(ctx, data.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error reading user", "Could not read user: "+err.Error())
			return
		}
		member = m
	} else {
		members, err := d.client.ListMembers(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error listing users", "Could not list users: "+err.Error())
			return
		}
		email := data.Email.ValueString()
		for i := range members {
			if members[i].Email == email {
				member = &members[i]
				break
			}
		}
		if member == nil {
			resp.Diagnostics.AddError(
				"User not found",
				fmt.Sprintf("No user with email %q found in the organization.", email),
			)
			return
		}
	}

	data.ID = types.StringValue(member.UserID)
	data.Email = types.StringValue(member.Email)
	data.Name = types.StringValue(member.Name)
	data.JoinedAt = types.StringValue(member.JoinedAt)

	roleValues := make([]attr.Value, len(member.AssignedRoles))
	for i, ar := range member.AssignedRoles {
		obj, diags := types.ObjectValue(assignedRoleAttrTypes, map[string]attr.Value{
			"id":   types.StringValue(ar.RoleID),
			"name": types.StringValue(ar.RoleName),
			"type": types.StringValue(ar.RoleType),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		roleValues[i] = obj
	}
	assignedRoles, diags := types.ListValue(types.ObjectType{AttrTypes: assignedRoleAttrTypes}, roleValues)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.AssignedRoles = assignedRoles

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
