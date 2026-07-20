package clickstack

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                   = (*webhookResource)(nil)
	_ resource.ResourceWithConfigure      = (*webhookResource)(nil)
	_ resource.ResourceWithImportState    = (*webhookResource)(nil)
	_ resource.ResourceWithValidateConfig = (*webhookResource)(nil)
)

// webhookServices is the set of accepted webhook service types.
var webhookServices = []string{"slack", "generic", "incidentio"}

// Attribute names for the write-only secret maps, referenced from the schema and
// from ValidateConfig.
const (
	headersAttr     = "headers"
	queryParamsAttr = "query_params"
)

// NewWebhookResource is a helper to register the resource with the provider.
func NewWebhookResource() resource.Resource {
	return &webhookResource{}
}

// webhookResource manages a ClickStack notification webhook.
type webhookResource struct {
	client *client.Client
}

// webhookResourceModel maps the resource schema data.
//
// Headers and QueryParams are write-only (never persisted to state): they are
// read from config on create/update and sent to the API, which never returns
// them. HeadersVersion/QueryParamsVersion are ordinary trigger attributes the
// user bumps to force the write-only secret to be re-sent (e.g. on rotation).
type webhookResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Team               types.String `tfsdk:"team"`
	Name               types.String `tfsdk:"name"`
	Service            types.String `tfsdk:"service"`
	URL                types.String `tfsdk:"url"`
	Description        types.String `tfsdk:"description"`
	Headers            types.Map    `tfsdk:"headers"`
	QueryParams        types.Map    `tfsdk:"query_params"`
	HeadersVersion     types.String `tfsdk:"headers_version"`
	QueryParamsVersion types.String `tfsdk:"query_params_version"`
	Body               types.String `tfsdk:"body"`
}

func (r *webhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_webhook"
}

func (r *webhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ClickStack notification webhook (Slack, generic HTTP, or incident.io), " +
			"used as the notification channel for alerts.\n\n" +
			"The secret-bearing `headers` and `query_params` are write-only: they are sent to the API " +
			"but never stored in Terraform state. Because Terraform cannot see a write-only value, a " +
			"changed secret is only re-sent when you also bump its companion trigger " +
			"(`headers_version` / `query_params_version`). Write-only attributes require Terraform >= 1.11.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:      true,
				Description:   "Identifier of the webhook.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID to manage this webhook under, sent as the `x-hdx-team` header. " +
					"Defaults to the API key's team. Changing this forces the webhook to be replaced.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			nameAttr: schema.StringAttribute{
				Required:    true,
				Description: "Display name for the webhook. Must be unique per service within a team.",
			},
			"service": schema.StringAttribute{
				Required:    true,
				Description: "Webhook service: one of `slack`, `generic`, or `incidentio`.",
			},
			"url": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
				Description: "Destination URL the ClickStack server calls. Marked sensitive because " +
					"Slack incoming-webhook URLs embed a channel token. Private-IP/SSRF validation is " +
					"enforced server-side.",
			},
			descriptionAttr: schema.StringAttribute{
				Optional:    true,
				Description: "Optional description of the webhook.",
			},
			headersAttr: schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				WriteOnly:   true,
				Sensitive:   true,
				Description: "Write-only HTTP headers sent with the webhook request (e.g. an " +
					"`Authorization` token). Never stored in state or returned by the API. Not allowed " +
					"for the `slack` service. Bump `headers_version` to re-send after a change. The " +
					"server keeps the last-sent headers when this field is omitted, EXCEPT when `url` or " +
					"`service` changes — that clears any omitted secret, so re-supply headers (and bump " +
					"`headers_version`) whenever you change the destination.",
			},
			queryParamsAttr: schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				WriteOnly:   true,
				Sensitive:   true,
				Description: "Write-only URL query parameters sent with the webhook request. Never " +
					"stored in state or returned by the API. Not allowed for the `slack` service. Bump " +
					"`query_params_version` to re-send after a change.",
			},
			"headers_version": schema.StringAttribute{
				Optional: true,
				Description: "Arbitrary value that, when changed, forces the write-only `headers` to be " +
					"re-sent to the API. Because `headers` is write-only, Terraform cannot see a change " +
					"to its value: editing the `headers` block alone produces no plan diff and no update, " +
					"so bump this version (any new value) to roll a rotated secret. Note: this re-sends " +
					"the CURRENT `headers` value; it does NOT clear the secret. Omitting `headers` leaves " +
					"the last-sent value in place server-side — clearing a secret is not supported through " +
					"this resource; recreate the webhook to remove it.",
			},
			"query_params_version": schema.StringAttribute{
				Optional: true,
				Description: "Arbitrary value that, when changed, forces the write-only `query_params` to " +
					"be re-sent to the API. As with `headers_version`, it re-sends the current value and " +
					"cannot clear the secret; recreate the webhook to remove it.",
			},
			"body": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "Request body template for `generic` and `incidentio` services. Not allowed " +
					"for the `slack` service.",
			},
		},
	}
}

func (r *webhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *webhookResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_clickstack_webhook", &resp.Diagnostics)
	var cfg webhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(cfg.validate()...)
}

// validate holds the webhook's config rules as a pure function of the model so
// it can be unit-tested directly.
func (m *webhookResourceModel) validate() diag.Diagnostics {
	var diags diag.Diagnostics
	if m.Service.IsNull() || m.Service.IsUnknown() {
		return diags
	}

	if !slices.Contains(webhookServices, m.Service.ValueString()) {
		diags.AddAttributeError(
			path.Root("service"),
			"Invalid service",
			fmt.Sprintf("service must be one of %s, got %q", strings.Join(webhookServices, ", "), m.Service.ValueString()),
		)
	}

	// Slack posts a fixed Block Kit payload and rejects custom headers, query
	// params, and body.
	if m.Service.ValueString() == "slack" {
		if !m.Headers.IsNull() {
			diags.AddAttributeError(path.Root(headersAttr), "headers not allowed for slack", "The slack service does not support custom headers.")
		}
		if !m.QueryParams.IsNull() {
			diags.AddAttributeError(path.Root(queryParamsAttr), "query_params not allowed for slack", "The slack service does not support custom query parameters.")
		}
		if !m.Body.IsNull() {
			diags.AddAttributeError(path.Root("body"), "body not allowed for slack", "The slack service does not support a custom body.")
		}
	}
	return diags
}

func (r *webhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// Write-only values live only in config, not plan.
	var config webhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := plan.toClient(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	wh, err := r.client.WithTeam(plan.Team.ValueString()).CreateWebhook(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Webhook", err.Error())
		return
	}

	plan.applyWebhook(wh)
	tflog.Trace(ctx, "created webhook resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	wh, err := r.client.WithTeam(state.Team.ValueString()).GetWebhook(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Webhook", err.Error())
		return
	}

	state.applyWebhook(wh)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var config webhookResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := plan.toClient(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	wh, err := r.client.WithTeam(plan.Team.ValueString()).UpdateWebhook(ctx, plan.ID.ValueString(), input)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Updating Webhook", err.Error())
		return
	}

	plan.applyWebhook(wh)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.WithTeam(state.Team.ValueString()).DeleteWebhook(ctx, state.ID.ValueString()); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting Webhook", err.Error())
	}
}

func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Accept "<id>" (default team) or "<team>/<id>" for a non-default team.
	// Write-only secrets are null on import (they live only in config).
	if team, id, ok := strings.Cut(req.ID, "/"); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team"), team)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// --- conversion helpers ---

// toClient builds the API request. Normal fields come from the model (plan);
// the write-only Headers/QueryParams come from cfg (config), since write-only
// values are absent from the plan.
func (m *webhookResourceModel) toClient(ctx context.Context, cfg *webhookResourceModel) (client.Webhook, diag.Diagnostics) {
	var diags diag.Diagnostics
	wh := client.Webhook{
		Service:     m.Service.ValueString(),
		Name:        m.Name.ValueString(),
		URL:         m.URL.ValueString(),
		Description: optStringPtr(m.Description),
		Body:        optStringPtr(m.Body),
	}

	headers, d := mapToStringMap(ctx, cfg.Headers)
	diags.Append(d...)
	wh.Headers = headers

	qp, d := mapToStringMap(ctx, cfg.QueryParams)
	diags.Append(d...)
	wh.QueryParams = qp

	return wh, diags
}

// applyWebhook copies the API representation into the model. The write-only
// Headers/QueryParams are never set (the API does not return them, and they must
// stay null in state).
func (m *webhookResourceModel) applyWebhook(wh *client.Webhook) {
	m.ID = types.StringValue(wh.ID)
	m.Name = types.StringValue(wh.Name)
	m.Service = types.StringValue(wh.Service)
	m.URL = types.StringValue(wh.URL)
	m.Description = types.StringPointerValue(wh.Description)
	// Body round-trips only for the generic service; incidentio accepts it on
	// write but never returns it. Overwrite only when the API actually returned a
	// value, otherwise keep the configured/prior value so a body-bearing incidentio
	// webhook does not produce an "inconsistent result after apply" (state null vs
	// planned value).
	if wh.Body != nil {
		m.Body = types.StringValue(*wh.Body)
	}
}

// mapToStringMap converts a types.Map of strings to a Go map, returning nil for
// a null/unknown map.
func mapToStringMap(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	if m.IsNull() || m.IsUnknown() {
		return nil, nil
	}
	out := make(map[string]string, len(m.Elements()))
	diags := m.ElementsAs(ctx, &out, false)
	return out, diags
}
