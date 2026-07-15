package resource

import (
	"context"
	_ "embed"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &ApiKeyResource{}
	_ resource.ResourceWithConfigure   = &ApiKeyResource{}
	_ resource.ResourceWithImportState = &ApiKeyResource{}
)

//go:embed descriptions/api_key.md
var apiKeyResourceDescription string

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
		MarkdownDescription: apiKeyResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier for the API key (same value as key_id).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key_id": schema.StringAttribute{
				Description: "Unique identifier for the API key.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the API key.",
				Required:    true,
			},
			"state": schema.StringAttribute{
				Description: "State of the key: 'enabled' or 'disabled'.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(api.ApiKeyStateValues...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"expire_at": schema.StringAttribute{
				Description: "Timestamp (RFC3339) when the key expires. Omit for a key that never expires.",
				Optional:    true,
			},
			"key_suffix": schema.StringAttribute{
				Description: "Last characters of the key, for identification.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key_secret": schema.StringAttribute{
				Description: "The generated key secret. Returned only at creation and stored in state. Empty for imported keys.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ip_access": schema.ListNestedAttribute{
				Description: "List of IP addresses/CIDRs allowed to use this key.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"source": schema.StringAttribute{
							Description: "IP address or CIDR.",
							Required:    true,
						},
						"description": schema.StringAttribute{
							Description: "Optional description of the entry.",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

func (r *ApiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			"Expected api.Client, got something else. Please report this issue to the provider developers.",
		)
		return
	}

	r.client = client
}

func (r *ApiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.ApiKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createReq, d := planToCreateRequest(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.client.CreateApiKey(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError("Error creating API key", "Could not create API key: "+err.Error())
		return
	}

	// Capture the one-time secret and the server key id before mapping the rest.
	plan.KeySecret = types.StringValue(result.KeySecret)
	key := result.Key
	if key.ID == "" {
		key.ID = result.KeyID
	}
	resp.Diagnostics.Append(applyApiKeyToState(&key, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ApiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ApiKeyResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	syncDiags, err := r.syncApiKeyState(ctx, &state)
	resp.Diagnostics.Append(syncDiags...)
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading API key",
			"Could not read API key: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// syncApiKeyState fetches the key and maps it into state. KeySecret is preserved
// by applyApiKeyToState because the GET endpoint never returns it.
func (r *ApiKeyResource) syncApiKeyState(ctx context.Context, state *models.ApiKeyResourceModel) (diag.Diagnostics, error) {
	key, err := r.client.GetApiKey(ctx, state.ID.ValueString())
	if err != nil {
		return nil, err
	}
	return applyApiKeyToState(key, state), nil
}

// applyApiKeyToState maps an API ApiKey into the Terraform state model.
// It deliberately does NOT set KeySecret: the secret is only available at
// create time and must be preserved from prior state on every read.
func applyApiKeyToState(key *api.ApiKey, state *models.ApiKeyResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	state.ID = types.StringValue(key.ID)
	state.KeyID = types.StringValue(key.ID)
	state.Name = types.StringValue(key.Name)
	state.State = types.StringValue(key.State)
	state.KeySuffix = types.StringValue(key.KeySuffix)

	if key.ExpireAt == "" {
		state.ExpireAt = types.StringNull()
	} else {
		state.ExpireAt = types.StringValue(key.ExpireAt)
	}

	ipList, d := ipAccessListToState(key.IpAccessList, state.IpAccessList)
	diags.Append(d...)
	state.IpAccessList = ipList

	return diags
}

// ipAccessListToState maps the API entries into a types.List, preserving the
// null-vs-empty distinction from prior state (mirrors role policies mapping).
func ipAccessListToState(entries []api.IpAccessListEntry, prior types.List) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	objType := models.IPAccessList{}.ObjectType()

	if len(entries) == 0 {
		if prior.IsNull() || prior.IsUnknown() {
			return types.ListNull(objType), diags
		}
		empty, d := types.ListValue(objType, []attr.Value{})
		diags.Append(d...)
		return empty, diags
	}

	values := make([]attr.Value, len(entries))
	for i, e := range entries {
		desc := types.StringNull()
		if e.Description != "" {
			desc = types.StringValue(e.Description)
		}
		values[i] = models.IPAccessList{
			Source:      types.StringValue(e.Source),
			Description: desc,
		}.ObjectValue()
	}
	list, d := types.ListValue(objType, values)
	diags.Append(d...)
	return list, diags
}

func (r *ApiKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.ApiKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state models.ApiKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateReq, d := planToUpdateRequest(ctx, plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	key, err := r.client.UpdateApiKey(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError("Error updating API key", "Could not update API key: "+err.Error())
		return
	}

	// Secret is immutable and not returned by update; carry it forward.
	plan.KeySecret = state.KeySecret
	resp.Diagnostics.Append(applyApiKeyToState(key, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ApiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ApiKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteApiKey(ctx, state.ID.ValueString())
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting API key", "Could not delete API key: "+err.Error())
	}
}

func planToCreateRequest(ctx context.Context, plan models.ApiKeyResourceModel) (api.ApiKeyCreateRequest, diag.Diagnostics) {
	ipList, diags := ipAccessListFromPlan(ctx, plan.IpAccessList)
	req := api.ApiKeyCreateRequest{
		Name:         plan.Name.ValueString(),
		IpAccessList: ipList,
	}
	if !plan.State.IsNull() && !plan.State.IsUnknown() {
		req.State = plan.State.ValueString()
	}
	if !plan.ExpireAt.IsNull() && !plan.ExpireAt.IsUnknown() {
		v := plan.ExpireAt.ValueString()
		req.ExpireAt = &v
	}
	return req, diags
}

func planToUpdateRequest(ctx context.Context, plan models.ApiKeyResourceModel) (api.ApiKeyUpdateRequest, diag.Diagnostics) {
	ipList, diags := ipAccessListFromPlan(ctx, plan.IpAccessList)
	req := api.ApiKeyUpdateRequest{
		Name:         plan.Name.ValueString(),
		IpAccessList: ipList,
	}
	if !plan.State.IsNull() && !plan.State.IsUnknown() {
		req.State = plan.State.ValueString()
	}
	if !plan.ExpireAt.IsNull() && !plan.ExpireAt.IsUnknown() {
		v := plan.ExpireAt.ValueString()
		req.ExpireAt = &v
	}
	return req, diags
}

// ipAccessListFromPlan converts the plan list into the API slice.
// A null/unknown list returns a non-nil empty slice so the request always
// sends "ipAccessList":[], letting the server clear prior entries (mirrors
// role's planPoliciesToAPICreate).
func ipAccessListFromPlan(ctx context.Context, list types.List) (*[]api.IpAccessListEntry, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return &[]api.IpAccessListEntry{}, diags
	}

	var entryModels []models.IPAccessList
	diags.Append(list.ElementsAs(ctx, &entryModels, false)...)
	if diags.HasError() {
		return nil, diags
	}

	entries := make([]api.IpAccessListEntry, len(entryModels))
	for i, m := range entryModels {
		entries[i] = api.IpAccessListEntry{
			Source:      m.Source.ValueString(),
			Description: m.Description.ValueString(),
		}
	}
	return &entries, diags
}

func (r *ApiKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
