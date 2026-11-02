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
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

//go:embed descriptions/byoc_infrastructures.md
var byocInfrastructuresDataSourceDescription string

var _ datasource.DataSource = &byocInfrastructuresDataSource{}

func NewByocInfrastructuresDataSource() datasource.DataSource {
	return &byocInfrastructuresDataSource{}
}

type byocInfrastructuresDataSource struct{ client api.Client }

type byocInfrastructuresDataSourceModel struct {
	Infrastructures types.List `tfsdk:"infrastructures"`
}

func byocInfrastructureSummaryObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":             types.StringType,
			"state":          types.StringType,
			"cloud_provider": types.StringType,
			"region_id":      types.StringType,
			"account_id":     types.StringType,
			"display_name":   types.StringType,
		},
	}
}

// byocInfrastructuresToListValue maps the organization's BYOC summaries into
// the "infrastructures" list value. Extracted from Read so the mapping is
// independently testable. A nil/empty slice yields a known empty list.
func byocInfrastructuresToListValue(items []api.ByocInfrastructure) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elems := make([]attr.Value, 0, len(items))
	for _, it := range items {
		obj, d := types.ObjectValue(byocInfrastructureSummaryObjectType().AttrTypes, map[string]attr.Value{
			"id":             types.StringValue(it.Id),
			"state":          types.StringValue(it.State),
			"cloud_provider": types.StringValue(it.CloudProvider),
			"region_id":      types.StringValue(it.RegionId),
			"account_id":     types.StringValue(it.AccountId),
			"display_name":   types.StringValue(it.DisplayName),
		})
		diags.Append(d...)
		elems = append(elems, obj)
	}
	list, d := types.ListValue(byocInfrastructureSummaryObjectType(), elems)
	diags.Append(d...)
	return list, diags
}

func (d *byocInfrastructuresDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
			"This data source requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).")
		return
	}
	d.client = providerData.API
}

func (d *byocInfrastructuresDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_byoc_infrastructures"
}

func (d *byocInfrastructuresDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: byocInfrastructuresDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"infrastructures": schema.ListNestedAttribute{
				Description: "All BYOC infrastructures of the organization.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "Unique ID of the BYOC infrastructure.",
							Computed:    true,
						},
						"state": schema.StringAttribute{
							Description: "Provisioning state of the infrastructure (e.g. 'infra-provisioning', 'infra-ready', 'infra-terminating').",
							Computed:    true,
						},
						"cloud_provider": schema.StringAttribute{
							Description: "Cloud provider the infrastructure runs in ('aws', 'gcp' or 'azure').",
							Computed:    true,
						},
						"region_id": schema.StringAttribute{
							Description: "Region the infrastructure is deployed in (e.g. 'us-east-1').",
							Computed:    true,
						},
						"account_id": schema.StringAttribute{
							Description: "Customer cloud account the infrastructure is deployed into: AWS account ID, GCP project ID or Azure subscription ID.",
							Computed:    true,
						},
						"display_name": schema.StringAttribute{
							Description: "Human readable name of the infrastructure.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *byocInfrastructuresDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	utils.BetaWarning("clickhouse_byoc_infrastructures", &resp.Diagnostics)

	org, err := d.client.GetOrganization(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing BYOC infrastructures", "Could not read the organization: "+err.Error())
		return
	}

	list, diags := byocInfrastructuresToListValue(org.ByocConfig)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &byocInfrastructuresDataSourceModel{Infrastructures: list})...)
}
