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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
		Blocks: map[string]schema.Block{
			"column": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
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
							Validators: []validator.Bool{
								boolvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("default")),
								boolvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("materialized")),
								boolvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("comment")),
								boolvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("codec")),
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
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("codec")),
							},
						},
						"codec": schema.StringAttribute{
							Optional: true,
						},
						"ttl": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"time_column": schema.StringAttribute{
									Description: "The name of the column to evaluate the interval from.",
									Required:    true,
								},
								"interval": schema.StringAttribute{
									Description: "Interval expression.",
									Required:    true,
								},
							},
						},
						"comment": schema.StringAttribute{
							Optional: true,
						},
					},
				},
			},
		},
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
			"order_by": schema.StringAttribute{
				Required:    true,
				Description: "Primary key",
				Validators:  nil,
			},
			"comment": schema.StringAttribute{
				Required:    true,
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
}

func tableFromPlan(ctx context.Context, plan models.TableResourceModel) (*tableBuilder.Table, diag.Diagnostics) {
	// get the set of columns from the .tf file and convert to a list of tableBuilder.Column
	tfColumns := make([]models.Column, 0, len(plan.Columns.Elements()))
	plan.Columns.ElementsAs(ctx, &tfColumns, false)
	chColumns := make([]tableBuilder.Column, 0, len(tfColumns))
	for _, tfColumn := range tfColumns {
		col := tableBuilder.Column{
			Name:         tfColumn.Name.ValueString(),
			Type:         tfColumn.Type.ValueString(),
			Nullable:     tfColumn.Nullable.ValueBool(),
			Default:      tfColumn.Default.ValueString(),
			Materialized: tfColumn.Materialized.ValueString(),
			Ephemeral:    tfColumn.Ephemeral.ValueBool(),
			Alias:        tfColumn.Alias.ValueString(),
			Codec:        tfColumn.Codec.ValueString(),
			Comment:      tfColumn.Comment.ValueString(),
		}

		if !tfColumn.TTL.IsNull() {
			ttl := models.TTL{}
			diagnostics := tfColumn.TTL.As(ctx, &ttl, basetypes.ObjectAsOptions{
				UnhandledNullAsEmpty:    false,
				UnhandledUnknownAsEmpty: false,
			})
			if diagnostics.HasError() {
				return nil, diagnostics
			}

			col.TTL = &tableBuilder.TTL{
				TimeColumn: ttl.TimeColumn.ValueString(),
				Interval:   ttl.Interval.ValueString(),
			}
		}

		chColumns = append(chColumns, col)
	}

	table := &tableBuilder.Table{
		Name:    plan.Name.ValueString(),
		Columns: chColumns,
		OrderBy: plan.OrderBy.ValueString(),
		Comment: plan.Comment.ValueString(),
	}

	return table, nil
}

// Read refreshes the Terraform state with the latest data.
func (r *TableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *TableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *TableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func (r *TableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
}
