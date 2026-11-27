package resource

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

const (
	byocEnrollmentTokenMinTTLSeconds = 60 * 60
	byocEnrollmentTokenMaxTTLSeconds = 24 * 60 * 60
)

// The API answers 409 until the cluster trust of a freshly registered
// infrastructure is admitted, which takes a few minutes. Variables so tests
// can shorten them.
var (
	byocEnrollmentTokenConflictWait     = 15 * time.Minute
	byocEnrollmentTokenConflictInterval = 15 * time.Second
)

var (
	_ resource.Resource                = (*ByocEnrollmentTokenResource)(nil)
	_ resource.ResourceWithConfigure   = (*ByocEnrollmentTokenResource)(nil)
	_ resource.ResourceWithImportState = (*ByocEnrollmentTokenResource)(nil)
)

//go:embed descriptions/byoc_enrollment_token.md
var byocEnrollmentTokenResourceDescription string

func NewByocEnrollmentTokenResource() resource.Resource {
	return &ByocEnrollmentTokenResource{}
}

type ByocEnrollmentTokenResource struct {
	client api.Client
}

type byocEnrollmentTokenModel struct {
	ID         types.String `tfsdk:"id"`
	ByocID     types.String `tfsdk:"byoc_id"`
	TTLSeconds types.Int64  `tfsdk:"ttl_seconds"`
	Token      types.String `tfsdk:"token"`
	ExpiresAt  types.String `tfsdk:"expires_at"`
}

func (r *ByocEnrollmentTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_byoc_enrollment_token"
}

func (r *ByocEnrollmentTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: byocEnrollmentTokenResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the BYOC infrastructure the token enrolls into.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"byoc_id": schema.StringAttribute{
				Description: "ID of the restricted-access BYOC infrastructure to mint the token for. Changing it mints a new token.",
				Required:    true,
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ttl_seconds": schema.Int64Attribute{
				Description: fmt.Sprintf("Lifetime of the token in seconds, in whole minutes between %d (one hour) and %d (one day). Defaults to the API default of four hours. Changing it mints a new token.", byocEnrollmentTokenMinTTLSeconds, byocEnrollmentTokenMaxTTLSeconds),
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.Between(byocEnrollmentTokenMinTTLSeconds, byocEnrollmentTokenMaxTTLSeconds),
					byocWholeMinutesValidator{},
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"token": schema.StringAttribute{
				Description: "The enrollment token. The API returns it only once, so it is null after import; replace the resource (for example with 'terraform apply -replace') to mint a new one.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"expires_at": schema.StringAttribute{
				Description: "When the token expires (RFC3339). Terraform does not mint a new token when it expires.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ByocEnrollmentTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ByocEnrollmentTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	utils.BetaWarning("clickhouse_byoc_enrollment_token", &resp.Diagnostics)

	var plan byocEnrollmentTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	byocId := plan.ByocID.ValueString()
	request := api.ByocEnrollmentTokenRequest{}
	if isKnown(plan.TTLSeconds) {
		request.TtlSeconds = plan.TTLSeconds.ValueInt64Pointer()
	}

	token, err := r.mintWhenTrustAdmitted(ctx, byocId, request)
	if err != nil {
		detail := fmt.Sprintf("Could not mint an enrollment token for BYOC infrastructure %s: %s", byocId, err.Error())
		if api.IsConflict(err) {
			detail += fmt.Sprintf("\n\nThe cluster trust of the infrastructure was still not admitted after %s. Check that the infrastructure is restricted-access and that its registration (restricted_facts) matches the cluster, then retry.", byocEnrollmentTokenConflictWait)
		}
		resp.Diagnostics.AddError("Error creating BYOC enrollment token", detail)
		return
	}

	plan.ID = plan.ByocID
	plan.Token = types.StringValue(token.Token)
	plan.ExpiresAt = types.StringValue(token.ExpiresAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ByocEnrollmentTokenResource) mintWhenTrustAdmitted(ctx context.Context, byocId string, request api.ByocEnrollmentTokenRequest) (*api.ByocEnrollmentToken, error) {
	ctx, cancel := context.WithTimeout(ctx, byocEnrollmentTokenConflictWait)
	defer cancel()

	var token *api.ByocEnrollmentToken
	var lastConflict error
	mint := func() error {
		t, err := r.client.CreateByocInfrastructureEnrollmentToken(ctx, byocId, request)
		if err == nil {
			token = t
			return nil
		}
		if api.IsConflict(err) {
			lastConflict = err
			return err
		}
		return backoff.Permanent(err)
	}
	err := backoff.Retry(mint, backoff.WithContext(backoff.NewConstantBackOff(byocEnrollmentTokenConflictInterval), ctx))
	if err != nil && lastConflict != nil && ctx.Err() != nil {
		return nil, lastConflict
	}
	return token, err
}

// Read never calls the API: a token is returned only when minted, and
// minting on refresh would rotate it on every plan.
func (r *ByocEnrollmentTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state byocEnrollmentTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable in practice: every configurable attribute requires
// replacement.
func (r *ByocEnrollmentTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan byocEnrollmentTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete only forgets the token: the API has no revocation, and the token
// expires on its own.
func (r *ByocEnrollmentTokenResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *ByocEnrollmentTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	utils.BetaWarning("clickhouse_byoc_enrollment_token", &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("byoc_id"), req.ID)...)
}

type byocWholeMinutesValidator struct{}

func (v byocWholeMinutesValidator) Description(context.Context) string {
	return "value must be a whole number of minutes"
}

func (v byocWholeMinutesValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v byocWholeMinutesValidator) ValidateInt64(_ context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if !isKnown(req.ConfigValue) {
		return
	}
	if req.ConfigValue.ValueInt64()%60 != 0 {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid ttl_seconds", fmt.Sprintf("ttl_seconds must be a whole number of minutes (a multiple of 60), got %d.", req.ConfigValue.ValueInt64()))
	}
}
