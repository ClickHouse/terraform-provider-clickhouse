package clickstack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
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
	_ resource.Resource                     = (*sourceLinksResource)(nil)
	_ resource.ResourceWithConfigure        = (*sourceLinksResource)(nil)
	_ resource.ResourceWithConfigValidators = (*sourceLinksResource)(nil)
	_ resource.ResourceWithImportState      = (*sourceLinksResource)(nil)
)

const (
	sourceIDAttr        = "source_id"
	logSourceIDAttr     = "log_source_id"
	traceSourceIDAttr   = "trace_source_id"
	metricSourceIDAttr  = "metric_source_id"
	sessionSourceIDAttr = "session_source_id"

	// importedKey marks a just-imported resource in private state, so the
	// first Read adopts every link the source holds instead of only the ones
	// already in state (none, after an import).
	importedKey = "imported"
)

// NewSourceLinksResource is a helper to register the resource with the provider.
func NewSourceLinksResource() resource.Resource {
	return &sourceLinksResource{}
}

// sourceLinksResource manages the correlated-source links of one ClickStack
// source, separately from the source itself, so two sources can link to each
// other: each link needs the other source's server-generated ID, which a single
// resource cannot reference without a cycle.
type sourceLinksResource struct {
	client *client.Client
}

type sourceLinksResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Team            types.String `tfsdk:"team"`
	SourceID        types.String `tfsdk:"source_id"`
	LogSourceID     types.String `tfsdk:"log_source_id"`
	TraceSourceID   types.String `tfsdk:"trace_source_id"`
	MetricSourceID  types.String `tfsdk:"metric_source_id"`
	SessionSourceID types.String `tfsdk:"session_source_id"`
}

type sourceLinkField struct {
	key string
	val *types.String
	api *string
}

// fields pairs each link attribute of m with its API key and with its value in
// src, which may be nil.
func (m *sourceLinksResourceModel) fields(src *client.Source) []sourceLinkField {
	if src == nil {
		src = &client.Source{}
	}
	return []sourceLinkField{
		{client.LinkKeyLogSource, &m.LogSourceID, src.LogSourceID},
		{client.LinkKeyTraceSource, &m.TraceSourceID, src.TraceSourceID},
		{client.LinkKeyMetricSource, &m.MetricSourceID, src.MetricSourceID},
		{client.LinkKeySessionSource, &m.SessionSourceID, src.SessionSourceID},
	}
}

// applyLinks copies the links of src into the attributes m manages; an
// attribute that is null in m is not managed and stays null.
func (m *sourceLinksResourceModel) applyLinks(src *client.Source) {
	for _, f := range m.fields(src) {
		*f.val = keepUnmanaged(*f.val, f.api)
	}
}

func (r *sourceLinksResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_source_links"
}

func (r *sourceLinksResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	linkAttr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{
			Optional: true,
			Description: desc + " When unset, this resource leaves the link as the server holds it. " +
				"Removing it from config clears it.",
		}
	}

	resp.Schema = schema.Schema{
		Description: "Manages the correlated-source links of a ClickStack source (`log_source_id`, " +
			"`trace_source_id`, `metric_source_id`, `session_source_id`), separately from " +
			"`clickhouse_clickstack_source`. Use it when two sources link to each other, which a " +
			"`clickhouse_clickstack_source` cannot express because each needs the other's ID: create both " +
			"sources without the reverse link and set it here.\n\n" +
			"Only the links set in config are managed; other links on the source are left alone. Destroying " +
			"the resource clears the links it manages. Use at most one of these resources per source, and do " +
			"not also set the same link on the `clickhouse_clickstack_source` itself, or the two will " +
			"overwrite each other.\n\n" +
			"Which links a source accepts depends on its kind: `log` takes `metric_source_id` and " +
			"`trace_source_id`; `trace` takes `log_source_id`, `metric_source_id` and `session_source_id`; " +
			"`session` takes `trace_source_id`; `metric` takes `log_source_id`.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:      true,
				Description:   "Identifier of the resource; the same as `source_id`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID the source belongs to, sent as the `x-hdx-team` header. Defaults to " +
					"the API key's team. Only honored by multi-team (EE) deployments. Changing this forces " +
					"the resource to be replaced.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			sourceIDAttr: schema.StringAttribute{
				Required:      true,
				Description:   "ID of the source whose links are managed. Changing this forces the resource to be replaced.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			logSourceIDAttr:     linkAttr("Correlated log source ID (trace, metric)."),
			traceSourceIDAttr:   linkAttr("Correlated trace source ID (log, session)."),
			metricSourceIDAttr:  linkAttr("Correlated metric source ID (log, trace)."),
			sessionSourceIDAttr: linkAttr("Correlated session source ID (trace)."),
		},
	}
}

func (r *sourceLinksResource) ConfigValidators(_ context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.AtLeastOneOf(
			path.MatchRoot(logSourceIDAttr),
			path.MatchRoot(traceSourceIDAttr),
			path.MatchRoot(metricSourceIDAttr),
			path.MatchRoot(sessionSourceIDAttr),
		),
	}
}

func (r *sourceLinksResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *sourceLinksResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning("clickhouse_clickstack_source_links", &resp.Diagnostics)

	var plan sourceLinksResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	links := map[string]*string{}
	for _, f := range plan.fields(nil) {
		if !f.val.IsNull() {
			links[f.key] = f.val.ValueStringPointer()
		}
	}

	src, err := r.client.WithTeam(plan.Team.ValueString()).SetSourceLinks(ctx, plan.SourceID.ValueString(), links)
	if err != nil {
		resp.Diagnostics.AddError("Error Setting Source Links", err.Error())
		return
	}

	plan.ID = plan.SourceID
	plan.applyLinks(src)
	tflog.Trace(ctx, "created source links resource")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sourceLinksResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sourceLinksResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	src, err := r.client.WithTeam(state.Team.ValueString()).GetSource(ctx, state.SourceID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Source", err.Error())
		return
	}

	imported, diags := req.Private.GetKey(ctx, importedKey)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if imported != nil {
		for _, f := range state.fields(src) {
			*f.val = emptyToNull(types.StringPointerValue(f.api).ValueString())
		}
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, nil)...)
	} else {
		state.applyLinks(src)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *sourceLinksResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning("clickhouse_clickstack_source_links", &resp.Diagnostics)

	var plan, state sourceLinksResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// A link null in the plan but set in state was removed from config and is
	// cleared; one null in both is not managed and is left out of the write.
	links := map[string]*string{}
	stateFields := state.fields(nil)
	for i, f := range plan.fields(nil) {
		switch {
		case !f.val.IsNull():
			links[f.key] = f.val.ValueStringPointer()
		case !stateFields[i].val.IsNull():
			links[f.key] = nil
		}
	}

	src, err := r.client.WithTeam(plan.Team.ValueString()).SetSourceLinks(ctx, plan.SourceID.ValueString(), links)
	if err != nil {
		resp.Diagnostics.AddError("Error Setting Source Links", err.Error())
		return
	}

	plan.applyLinks(src)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sourceLinksResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sourceLinksResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	links := map[string]*string{}
	for _, f := range state.fields(nil) {
		if !f.val.IsNull() {
			links[f.key] = nil
		}
	}
	if len(links) == 0 {
		return
	}

	if _, err := r.client.WithTeam(state.Team.ValueString()).SetSourceLinks(ctx, state.SourceID.ValueString(), links); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Clearing Source Links", err.Error())
	}
}

func (r *sourceLinksResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning("clickhouse_clickstack_source_links", &resp.Diagnostics)

	// Accept "<source_id>" (default team) or "<team>/<source_id>" for a non-default team.
	sourceID := req.ID
	if team, id, ok := strings.Cut(req.ID, "/"); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(teamAttr), team)...)
		sourceID = id
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(idAttr), sourceID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(sourceIDAttr), sourceID)...)
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, importedKey, []byte("true"))...)
}
