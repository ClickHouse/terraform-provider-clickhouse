package clickstack

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = (*connectionResource)(nil)
	_ resource.ResourceWithConfigure   = (*connectionResource)(nil)
	_ resource.ResourceWithImportState = (*connectionResource)(nil)
)

// agentClickhouseRoleNameRegexp matches the role names the ClickStack API
// accepts for agentClickhouseRoleName.
var agentClickhouseRoleNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// NewConnectionResource is a helper to register the resource with the provider.
func NewConnectionResource() resource.Resource {
	return &connectionResource{}
}

// connectionResource manages a ClickHouse connection in ClickStack.
type connectionResource struct {
	client *client.Client
}

// connectionResourceModel maps the resource schema data.
type connectionResourceModel struct {
	ID                      types.String `tfsdk:"id"`
	Team                    types.String `tfsdk:"team"`
	Name                    types.String `tfsdk:"name"`
	Host                    types.String `tfsdk:"host"`
	Username                types.String `tfsdk:"username"`
	Password                types.String `tfsdk:"password"`
	HyperdxSettingPrefix    types.String `tfsdk:"hyperdx_setting_prefix"`
	IsPrometheusEndpoint    types.Bool   `tfsdk:"is_prometheus_endpoint"`
	AgentClickhouseRoleName types.String `tfsdk:"agent_clickhouse_role_name"`
}

func (r *connectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_connection"
}

func (r *connectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ClickHouse connection in ClickStack. Connections hold the " +
			"credentials and endpoint used by sources to query ClickHouse, or a Prometheus-compatible " +
			"API for PromQL sources. " +
			"**Note:** on ClickHouse Cloud the connections endpoint is not exposed, and each " +
			"service has a single connection to itself that the platform creates; this resource " +
			"is for self-hosted ClickStack. To point a source at the Cloud connection, read its " +
			"id from an existing source rather than creating one here.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:    true,
				Description: "Identifier of the connection.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID to manage this connection under, sent as the `x-hdx-team` " +
					"header. Defaults to the API key's team. Only honored by multi-team (EE) " +
					"deployments, which validate the API key's membership in the team; single-team " +
					"(OSS) deployments ignore it. Changing this forces the connection to be " +
					"replaced, since a connection ID is scoped to a single team.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			nameAttr: schema.StringAttribute{
				Required:    true,
				Description: "Display name for the connection.",
			},
			"host": schema.StringAttribute{
				Required: true,
				Description: "ClickHouse HTTP endpoint URL, e.g. `https://clickhouse.example.com:8443`. When " +
					"`is_prometheus_endpoint` is true, the Prometheus-compatible API URL instead, e.g. " +
					"`http://prometheus:9090`; put any Basic auth credentials in the URL " +
					"(`https://user:pass@prometheus.example.com`). This attribute is not sensitive, so " +
					"credentials in it appear in plan output unless `host` comes from a sensitive variable.",
			},
			"username": schema.StringAttribute{
				Required:    true,
				Description: "ClickHouse username. Ignored when `is_prometheus_endpoint` is true; set it to `\"\"`.",
			},
			passwordAttr: schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "ClickHouse password. The API never returns the password, so drift in " +
					"this attribute cannot be detected. After import, the next apply re-sends the " +
					"configured password. Ignored when `is_prometheus_endpoint` is true.",
			},
			"hyperdx_setting_prefix": schema.StringAttribute{
				Optional: true,
				Description: "Prefix for ClickStack-specific ClickHouse settings. Must only contain " +
					"alphanumeric characters and underscores.",
			},
			"is_prometheus_endpoint": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				Description: "When true, `host` is a Prometheus-compatible API endpoint (e.g. Prometheus " +
					"or Thanos) and PromQL sources on this connection proxy their queries to it. Such a " +
					"connection serves only `promql` sources. Defaults to false, where `host` is a " +
					"ClickHouse HTTP endpoint.",
			},
			"agent_clickhouse_role_name": schema.StringAttribute{
				Optional: true,
				Description: "ClickHouse role applied to every query an AI agent runs on its own on this " +
					"connection, across all of ClickStack's AI features, so column and row access " +
					"policies attached to the role are enforced by ClickHouse itself. The role must already exist and be granted to the " +
					"connection's `username`; if it is not, the API still saves it and the provider " +
					"surfaces the API's warning. Must only contain alphanumeric characters and " +
					"underscores. Removing it from the configuration clears the role. Enterprise (EE) " +
					"only: OSS deployments ignore it, so it never takes effect there and plans a change " +
					"on every apply. Does not apply when `is_prometheus_endpoint` is true. Not currently " +
					"applied on connections with ClickHouse RBAC enforcement enabled, where AI queries run " +
					"under each user's own ClickHouse role.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(agentClickhouseRoleNameRegexp,
						"must only contain alphanumeric characters and underscores"),
				},
			},
		},
	}
}

func (r *connectionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
		addNotConfiguredError(&resp.Diagnostics, "resource")
		return
	}
	r.client = providerData.ClickStack
}

func (r *connectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan connectionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, warning, err := r.client.WithTeam(plan.Team.ValueString()).CreateConnection(ctx, client.CreateConnectionInput{
		Name:                    plan.Name.ValueString(),
		Host:                    plan.Host.ValueString(),
		Username:                plan.Username.ValueString(),
		Password:                plan.Password.ValueString(),
		HyperdxSettingPrefix:    plan.HyperdxSettingPrefix.ValueStringPointer(),
		IsPrometheusEndpoint:    plan.IsPrometheusEndpoint.ValueBool(),
		AgentClickhouseRoleName: plan.AgentClickhouseRoleName.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Connection", err.Error())
		return
	}

	plan.applyWriteResult(conn, warning, &resp.Diagnostics)
	tflog.Trace(ctx, "created connection resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *connectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state connectionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, err := r.client.WithTeam(state.Team.ValueString()).GetConnection(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Connection", err.Error())
		return
	}

	// The password is write-only; the value already in state is kept as-is.
	state.applyConnection(conn)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *connectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan connectionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := client.UpdateConnectionInput{
		Name:     plan.Name.ValueString(),
		Host:     plan.Host.ValueString(),
		Username: plan.Username.ValueString(),
		// nil clears the value on the server; non-nil sets it.
		HyperdxSettingPrefix: plan.HyperdxSettingPrefix.ValueStringPointer(),
		IsPrometheusEndpoint: plan.IsPrometheusEndpoint.ValueBool(),
	}
	// A nil password means "keep the existing password" server-side, which
	// matches the write-only semantics of the attribute.
	if !plan.Password.IsNull() {
		input.Password = plan.Password.ValueStringPointer()
	}
	var state connectionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	input.AgentClickhouseRoleName = agentRoleUpdateValue(plan.AgentClickhouseRoleName, state.AgentClickhouseRoleName)

	conn, warning, err := r.client.WithTeam(plan.Team.ValueString()).UpdateConnection(ctx, plan.ID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Connection", err.Error())
		return
	}

	plan.applyWriteResult(conn, warning, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *connectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state connectionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.WithTeam(state.Team.ValueString()).DeleteConnection(ctx, state.ID.ValueString()); err != nil {
		// A connection already deleted out-of-band is not an error.
		if errors.Is(err, client.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting Connection", err.Error())
	}
}

func (r *connectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Accept either "<id>" (default team) or "<team>/<id>" so connections in a
	// non-default team can be imported. The team is required by the API to
	// resolve the team-scoped connection ID during the import Read.
	if team, id, ok := strings.Cut(req.ID, "/"); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team"), team)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// applyConnection copies the API representation into the model. The password
// is intentionally untouched: the API never returns it, so the configured
// value in plan/state is authoritative.
func (m *connectionResourceModel) applyConnection(conn *client.Connection) {
	m.ID = types.StringValue(conn.ID)
	m.Name = types.StringValue(conn.Name)
	m.Host = types.StringValue(conn.Host)
	m.Username = types.StringValue(conn.Username)
	m.HyperdxSettingPrefix = types.StringPointerValue(conn.HyperdxSettingPrefix)
	m.IsPrometheusEndpoint = types.BoolValue(conn.IsPrometheusEndpoint != nil && *conn.IsPrometheusEndpoint)
	// The API omits the role when unset; "" is treated the same so an unset
	// role is always null and matches an omitted attribute.
	if conn.AgentClickhouseRoleName == nil || *conn.AgentClickhouseRoleName == "" {
		m.AgentClickhouseRoleName = types.StringNull()
	} else {
		m.AgentClickhouseRoleName = types.StringValue(*conn.AgentClickhouseRoleName)
	}
}

// applyWriteResult copies a create/update response into the planned model and
// surfaces the API's non-fatal warning, if any. A configured role the server
// did not echo back (OSS, or an EE server that predates the field) keeps the
// planned value: writing null would fail the apply with "inconsistent result",
// and the next refresh reports the real server value.
func (m *connectionResourceModel) applyWriteResult(conn *client.Connection, warning string, diags *diag.Diagnostics) {
	planned := m.AgentClickhouseRoleName
	m.applyConnection(conn)
	if !planned.IsNull() && m.AgentClickhouseRoleName.IsNull() {
		m.AgentClickhouseRoleName = planned
		diags.AddAttributeWarning(path.Root("agent_clickhouse_role_name"),
			"Agent ClickHouse Role Not Applied",
			"The ClickStack API did not return agent_clickhouse_role_name after saving the connection. "+
				"The server most likely does not support it (it is Enterprise-only), so the role is not "+
				"enforced and the next plan will show a change.")
	}
	if warning != "" {
		diags.AddWarning("ClickStack Connection Warning", warning)
	}
}

// agentRoleUpdateValue returns the agentClickhouseRoleName to send on update:
// the planned role when set, "" to clear a role that is currently set, and nil
// (omit the field, keeping the server value) when there is nothing to change,
// so servers without the field never see it unless it is in use.
func agentRoleUpdateValue(plan, state types.String) *string {
	if !plan.IsNull() {
		return plan.ValueStringPointer()
	}
	if !state.IsNull() {
		empty := ""
		return &empty
	}
	return nil
}
