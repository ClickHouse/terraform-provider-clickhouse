package resource

import (
	"context"
	_ "embed"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

//go:embed descriptions/apikey.md
var apiKeyResourceDescription string

var (
	_ resource.Resource              = &ApiKeyResource{}
	_ resource.ResourceWithConfigure = &ApiKeyResource{}
)

const expirationDateFmt = "2006-01-02 15:04"

func NewApiKeyResource() resource.Resource {
	return &ApiKeyResource{}
}

type ApiKeyResource struct {
	client api.Client
}

func (r *ApiKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *ApiKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the api key",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "If true the API key is active, otherwise it's not usable. Defaults to true.",
			},
			"expiration_date": schema.StringAttribute{
				Optional:    true,
				Description: "Expiration date for the API key in YYYY-MM-DD HH:mm format. Leave out this attribute or set to null to create a key with no expiration date.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile("^20[0-9]{2}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01]) ([01][0-9]|2[0-3]):([0-5][0-9])$"), "Must be in format YYYY-MM-DD HH:mm"),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the api key",
			},
			"roles": schema.ListAttribute{
				ElementType: types.StringType,
				Required:    true,
				Description: "Roles to assign to the API key",
			},
		},
		MarkdownDescription: apiKeyResourceDescription,
	}
}

func (r *ApiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(api.Client)
}

func (r *ApiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.ApiKey
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	roles := make([]string, 0)
	diags = plan.Roles.ElementsAs(ctx, &roles, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	keyState := api.ApiKeyStateDisabled
	if plan.Enabled.ValueBool() {
		keyState = api.ApiKeyStateEnabled
	}

	key := api.ApiKey{
		Name:  plan.Name.ValueString(),
		Roles: roles,
		State: keyState,
	}

	if !plan.ExpirationDate.IsNull() && !plan.ExpirationDate.IsUnknown() {
		parsed, err := time.Parse(expirationDateFmt, plan.ExpirationDate.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Parsing ExpirationDate",
				"Could not parse expiration date, unexpected error: "+err.Error(),
			)
			return
		}

		key.ExpirationDate = &parsed
	}

	createdKey, _, _, err := r.client.CreateApiKey(ctx, key)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating ClickHouse Api Key",
			"Could not create api key, unexpected error: "+err.Error(),
		)
		return
	}
	state := models.ApiKey{}

	diags = r.syncState(ctx, createdKey, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

func (r *ApiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ApiKey
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := r.client.GetApiKey(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading ClickHouse Api Key",
			"Could not read api key, unexpected error: "+err.Error(),
		)
		return
	}

	if key != nil {
		diags = r.syncState(ctx, key, &state)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}

		diags = resp.State.Set(ctx, &state)
		resp.Diagnostics.Append(diags...)
	} else {
		resp.State.RemoveResource(ctx)
	}
}

func (r *ApiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state models.ApiKey
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var plan models.ApiKey
	diags = req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var expiration *time.Time
	if !plan.ExpirationDate.IsNull() {
		parsed, err := time.Parse(expirationDateFmt, plan.ExpirationDate.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Parsing ExpirationDate",
				"Could not parse expiration date, unexpected error: "+err.Error(),
			)
			return
		}

		expiration = &parsed
	}

	keyState := api.ApiKeyStateDisabled
	if plan.Enabled.ValueBool() {
		keyState = api.ApiKeyStateEnabled
	}

	roles := make([]string, 0)
	diags = plan.Roles.ElementsAs(ctx, &roles, false)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	update := api.ApiKeyUpdate{
		Name:           plan.Name.ValueString(),
		ExpirationDate: expiration,
		State:          keyState,
		Roles:          roles,
	}

	key, err := r.client.UpdateApiKey(ctx, state.ID.ValueString(), update)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating ClickHouse Api Key",
			"Could not update api key, unexpected error: "+err.Error(),
		)
		return
	}

	diags = r.syncState(ctx, key, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

func (r *ApiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ApiKey
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteApiKey(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting ClickHouse Api key",
			"Could not delete api key, unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *ApiKeyResource) syncState(ctx context.Context, key *api.ApiKey, state *models.ApiKey) diag.Diagnostics {
	state.ID = types.StringValue(key.ID)
	state.Name = types.StringValue(key.Name)
	state.Enabled = types.BoolValue(key.State == api.ApiKeyStateEnabled)

	// Roles
	{
		lv, diags := types.ListValueFrom(ctx, types.StringType, key.Roles)
		if diags.HasError() {
			return diags
		}
		state.Roles = lv
	}

	if key.ExpirationDate != nil {
		state.ExpirationDate = types.StringValue(key.ExpirationDate.Format(expirationDateFmt))
	} else {
		state.ExpirationDate = types.StringNull()
	}

	return nil
}
