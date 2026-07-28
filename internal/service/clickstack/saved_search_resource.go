package clickstack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
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
	_ resource.Resource                   = (*savedSearchResource)(nil)
	_ resource.ResourceWithConfigure      = (*savedSearchResource)(nil)
	_ resource.ResourceWithImportState    = (*savedSearchResource)(nil)
	_ resource.ResourceWithValidateConfig = (*savedSearchResource)(nil)
)

// NewSavedSearchResource is a helper to register the resource with the provider.
func NewSavedSearchResource() resource.Resource {
	return &savedSearchResource{}
}

// savedSearchResource manages a ClickStack saved search.
type savedSearchResource struct {
	client *client.Client
}

// savedSearchResourceModel maps the resource schema data. Filters is an opaque
// JSON string round-tripped verbatim (KTD6): the pinned-filter shapes are a
// union the provider does not model, so passing the raw JSON through guarantees
// no filter is lost on the full-replace PUT.
type savedSearchResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Team          types.String `tfsdk:"team"`
	Name          types.String `tfsdk:"name"`
	SourceID      types.String `tfsdk:"source_id"`
	Select        types.String `tfsdk:"select"`
	Where         types.String `tfsdk:"where"`
	WhereLanguage types.String `tfsdk:"where_language"`
	OrderBy       types.String `tfsdk:"order_by"`
	Tags          types.List   `tfsdk:"tags"`
	Filters       types.String `tfsdk:"filters"`
}

func (r *savedSearchResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_saved_search"
}

func (r *savedSearchResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a ClickStack saved search: a named query over a source that alerts can " +
			"target. The API PUT is a full replace, so every attribute is always sent; omitted " +
			"optional fields reset to their server defaults.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Computed:      true,
				Description:   "Identifier of the saved search.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			teamAttr: schema.StringAttribute{
				Optional: true,
				Description: "Team ID to manage this saved search under (`x-hdx-team`). " +
					"Changing this forces the saved search to be replaced.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			nameAttr: schema.StringAttribute{
				Required:    true,
				Description: "Display name for the saved search.",
			},
			"source_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the ClickStack source this saved search queries.",
			},
			"select": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(""),
				Description:   "Comma-separated column expressions to select. Defaults to empty.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"where": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(""),
				Description:   "Row filter expression. Defaults to empty.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"where_language": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString("lucene"),
				Description:   "Language of `where`: `lucene` (default) or `sql`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"order_by": schema.StringAttribute{
				Optional:      true,
				Computed:      true,
				Default:       stringdefault.StaticString(""),
				Description:   "Order-by expression. Defaults to empty.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"tags": schema.ListAttribute{
				ElementType:   types.StringType,
				Optional:      true,
				Computed:      true,
				Description:   "Tags applied to the saved search.",
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
			"filters": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("[]"),
				Description: "Pinned sidebar filters as a JSON array string. Defaults to `[]`; the " +
					"authored value is kept as-is (removing the attribute resets it to `[]`). Each " +
					"filter object should include its `type` (`sql`, `lucene`, or `sql_ast`) and only " +
					"the keys the API recognizes.",
				PlanModifiers: []planmodifier.String{
					jsonEqualPlanModifier{},
				},
			},
		},
	}
}

func (r *savedSearchResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ValidateConfig checks enum and JSON-shape constraints at plan time so invalid
// values surface before apply rather than as opaque API errors.
func (r *savedSearchResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_clickstack_saved_search", &resp.Diagnostics)
	var cfg savedSearchResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(cfg.validate()...)
}

// validate holds the saved search's config rules as a pure function of the model
// so it can be unit-tested directly.
func (m *savedSearchResourceModel) validate() diag.Diagnostics {
	var diags diag.Diagnostics

	if known(m.WhereLanguage) {
		switch m.WhereLanguage.ValueString() {
		case "lucene", "sql":
		default:
			diags.AddAttributeError(path.Root("where_language"), "Invalid where_language",
				`where_language must be "lucene" or "sql"`)
		}
	}

	if known(m.Filters) && !json.Valid([]byte(m.Filters.ValueString())) {
		diags.AddAttributeError(path.Root("filters"), "Invalid filters",
			"filters must be a valid JSON array string")
	}

	return diags
}

func (r *savedSearchResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan savedSearchResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := plan.toClient(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ss, err := r.client.WithTeam(plan.Team.ValueString()).CreateSavedSearch(ctx, input)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Saved Search", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applySavedSearch(ss)...)
	tflog.Trace(ctx, "created saved search resource")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *savedSearchResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state savedSearchResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ss, err := r.client.WithTeam(state.Team.ValueString()).GetSavedSearch(ctx, state.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Saved Search", err.Error())
		return
	}

	resp.Diagnostics.Append(state.applySavedSearch(ss)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *savedSearchResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan savedSearchResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input, diags := plan.toClient(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ss, err := r.client.WithTeam(plan.Team.ValueString()).UpdateSavedSearch(ctx, plan.ID.ValueString(), input)
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Updating Saved Search", err.Error())
		return
	}

	resp.Diagnostics.Append(plan.applySavedSearch(ss)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *savedSearchResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state savedSearchResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.WithTeam(state.Team.ValueString()).DeleteSavedSearch(ctx, state.ID.ValueString()); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			return
		}
		resp.Diagnostics.AddError("Error Deleting Saved Search", err.Error())
	}
}

func (r *savedSearchResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if team, id, ok := strings.Cut(req.ID, "/"); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("team"), team)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// --- conversion helpers ---

func (m *savedSearchResourceModel) toClient(ctx context.Context) (client.SavedSearch, diag.Diagnostics) {
	var diags diag.Diagnostics
	ss := client.SavedSearch{
		Name:          m.Name.ValueString(),
		SourceID:      m.SourceID.ValueString(),
		Select:        m.Select.ValueString(),
		Where:         m.Where.ValueString(),
		WhereLanguage: m.WhereLanguage.ValueString(),
		OrderBy:       m.OrderBy.ValueString(),
	}

	// Full-replace PUT: always send tags as an array ([] when unset), never null.
	tags := []string{}
	if !m.Tags.IsNull() && !m.Tags.IsUnknown() {
		diags.Append(m.Tags.ElementsAs(ctx, &tags, false)...)
	}
	ss.Tags = tags

	// Filters is an opaque JSON array string; default to [] so the full-replace
	// PUT always carries a valid value.
	filters := strings.TrimSpace(m.Filters.ValueString())
	if m.Filters.IsNull() || m.Filters.IsUnknown() || filters == "" {
		filters = "[]"
	}
	ss.Filters = json.RawMessage(filters)

	return ss, diags
}

func (m *savedSearchResourceModel) applySavedSearch(ss *client.SavedSearch) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(ss.ID)
	m.Name = types.StringValue(ss.Name)
	m.SourceID = types.StringValue(ss.SourceID)
	m.Select = types.StringValue(ss.Select)
	m.Where = types.StringValue(ss.Where)
	m.WhereLanguage = types.StringValue(ss.WhereLanguage)
	m.OrderBy = types.StringValue(ss.OrderBy)

	list, d := stringSliceToList(ss.Tags)
	diags.Append(d...)
	m.Tags = list

	// Keep the authored filters value (config is the source of truth for this
	// opaque passthrough). Adopting the server's value would be a hard
	// "inconsistent result after apply" whenever the server normalizes filters —
	// injecting a default `type` or stripping unknown keys makes the returned
	// value differ from the known planned value. Only fall back to the server
	// value if the config left filters unset (should not happen given the []
	// default, but defensive).
	if !known(m.Filters) {
		if len(ss.Filters) > 0 {
			m.Filters = types.StringValue(string(ss.Filters))
		} else {
			m.Filters = types.StringValue("[]")
		}
	}

	return diags
}

// stringSliceToList converts a []string to a types.List, producing an empty
// (non-null) list for a nil/empty slice.
func stringSliceToList(ss []string) (types.List, diag.Diagnostics) {
	elems := make([]attr.Value, len(ss))
	for i, s := range ss {
		elems[i] = types.StringValue(s)
	}
	return types.ListValue(types.StringType, elems)
}
