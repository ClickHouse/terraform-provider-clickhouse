package clickstack

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = (*dashboardDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*dashboardDataSource)(nil)
)

// NewDashboardDataSource is a helper to register the data source with the provider.
func NewDashboardDataSource() datasource.DataSource {
	return &dashboardDataSource{}
}

// dashboardDataSource fetches a ClickStack dashboard by ID and exposes its JSON body.
type dashboardDataSource struct {
	client *client.Client
}

// dashboardDataSourceModel maps the data source schema data.
type dashboardDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Team          types.String `tfsdk:"team"`
	DashboardJSON types.String `tfsdk:"dashboard_json"`
}

func (d *dashboardDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_dashboard"
}

func (d *dashboardDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches a ClickStack dashboard by ID and exposes its JSON body.",
		Attributes: map[string]schema.Attribute{
			idAttr: schema.StringAttribute{
				Required:    true,
				Description: "Identifier of the dashboard to fetch.",
			},
			teamAttr: schema.StringAttribute{
				Optional:    true,
				Description: "Team ID to look the dashboard up under, sent as the `x-hdx-team` header. Defaults to the API key's team.",
			},
			dashboardJSONAttr: schema.StringAttribute{
				Computed: true,
				Description: "The server-canonical dashboard body as a JSON string, in the v2 API format. " +
					"Note this is the body the API returns (with server-assigned ids/timestamps), equivalent " +
					"to a resource's `normalized_json` — not the authored source-of-truth `dashboard_json` of " +
					"the `clickstack_dashboard` resource. Feeding it directly into a resource's `dashboard_json` " +
					"passes server-canonical content, including ids and timestamps.",
			},
		},
	}
}

func (d *dashboardDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("expected *service.ProviderData, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}

	if providerData.ClickStack == nil {
		resp.Diagnostics.AddError("ClickStack not configured",
			"This data source requires ClickStack credentials. For self-hosted ClickStack, set clickstack_endpoint and "+
				"clickstack_api_key on the provider (or the CLICKSTACK_ENDPOINT / CLICKSTACK_API_KEY environment variables). "+
				"For ClickStack on ClickHouse Cloud, set clickstack_service_id (or CLICKSTACK_SERVICE_ID) together with "+
				"the ClickHouse Cloud credentials (organization_id, token_key, token_secret).")
		return
	}
	d.client = providerData.ClickStack
}

func (d *dashboardDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	utils.AlphaWarning("clickhouse_clickstack_dashboard", &resp.Diagnostics)
	var config dashboardDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body, err := d.client.WithTeam(config.Team.ValueString()).GetDashboard(ctx, config.ID.ValueString())
	if err != nil {
		if errors.Is(err, client.ErrNotFound) {
			resp.Diagnostics.AddError(
				"Dashboard Not Found",
				fmt.Sprintf("No dashboard with ID %q was found.", config.ID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError("Error Reading Dashboard", err.Error())
		return
	}

	config.DashboardJSON = types.StringValue(string(body))
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
