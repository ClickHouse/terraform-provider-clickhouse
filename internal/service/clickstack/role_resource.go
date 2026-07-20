package clickstack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = (*roleResource)(nil)
	_ resource.ResourceWithConfigure   = (*roleResource)(nil)
	_ resource.ResourceWithImportState = (*roleResource)(nil)
)

// integrationMongoDB is the default permission integration. ClickHouse
// (data-RBAC) permissions can be set explicitly via the integration attribute.
const integrationMongoDB = "mongodb"

// NewRoleResource is a helper to register the resource with the provider.
func NewRoleResource() resource.Resource {
	return &roleResource{}
}

// roleResource manages a custom RBAC role in ClickStack.
type roleResource struct {
	client *client.Client
}

// rolePermissionModel maps a single CASL permission. Conditions is held as a
// JSON-encoded string so the provider does not need to model every
// integration-specific shape.
type rolePermissionModel struct {
	Action      types.String `tfsdk:"action"`
	Subject     types.String `tfsdk:"subject"`
	Integration types.String `tfsdk:"integration"`
	Inverted    types.Bool   `tfsdk:"inverted"`
	Fields      types.List   `tfsdk:"fields"`
	Conditions  types.String `tfsdk:"conditions"`
}

// roleResourceModel maps the resource schema data.
type roleResourceModel struct {
	ID          types.String          `tfsdk:"id"`
	Team        types.String          `tfsdk:"team"`
	Name        types.String          `tfsdk:"name"`
	Description types.String          `tfsdk:"description"`
	Permissions []rolePermissionModel `tfsdk:"permissions"`
}

func (r *roleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_role"
}

func (r *roleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a custom RBAC role in ClickStack. " +
			"**Note:** RBAC is only available on ClickStack Cloud and Enterprise deployments. " +
			"Predefined roles (Admin, Member, " +
			"ReadOnly) are not managed by this resource; reference them with the `clickstack_role` " +
			"data source instead. Note: the API always ensures a `read` permission on `Connection` " +
			"is present; the provider reconciles this automatically so it does not appear as drift.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:    true,
				Description: "Identifier of the role.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID to manage this role under, sent as the `x-hdx-team` header. " +
					"Defaults to the API key's team. Changing this forces the role to be replaced.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			nameAttr: schema.StringAttribute{
				Required:    true,
				Description: "Name of the role. Must not match a predefined role name.",
			},
			descriptionAttr: schema.StringAttribute{
				Optional:    true,
				Description: "Human-readable description of the role.",
			},
			"permissions": schema.SetNestedAttribute{
				Required:    true,
				Description: "Set of CASL permission rules granted by the role.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"action": schema.StringAttribute{
							Required:    true,
							Description: "Action: one of `create`, `read`, `update`, `delete`, `manage`.",
						},
						"subject": schema.StringAttribute{
							Required: true,
							Description: "Subject (resource): e.g. `Alert`, `Connection`, `Dashboard`, " +
								"`SavedSearch`, `Source`, `Team`, `User`, `Webhook`, `Role`, `Notebook`, or `all`.",
						},
						"integration": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Default:     stringdefault.StaticString(integrationMongoDB),
							Description: "Integration the permission applies to: `mongodb` (API) or `clickhouse` (data RBAC). Defaults to `mongodb`.",
						},
						"inverted": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
							Description: "When true, the rule denies rather than allows the action. Defaults to false.",
						},
						"fields": schema.ListAttribute{
							ElementType: types.StringType,
							Optional:    true,
							Description: "Optional list of fields the permission is restricted to.",
						},
						"conditions": schema.StringAttribute{
							Optional:    true,
							Description: "Optional JSON-encoded conditions object constraining the permission.",
						},
					},
				},
			},
		},
	}
}

func (r *roleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("expected *service.ProviderData, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}

	if providerData.ClickStack == nil {
		resp.Diagnostics.AddError("ClickStack not configured",
			"This resource requires ClickStack credentials. Set clickstack_api_key on the "+
				"provider (or the CLICKSTACK_API_KEY environment variable), and clickstack_endpoint if not using ClickHouse Cloud.")
		return
	}
	r.client = providerData.ClickStack
}

func (r *roleResource) ValidateConfig(_ context.Context, _ resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_clickstack_role", &resp.Diagnostics)
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	perms, diags := buildPermissions(ctx, plan.Permissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.WithTeam(plan.Team.ValueString()).CreateRole(ctx, client.CreateRoleInput{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueStringPointer(),
		Permissions: perms,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Role", err.Error())
		return
	}

	// Store the configured permissions verbatim: the API may auto-inject a
	// `read Connection` permission, but reflecting that here would produce a
	// perpetual diff. Read reconciles real drift semantically.
	plan.ID = types.StringValue(role.ID)
	tflog.Trace(ctx, "created role resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	utils.AlphaWarning("clickhouse_clickstack_role", &resp.Diagnostics)
	var state roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.WithTeam(state.Team.ValueString()).GetRole(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Role", err.Error())
		return
	}

	state.Name = types.StringValue(role.Name)
	state.Description = types.StringPointerValue(role.Description)

	serverPerms, diags := flattenPermissions(role.Permissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Drop the auto-injected `read Connection` permission unless it is part of
	// the configured set, then only overwrite state when the permission sets
	// differ semantically (ignoring order and JSON formatting).
	serverPerms = filterAutoConnectionRead(serverPerms, state.Permissions)
	if !permissionsEqual(state.Permissions, serverPerms) {
		state.Permissions = serverPerms
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	perms, diags := buildPermissions(ctx, plan.Permissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	role, err := r.client.WithTeam(plan.Team.ValueString()).UpdateRole(ctx, plan.ID.ValueString(), client.UpdateRoleInput{
		Name:        plan.Name.ValueStringPointer(),
		Description: plan.Description.ValueStringPointer(),
		Permissions: perms,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Role", err.Error())
		return
	}

	plan.ID = types.StringValue(role.ID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.WithTeam(state.Team.ValueString()).DeleteRole(ctx, state.ID.ValueString()); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting Role", err.Error())
	}
}

func (r *roleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if team, id, ok := strings.Cut(req.ID, "/"); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team"), team)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildPermissions converts the Terraform permission models into client
// permissions for an API request.
func buildPermissions(ctx context.Context, models []rolePermissionModel) ([]client.Permission, diag.Diagnostics) {
	var diags diag.Diagnostics
	perms := make([]client.Permission, 0, len(models))
	for _, m := range models {
		inverted := m.Inverted.ValueBool()
		p := client.Permission{
			Action:      m.Action.ValueString(),
			Subject:     m.Subject.ValueString(),
			Integration: m.Integration.ValueString(),
			Inverted:    &inverted,
		}
		if p.Integration == "" {
			p.Integration = integrationMongoDB
		}
		if !m.Fields.IsNull() && !m.Fields.IsUnknown() {
			var fields []string
			diags.Append(m.Fields.ElementsAs(ctx, &fields, false)...)
			p.Fields = fields
		}
		if !m.Conditions.IsNull() && m.Conditions.ValueString() != "" {
			p.Conditions = json.RawMessage(m.Conditions.ValueString())
		}
		perms = append(perms, p)
	}
	return perms, diags
}

// flattenPermissions converts client permissions into Terraform models.
func flattenPermissions(perms []client.Permission) ([]rolePermissionModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	models := make([]rolePermissionModel, 0, len(perms))
	for _, p := range perms {
		inverted := false
		if p.Inverted != nil {
			inverted = *p.Inverted
		}

		fields := types.ListNull(types.StringType)
		if len(p.Fields) > 0 {
			lv, d := types.ListValueFrom(context.Background(), types.StringType, p.Fields)
			diags.Append(d...)
			fields = lv
		}

		conditions := types.StringNull()
		if len(p.Conditions) > 0 && string(p.Conditions) != "null" {
			conditions = types.StringValue(string(p.Conditions))
		}

		integration := p.Integration
		if integration == "" {
			integration = integrationMongoDB
		}

		models = append(models, rolePermissionModel{
			Action:      types.StringValue(p.Action),
			Subject:     types.StringValue(p.Subject),
			Integration: types.StringValue(integration),
			Inverted:    types.BoolValue(inverted),
			Fields:      fields,
			Conditions:  conditions,
		})
	}
	return models, diags
}

// permKey returns a canonical, comparable key for a permission so that
// permission sets can be compared independently of order and JSON formatting.
func permKey(m rolePermissionModel) string {
	var fields []string
	for _, e := range m.Fields.Elements() {
		if s, ok := e.(types.String); ok {
			fields = append(fields, s.ValueString())
		}
	}

	conditions := ""
	if !m.Conditions.IsNull() {
		conditions = canonicalJSON(m.Conditions.ValueString())
	}

	integration := m.Integration.ValueString()
	if integration == "" {
		integration = integrationMongoDB
	}

	return strings.Join([]string{
		m.Action.ValueString(),
		m.Subject.ValueString(),
		integration,
		strconv.FormatBool(m.Inverted.ValueBool()),
		strings.Join(fields, ","),
		conditions,
	}, "|")
}

// canonicalJSON normalizes a JSON string by re-encoding it, so that equivalent
// objects with different whitespace compare equal. Invalid JSON is returned
// unchanged.
func canonicalJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}

// connectionReadPermKey is the canonical key of the permission the API
// auto-injects into every custom role.
var connectionReadPermKey = strings.Join([]string{"read", "Connection", integrationMongoDB, "false", "", ""}, "|")

// filterAutoConnectionRead removes the API's auto-injected `read Connection`
// permission from server unless it is present in the configured set.
func filterAutoConnectionRead(server, configured []rolePermissionModel) []rolePermissionModel {
	for _, m := range configured {
		if permKey(m) == connectionReadPermKey {
			return server // configured explicitly, keep as-is
		}
	}
	filtered := make([]rolePermissionModel, 0, len(server))
	for _, m := range server {
		if permKey(m) == connectionReadPermKey {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}

// permissionsEqual reports whether two permission sets are equal, ignoring
// order and JSON formatting.
func permissionsEqual(a, b []rolePermissionModel) bool {
	if len(a) != len(b) {
		return false
	}
	ka := make([]string, len(a))
	kb := make([]string, len(b))
	for i, m := range a {
		ka[i] = permKey(m)
	}
	for i, m := range b {
		kb[i] = permKey(m)
	}
	sort.Strings(ka)
	sort.Strings(kb)
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return true
}
