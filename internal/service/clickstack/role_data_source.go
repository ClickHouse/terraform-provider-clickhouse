package clickstack

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = (*roleDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*roleDataSource)(nil)
)

// NewRoleDataSource is a helper to register the data source with the provider.
func NewRoleDataSource() datasource.DataSource {
	return &roleDataSource{}
}

// roleDataSource looks up an RBAC role by name. This is primarily used to
// resolve the ID of a predefined role (Admin, Member, ReadOnly) for use in
// `clickstack_team_member` and `clickstack_team`.
type roleDataSource struct {
	client *client.Client
}

// roleDataSourceModel maps the data source schema data.
type roleDataSourceModel struct {
	Team         types.String `tfsdk:"team"`
	Name         types.String `tfsdk:"name"`
	ID           types.String `tfsdk:"id"`
	Description  types.String `tfsdk:"description"`
	IsPredefined types.Bool   `tfsdk:"is_predefined"`
}

func (d *roleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_role"
}

func (d *roleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up an RBAC role by name, including predefined roles (Admin, Member, ReadOnly). " +
			"**Note:** RBAC is only available on self-hosted Enterprise (multi-team) ClickStack deployments; " +
			"it is not currently exposed by ClickStack on ClickHouse Cloud (`clickstack_service_id`).",
		Attributes: map[string]schema.Attribute{
			teamAttr: schema.StringAttribute{
				Optional:    true,
				Description: "Team ID to look the role up under, sent as the `x-hdx-team` header. Defaults to the API key's team.",
			},
			nameAttr: schema.StringAttribute{
				Required:    true,
				Description: "Name of the role to look up.",
			},
			idAttr: schema.StringAttribute{
				Computed:    true,
				Description: "Identifier of the role.",
			},
			descriptionAttr: schema.StringAttribute{
				Computed:    true,
				Description: "Human-readable description of the role.",
			},
			"is_predefined": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether the role is a predefined (built-in) role.",
			},
		},
	}
}

func (d *roleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("expected *service.ProviderData, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}

	if providerData.ClickStack == nil {
		resp.Diagnostics.AddError("ClickStack not configured",
			"This data source requires ClickStack credentials. For self-hosted ClickStack, set clickstack_endpoint and "+
				"clickstack_api_key on the provider (or the CLICKSTACK_ENDPOINT / CLICKSTACK_API_KEY environment variables). "+
				"For ClickStack on ClickHouse Cloud, set clickstack_service_id (or CLICKSTACK_SERVICE_ID) together with "+
				"the ClickHouse Cloud credentials (organization_id, token_key, token_secret).")
		return
	}
	d.client = providerData.ClickStack
}

func (d *roleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	utils.AlphaWarning("clickhouse_clickstack_role", &resp.Diagnostics)
	var config roleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roles, err := d.client.WithTeam(config.Team.ValueString()).ListRoles(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Roles", err.Error())
		return
	}

	name := config.Name.ValueString()
	for _, role := range roles {
		if role.Name == name {
			config.ID = types.StringValue(role.ID)
			config.Description = types.StringPointerValue(role.Description)
			config.IsPredefined = types.BoolValue(role.IsPredefined)
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Role Not Found",
		fmt.Sprintf("No role named %q was found for the team.", name),
	)
}
