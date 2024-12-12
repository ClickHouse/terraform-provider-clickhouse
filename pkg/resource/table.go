package resource

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/boolvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/queryApi"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/tableBuilder"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &TableResource{}
	_ resource.ResourceWithConfigure   = &TableResource{}
	_ resource.ResourceWithImportState = &TableResource{}
)

// NewTableResource is a helper function to simplify the provider implementation.
func NewTableResource() resource.Resource {
	return &TableResource{}
}

// TableResource is the resource implementation.
type TableResource struct{}

// Metadata returns the resource type name.
func (r *TableResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_table"
}

// Schema defines the schema for the resource.
func (r *TableResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"query_api_endpoint": schema.StringAttribute{
				Description: "The URL for the query API endpoint",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the table",
				Validators:  nil,
			},
			"engine": schema.SingleNestedAttribute{
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Table engine to use",
						Validators:  nil,
						Default:     stringdefault.StaticString("MergeTree"),
					},
					"params": schema.ListAttribute{
						ElementType: types.StringType,
						Optional:    true,
					},
				},
			},
			"columns": schema.MapNestedAttribute{
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required: true,
						},
						"nullable": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
						},
						"default": schema.StringAttribute{
							Optional: true,
							Validators: []validator.String{
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("materialized")),
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("ephemeral")),
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("alias")),
							},
						},
						"materialized": schema.StringAttribute{
							Optional: true,
							Validators: []validator.String{
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("default")),
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("ephemeral")),
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("alias")),
							},
						},
						"ephemeral": schema.BoolAttribute{
							Optional: true,
							Computed: true,
							Default:  booldefault.StaticBool(false),
							Validators: []validator.Bool{
								boolvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("default")),
								boolvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("materialized")),
								boolvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("comment")),
								boolvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("alias")),
							},
						},
						"alias": schema.StringAttribute{
							Optional: true,
							Validators: []validator.String{
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("default")),
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("materialized")),
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("comment")),
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("ephemeral")),
							},
						},
						"comment": schema.StringAttribute{
							Optional: true,
						},
					},
				},
				Required: true,
			},
			"order_by": schema.StringAttribute{
				Required:    true,
				Description: "Primary key",
				Validators:  nil,
			},
			"settings": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Description: "Table comment",
				Validators:  nil,
			},
		},
		MarkdownDescription: `CHANGEME`,
	}
}

// Configure adds the provider configured client to the resource.
func (r *TableResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
}

//func (r *TableResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
//
//}

// Create a new table
func (r *TableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan models.TableResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	queryApiClient, err := queryApi.New(plan.QueryAPIEndpoint.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating table",
			"Could not create table, unexpected error: "+err.Error(),
		)
		return
	}

	builder, err := tableBuilder.New(queryApiClient)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating table",
			"Could not create table, unexpected error: "+err.Error(),
		)
		return
	}

	table, diagnostics := tableFromPlan(ctx, plan)
	if diagnostics.HasError() {
		resp.Diagnostics.Append(diagnostics.Errors()...)
		return
	}

	err = builder.CreateTable(ctx, *table)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating table",
			"Could not create table, unexpected error: "+err.Error(),
		)
		return
	}

	state, diagnostics := r.syncTableState(ctx, builder, plan.Name.ValueString())
	if diagnostics.HasError() {
		resp.Diagnostics.Append(diagnostics...)
		return
	}

	state.QueryAPIEndpoint = plan.QueryAPIEndpoint

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *TableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var plan models.TableResourceModel
	diags := req.State.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	queryApiClient, err := queryApi.New(plan.QueryAPIEndpoint.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating table",
			"Could not create table, unexpected error: "+err.Error(),
		)
		return
	}

	builder, err := tableBuilder.New(queryApiClient)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating table",
			"Could not create table, unexpected error: "+err.Error(),
		)
		return
	}

	state, diagnostics := r.syncTableState(ctx, builder, plan.Name.ValueString())
	if diagnostics.HasError() {
		resp.Diagnostics.Append(diagnostics...)
		return
	}

	state.QueryAPIEndpoint = plan.QueryAPIEndpoint

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *TableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *TableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *TableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

// syncTableState reads table structure and settings from clickhouse and returns a TableResourceModel to be stored as terraform state.
func (r *TableResource) syncTableState(ctx context.Context, builder tableBuilder.Builder, tableName string) (*models.TableResourceModel, diag.Diagnostics) {
	// Read table spec and settings from clickhouse.
	table, err := builder.GetTable(ctx, tableName)
	if err != nil {
		return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Error reading table state", err.Error())}
	}

	state := &models.TableResourceModel{
		Name:    types.StringValue(table.Name),
		OrderBy: types.StringValue(table.OrderBy),
		Comment: types.StringValue(table.Comment),
	}

	// Engine
	{
		params, diagnostics := types.ListValueFrom(ctx, types.StringType, table.Engine.Params)
		if diagnostics.HasError() {
			return nil, diagnostics
		}

		engineModel := models.Engine{
			Name:   types.StringValue(table.Engine.Name),
			Params: params,
		}

		engine, diagnostics := types.ObjectValueFrom(ctx, engineModel.AttributeTypes(), engineModel)
		if diagnostics.HasError() {
			return nil, diagnostics
		}

		state.Engine = engine
	}

	// Settings
	{
		settings, diagnostics := types.MapValueFrom(ctx, types.StringType, table.Settings)
		if diagnostics.HasError() {
			return nil, diagnostics
		}
		state.Settings = settings
	}

	// Columns
	{
		modelColumns := make(map[string]models.Column)

		for _, column := range table.Columns {
			modelColumn := models.Column{
				Type:      types.StringValue(column.Type),
				Nullable:  types.BoolValue(column.Nullable),
				Ephemeral: types.BoolValue(column.Ephemeral),
			}

			if column.Default != nil {
				modelColumn.Default = types.StringValue(*column.Default)
			}

			if column.Materialized != nil {
				modelColumn.Materialized = types.StringValue(*column.Materialized)
			}

			if column.Alias != nil {
				modelColumn.Alias = types.StringValue(*column.Alias)
			}

			if column.Comment != nil {
				modelColumn.Comment = types.StringValue(*column.Comment)
			}

			modelColumns[column.Name] = modelColumn
		}

		columns, diagnostics := types.MapValueFrom(ctx, models.Column{}.ObjectType(), modelColumns)
		if diagnostics.HasError() {
			return nil, diagnostics
		}

		state.Columns = columns
	}
	return state, nil
}

// tableFromPlan takes a terraform plan (TableResourceModel) and creates a Table struct to be used by tableBuilder
func tableFromPlan(ctx context.Context, plan models.TableResourceModel) (*tableBuilder.Table, diag.Diagnostics) {
	// get the set of columns from the .tf file and convert to a list of tableBuilder.Column
	tfColumns := make(map[string]models.Column)
	diagnostics := plan.Columns.ElementsAs(ctx, &tfColumns, false)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	chColumns := make([]tableBuilder.Column, 0, len(tfColumns))
	for name, tfColumn := range tfColumns {
		col := tableBuilder.Column{
			Name:         name,
			Type:         tfColumn.Type.ValueString(),
			Nullable:     tfColumn.Nullable.ValueBool(),
			Default:      tfColumn.Default.ValueStringPointer(),
			Materialized: tfColumn.Materialized.ValueStringPointer(),
			Ephemeral:    tfColumn.Ephemeral.ValueBool(),
			Alias:        tfColumn.Alias.ValueStringPointer(),
			Comment:      tfColumn.Comment.ValueStringPointer(),
		}

		chColumns = append(chColumns, col)
	}

	table := &tableBuilder.Table{
		Name:    plan.Name.ValueString(),
		Columns: chColumns,
		OrderBy: plan.OrderBy.ValueString(),
		Comment: plan.Comment.ValueString(),
	}

	if !plan.Engine.IsNull() {
		engine := models.Engine{}
		diagnostics := plan.Engine.As(ctx, &engine, basetypes.ObjectAsOptions{
			UnhandledNullAsEmpty:    false,
			UnhandledUnknownAsEmpty: false,
		})
		if diagnostics.HasError() {
			return nil, diagnostics
		}

		params := make([]string, len(engine.Params.Elements()))
		diagnostics = engine.Params.ElementsAs(ctx, &params, false)
		if diagnostics.HasError() {
			return nil, diagnostics
		}

		table.Engine = tableBuilder.Engine{
			Name:   engine.Name.ValueString(),
			Params: params,
		}
	}

	if !plan.Settings.IsNull() && !plan.Settings.IsUnknown() {
		settings := make(map[string]types.String, len(plan.Settings.Elements()))
		diagnostics := plan.Settings.ElementsAs(ctx, &settings, false)
		if diagnostics.HasError() {
			return nil, diagnostics
		}

		table.Settings = make(map[string]string)
		for name, value := range settings {
			table.Settings[name] = value.ValueString()
		}
	}

	return table, nil
}
