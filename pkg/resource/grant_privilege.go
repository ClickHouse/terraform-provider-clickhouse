//go:build alpha

package resource

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"log"
	"strings"

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

//go:embed descriptions/grant_privilege.md
var grantPrivilegeDescription string

//go:embed data/grants.tsv
var grants string

type availableGrants struct {
	Aliases map[string]string   `json:"aliases"`
	Groups  map[string][]string `json:"groups"`
	Scopes  map[string]string   `json:"scopes"`
}

var (
	_ resource.Resource              = &GrantPrivilegeResource{}
	_ resource.ResourceWithConfigure = &GrantPrivilegeResource{}
)

func NewGrantPrivilegeResource() resource.Resource {
	return &GrantPrivilegeResource{}
}

type GrantPrivilegeResource struct {
	client api.Client
}

func (r *GrantPrivilegeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_grant_privilege"
}

func (r *GrantPrivilegeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	validPrivileges := make([]string, 0)

	upstrGrts := parseGrants()

	for privilege := range upstrGrts.Scopes {
		validPrivileges = append(validPrivileges, privilege)
	}

	for alias := range upstrGrts.Aliases {
		validPrivileges = append(validPrivileges, alias)
	}

	for groupName := range upstrGrts.Groups {
		validPrivileges = append(validPrivileges, groupName)
	}

	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Service ID.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"privilege_name": schema.StringAttribute{
				Required:    true,
				Description: "The privilege to grant, such as `CREATE DATABASE`, `SELECT`, etc. See https://clickhouse.com/docs/en/sql-reference/statements/grant#privileges.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(validPrivileges...),
				},
			},
			"database_name": schema.StringAttribute{
				Optional:    true,
				Description: "The name of the database to grant privilege on. Defaults to all databases if left null",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.NoneOf("*"),
				},
			},
			"table_name": schema.StringAttribute{
				Optional:    true,
				Description: "The name of the table to grant privilege on.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"column_name": schema.StringAttribute{
				Optional:    true,
				Description: "The name of the column in `table_name` to grant privilege on.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.AlsoRequires(path.Expressions{path.MatchRoot("table_name")}...),
				},
			},
			"grantee_user_name": schema.StringAttribute{
				Optional:    true,
				Description: "Name of the `user` to grant privileges to.",
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
				Description: "Name of the `role` to grant privileges to.",
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
			"grant_option": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "If true, the grantee will be able to grant the same privileges to others.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
		},
		MarkdownDescription: grantPrivilegeDescription,
	}
}

func (r *GrantPrivilegeResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(api.Client)
}

func (r *GrantPrivilegeResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		// If the entire plan is null, the resource is planned for destruction.
		return
	}

	upstrGrts := parseGrants()

	var plan, state, config models.GrantPrivilege
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if !req.State.Raw.IsNull() {
		diags = req.State.Get(ctx, &state)
		resp.Diagnostics.Append(diags...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if !req.Config.Raw.IsNull() {
		diags = req.Config.Get(ctx, &config)
		resp.Diagnostics.Append(diags...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if using an alias.
	if alias := upstrGrts.Aliases[plan.Privilege.ValueString()]; alias != "" {
		// Using an alias, block.
		resp.Diagnostics.AddAttributeError(
			path.Root("privilege_name"),
			"Cannot use alias",
			fmt.Sprintf("%q is an alias for %q. Please use %q instead", plan.Privilege.ValueString(), alias, alias),
		)
		return
	}

	// Check required fields which depend on the grant's scope.
	{
		scope := upstrGrts.Scopes[plan.Privilege.ValueString()]
		switch scope {
		case "GLOBAL":
			if !plan.Database.IsNull() {
				resp.Diagnostics.AddAttributeError(
					path.Root("database"),
					"Invalid Grant Privilege",
					fmt.Sprintf("'database' must be null when 'privilege_name' is %q", plan.Privilege.ValueString()),
				)
				return
			}
		case "COLUMN":
			fallthrough
		case "DICTIONARY":
			fallthrough
		case "VIEW":
			if plan.Database.IsNull() {
				resp.Diagnostics.AddAttributeError(
					path.Root("database"),
					"Invalid Grant Privilege",
					fmt.Sprintf("'database' must be set when privilege_name is %q", plan.Privilege.ValueString()),
				)
				return
			}
		case "NAMED_COLLECTION":
			fallthrough
		case "USER_NAME":
			fallthrough
		case "TABLE ENGINE":
			resp.Diagnostics.AddAttributeError(
				path.Root("privilege_name"),
				"Unsupported Privilege",
				fmt.Sprintf("%q privilege_name is currently unsupported", plan.Privilege.ValueString()),
			)
			return
		}
	}
}

func (r *GrantPrivilegeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.GrantPrivilege
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	grant := api.GrantPrivilege{
		AccessType:      plan.Privilege.ValueString(),
		DatabaseName:    plan.Database.ValueStringPointer(),
		TableName:       plan.Table.ValueStringPointer(),
		ColumnName:      plan.Column.ValueStringPointer(),
		GranteeUserName: plan.GranteeUserName.ValueStringPointer(),
		GranteeRoleName: plan.GranteeRoleName.ValueStringPointer(),
		GrantOption:     plan.GrantOption.ValueBool(),
	}

	createdGrant, err := r.client.GrantPrivilege(ctx, plan.ServiceID.ValueString(), grant)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating ClickHouse Privilege Grant",
			"Could not create privilege grant, unexpected error: "+err.Error(),
		)
		return
	}

	state := models.GrantPrivilege{
		ServiceID:       plan.ServiceID,
		Privilege:       types.StringValue(createdGrant.AccessType),
		Database:        types.StringPointerValue(createdGrant.DatabaseName),
		Table:           types.StringPointerValue(createdGrant.TableName),
		Column:          types.StringPointerValue(createdGrant.ColumnName),
		GranteeUserName: types.StringPointerValue(createdGrant.GranteeUserName),
		GranteeRoleName: types.StringPointerValue(createdGrant.GranteeRoleName),
		GrantOption:     types.BoolValue(createdGrant.GrantOption),
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *GrantPrivilegeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.GrantPrivilege
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	grant, err := r.client.GetGrantPrivilege(ctx, state.ServiceID.ValueString(), state.Privilege.ValueString(), state.Database.ValueStringPointer(), state.Table.ValueStringPointer(), state.Column.ValueStringPointer(), state.GranteeUserName.ValueStringPointer(), state.GranteeRoleName.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading ClickHouse Privilege Grant",
			"Could not read privilege grant, unexpected error: "+err.Error(),
		)
		return
	}

	if grant != nil {
		state.Privilege = types.StringValue(grant.AccessType)
		state.Database = types.StringPointerValue(grant.DatabaseName)
		state.Table = types.StringPointerValue(grant.TableName)
		state.Column = types.StringPointerValue(grant.ColumnName)
		state.GranteeUserName = types.StringPointerValue(grant.GranteeUserName)
		state.GranteeRoleName = types.StringPointerValue(grant.GranteeRoleName)
		state.GrantOption = types.BoolValue(grant.GrantOption)

		diags = resp.State.Set(ctx, &state)
		resp.Diagnostics.Append(diags...)
	} else {
		resp.State.RemoveResource(ctx)
	}
}

func (r *GrantPrivilegeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	panic("Update of grant privilege resource is not supported")
}

func (r *GrantPrivilegeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.GrantPrivilege
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.RevokeGrantPrivilege(ctx, state.ServiceID.ValueString(), state.Privilege.ValueString(), state.Database.ValueStringPointer(), state.Table.ValueStringPointer(), state.Column.ValueStringPointer(), state.GranteeUserName.ValueStringPointer(), state.GranteeRoleName.ValueStringPointer())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting ClickHouse Privilege Grant",
			"Could not delete privilege grant, unexpected error: "+err.Error(),
		)
		return
	}
}

func parseGrants() availableGrants {
	aliases := make(map[string]string)
	groups := make(map[string][]string)
	scopes := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(grants))
	for scanner.Scan() {
		line := scanner.Text()

		splitted := strings.Split(line, "\t")

		clean := strings.ReplaceAll(strings.Trim(splitted[1], "[]"), "'", "")
		if clean != "" {
			for _, a := range strings.Split(clean, ",") {
				if a != splitted[0] {
					aliases[a] = splitted[0]
				}
			}
		}

		if splitted[3] != "\\N" {
			if groups[splitted[3]] == nil {
				groups[splitted[3]] = make([]string, 0)
			}
			groups[splitted[3]] = append(groups[splitted[3]], splitted[0])
		}

		if splitted[2] != "\\N" {
			scopes[splitted[0]] = splitted[2]
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	ret := availableGrants{
		Aliases: aliases,
		Groups:  groups,
		Scopes:  scopes,
	}

	return ret
}
