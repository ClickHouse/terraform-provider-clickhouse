package resource

import (
	"context"
	_ "embed"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &OrganizationResource{}
	_ resource.ResourceWithConfigure   = &OrganizationResource{}
	_ resource.ResourceWithImportState = &OrganizationResource{}
	_ resource.ResourceWithModifyPlan  = &OrganizationResource{}
)

//go:embed descriptions/organization.md
var organizationResourceDescription string

// NewOrganizationResource is a helper function to simplify the provider implementation.
func NewOrganizationResource() resource.Resource {
	return &OrganizationResource{}
}

// OrganizationResource is the resource implementation.
type OrganizationResource struct {
	client api.Client
}

// Metadata returns the resource type name.
func (r *OrganizationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

// Schema defines the schema for the resource.
func (r *OrganizationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: organizationResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the organization. This is set automatically from the provider configuration.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"core_dumps_enabled": schema.BoolAttribute{
				Description: "Whether core dumps collection is enabled for services in the organization. Defaults to the organization's current setting if not specified.",
				Optional:    true,
				Computed:    true,
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *OrganizationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan adds warnings during the plan phase.
func (r *OrganizationResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Only show warnings when creating or destroying
	if req.State.Raw.IsNull() {
		// Enforce singleton during plan phase to give early feedback
		if clientImpl, ok := r.client.(*api.ClientImpl); ok {
			if err := clientImpl.RegisterOrganizationResource(); err != nil {
				resp.Diagnostics.AddError(
					"Multiple Organization Resources",
					err.Error(),
				)
				return
			}
		}

		resp.Diagnostics.AddWarning(
			"Managing Existing Organization",
			"This resource manages settings for the organization configured in the provider. It does not create a new organization.",
		)
	} else if req.Plan.Raw.IsNull() {
		resp.Diagnostics.AddWarning(
			"Organization Not Deleted",
			"Removing this resource from your configuration only stops managing the organization's settings. The organization itself still exists with its current settings.",
		)
	}
}

// Create creates the resource and sets the initial Terraform state.
func (r *OrganizationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan models.OrganizationResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update organization with the desired configuration
	orgUpdate := api.OrganizationUpdate{}
	hasChanges := false

	// Only set EnableCoreDumps if explicitly configured
	if !plan.CoreDumpsEnabled.IsNull() && !plan.CoreDumpsEnabled.IsUnknown() {
		enabled := plan.CoreDumpsEnabled.ValueBool()
		orgUpdate.EnableCoreDumps = &enabled
		hasChanges = true
	}

	var result *api.OrgResult
	var err error

	if hasChanges {
		// Update organization if there are changes
		result, err = r.client.UpdateOrganization(ctx, orgUpdate)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error updating organization",
				"Could not update organization: "+err.Error(),
			)
			return
		}
	} else {
		// No changes specified, just read current state
		result, err = r.client.GetOrganization(ctx)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error reading organization",
				"Could not read organization: "+err.Error(),
			)
			return
		}
	}

	// Map response to state
	plan.ID = types.StringValue(result.ID)

	if result.EnableCoreDumps != nil {
		plan.CoreDumpsEnabled = types.BoolValue(*result.EnableCoreDumps)
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *OrganizationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state models.OrganizationResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get refreshed organization details from ClickHouse Cloud
	result, err := r.client.GetOrganization(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading organization",
			"Could not read organization: "+err.Error(),
		)
		return
	}

	// Overwrite items with refreshed state
	state.ID = types.StringValue(result.ID)

	if result.EnableCoreDumps != nil {
		state.CoreDumpsEnabled = types.BoolValue(*result.EnableCoreDumps)
	}

	// Set refreshed state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *OrganizationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan models.OrganizationResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update organization with the desired configuration
	orgUpdate := api.OrganizationUpdate{}

	// Only set EnableCoreDumps if explicitly configured
	if !plan.CoreDumpsEnabled.IsNull() && !plan.CoreDumpsEnabled.IsUnknown() {
		enabled := plan.CoreDumpsEnabled.ValueBool()
		orgUpdate.EnableCoreDumps = &enabled
	}

	result, err := r.client.UpdateOrganization(ctx, orgUpdate)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error updating organization",
			"Could not update organization: "+err.Error(),
		)
		return
	}

	// Map response to state
	plan.ID = types.StringValue(result.ID)

	if result.EnableCoreDumps != nil {
		plan.CoreDumpsEnabled = types.BoolValue(*result.EnableCoreDumps)
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *OrganizationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Organization settings can't really be "deleted", so we'll just remove from state
	// The actual organization continues to exist with its current settings
}

// ImportState imports the resource state.
func (r *OrganizationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The organization ID is automatically determined from the provider configuration
	// So we just need to read the current state
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
