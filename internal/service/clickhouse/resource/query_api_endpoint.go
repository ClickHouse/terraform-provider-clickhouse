package resource

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

var (
	_ resource.Resource                   = (*QueryAPIEndpointResource)(nil)
	_ resource.ResourceWithConfigure      = (*QueryAPIEndpointResource)(nil)
	_ resource.ResourceWithImportState    = (*QueryAPIEndpointResource)(nil)
	_ resource.ResourceWithValidateConfig = (*QueryAPIEndpointResource)(nil)

	queryAPIEndpointNonWhitespacePattern = regexp.MustCompile(`\S`)
)

//go:embed descriptions/query_api_endpoint.md
var queryAPIEndpointResourceDescription string

func NewQueryAPIEndpointResource() resource.Resource {
	return &QueryAPIEndpointResource{}
}

type QueryAPIEndpointResource struct {
	client api.Client
}

func (r *QueryAPIEndpointResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_query_api_endpoint"
}

func (r *QueryAPIEndpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: queryAPIEndpointResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique ID of the Query API endpoint.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": schema.StringAttribute{
				Description: "ID of the ClickHouse Cloud service that runs the query.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(uuidPattern, "must be a UUID"),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the Query API endpoint.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(queryAPIEndpointNonWhitespacePattern, "must contain at least one non-whitespace character"),
				},
			},
			"sql": schema.StringAttribute{
				Description: "SQL executed by the endpoint.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(queryAPIEndpointNonWhitespacePattern, "must contain at least one non-whitespace character"),
				},
			},
			"database": schema.StringAttribute{
				Description: "Database used by the Query API endpoint.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(queryAPIEndpointNonWhitespacePattern, "must contain at least one non-whitespace character"),
				},
			},
			"parameters": schema.MapAttribute{
				Description: "Default query parameters.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Default:     mapdefault.StaticValue(types.MapValueMust(types.StringType, nil)),
			},
			"api_key_ids": schema.SetAttribute{
				Description: "API key IDs allowed to call the endpoint.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(
						stringvalidator.RegexMatches(uuidPattern, "must be a UUID"),
					),
				},
			},
			"roles": schema.SetAttribute{
				Description: "Database roles used to run the query.",
				Required:    true,
				ElementType: types.StringType,
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.ValueStringsAre(stringvalidator.LengthAtLeast(1)),
				},
			},
			"allowed_origins": schema.SetAttribute{
				Description: "Origins allowed by the endpoint CORS policy. Omit this field to restrict access to backend servers.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
			},
			"url": schema.StringAttribute{
				Description: "Public URL used to execute the endpoint.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *QueryAPIEndpointResource) Configure(
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

func (r *QueryAPIEndpointResource) ValidateConfig(
	_ context.Context,
	_ resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	utils.BetaWarning("clickhouse_query_api_endpoint", &resp.Diagnostics)
}

func (r *QueryAPIEndpointResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan models.QueryAPIEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request, diags := queryAPIEndpointRequestFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := r.client.CreateQueryAPIEndpoint(
		ctx,
		plan.ServiceID.ValueString(),
		request,
	)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Query API endpoint", err.Error())
		return
	}

	resp.Diagnostics.Append(applyQueryAPIEndpointToState(ctx, endpoint, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *QueryAPIEndpointResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	utils.BetaWarning("clickhouse_query_api_endpoint", &resp.Diagnostics)

	var state models.QueryAPIEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := r.client.GetQueryAPIEndpoint(
		ctx,
		state.ServiceID.ValueString(),
		state.ID.ValueString(),
	)
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Query API endpoint", err.Error())
		return
	}
	if endpoint.OwnerType != api.QueryAPIEndpointOwnerType {
		addUserOwnedQueryAPIEndpointDiagnostic(&resp.Diagnostics, endpoint.ID)
		return
	}

	resp.Diagnostics.Append(applyQueryAPIEndpointToState(ctx, endpoint, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *QueryAPIEndpointResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan models.QueryAPIEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	request, diags := queryAPIEndpointRequestFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint, err := r.client.UpdateQueryAPIEndpoint(
		ctx,
		plan.ServiceID.ValueString(),
		plan.ID.ValueString(),
		request,
	)
	if err != nil {
		if api.IsConflict(err) {
			addUserOwnedQueryAPIEndpointDiagnostic(&resp.Diagnostics, plan.ID.ValueString())
			return
		}
		resp.Diagnostics.AddError("Error updating Query API endpoint", err.Error())
		return
	}

	resp.Diagnostics.Append(applyQueryAPIEndpointToState(ctx, endpoint, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *QueryAPIEndpointResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state models.QueryAPIEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteQueryAPIEndpoint(
		ctx,
		state.ServiceID.ValueString(),
		state.ID.ValueString(),
	)
	if err == nil || api.IsNotFound(err) {
		return
	}
	if api.IsConflict(err) {
		addUserOwnedQueryAPIEndpointDiagnostic(&resp.Diagnostics, state.ID.ValueString())
		return
	}
	resp.Diagnostics.AddError("Error deleting Query API endpoint", err.Error())
}

func (r *QueryAPIEndpointResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	parts := strings.Split(req.ID, ":")
	if len(parts) != 2 || !uuidPattern.MatchString(parts[0]) || !uuidPattern.MatchString(parts[1]) {
		resp.Diagnostics.AddError(
			"Invalid Query API endpoint import ID",
			"Use service_id:endpoint_id, with a UUID for each value.",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

func queryAPIEndpointRequestFromModel(
	ctx context.Context,
	model *models.QueryAPIEndpointResourceModel,
) (api.QueryAPIEndpointRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	parameters := map[string]string{}
	var apiKeyIDs []string
	var roles []string
	allowedOrigins := []string{}

	if !model.Parameters.IsNull() {
		diags.Append(model.Parameters.ElementsAs(ctx, &parameters, false)...)
	}
	diags.Append(model.APIKeyIDs.ElementsAs(ctx, &apiKeyIDs, false)...)
	diags.Append(model.Roles.ElementsAs(ctx, &roles, false)...)
	if !model.AllowedOrigins.IsNull() {
		diags.Append(model.AllowedOrigins.ElementsAs(ctx, &allowedOrigins, false)...)
	}
	if diags.HasError() {
		return api.QueryAPIEndpointRequest{}, diags
	}

	return api.QueryAPIEndpointRequest{
		Name:           model.Name.ValueString(),
		SQL:            model.SQL.ValueString(),
		Database:       model.Database.ValueString(),
		Parameters:     parameters,
		APIKeyIDs:      apiKeyIDs,
		Roles:          roles,
		AllowedOrigins: allowedOrigins,
	}, diags
}

func applyQueryAPIEndpointToState(
	ctx context.Context,
	endpoint *api.QueryAPIEndpoint,
	state *models.QueryAPIEndpointResourceModel,
) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(endpoint.ID)
	state.Name = types.StringValue(endpoint.Name)
	state.SQL = types.StringValue(endpoint.SQL)
	state.Database = types.StringValue(endpoint.Database)
	state.URL = types.StringValue(endpoint.URL)

	parameters, d := types.MapValueFrom(ctx, types.StringType, endpoint.Parameters)
	diags.Append(d...)
	apiKeyIDs, d := types.SetValueFrom(ctx, types.StringType, endpoint.APIKeyIDs)
	diags.Append(d...)
	roles, d := types.SetValueFrom(ctx, types.StringType, endpoint.Roles)
	diags.Append(d...)
	allowedOrigins, d := types.SetValueFrom(ctx, types.StringType, endpoint.AllowedOrigins)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	state.Parameters = parameters
	state.APIKeyIDs = apiKeyIDs
	state.Roles = roles
	state.AllowedOrigins = allowedOrigins
	return diags
}

func addUserOwnedQueryAPIEndpointDiagnostic(diags *diag.Diagnostics, endpointID string) {
	diags.AddError(
		"Query API endpoint cannot be managed by this resource",
		fmt.Sprintf("Endpoint %s is not compatible with this resource.", endpointID),
	)
}
