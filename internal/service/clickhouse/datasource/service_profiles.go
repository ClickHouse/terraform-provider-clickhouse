package datasource

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
)

//go:embed descriptions/service_profiles.md
var serviceProfilesDataSourceDescription string

var _ datasource.DataSource = &serviceProfilesDataSource{}

func NewServiceProfilesDataSource() datasource.DataSource { return &serviceProfilesDataSource{} }

type serviceProfilesDataSource struct{ client api.Client }

type serviceProfilesDataSourceModel struct {
	RegionID types.String `tfsdk:"region_id"`
	ByocID   types.String `tfsdk:"byoc_id"`
	Profiles types.List   `tfsdk:"profiles"`
}

func serviceProfileObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"profile":   types.StringType,
			"cpu_cores": types.Float64Type,
			"memory_gi": types.Float64Type,
		},
	}
}

// serviceProfilesToListValue maps a slice of api.ServiceProfile into the list
// value used by the "profiles" attribute. Extracted from Read so the mapping
// is independently testable. A nil/empty slice yields a known empty list, not
// a null one.
func serviceProfilesToListValue(items []api.ServiceProfile) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elems := make([]attr.Value, 0, len(items))
	for _, it := range items {
		obj, d := types.ObjectValue(serviceProfileObjectType().AttrTypes, map[string]attr.Value{
			"profile":   types.StringValue(it.Profile),
			"cpu_cores": types.Float64Value(it.CpuCores),
			"memory_gi": types.Float64Value(it.MemoryGi),
		})
		diags.Append(d...)
		elems = append(elems, obj)
	}
	list, d := types.ListValue(serviceProfileObjectType(), elems)
	diags.Append(d...)
	return list, diags
}

func (d *serviceProfilesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError("ClickHouse Cloud API not configured",
			"This resource requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).")
		return
	}
	d.client = providerData.API
}

func (d *serviceProfilesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_profiles"
}

func (d *serviceProfilesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: serviceProfilesDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"region_id": schema.StringAttribute{
				Description: "Region to list available profiles for (e.g. 'us-east-1').",
				Required:    true,
			},
			"byoc_id": schema.StringAttribute{
				Description: "BYOC infrastructure ID. Required to include dynamic BYOC profiles in the result.",
				Optional:    true,
			},
			"profiles": schema.ListNestedAttribute{
				Description: "Custom instance profiles available to the organization in the given region.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"profile": schema.StringAttribute{
							Description: "Profile name, usable as the 'profile' attribute of the clickhouse_service resource.",
							Computed:    true,
						},
						"cpu_cores": schema.Float64Attribute{
							Description: "Number of CPU cores per replica.",
							Computed:    true,
						},
						"memory_gi": schema.Float64Attribute{
							Description: "Memory per replica in GiB. For BYOC profiles, 'min_replica_memory_gb' and 'max_replica_memory_gb' on the service must both equal this value.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *serviceProfilesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg serviceProfilesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := d.client.ListServiceProfiles(ctx, cfg.RegionID.ValueString(), cfg.ByocID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error listing service profiles", "Could not list service profiles: "+err.Error())
		return
	}

	list, diags := serviceProfilesToListValue(items)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg.Profiles = list
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
