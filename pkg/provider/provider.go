package provider

import (
	"context"
	_ "embed"
	"os"

	upstreamdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	upstreamresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/datasource"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
)

// Ensure the implementation satisfies the expected interfaces
var (
	_ provider.Provider = &clickhouseProvider{}
)

//go:embed README.md
var providerDescription string

func NewBuilder(resources []func() upstreamresource.Resource) func() provider.Provider {
	return func() provider.Provider {
		return &clickhouseProvider{
			resources: resources,
		}
	}
}

// clickhouseProvider is the provider implementation.
type clickhouseProvider struct {
	resources []func() upstreamresource.Resource
}

type clickhouseProviderModel struct {
	ApiUrl         types.String `tfsdk:"api_url"`
	OrganizationID types.String `tfsdk:"organization_id"`
	TokenKey       types.String `tfsdk:"token_key"`
	TokenSecret    types.String `tfsdk:"token_secret"`
}

// Metadata returns the provider type name.
func (p *clickhouseProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "clickhouse"
}

// Schema defines the provider-level schema for configuration data.
func (p *clickhouseProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Description: "API URL of the ClickHouse OpenAPI the provider will interact with. Alternatively, can be configured using the `CLICKHOUSE_API_URL` environment variable. Only specify if you have a specific deployment of the ClickHouse OpenAPI you want to run against.",
				Optional:    true,
			},
			"organization_id": schema.StringAttribute{
				Description: "ID of the organization the provider will create services under. Alternatively, can be configured using the `CLICKHOUSE_ORG_ID` environment variable.",
				Optional:    true,
			},
			"token_key": schema.StringAttribute{
				Description: "Token key of the key/secret pair. Used to authenticate with OpenAPI. Alternatively, can be configured using the `CLICKHOUSE_TOKEN_KEY` environment variable.",
				Optional:    true,
			},
			"token_secret": schema.StringAttribute{
				Description: "Token secret of the key/secret pair. Used to authenticate with OpenAPI. Alternatively, can be configured using the `CLICKHOUSE_TOKEN_SECRET` environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
		},
		MarkdownDescription: providerDescription,
	}
}

// Configure prepares a ClickHouse OpenAPI client for data sources and resources.
func (p *clickhouseProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Retrieve provider data from configuration
	var config clickhouseProviderModel
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If practitioner provided a configuration value for any of the
	// attributes, it must be a known value.

	if config.ApiUrl.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Unknown ClickHouse OpenAPI API URL",
			"The provider cannot create the ClickHouse OpenAPI client as there is an unknown configuration value for the Environment. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CLICKHOUSE_API_URL environment variable.",
		)
	}

	if config.OrganizationID.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("organization_id"),
			"Unknown ClickHouse OpenAPI Organization ID",
			"The provider cannot create the ClickHouse OpenAPI client as there is an unknown configuration value for the organization id. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CLICKHOUSE_ORG_ID environment variable.",
		)
	}

	if config.TokenKey.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token_key"),
			"Unknown ClickHouse OpenAPI Token Key",
			"The provider cannot create the ClickHouse OpenAPI client as there is an unknown configuration value for the token key. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CLICKHOUSE_TOKEN_KEY environment variable.",
		)
	}

	if config.TokenSecret.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token_secret"),
			"Unknown ClickHouse OpenAPI Token Secret",
			"The provider cannot create the ClickHouse OpenAPI client as there is an unknown configuration value for the token secret. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CLICKHOUSE_TOKEN_SECRET environment variable.",
		)
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	apiUrl := os.Getenv("CLICKHOUSE_API_URL")
	organizationId := os.Getenv("CLICKHOUSE_ORG_ID")
	tokenKey := os.Getenv("CLICKHOUSE_TOKEN_KEY")
	tokenSecret := os.Getenv("CLICKHOUSE_TOKEN_SECRET")

	if !config.ApiUrl.IsNull() {
		apiUrl = config.ApiUrl.ValueString()
	}

	if apiUrl == "" {
		apiUrl = "https://api.clickhouse.cloud/v1"
	}

	if !config.OrganizationID.IsNull() {
		organizationId = config.OrganizationID.ValueString()
	}

	if !config.TokenKey.IsNull() {
		tokenKey = config.TokenKey.ValueString()
	}

	if !config.TokenSecret.IsNull() {
		tokenSecret = config.TokenSecret.ValueString()
	}

	// If any of the expected configurations are missing, return
	// errors with provider-specific guidance.

	if apiUrl == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_url"),
			"Missing ClickHouse OpenAPI API URL",
			"The provider cannot create the ClickHouse OpenAPI client: missing or empty value for the API url. "+
				"Set the API url value in the configuration or use the CLICKHOUSE_API_URL environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if organizationId == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("organizationId"),
			"Missing ClickHouse OpenAPI Organization ID",
			"The provider cannot create the ClickHouse OpenAPI client: missing or empty value for the organization id. "+
				"Set the organization_id value in the configuration or use the CLICKHOUSE_ORG_ID environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if tokenKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token_key"),
			"Missing ClickHouse OpenAPI Token Key",
			"The provider cannot create the ClickHouse OpenAPI client: missing or empty value for the token key. "+
				"Set the token_key value in the configuration or use the CLICKHOUSE_TOKEN_KEY environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	if tokenSecret == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("token_secret"),
			"Missing ClickHouse OpenAPI Token Key",
			"The provider cannot create the ClickHouse OpenAPI client: missing or empty value for the token secret. "+
				"Set the token_secret value in the configuration or use the CLICKHOUSE_TOKEN_SECRET environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	// Create a new ClickHouse client using the configuration values
	client, err := api.NewClient(apiUrl, organizationId, tokenKey, tokenSecret)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create ClickHouse OpenAPI Client",
			"An unexpected error occurred when creating the ClickHouse OpenAPI client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"ClickHouse Client Error: "+err.Error(),
		)
		return
	}

	// Make the ClickHouse client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

// DataSources defines the data sources implemented in the provider.
func (p *clickhouseProvider) DataSources(_ context.Context) []func() upstreamdatasource.DataSource {
	return []func() upstreamdatasource.DataSource{
		datasource.NewPrivateEndpointConfigDataSource,
		datasource.NewApiKeyIdDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *clickhouseProvider) Resources(_ context.Context) []func() upstreamresource.Resource {
	return p.resources
}
