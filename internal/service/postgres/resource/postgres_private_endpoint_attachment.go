package resource

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
	"regexp"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

const postgresPrivateEndpointAttachmentResourceName = "clickhouse_postgres_private_endpoint_attachment"

// Server-side limits on the allowed VPC endpoints.
const (
	postgresMaxAllowedVpcEndpoints       = 50
	postgresVpcEndpointDescriptionMaxLen = 255
)

var (
	postgresVpcEndpointIDRegexp = regexp.MustCompile(`^vpce-[0-9a-f]{17}$`)
	// The server accepts one line of printable characters and strips leading
	// and trailing whitespace (Ruby String#strip), which would make the applied
	// value differ from the planned one.
	postgresVpcEndpointDescriptionRegexp = regexp.MustCompile(`^([^\p{Cc} ]([^\p{Cc}]*[^\p{Cc} ])?)?$`)
)

var (
	_ resource.Resource                = &PostgresPrivateEndpointAttachmentResource{}
	_ resource.ResourceWithConfigure   = &PostgresPrivateEndpointAttachmentResource{}
	_ resource.ResourceWithImportState = &PostgresPrivateEndpointAttachmentResource{}
)

//go:embed descriptions/postgres_private_endpoint_attachment.md
var postgresPrivateEndpointAttachmentResourceDescription string

// NewPostgresPrivateEndpointAttachmentResource constructs the resource that
// owns the VPC endpoints allowed to connect to a Postgres private endpoint.
func NewPostgresPrivateEndpointAttachmentResource() resource.Resource {
	return &PostgresPrivateEndpointAttachmentResource{}
}

// PostgresPrivateEndpointAttachmentResource owns the full list of VPC
// endpoints allowed to connect to a Postgres private endpoint service. It is
// separate from PostgresPrivateEndpointResource because the VPC endpoint
// needs the service name, and the allow-list needs the VPC endpoint's ID.
type PostgresPrivateEndpointAttachmentResource struct {
	client api.Client
}

func (r *PostgresPrivateEndpointAttachmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_private_endpoint_attachment"
}

func (r *PostgresPrivateEndpointAttachmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: postgresPrivateEndpointAttachmentResourceDescription,
		Attributes: map[string]schema.Attribute{
			"postgres_id": schema.StringAttribute{
				Description:   "ID of the Postgres service whose private endpoint service the VPC endpoints connect to.",
				Required:      true,
				Validators:    postgresIDValidators,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"allowed_vpc_endpoints": schema.MapNestedAttribute{
				Description: fmt.Sprintf("Every VPC endpoint allowed to connect, at most %d, keyed by the ID of the AWS VPC interface endpoint, such as `vpce-0123456789abcdef0`. "+
					"Endpoints left out can no longer connect, and an empty map removes all of them.", postgresMaxAllowedVpcEndpoints),
				Required: true,
				Validators: []validator.Map{
					mapvalidator.SizeAtMost(postgresMaxAllowedVpcEndpoints),
					mapvalidator.NoNullValues(),
					mapvalidator.KeysAre(stringvalidator.RegexMatches(postgresVpcEndpointIDRegexp, "must be an AWS VPC endpoint ID: vpce- followed by 17 lower-case hex characters")),
				},
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"description": schema.StringAttribute{
						Description: fmt.Sprintf("Description of the VPC endpoint: one line of at most %d characters, without leading or trailing whitespace. Defaults to an empty string.", postgresVpcEndpointDescriptionMaxLen),
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString(""),
						Validators: []validator.String{
							stringvalidator.UTF8LengthAtMost(postgresVpcEndpointDescriptionMaxLen),
							stringvalidator.RegexMatches(postgresVpcEndpointDescriptionRegexp, "must be one line of printable characters that does not start or end with whitespace"),
						},
					},
				}},
			},
		},
	}
}

func (r *PostgresPrivateEndpointAttachmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError("ClickHouse Cloud API not configured",
			"This resource requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).")
		return
	}
	r.client = providerData.API
}

// planToAllowedVpcEndpoints returns the planned VPC endpoints sorted by ID.
func planToAllowedVpcEndpoints(ctx context.Context, planned types.Map) ([]api.PostgresAllowedVpcEndpoint, diag.Diagnostics) {
	var elements map[string]models.PostgresAllowedVpcEndpointModel
	diags := planned.ElementsAs(ctx, &elements, false)
	endpoints := make([]api.PostgresAllowedVpcEndpoint, 0, len(elements))
	for _, id := range slices.Sorted(maps.Keys(elements)) {
		endpoints = append(endpoints, api.PostgresAllowedVpcEndpoint{VpcEndpointId: id, Description: elements[id].Description.ValueString()})
	}
	return endpoints, diags
}

func allowedVpcEndpointsToMap(ctx context.Context, endpoints []api.PostgresAllowedVpcEndpoint) (types.Map, diag.Diagnostics) {
	elements := make(map[string]models.PostgresAllowedVpcEndpointModel, len(endpoints))
	for _, e := range endpoints {
		elements[e.VpcEndpointId] = models.PostgresAllowedVpcEndpointModel{Description: types.StringValue(e.Description)}
	}
	return types.MapValueFrom(ctx, models.PostgresAllowedVpcEndpointModel{}.ObjectType(), elements)
}

// apply replaces the allowed VPC endpoints with the planned ones and stores
// what the API returns.
func (r *PostgresPrivateEndpointAttachmentResource) apply(ctx context.Context, plan models.PostgresPrivateEndpointAttachmentResourceModel, state *tfsdk.State) diag.Diagnostics {
	endpoints, diags := planToAllowedVpcEndpoints(ctx, plan.AllowedVpcEndpoints)
	if diags.HasError() {
		return diags
	}
	postgresID := plan.PostgresID.ValueString()
	e, err := r.client.UpdatePostgresPrivateEndpoint(ctx, postgresID, api.PostgresPrivateEndpointUpdate{AllowedVpcEndpoints: &endpoints})
	if err != nil {
		diags.AddError(
			"Error setting Postgres allowed VPC endpoints",
			"Could not set the VPC endpoints allowed to connect to Postgres service "+postgresID+": "+err.Error(),
		)
		return diags
	}
	plan.AllowedVpcEndpoints, diags = allowedVpcEndpointsToMap(ctx, e.AllowedVpcEndpoints)
	if diags.HasError() {
		return diags
	}
	return state.Set(ctx, plan)
}

func (r *PostgresPrivateEndpointAttachmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning(postgresPrivateEndpointAttachmentResourceName, &resp.Diagnostics)

	var plan models.PostgresPrivateEndpointAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, plan, &resp.State)...)
}

func (r *PostgresPrivateEndpointAttachmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.PostgresPrivateEndpointAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	e, err := r.client.GetPostgresPrivateEndpoint(ctx, state.PostgresID.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading Postgres allowed VPC endpoints",
			"Could not read the allowed VPC endpoints of Postgres service "+state.PostgresID.ValueString()+": "+err.Error(),
		)
		return
	}
	var d diag.Diagnostics
	state.AllowedVpcEndpoints, d = allowedVpcEndpointsToMap(ctx, e.AllowedVpcEndpoints)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PostgresPrivateEndpointAttachmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning(postgresPrivateEndpointAttachmentResourceName, &resp.Diagnostics)

	var plan models.PostgresPrivateEndpointAttachmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, plan, &resp.State)...)
}

// Delete removes every allowed VPC endpoint. A missing private endpoint
// service already allows none, so a 404 counts as deleted.
func (r *PostgresPrivateEndpointAttachmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.PostgresPrivateEndpointAttachmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	postgresID := state.PostgresID.ValueString()
	none := []api.PostgresAllowedVpcEndpoint{}
	_, err := r.client.UpdatePostgresPrivateEndpoint(ctx, postgresID, api.PostgresPrivateEndpointUpdate{AllowedVpcEndpoints: &none})
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Error removing Postgres allowed VPC endpoints",
			"Could not remove the VPC endpoints allowed to connect to Postgres service "+postgresID+": "+err.Error(),
		)
	}
}

func (r *PostgresPrivateEndpointAttachmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning(postgresPrivateEndpointAttachmentResourceName, &resp.Diagnostics)

	importPostgresID(ctx, req, resp)
}
