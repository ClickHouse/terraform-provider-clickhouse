package resource

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

const postgresPrivateEndpointResourceName = "clickhouse_postgres_private_endpoint"

// The Postgres service ID goes into request paths unescaped. Requiring a UUID
// keeps an ID containing `?` or `#` from cutting off `/privateEndpoint`, which
// would send the private endpoint's DELETE to the Postgres service itself.
var postgresIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

var postgresIDValidators = []validator.String{stringvalidator.RegexMatches(postgresIDPattern, "must be a UUID")}

var (
	_ resource.Resource                = &PostgresPrivateEndpointResource{}
	_ resource.ResourceWithConfigure   = &PostgresPrivateEndpointResource{}
	_ resource.ResourceWithImportState = &PostgresPrivateEndpointResource{}
)

//go:embed descriptions/postgres_private_endpoint.md
var postgresPrivateEndpointResourceDescription string

// NewPostgresPrivateEndpointResource constructs the Postgres private endpoint
// resource.
func NewPostgresPrivateEndpointResource() resource.Resource {
	return &PostgresPrivateEndpointResource{}
}

// PostgresPrivateEndpointResource manages the AWS private endpoint service of
// a Managed Postgres service. The allowed VPC endpoints are owned by
// PostgresPrivateEndpointAttachmentResource.
type PostgresPrivateEndpointResource struct {
	client api.Client
}

func (r *PostgresPrivateEndpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_private_endpoint"
}

func (r *PostgresPrivateEndpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{
			Description:   description,
			Computed:      true,
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: postgresPrivateEndpointResourceDescription,
		Attributes: map[string]schema.Attribute{
			"postgres_id": schema.StringAttribute{
				Description:   "ID of the AWS Postgres service. Changing it replaces the private endpoint service.",
				Required:      true,
				Validators:    postgresIDValidators,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"state":                 computed("State of the private endpoint service, such as `available`."),
			"endpoint_service_name": computed("AWS endpoint service name. Use it as the `service_name` of an `aws_vpc_endpoint`."),
			"private_hostname":      computed("Hostname for connecting to the Postgres service through an allowed VPC endpoint."),
		},
	}
}

func (r *PostgresPrivateEndpointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func isPostgresPrivateEndpointReady(e *api.PostgresPrivateEndpoint) bool {
	return e != nil && e.State == api.PostgresPrivateEndpointStateAvailable && e.EndpointServiceName != ""
}

func isPostgresPrivateEndpointGone(e *api.PostgresPrivateEndpoint) bool {
	return e == nil
}

func syncPostgresPrivateEndpoint(e *api.PostgresPrivateEndpoint, model *models.PostgresPrivateEndpointResourceModel) {
	model.State = types.StringValue(e.State)
	model.EndpointServiceName = stringOrNull(e.EndpointServiceName)
	model.PrivateHostname = stringOrNull(e.PrivateHostname)
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// Create creates the private endpoint service and waits until AWS has
// assigned its service name. The state is saved before the wait, so a failed
// wait taints the resource instead of leaving an untracked service behind.
func (r *PostgresPrivateEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning(postgresPrivateEndpointResourceName, &resp.Diagnostics)

	var plan models.PostgresPrivateEndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	postgresID := plan.PostgresID.ValueString()

	created, err := r.client.CreatePostgresPrivateEndpoint(ctx, postgresID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Postgres private endpoint",
			"Could not create the private endpoint service of Postgres service "+postgresID+": "+err.Error(),
		)
		return
	}
	syncPostgresPrivateEndpoint(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.WaitForPostgresPrivateEndpoint(ctx, postgresID, isPostgresPrivateEndpointReady, postgresDefaultCreateTimeoutSeconds); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for Postgres private endpoint to become available",
			"Could not finish creating the private endpoint service of Postgres service "+postgresID+": "+err.Error(),
		)
		return
	}
	r.refresh(ctx, postgresID, &plan, &resp.State, &resp.Diagnostics)
}

func (r *PostgresPrivateEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.PostgresPrivateEndpointResourceModel
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
			"Error reading Postgres private endpoint",
			"Could not read the private endpoint service of Postgres service "+state.PostgresID.ValueString()+": "+err.Error(),
		)
		return
	}
	syncPostgresPrivateEndpoint(e, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is never called: changing postgres_id replaces the resource, and
// every other attribute is computed.
func (r *PostgresPrivateEndpointResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update Not Supported",
		"Postgres private endpoints do not support updates. Changing postgres_id replaces the private endpoint service.",
	)
}

// Delete schedules deletion and waits until the private endpoint service is
// gone, so a replacement can create a new one for the same Postgres service.
func (r *PostgresPrivateEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.PostgresPrivateEndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	postgresID := state.PostgresID.ValueString()

	if err := r.client.DeletePostgresPrivateEndpoint(ctx, postgresID); err != nil {
		resp.Diagnostics.AddError(
			"Error deleting Postgres private endpoint",
			"Could not delete the private endpoint service of Postgres service "+postgresID+": "+err.Error(),
		)
		return
	}
	if err := r.client.WaitForPostgresPrivateEndpoint(ctx, postgresID, isPostgresPrivateEndpointGone, postgresDefaultCreateTimeoutSeconds); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for Postgres private endpoint deletion",
			"Could not confirm the private endpoint service of Postgres service "+postgresID+" was deleted: "+err.Error(),
		)
	}
}

func (r *PostgresPrivateEndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning(postgresPrivateEndpointResourceName, &resp.Diagnostics)

	importPostgresID(ctx, req, resp)
}

// importPostgresID imports a Postgres private endpoint resource by the
// Postgres service ID, which must be a UUID (see postgresIDPattern).
func importPostgresID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !postgresIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError(
			"Invalid Postgres service ID",
			fmt.Sprintf("Expected the UUID of a Postgres service, got %q.", req.ID),
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("postgres_id"), req, resp)
}

func (r *PostgresPrivateEndpointResource) refresh(ctx context.Context, postgresID string, model *models.PostgresPrivateEndpointResourceModel, state *tfsdk.State, diags *diag.Diagnostics) {
	e, err := r.client.GetPostgresPrivateEndpoint(ctx, postgresID)
	if err != nil {
		diags.AddError(
			"Error reading Postgres private endpoint",
			"Could not read the private endpoint service of Postgres service "+postgresID+": "+err.Error(),
		)
		return
	}
	syncPostgresPrivateEndpoint(e, model)
	diags.Append(state.Set(ctx, model)...)
}
