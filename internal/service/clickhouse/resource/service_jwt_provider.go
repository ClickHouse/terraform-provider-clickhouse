package resource

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	_ resource.Resource                = &ServiceJWTProviderResource{}
	_ resource.ResourceWithConfigure   = &ServiceJWTProviderResource{}
	_ resource.ResourceWithImportState = &ServiceJWTProviderResource{}
)

//go:embed descriptions/service_jwt_provider.md
var serviceJWTProviderResourceDescription string

// The validation rules below mirror the server's (`@cp/common/protocol/Jwt`),
// so mistakes fail at plan time instead of after the JWKS fetch. They also
// reject surrounding whitespace, which the server would silently trim and
// leave the state inconsistent with the configuration.
const jwtFieldMaxLength = 256

var (
	jwtProviderNamePattern = regexp.MustCompile(`^[a-z0-9-]{1,59}$`)
	// Printable ASCII without whitespace (0x21-0x7E).
	jwtPrintablePattern = regexp.MustCompile(`^[\x21-\x7E]+$`)
	jwtHTTPSURLPattern  = regexp.MustCompile(`^https://[\x21-\x7E]+$`)

	// A name may not equal one of these or start with one followed by a hyphen.
	jwtProviderReservedNames = []string{"cloud", "clickhouse", "ch", "internal", "system", "default"}
	// Registered JWT claims that cannot carry the roles.
	jwtReservedClaimNames = []string{"iss", "sub", "aud", "exp", "nbf", "iat", "jti"}
)

func NewServiceJWTProviderResource() resource.Resource {
	return &ServiceJWTProviderResource{}
}

type ServiceJWTProviderResource struct {
	client api.Client
}

func (r *ServiceJWTProviderResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_jwt_provider"
}

func (r *ServiceJWTProviderResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	printable := stringvalidator.RegexMatches(jwtPrintablePattern, "must be printable ASCII without whitespace")
	resp.Schema = schema.Schema{
		MarkdownDescription: serviceJWTProviderResourceDescription,
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				Description: "ID of the ClickHouse Cloud service the JWT provider is configured on.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(uuidPattern, "must be a UUID"),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the JWT provider, unique within the service. Lowercase letters, digits and hyphens, at most 59 characters. Cannot be `cloud`, `clickhouse`, `ch`, `internal`, `system` or `default`, or start with one of these followed by a hyphen.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(jwtProviderNamePattern, "must be 1-59 lowercase letters, digits or hyphens"),
					jwtProviderNameNotReservedValidator{},
				},
			},
			"issuer": schema.StringAttribute{
				Description: "Expected `iss` claim of the tokens.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(jwtFieldMaxLength),
					printable,
				},
			},
			"audience": schema.StringAttribute{
				Description: "Expected `aud` claim of the tokens.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(jwtFieldMaxLength),
					printable,
				},
			},
			"jwks_url": schema.StringAttribute{
				Description: "Public `https://` URL of the JSON Web Key Set used to verify token signatures. It must contain at least one RS256 (RSA) key and is fetched and validated on every create and update.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(jwtFieldMaxLength),
					stringvalidator.RegexMatches(jwtHTTPSURLPattern, "must be an https:// URL without whitespace"),
				},
			},
			"roles_claim": schema.StringAttribute{
				Description: "Name of the token claim that lists the ClickHouse roles to grant the user. Cannot be a registered JWT claim (`iss`, `sub`, `aud`, `exp`, `nbf`, `iat`, `jti`). If omitted, users only get the permissions of the `default` user.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(jwtFieldMaxLength),
					printable,
					stringvalidator.NoneOf(jwtReservedClaimNames...),
				},
			},
		},
	}
}

func (r *ServiceJWTProviderResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ServiceJWTProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning("clickhouse_service_jwt_provider", &resp.Diagnostics)

	var plan models.ServiceJWTProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()
	name := plan.Name.ValueString()
	provider, err := r.client.CreateJWTProvider(ctx, serviceID, api.JWTProvider{
		Name:       name,
		Issuer:     plan.Issuer.ValueString(),
		Audience:   plan.Audience.ValueString(),
		JWKSURL:    plan.JWKSURL.ValueString(),
		RolesClaim: plan.RolesClaim.ValueStringPointer(),
	})
	if err != nil {
		if api.IsConflict(err) {
			resp.Diagnostics.AddError(
				"JWT provider already exists",
				fmt.Sprintf("Service %s already has a JWT provider named %q. Import it into Terraform with: terraform import clickhouse_service_jwt_provider.<name> %s:%s", serviceID, name, serviceID, name),
			)
			return
		}
		addJWTProviderErrorDiagnostic(&resp.Diagnostics, "creating", serviceID, name, err)
		return
	}

	applyJWTProviderToState(provider, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceJWTProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ServiceJWTProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	provider, err := r.client.GetJWTProvider(ctx, state.ServiceID.ValueString(), state.Name.ValueString())
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		addJWTProviderErrorDiagnostic(&resp.Diagnostics, "reading", state.ServiceID.ValueString(), state.Name.ValueString(), err)
		return
	}

	applyJWTProviderToState(provider, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceJWTProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	utils.BetaWarning("clickhouse_service_jwt_provider", &resp.Diagnostics)

	var plan models.ServiceJWTProviderResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()
	name := plan.Name.ValueString()
	provider, err := r.client.UpdateJWTProvider(ctx, serviceID, name, api.JWTProviderUpdate{
		Issuer:     plan.Issuer.ValueString(),
		Audience:   plan.Audience.ValueString(),
		JWKSURL:    plan.JWKSURL.ValueString(),
		RolesClaim: plan.RolesClaim.ValueStringPointer(),
	})
	if err != nil {
		addJWTProviderErrorDiagnostic(&resp.Diagnostics, "updating", serviceID, name, err)
		return
	}

	applyJWTProviderToState(provider, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceJWTProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ServiceJWTProviderResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteJWTProvider(ctx, state.ServiceID.ValueString(), state.Name.ValueString())
	if err != nil && !api.IsNotFound(err) {
		addJWTProviderErrorDiagnostic(&resp.Diagnostics, "deleting", state.ServiceID.ValueString(), state.Name.ValueString(), err)
	}
}

func (r *ServiceJWTProviderResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning("clickhouse_service_jwt_provider", &resp.Diagnostics)

	serviceID, name, ok := strings.Cut(req.ID, ":")
	if !ok || !uuidPattern.MatchString(serviceID) || !jwtProviderNamePattern.MatchString(name) {
		resp.Diagnostics.AddError(
			"Invalid JWT provider import ID",
			fmt.Sprintf("Expected service_id:name with a UUID service ID and a valid provider name, got %q.", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), serviceID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

// addJWTProviderErrorDiagnostic specializes the diagnostic for the documented
// client errors of the JWT provider endpoints. A 400 is left as the raw error:
// its body already says what was rejected (e.g. an unreachable JWKS URL).
// `verb` is "creating", "reading", "updating" or "deleting".
func addJWTProviderErrorDiagnostic(diags *diag.Diagnostics, verb, serviceID, name string, err error) {
	switch {
	case api.IsForbidden(err):
		diags.AddError(
			fmt.Sprintf("Error %s JWT provider: forbidden", verb),
			fmt.Sprintf("Managing JWT providers requires the Enterprise tier and an API key with `control-plane:service:manage` on service %s. Upgrade the organization or contact ClickHouse support if the feature should be available.\n\n%s", serviceID, err.Error()),
		)
	case api.IsNotFound(err):
		diags.AddError(
			fmt.Sprintf("Error %s JWT provider: not found", verb),
			fmt.Sprintf("Service %s or its JWT provider %q does not exist or is not visible to the caller. Confirm the service ID is correct and the API key has access.\n\n%s", serviceID, name, err.Error()),
		)
	default:
		diags.AddError(fmt.Sprintf("Error %s JWT provider", verb), err.Error())
	}
}

// applyJWTProviderToState maps an API JWTProvider response into the
// Terraform state model.
func applyJWTProviderToState(provider *api.JWTProvider, state *models.ServiceJWTProviderResourceModel) {
	state.Name = types.StringValue(provider.Name)
	state.Issuer = types.StringValue(provider.Issuer)
	state.Audience = types.StringValue(provider.Audience)
	state.JWKSURL = types.StringValue(provider.JWKSURL)
	state.RolesClaim = types.StringPointerValue(provider.RolesClaim)
}

// jwtProviderNameNotReservedValidator rejects the names the server reserves.
// It is not a regex because RE2 has no negative lookahead.
type jwtProviderNameNotReservedValidator struct{}

func (v jwtProviderNameNotReservedValidator) Description(_ context.Context) string {
	return fmt.Sprintf("must not be one of %s, or start with one of them followed by a hyphen", strings.Join(jwtProviderReservedNames, ", "))
}

func (v jwtProviderNameNotReservedValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v jwtProviderNameNotReservedValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	name := req.ConfigValue.ValueString()
	for _, reserved := range jwtProviderReservedNames {
		if name == reserved || strings.HasPrefix(name, reserved+"-") {
			resp.Diagnostics.AddAttributeError(req.Path, "Reserved JWT provider name",
				fmt.Sprintf("Attribute %s %s, got: %q", req.Path, v.Description(ctx), name))
			return
		}
	}
}
