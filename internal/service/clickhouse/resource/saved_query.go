package resource

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

var (
	_ resource.Resource                = (*SavedQueryResource)(nil)
	_ resource.ResourceWithConfigure   = (*SavedQueryResource)(nil)
	_ resource.ResourceWithImportState = (*SavedQueryResource)(nil)
)

//go:embed descriptions/saved_query.md
var savedQueryResourceDescription string

func NewSavedQueryResource() resource.Resource {
	return &SavedQueryResource{}
}

type SavedQueryResource struct {
	client api.Client
}

func (r *SavedQueryResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_saved_query"
}

func (r *SavedQueryResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: savedQueryResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique ID of the saved query.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": schema.StringAttribute{
				Description: "ID of the ClickHouse Cloud service for the saved query.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(uuidPattern, "must be a UUID"),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the saved query.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"sql": schema.StringAttribute{
				Description: "Saved SQL query.",
				Required:    true,
			},
			"database": schema.StringAttribute{
				Description: "Database used by the saved query.",
				Required:    true,
			},
			"parameters": schema.MapAttribute{
				Description: "Default query parameters.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Default:     mapdefault.StaticValue(types.MapValueMust(types.StringType, nil)),
			},
		},
	}
}

func (r *SavedQueryResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError(
			"ClickHouse Cloud API not configured",
			"This resource requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).",
		)
		return
	}
	r.client = providerData.API
}

func (r *SavedQueryResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	utils.BetaWarning("clickhouse_saved_query", &resp.Diagnostics)

	var plan models.SavedQueryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request, diags := savedQueryRequestFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	query, err := r.client.CreateSavedQuery(ctx, plan.ServiceID.ValueString(), request)
	if err != nil {
		resp.Diagnostics.AddError("Error creating saved query", err.Error())
		return
	}

	resp.Diagnostics.Append(applySavedQueryToState(ctx, query, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SavedQueryResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state models.SavedQueryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	query, err := r.client.GetSavedQuery(ctx, state.ServiceID.ValueString(), state.ID.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading saved query", err.Error())
		return
	}

	resp.Diagnostics.Append(applySavedQueryToState(ctx, query, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SavedQueryResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	utils.BetaWarning("clickhouse_saved_query", &resp.Diagnostics)

	var plan models.SavedQueryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request, diags := savedQueryRequestFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	query, err := r.client.UpdateSavedQuery(ctx, plan.ServiceID.ValueString(), plan.ID.ValueString(), request)
	if err != nil {
		resp.Diagnostics.AddError("Error updating saved query", err.Error())
		return
	}

	resp.Diagnostics.Append(applySavedQueryToState(ctx, query, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SavedQueryResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state models.SavedQueryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSavedQuery(ctx, state.ServiceID.ValueString(), state.ID.ValueString())
	if err == nil || api.IsNotFound(err) {
		return
	}
	if api.IsConflict(err) {
		addIncompatibleSavedQueryDiagnostic(&resp.Diagnostics, state.ID.ValueString())
		return
	}
	resp.Diagnostics.AddError("Error deleting saved query", err.Error())
}

func (r *SavedQueryResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	utils.BetaWarning("clickhouse_saved_query", &resp.Diagnostics)

	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 || !uuidPattern.MatchString(parts[0]) || !uuidPattern.MatchString(parts[1]) {
		resp.Diagnostics.AddError(
			"Invalid saved query import ID",
			"Use service_id:query_id, with a UUID for each value.",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func savedQueryRequestFromModel(
	ctx context.Context,
	model *models.SavedQueryResourceModel,
) (api.SavedQueryRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	parameters := map[string]string{}
	if !model.Parameters.IsNull() {
		diags.Append(model.Parameters.ElementsAs(ctx, &parameters, false)...)
	}
	if diags.HasError() {
		return api.SavedQueryRequest{}, diags
	}

	return api.SavedQueryRequest{
		Name:       model.Name.ValueString(),
		SQL:        model.SQL.ValueString(),
		Database:   model.Database.ValueString(),
		Parameters: parameters,
	}, diags
}

func applySavedQueryToState(
	ctx context.Context,
	query *api.SavedQuery,
	state *models.SavedQueryResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(query.ID)
	state.Name = types.StringValue(query.Name)
	state.SQL = types.StringValue(query.SQL)
	state.Database = types.StringValue(query.Database)

	parametersFromAPI := query.Parameters
	if parametersFromAPI == nil {
		parametersFromAPI = map[string]string{}
	}
	parameters, d := types.MapValueFrom(ctx, types.StringType, parametersFromAPI)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.Parameters = parameters
	return diags
}

func addIncompatibleSavedQueryDiagnostic(diags *diag.Diagnostics, queryID string) {
	diags.AddError(
		"Saved query cannot be managed by this resource",
		fmt.Sprintf("Saved query %s is not compatible with this resource.", queryID),
	)
}
