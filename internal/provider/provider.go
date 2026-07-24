package provider

import (
	"cmp"
	"context"
	_ "embed"
	"os"
	"time"

	upstreamdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	upstreamresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	retryablehttp "github.com/hashicorp/go-retryablehttp"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	clickstackclient "github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

// Ensure the implementation satisfies the expected interfaces
var (
	_ provider.Provider = &clickhouseProvider{}
)

//go:embed README.md
var providerDescription string

func NewBuilder(packages []service.ServicePackage) func() provider.Provider {
	return func() provider.Provider {
		return &clickhouseProvider{
			servicePackages: packages,
		}
	}
}

// clickhouseProvider is the provider implementation.
type clickhouseProvider struct {
	servicePackages []service.ServicePackage
}

type clickhouseProviderModel struct {
	ApiUrl              types.String `tfsdk:"api_url"`
	OrganizationID      types.String `tfsdk:"organization_id"`
	TokenKey            types.String `tfsdk:"token_key"`
	TokenSecret         types.String `tfsdk:"token_secret"`
	TimeoutSeconds      types.Int32  `tfsdk:"timeout_seconds"`
	ClickStackEndpoint  types.String `tfsdk:"clickstack_endpoint"`
	ClickStackAPIKey    types.String `tfsdk:"clickstack_api_key"`
	ClickStackServiceID types.String `tfsdk:"clickstack_service_id"`
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
				Description: "Token key of the key/secret pair. Used to authenticate with OpenAPI. Alternatively, can be configured using the `CLICKHOUSE_CLOUD_API_KEY` environment variable.",
				Optional:    true,
			},
			"token_secret": schema.StringAttribute{
				Description: "Token secret of the key/secret pair. Used to authenticate with OpenAPI. Alternatively, can be configured using the `CLICKHOUSE_CLOUD_API_SECRET` environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"timeout_seconds": schema.Int32Attribute{
				Description: "Timeout in seconds for the HTTP client.",
				Optional:    true,
			},
			"clickstack_endpoint": schema.StringAttribute{
				Description: "Endpoint of a self-hosted ClickStack API used by clickhouse_clickstack_* resources, e.g. http://localhost:8000. Required together with `clickstack_api_key`. Alternatively use the `CLICKSTACK_ENDPOINT` environment variable. For ClickStack on ClickHouse Cloud, leave unset and use `clickstack_service_id` instead.",
				Optional:    true,
			},
			"clickstack_api_key": schema.StringAttribute{
				Description: "Personal API access key for a self-hosted ClickStack API, used by clickhouse_clickstack_* resources. Alternatively use the `CLICKSTACK_API_KEY` environment variable. ClickStack on ClickHouse Cloud does not accept API keys; use `clickstack_service_id` with the Cloud credentials instead.",
				Optional:    true,
				Sensitive:   true,
			},
			"clickstack_service_id": schema.StringAttribute{
				Description: "ID of the ClickHouse Cloud service running managed ClickStack. When set, clickhouse_clickstack_* resources are served through the ClickHouse Cloud API, authenticating with `organization_id`, `token_key` and `token_secret`. Alternatively use the `CLICKSTACK_SERVICE_ID` environment variable. Mutually exclusive with `clickstack_api_key` and `clickstack_endpoint`.",
				Optional:    true,
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
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CLICKHOUSE_CLOUD_API_KEY environment variable.",
		)
	}

	if config.TokenSecret.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token_secret"),
			"Unknown ClickHouse OpenAPI Token Secret",
			"The provider cannot create the ClickHouse OpenAPI client as there is an unknown configuration value for the token secret. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CLICKHOUSE_CLOUD_API_SECRET environment variable.",
		)
	}

	// Unknown ClickStack values (e.g. clickstack_service_id referencing a
	// not-yet-created service) would otherwise read as empty strings and
	// silently disable or misconfigure the ClickStack client.
	for attr, value := range map[string]types.String{
		"clickstack_endpoint":   config.ClickStackEndpoint,
		"clickstack_api_key":    config.ClickStackAPIKey,
		"clickstack_service_id": config.ClickStackServiceID,
	} {
		if value.IsUnknown() {
			resp.Diagnostics.AddAttributeError(
				path.Root(attr),
				"Unknown ClickStack configuration value",
				"The provider cannot configure the ClickStack client as there is an unknown configuration value for "+attr+". "+
					"Either target apply the source of the value first, set the value statically in the configuration, or use the corresponding CLICKSTACK_* environment variable.",
			)
		}
	}

	if resp.Diagnostics.HasError() {
		return
	}

	// Default values to environment variables, but override
	// with Terraform configuration value if set.

	apiUrl := os.Getenv("CLICKHOUSE_API_URL")
	organizationId := os.Getenv("CLICKHOUSE_ORG_ID")
	// Read credentials from env: prefer new CLICKHOUSE_CLOUD_API_{KEY,SECRET},
	// fall back to legacy CLICKHOUSE_TOKEN_{KEY,SECRET}.
	tokenKey := cmp.Or(
		os.Getenv("CLICKHOUSE_CLOUD_API_KEY"),
		os.Getenv("CLICKHOUSE_TOKEN_KEY"),
	)
	tokenSecret := cmp.Or(
		os.Getenv("CLICKHOUSE_CLOUD_API_SECRET"),
		os.Getenv("CLICKHOUSE_TOKEN_SECRET"),
	)

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

	// Resolve ClickStack credentials (configuration overrides environment).
	// Self-hosted ClickStack authenticates with an endpoint + API key; ClickStack
	// on ClickHouse Cloud is served through the Cloud OpenAPI and authenticates
	// with the Cloud credentials plus the ID of the service running it.
	clickstackEndpoint := os.Getenv("CLICKSTACK_ENDPOINT")
	clickstackAPIKey := os.Getenv("CLICKSTACK_API_KEY")
	clickstackServiceID := os.Getenv("CLICKSTACK_SERVICE_ID")
	if !config.ClickStackEndpoint.IsNull() {
		clickstackEndpoint = config.ClickStackEndpoint.ValueString()
	}
	if !config.ClickStackAPIKey.IsNull() {
		clickstackAPIKey = config.ClickStackAPIKey.ValueString()
	}
	if !config.ClickStackServiceID.IsNull() {
		clickstackServiceID = config.ClickStackServiceID.ValueString()
	}

	// Written config wins: when the provider block explicitly selects one
	// ClickStack mode, ignore any stray CLICKSTACK_* environment variable for the
	// other mode so it cannot manufacture a false "ambiguous configuration"
	// conflict. Environment variables only fill gaps the config leaves open.
	// A mode counts as chosen only when its attribute is set to a non-empty
	// value: an attribute wired to an empty-defaulting variable (e.g.
	// clickstack_endpoint = var.endpoint) is not null but selects nothing.
	cloudInConfig := configSelected(config.ClickStackServiceID)
	selfHostedInConfig := configSelected(config.ClickStackEndpoint) || configSelected(config.ClickStackAPIKey)
	clickstackEndpoint, clickstackAPIKey, clickstackServiceID = resolveClickStackCreds(
		clickstackEndpoint, clickstackAPIKey, clickstackServiceID, cloudInConfig, selfHostedInConfig)

	if clickstackServiceID != "" && (clickstackAPIKey != "" || clickstackEndpoint != "") {
		resp.Diagnostics.AddAttributeError(
			path.Root("clickstack_service_id"),
			"Ambiguous ClickStack configuration",
			"clickstack_service_id selects ClickStack on ClickHouse Cloud and cannot be combined with "+
				"clickstack_api_key or clickstack_endpoint, which select a self-hosted ClickStack deployment. "+
				"Set one or the other (also check the CLICKSTACK_* environment variables).",
		)
		return
	}
	if clickstackAPIKey != "" && clickstackEndpoint == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("clickstack_endpoint"),
			"Missing ClickStack endpoint",
			"clickstack_api_key authenticates a self-hosted ClickStack deployment; set clickstack_endpoint "+
				"(or the CLICKSTACK_ENDPOINT environment variable) to its API base URL. "+
				"For ClickStack on ClickHouse Cloud, set clickstack_service_id instead — the Cloud API "+
				"authenticates with organization_id, token_key and token_secret, not a ClickStack API key.",
		)
		return
	}
	if clickstackEndpoint != "" && clickstackAPIKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("clickstack_api_key"),
			"Missing ClickStack API key",
			"clickstack_endpoint points at a self-hosted ClickStack deployment, which also needs "+
				"clickstack_api_key (or the CLICKSTACK_API_KEY environment variable) to authenticate. "+
				"Without it the endpoint would be silently ignored.",
		)
		return
	}

	// A ClickStack service ID requires the Cloud credentials, so it also drives
	// the cloud credential validation below.
	cloudConfigured := organizationId != "" || tokenKey != "" || tokenSecret != "" || clickstackServiceID != ""
	clickstackConfigured := clickstackAPIKey != "" || clickstackServiceID != ""

	data := &service.ProviderData{}

	// Validate and build the ClickHouse Cloud client only when cloud credentials
	// are (partially) provided, or when nothing at all is configured — a bare
	// provider block should still surface the cloud credential guidance. A
	// ClickStack-only configuration skips these checks; cloud resources then fail
	// individually in their own Configure with a clear "not configured" error.
	if cloudConfigured || !clickstackConfigured {
		clientConfig := api.ClientConfig{
			ApiURL:         apiUrl,
			OrganizationID: organizationId,
			TokenKey:       tokenKey,
			TokenSecret:    tokenSecret,
		}
		if !config.TimeoutSeconds.IsUnknown() && !config.TimeoutSeconds.IsNull() {
			clientConfig.Timeout = time.Second * time.Duration(config.TimeoutSeconds.ValueInt32())
		}

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
					"Set the token_key value in the configuration or use the CLICKHOUSE_CLOUD_API_KEY environment variable. "+
					"If either is already set, ensure the value is not empty.",
			)
		}
		if tokenSecret == "" {
			resp.Diagnostics.AddAttributeError(
				path.Root("token_secret"),
				"Missing ClickHouse OpenAPI Token Secret",
				"The provider cannot create the ClickHouse OpenAPI client: missing or empty value for the token secret. "+
					"Set the token_secret value in the configuration or use the CLICKHOUSE_CLOUD_API_SECRET environment variable. "+
					"If either is already set, ensure the value is not empty.",
			)
		}
		if resp.Diagnostics.HasError() {
			return
		}

		// Create a new ClickHouse client using the configuration values
		client, err := api.NewClient(clientConfig)
		if err != nil {
			resp.Diagnostics.AddError(
				"Unable to Create ClickHouse OpenAPI Client",
				"An unexpected error occurred when creating the ClickHouse OpenAPI client. "+
					"If the error is not clear, please contact the provider developers.\n\n"+
					"ClickHouse Client Error: "+err.Error(),
			)
			return
		}
		data.API = client
	}

	// Build the ClickStack client: self-hosted (endpoint + API key) or
	// ClickHouse Cloud (Cloud credentials + service ID).
	if clickstackConfigured {
		retryClient := retryablehttp.NewClient()
		retryClient.Logger = nil
		if !config.TimeoutSeconds.IsUnknown() && !config.TimeoutSeconds.IsNull() {
			retryClient.HTTPClient.Timeout = time.Second * time.Duration(config.TimeoutSeconds.ValueInt32())
		}

		if clickstackServiceID != "" {
			csClient, err := clickstackclient.NewCloud(apiUrl, organizationId, clickstackServiceID, tokenKey, tokenSecret, retryClient.StandardClient())
			if err != nil {
				resp.Diagnostics.AddAttributeError(
					path.Root("api_url"),
					"Invalid ClickHouse API URL for ClickStack",
					err.Error(),
				)
				return
			}
			data.ClickStack = csClient
		} else {
			csClient, err := clickstackclient.New(clickstackEndpoint, clickstackAPIKey, retryClient.StandardClient())
			if err != nil {
				resp.Diagnostics.AddAttributeError(
					path.Root("clickstack_endpoint"),
					"Invalid ClickStack endpoint",
					err.Error(),
				)
				return
			}
			data.ClickStack = csClient
		}
	}

	// Make the client container available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = data
	resp.ResourceData = data
}

// configSelected reports whether a provider attribute was explicitly set to a
// non-empty value in the config. It is deliberately stricter than !IsNull():
// terraform-plugin-framework reports an attribute wired to an empty-defaulting
// variable as non-null with an empty string, which must not count as selecting
// a ClickStack mode. Unknown values are handled earlier and read as unset here.
func configSelected(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}

// resolveClickStackCreds applies "written config wins" to the already-merged
// (config-over-environment) ClickStack credentials. When the provider config
// explicitly names one mode, environment variables for the other mode are
// dropped so a stray CLICKSTACK_* value cannot contradict the config. When the
// config names neither mode (or both), the merged values pass through unchanged
// and the caller's validation handles any genuine conflict.
func resolveClickStackCreds(endpoint, apiKey, serviceID string, cloudInConfig, selfHostedInConfig bool) (string, string, string) {
	switch {
	case cloudInConfig && !selfHostedInConfig:
		// Config chose Cloud; ignore stray self-hosted env values.
		return "", "", serviceID
	case selfHostedInConfig && !cloudInConfig:
		// Config chose self-hosted; ignore a stray Cloud service-id env value.
		return endpoint, apiKey, ""
	default:
		return endpoint, apiKey, serviceID
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *clickhouseProvider) DataSources(_ context.Context) []func() upstreamdatasource.DataSource {
	var out []func() upstreamdatasource.DataSource
	for _, sp := range p.servicePackages {
		out = append(out, sp.DataSources()...)
	}
	return out
}

// Resources defines the resources implemented in the provider.
func (p *clickhouseProvider) Resources(_ context.Context) []func() upstreamresource.Resource {
	var out []func() upstreamresource.Resource
	for _, sp := range p.servicePackages {
		out = append(out, sp.Resources()...)
	}
	return out
}
