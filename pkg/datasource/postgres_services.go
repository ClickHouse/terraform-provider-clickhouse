package datasource

import (
	"context"
	_ "embed"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

//go:embed descriptions/postgres_services.md
var postgresServicesDataSourceDescription string

var _ datasource.DataSource = &postgresServicesDataSource{}

// NewPostgresServicesDataSource lists all Managed Postgres services in the org.
func NewPostgresServicesDataSource() datasource.DataSource {
	return &postgresServicesDataSource{}
}

type postgresServicesDataSource struct {
	client api.Client
}

type postgresServicesDataSourceModel struct {
	Services types.List `tfsdk:"services"`
}

// postgresServiceSummaryObjectType is the element type of the services list.
// Summary fields only — the list endpoint does not return connection_string,
// password, or pg_config.
func postgresServiceSummaryObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":               types.StringType,
			"name":             types.StringType,
			"cloud_provider":   types.StringType,
			"region":           types.StringType,
			"postgres_version": types.StringType,
			"size":             types.StringType,
			"ha_type":          types.StringType,
			"state":            types.StringType,
			"created_at":       types.StringType,
			"is_primary":       types.BoolType,
		},
	}
}

func (d *postgresServicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			"Expected api.Client, got something else. Please report this issue to the provider developers.",
		)
		return
	}
	d.client = client
}

func (d *postgresServicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_services"
}

func (d *postgresServicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: postgresServicesDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"services": schema.ListNestedAttribute{
				Description: "All Managed Postgres services in the organization (summary fields).",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":               schema.StringAttribute{Computed: true},
						"name":             schema.StringAttribute{Computed: true},
						"cloud_provider":   schema.StringAttribute{Computed: true},
						"region":           schema.StringAttribute{Computed: true},
						"postgres_version": schema.StringAttribute{Computed: true},
						"size":             schema.StringAttribute{Computed: true},
						"ha_type":          schema.StringAttribute{Computed: true},
						"state":            schema.StringAttribute{Computed: true},
						"created_at":       schema.StringAttribute{Computed: true},
						"is_primary":       schema.BoolAttribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *postgresServicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	items, err := d.client.ListPostgres(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing Postgres services", "Could not list Postgres services: "+err.Error())
		return
	}

	objType := postgresServiceSummaryObjectType()
	elems := make([]attr.Value, 0, len(items))
	for _, it := range items {
		haType := it.HaType
		if haType == "" {
			haType = "none"
		}
		obj, diags := types.ObjectValue(objType.AttrTypes, map[string]attr.Value{
			"id":               types.StringValue(it.Id),
			"name":             types.StringValue(it.Name),
			"cloud_provider":   types.StringValue(it.Provider),
			"region":           types.StringValue(it.Region),
			"postgres_version": types.StringValue(it.PostgresVersion),
			"size":             types.StringValue(it.Size),
			"ha_type":          types.StringValue(haType),
			"state":            types.StringValue(it.State),
			"created_at":       types.StringValue(it.CreatedAt),
			"is_primary":       types.BoolValue(it.IsPrimary),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		elems = append(elems, obj)
	}

	list, diags := types.ListValue(objType, elems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &postgresServicesDataSourceModel{Services: list})...)
}
