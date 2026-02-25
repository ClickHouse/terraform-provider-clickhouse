//go:build alpha

package resource

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &RoleAssignmentResource{}
	_ resource.ResourceWithConfigure   = &RoleAssignmentResource{}
	_ resource.ResourceWithImportState = &RoleAssignmentResource{}
)

//go:embed descriptions/role_assignment.md
var roleAssignmentResourceDescription string

func NewRoleAssignmentResource() resource.Resource {
	return &RoleAssignmentResource{}
}

type RoleAssignmentResource struct {
	client api.Client
}

type RoleAssignmentModel struct {
	ID        types.String `tfsdk:"id"`
	RoleID    types.String `tfsdk:"role_id"`
	UserIDs   types.Set    `tfsdk:"user_ids"`
	APIKeyIDs types.Set    `tfsdk:"api_key_ids"`
}

func (r *RoleAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role_assignment"
}

func (r *RoleAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: roleAssignmentResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Same as role_id.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"role_id": schema.StringAttribute{
				Description: "ID of the role to assign actors to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_ids": schema.SetAttribute{
				Description: "Set of user IDs to assign to the role.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"api_key_ids": schema.SetAttribute{
				Description: "Set of API key IDs to assign to the role.",
				Optional:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (r *RoleAssignmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RoleAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RoleAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	actors, d := buildActorsList(ctx, plan.UserIDs, plan.APIKeyIDs)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleID := plan.RoleID.ValueString()
	_, err := r.client.UpdateRole(ctx, roleID, api.RoleUpdateRequest{
		Actors: &actors,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error assigning actors to role", "Could not update role actors: "+err.Error())
		return
	}

	plan.ID = types.StringValue(roleID)
	syncDiags, err := r.syncAssignmentState(ctx, &plan)
	resp.Diagnostics.Append(syncDiags...)
	if err != nil {
		resp.Diagnostics.AddError("Error reading role after create", "Could not read role actors: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *RoleAssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	syncDiags, err := r.syncAssignmentState(ctx, &state)
	resp.Diagnostics.Append(syncDiags...)
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading role", "Could not read role: "+err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// syncAssignmentState fetches the role from the API and updates the assignment
// state with the current actor sets. Returns diagnostics (which may include
// warnings for unrecognised actor types) and a separate error for API failures;
// callers should check api.IsNotFound on the error.
func (r *RoleAssignmentResource) syncAssignmentState(ctx context.Context, state *RoleAssignmentModel) (diag.Diagnostics, error) {
	var diags diag.Diagnostics

	role, err := r.client.GetRole(ctx, state.RoleID.ValueString())
	if err != nil {
		return diags, err
	}

	userIDs, apiKeyIDs, d := parseActors(role.Actors)
	diags.Append(d...)

	usersSet, d := toStringSet(userIDs, state.UserIDs.IsNull())
	diags.Append(d...)

	apiKeysSet, d := toStringSet(apiKeyIDs, state.APIKeyIDs.IsNull())
	diags.Append(d...)

	if diags.HasError() {
		return diags, nil
	}

	state.UserIDs = usersSet
	state.APIKeyIDs = apiKeysSet
	return diags, nil
}

func (r *RoleAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan RoleAssignmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state RoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	actors, d := buildActorsList(ctx, plan.UserIDs, plan.APIKeyIDs)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.UpdateRole(ctx, state.RoleID.ValueString(), api.RoleUpdateRequest{
		Actors: &actors,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating role actors", "Could not update role actors: "+err.Error())
		return
	}

	syncDiags, err := r.syncAssignmentState(ctx, &plan)
	resp.Diagnostics.Append(syncDiags...)
	if err != nil {
		resp.Diagnostics.AddError("Error reading role after update", "Could not read role actors: "+err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *RoleAssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RoleAssignmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	emptyActors := []string{}
	_, err := r.client.UpdateRole(ctx, state.RoleID.ValueString(), api.RoleUpdateRequest{
		Actors: &emptyActors,
	})
	if err != nil {
		if api.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Error removing actors from role", "Could not update role actors: "+err.Error())
	}
}

func (r *RoleAssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID is just the role_id. Read will populate user_ids and api_key_ids from the API.
	emptySet, diags := types.SetValue(types.StringType, []attr.Value{})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, RoleAssignmentModel{
		ID:        types.StringValue(req.ID),
		RoleID:    types.StringValue(req.ID),
		UserIDs:   emptySet,
		APIKeyIDs: emptySet,
	})...)
}

// buildActorsList converts user_ids and api_key_ids sets into the API actors format ("type/id").
func buildActorsList(ctx context.Context, userIDs types.Set, apiKeyIDs types.Set) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	var users, apiKeys []string

	if !userIDs.IsNull() && !userIDs.IsUnknown() {
		diags.Append(userIDs.ElementsAs(ctx, &users, false)...)
		if diags.HasError() {
			return nil, diags
		}
	}

	if !apiKeyIDs.IsNull() && !apiKeyIDs.IsUnknown() {
		diags.Append(apiKeyIDs.ElementsAs(ctx, &apiKeys, false)...)
		if diags.HasError() {
			return nil, diags
		}
	}

	actors := make([]string, 0, len(users)+len(apiKeys))
	for _, id := range users {
		actors = append(actors, api.ActorTypeUser+"/"+id)
	}
	for _, id := range apiKeys {
		actors = append(actors, api.ActorTypeAPIKey+"/"+id)
	}
	return actors, diags
}

// parseActors splits the API actors list (each in "type/id" format) into user IDs
// and API key IDs. A warning diagnostic is emitted for any actor string that does
// not conform to the expected format or carries an unrecognised type, so failures
// are surfaced rather than silently dropped.
func parseActors(actors []string) (userIDs []string, apiKeyIDs []string, diags diag.Diagnostics) {
	userIDs = []string{}
	apiKeyIDs = []string{}
	for _, a := range actors {
		actorType, id, ok := strings.Cut(a, "/")
		if !ok {
			diags.AddWarning(
				"Unrecognised actor format",
				fmt.Sprintf("Actor %q does not match the expected \"type/id\" format and will be ignored.", a),
			)
			continue
		}
		switch actorType {
		case api.ActorTypeUser:
			userIDs = append(userIDs, id)
		case api.ActorTypeAPIKey:
			apiKeyIDs = append(apiKeyIDs, id)
		default:
			diags.AddWarning(
				"Unknown actor type",
				fmt.Sprintf("Actor %q has an unrecognised type %q and will be ignored.", a, actorType),
			)
		}
	}
	return
}

// toStringSet builds a types.Set from a slice of strings.
// If the slice is empty and keepNull is true, it returns a null set to preserve
// the distinction between "not configured" (null) and "configured but empty" (set([])).
func toStringSet(ids []string, keepNull bool) (types.Set, diag.Diagnostics) {
	if len(ids) == 0 && keepNull {
		return types.SetNull(types.StringType), nil
	}
	values := make([]attr.Value, len(ids))
	for i, id := range ids {
		values[i] = types.StringValue(id)
	}
	return types.SetValue(types.StringType, values)
}
