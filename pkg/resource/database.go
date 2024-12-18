package resource

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &DatabaseResource{}
	_ resource.ResourceWithConfigure   = &DatabaseResource{}
	_ resource.ResourceWithImportState = &DatabaseResource{}
)

// NewDatabaseResource is a helper function to simplify the provider implementation.
func NewDatabaseResource() resource.Resource {
	return &DatabaseResource{}
}

// DatabaseResource is the resource implementation.
type DatabaseResource struct {
	client api.Client
}

// Metadata returns the resource type name.
func (r *DatabaseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

// Schema defines the schema for the resource.
func (r *DatabaseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Service ID",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the database",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Description: "Comment associated with the database",
				Validators: []validator.String{
					// If user specifies the comment field, it can't be the empty string otherwise we get an error from terraform
					// due to the difference between null and empty string. User can always set this field to null or leave it out completely.
					stringvalidator.LengthAtLeast(1),
					stringvalidator.LengthAtMost(255),
				},
				PlanModifiers: []planmodifier.String{
					// Changing comment is not implemented: https://github.com/ClickHouse/ClickHouse/issues/73351
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
		MarkdownDescription: `Use the *clickhouse_database* resource to create a database in a ClickHouse cloud *service*.`,
	}
}

func (r *DatabaseResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(api.Client)
}

func (r *DatabaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.DatabaseResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	db, diagnostics := databaseFromPlan(ctx, plan)
	if diagnostics.HasError() {
		resp.Diagnostics.Append(diagnostics...)
		return
	}

	err := r.client.CreateDatabase(ctx, plan.ServiceID.ValueString(), *db)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating database",
			"Could not create database, unexpected error: "+err.Error(),
		)
		return
	}

	state, diagnostics := r.syncDatabaseState(ctx, plan.ServiceID.ValueString(), plan.Name.ValueString())
	if diagnostics.HasError() {
		resp.Diagnostics.Append(diagnostics...)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *DatabaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var plan models.DatabaseResourceModel
	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state, diagnostics := r.syncDatabaseState(ctx, plan.ServiceID.ValueString(), plan.Name.ValueString())
	if diagnostics.HasError() {
		resp.Diagnostics.Append(diagnostics...)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *DatabaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.DatabaseResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	table, diagnostics := databaseFromPlan(ctx, plan)
	if diagnostics.HasError() {
		resp.Diagnostics.Append(diagnostics.Errors()...)
		return
	}

	err := r.client.SyncDatabase(ctx, plan.ServiceID.ValueString(), *table)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error syncing table",
			"Could not sync table, unexpected error: "+err.Error(),
		)
		return
	}

	state, diagnostics := r.syncDatabaseState(ctx, plan.ServiceID.ValueString(), plan.Name.ValueString())
	if diagnostics.HasError() {
		resp.Diagnostics.Append(diagnostics...)
		return
	}

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *DatabaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var plan models.DatabaseResourceModel
	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteDatabase(ctx, plan.ServiceID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting database",
			"Could not delete database, unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *DatabaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// syncDatabaseState reads database settings from clickhouse and returns a DatabaseResourceModel
func (r *DatabaseResource) syncDatabaseState(ctx context.Context, serviceID string, dbName string) (*models.DatabaseResourceModel, diag.Diagnostics) {
	// Read table spec and settings from clickhouse.
	db, err := r.client.GetDatabase(ctx, serviceID, dbName)
	if err != nil {
		return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Error reading database state", err.Error())}
	}

	comment := types.StringNull()
	if db.Comment != "" {
		comment = types.StringValue(db.Comment)
	}

	state := &models.DatabaseResourceModel{
		ServiceID: types.StringValue(serviceID),
		Name:      types.StringValue(db.Name),
		Comment:   comment,
	}

	return state, nil
}

// databaseFromPlan takes a terraform plan (DatabaseResourceModel) and creates a Database struct
func databaseFromPlan(ctx context.Context, plan models.DatabaseResourceModel) (*api.Database, diag.Diagnostics) {
	db := &api.Database{
		Name:    plan.Name.ValueString(),
		Comment: plan.Comment.ValueString(),
	}

	return db, nil
}
