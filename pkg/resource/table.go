package resource

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"

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
						},
						"codec": schema.StringAttribute{
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

	table, err := tableFromPlan(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating table",
			"Could not create table, unexpected error: "+err.Error(),
		)
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

func tableFromPlan(ctx context.Context, plan models.TableResourceModel) (*tableBuilder.Table, error) {
	// get the set of columns from the .tf file and convert to a list of tableBuilder.Column
	tfColumns := make([]models.TableColumnModel, 0, len(plan.Columns.Elements()))
	plan.Columns.ElementsAs(ctx, &tfColumns, false)
	chColumns := make([]tableBuilder.Column, 0, len(tfColumns))
	for _, tfColumn := range tfColumns {
		chColumns = append(chColumns, tableBuilder.Column{
			Name:     tfColumn.Name.ValueString(),
			Type:     tfColumn.Type.ValueString(),
			Nullable: tfColumn.Nullable.ValueBool(),
			Default:  tfColumn.Default.ValueString(),
			Codec:    tfColumn.Codec.ValueString(),
		})
	}

	table := &tableBuilder.Table{
		Name:    plan.Name.ValueString(),
		Columns: chColumns,
		OrderBy: plan.OrderBy.ValueString(),
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
