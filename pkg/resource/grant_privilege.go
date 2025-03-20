//go:build alpha

package resource

import (
	"context"
	_ "embed"
	"fmt"

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

var (
	validDatabasePrivileges = []string{
		"SHOW COLUMNS",
		"ALTER UPDATE",
		"ALTER DELETE",
		"ALTER ADD COLUMN",
		"ALTER MODIFY COLUMN",
		"ALTER DROP COLUMN",
		"ALTER COMMENT COLUMN",
		"ALTER CLEAR COLUMN",
		"ALTER RENAME COLUMN",
		"ALTER MATERIALIZE COLUMN",
		"ALTER COLUMN",
		"ALTER MODIFY COMMENT",
		"ALTER ORDER BY",
		"ALTER SAMPLE BY",
		"ALTER ADD INDEX",
		"ALTER DROP INDEX",
		"ALTER MATERIALIZE INDEX",
		"ALTER CLEAR INDEX",
		"ALTER INDEX",
		"ALTER ADD STATISTICS",
		"ALTER DROP STATISTICS",
		"ALTER MODIFY STATISTICS",
		"ALTER MATERIALIZE STATISTICS",
		"ALTER STATISTICS",
		"ALTER ADD PROJECTION",
		"ALTER DROP PROJECTION",
		"ALTER MATERIALIZE PROJECTION",
		"ALTER CLEAR PROJECTION",
		"ALTER PROJECTION",
		"ALTER ADD CONSTRAINT",
		"ALTER DROP CONSTRAINT",
		"ALTER CONSTRAINT",
		"ALTER TTL",
		"ALTER MATERIALIZE TTL",
		"ALTER SETTINGS",
		"ALTER MOVE PARTITION",
		"ALTER FETCH PARTITION",
		"ALTER FREEZE PARTITION",
		"ALTER TABLE",
		"ALTER DATABASE",
		"ALTER VIEW MODIFY QUERY",
		"ALTER VIEW MODIFY REFRESH",
		"ALTER VIEW MODIFY SQL SECURITY",
		"ALTER VIEW",
		"CREATE TABLE",
		"CREATE VIEW",
		"CREATE DICTIONARY",
		"DROP TABLE",
		"DROP VIEW",
		"DROP DICTIONARY",
		"TRUNCATE",
		"OPTIMIZE",
		"CREATE ROW POLICY",
		"ALTER ROW POLICY",
		"DROP ROW POLICY",
		"SHOW ROW POLICIES",
		"SYSTEM VIEWS",
		"dictGet",
	}

	validGlobalPrivileges = []string{
		"CREATE TEMPORARY TABLE",
		"CREATE ARBITRARY TEMPORARY TABLE",
		"CREATE FUNCTION",
		"DROP FUNCTION",
		"KILL QUERY",
		"CREATE USER",
		"ALTER USER",
		"DROP USER",
		"CREATE ROLE",
		"ALTER ROLE",
		"DROP ROLE",
		"ROLE ADMIN",
		"CREATE QUOTA",
		"ALTER QUOTA",
		"DROP QUOTA",
		"CREATE SETTINGS PROFILE",
		"ALTER SETTINGS PROFILE",
		"DROP SETTINGS PROFILE",
		"ALLOW SQL SECURITY NONE",
		"SHOW USERS",
		"SHOW ROLES",
		"SHOW QUOTAS",
		"SHOW SETTINGS PROFILES",
		"SYSTEM DROP DNS CACHE",
		"SYSTEM DROP CONNECTIONS CACHE",
		"SYSTEM PREWARM PRIMARY INDEX CACHE",
		"SYSTEM DROP MARK CACHE",
		"SYSTEM PREWARM PRIMARY INDEX CACHE",
		"SYSTEM DROP PRIMARY INDEX CACHE",
		"SYSTEM DROP UNCOMPRESSED CACHE",
		"SYSTEM DROP MMAP CACHE",
		"SYSTEM DROP QUERY CACHE",
		"SYSTEM DROP COMPILED EXPRESSION CACHE",
		"SYSTEM DROP FILESYSTEM CACHE",
		"SYSTEM DROP DISTRIBUTED CACHE",
		"SYSTEM DROP PAGE CACHE",
		"SYSTEM DROP SCHEMA CACHE",
		// "SYSTEM DROP FORMAT SCHEMA CACHE", // Server side error
		"SYSTEM DROP S3 CLIENT CACHE",
		"SYSTEM DROP CACHE",
		"SYSTEM RELOAD CONFIG",
		"SYSTEM RELOAD DICTIONARY",
		"SYSTEM RELOAD MODEL",
		"SYSTEM RELOAD FUNCTION",
		"SYSTEM RELOAD EMBEDDED DICTIONARIES",
		"SYSTEM FLUSH LOGS",
		"addressToLine",
		"addressToLineWithInlines",
		"addressToSymbol",
		"demangle",
		"INTROSPECTION",
		"URL",
		"REMOTE",
		"MONGO",
		"REDIS",
		"MYSQL",
		"POSTGRES",
		"S3",
		"AZURE",
		"KAFKA",
		"NATS",
		"RABBITMQ",
		"CLUSTER",
	}

	validGlobalOrDatabasePrivileges = []string{
		"SHOW DATABASES",
		"SHOW TABLES",
		"SHOW DICTIONARIES",
		"CREATE DATABASE",
		"SYSTEM SYNC REPLICA",
		"SYSTEM RESTART REPLICA",
		"SYSTEM SYNC DATABASE REPLICA",
		"SYSTEM FLUSH DISTRIBUTED",
	}
)

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
					//stringvalidator.OneOf(append(validDatabasePrivileges, append(validGlobalPrivileges, validGlobalOrDatabasePrivileges...)...)...),
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

	if plan.Database.IsNull() {
		for _, p := range validDatabasePrivileges {
			if p == plan.Privilege.ValueString() {
				resp.Diagnostics.AddAttributeError(
					path.Root("database"),
					"Invalid Grant Privilege",
					fmt.Sprintf("'database' must be set when privilege_name is %q", p),
				)
				return
			}
		}
	} else {
		for _, p := range validGlobalPrivileges {
			if p == plan.Privilege.ValueString() {
				resp.Diagnostics.AddAttributeError(
					path.Root("database"),
					"Invalid Grant Privilege",
					fmt.Sprintf("'database' must be null when 'privilege_name' is %q", p),
				)
				return
			}
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
