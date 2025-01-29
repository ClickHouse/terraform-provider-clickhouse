package datasource

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
)

// Ensure the implementation satisfies the desired interfaces.
var _ datasource.DataSource = &apiKeyIdDataSource{}

// NewApiKeyIdDataSource is a helper function to simplify the provider implementation.
func NewApiKeyIdDataSource() datasource.DataSource {
	return &apiKeyIdDataSource{}
}

type apiKeyIdDataSource struct {
	client api.Client
}

func (d *apiKeyIdDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Access DataSourceData from the provider configuration
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(api.Client)
}

func (d *apiKeyIdDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "clickhouse_api_key_id"
}

type apiKeyIdDataSourceModel struct {
	Id types.String `tfsdk:"id"`
}

func (d *apiKeyIdDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the API key used by the provider to connect to the service. This is a read-only attribute.",
				Computed:    true,
			},
		},
	}
}

func (d *apiKeyIdDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data apiKeyIdDataSourceModel
	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	// Make the API request to get the apiKeyID
	apiKeyId, err := d.client.GetApiKeyID(ctx)
	if err != nil {
		resp.Diagnostics.AddError("failed get", fmt.Sprintf("error getting privateEndpointConfig: %v", err))
		return
	}
	data.Id = types.StringValue(apiKeyId)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
