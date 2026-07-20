package clickstack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = (*connectionResource)(nil)
	_ resource.ResourceWithConfigure   = (*connectionResource)(nil)
	_ resource.ResourceWithImportState = (*connectionResource)(nil)
)

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
	ID                   types.String `tfsdk:"id"`
	Team                 types.String `tfsdk:"team"`
	Name                 types.String `tfsdk:"name"`
	Host                 types.String `tfsdk:"host"`
	Username             types.String `tfsdk:"username"`
	Password             types.String `tfsdk:"password"`
	HyperdxSettingPrefix types.String `tfsdk:"hyperdx_setting_prefix"`
	PrometheusEndpoint   types.String `tfsdk:"prometheus_endpoint"`
}

func (r *connectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_connection"
}

func (r *connectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ClickHouse connection in ClickStack. Connections hold the " +
			"credentials and endpoint used by sources to query ClickHouse.",
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
				Required:    true,
				Description: "ClickHouse HTTP endpoint URL, e.g. `https://clickhouse.example.com:8443`.",
			},
			"username": schema.StringAttribute{
				Required:    true,
				Description: "ClickHouse username.",
			},
			passwordAttr: schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "ClickHouse password. The API never returns the password, so drift in " +
					"this attribute cannot be detected. After import, the next apply re-sends the " +
					"configured password.",
			},
			"hyperdx_setting_prefix": schema.StringAttribute{
				Optional: true,
				Description: "Prefix for HyperDX-specific ClickHouse settings. Must only contain " +
					"alphanumeric characters and underscores.",
			},
			"prometheus_endpoint": schema.StringAttribute{
				Optional: true,
				Description: "Prometheus-compatible API endpoint, e.g. `http://prometheus:9090`. " +
					"When set, PromQL queries are proxied to this endpoint.",
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
		resp.Diagnostics.AddError("ClickStack not configured",
			"This resource requires ClickStack credentials. Set clickstack_api_key on the "+
				"provider (or the CLICKSTACK_API_KEY environment variable), and clickstack_endpoint if not using ClickHouse Cloud.")
		return
	}
	r.client = providerData.ClickStack
}

func (r *connectionResource) ValidateConfig(_ context.Context, _ resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_clickstack_connection", &resp.Diagnostics)
}

func (r *connectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan connectionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	conn, err := r.client.WithTeam(plan.Team.ValueString()).CreateConnection(ctx, client.CreateConnectionInput{
		Name:                 plan.Name.ValueString(),
		Host:                 plan.Host.ValueString(),
		Username:             plan.Username.ValueString(),
		Password:             plan.Password.ValueString(),
		HyperdxSettingPrefix: plan.HyperdxSettingPrefix.ValueStringPointer(),
		PrometheusEndpoint:   plan.PrometheusEndpoint.ValueStringPointer(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Connection", err.Error())
		return
	}

	plan.applyConnection(conn)
	tflog.Trace(ctx, "created connection resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *connectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	utils.AlphaWarning("clickhouse_clickstack_connection", &resp.Diagnostics)
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
		PrometheusEndpoint:   plan.PrometheusEndpoint.ValueStringPointer(),
	}
	// A nil password means "keep the existing password" server-side, which
	// matches the write-only semantics of the attribute.
	if !plan.Password.IsNull() {
		input.Password = plan.Password.ValueStringPointer()
	}

	conn, err := r.client.WithTeam(plan.Team.ValueString()).UpdateConnection(ctx, plan.ID.ValueString(), input)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Connection", err.Error())
		return
	}

	plan.applyConnection(conn)

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
	m.PrometheusEndpoint = types.StringPointerValue(conn.PrometheusEndpoint)
}
