package clickhouse

import (
	"context"
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces
var (
	_ provider.Provider = &clickhouseProvider{}
)

// New is a helper function to simplify provider server and testing implementation.
func New() provider.Provider {
	return &clickhouseProvider{}
}

// hashicupsProvider is the provider implementation.
type clickhouseProvider struct{}

type clickhouseProviderModel struct {
	Environment    types.String `tfdisk:"environment"`
	OrganizationID types.String `tfdisk:"organization_id"`
	TokenKey       types.String `tfdisk:"token_key"`
	TokenSecret    types.String `tfdisk:"token_secret"`
}

// Metadata returns the provider type name.
func (p *clickhouseProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "clickhouse"
}

// Schema defines the provider-level schema for configuration data.
func (p *clickhouseProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"environment": schema.StringAttribute{
				Optional: true,
			},
			"organization_id": schema.StringAttribute{
				Required: true,
			},
			"token_key": schema.StringAttribute{
				Required: true,
			},
			"token_secret": schema.StringAttribute{
				Required:  true,
				Sensitive: true,
			},
		},
	}
}

var environmentMap = map[string]bool{
	"production": true,
	"staging":    true,
	"qa":         true,
	"local":      true,
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

	if config.Environment.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("environment"),
			"Unknown ClickHouse OpenAPI Environment",
			"The provider cannot create the ClickHouse OpenAPI client as there is an unknown configuration value for the Environment. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the CLICKHOUSE_ENV environment variable.",
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

	environment := os.Getenv("CLICKHOUSE_ENV")
	organizationId := os.Getenv("CLICKHOUSE_ORG_ID")
	tokenKey := os.Getenv("CLICKHOUSE_TOKEN_KEY")
	tokenSecret := os.Getenv("CLICKHOUSE_TOKEN_KEY")

	if !config.Environment.IsNull() {
		environment = config.Environment.ValueString()
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

	if environment == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("environment"),
			"Missing ClickHouse OpenAPI Environment",
			"The provider cannot create the ClickHouse OpenAPI client: missing or empty value for the environment. "+
				"Set the environment value in the configuration or use the CLICKHOUSE_ENV environment variable. "+
				"If either is already set, ensure the value is not empty.",
		)
	}

	_, validEnvironment := environmentMap[environment]
	if !validEnvironment {
		resp.Diagnostics.AddAttributeError(
			path.Root("environment"),
			"Invalid ClickHouse OpenAPI Environment",
			fmt.Sprintf("The provider cannot create the ClickHouse OpenAPI client: invalid value \"%s\" must be "+
				"one of \"production\", \"staging\", \"qa\", or \"local\"", environment),
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

	// Create a new HashiCups client using the configuration values
	client, err := NewClient(environment, organizationId, tokenKey, tokenSecret)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create ClickHouse OpenAPI Client",
			"An unexpected error occurred when creating the ClickHouse OpenAPI client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"ClickHouse Client Error: "+err.Error(),
		)
		return
	}

	// Make the HashiCups client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = client
	resp.ResourceData = client
}

// DataSources defines the data sources implemented in the provider.
func (p *clickhouseProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

// Resources defines the resources implemented in the provider.
func (p *clickhouseProvider) Resources(_ context.Context) []func() resource.Resource {
	return nil
}

// package clickhouse

// import (
// 	"context"

// 	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
// 	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
// )

// func Provider() *schema.Provider {
// 	return &schema.Provider{
// 		Schema: map[string]*schema.Schema{
// 			// "credentials_file": {
// 			// 	Type:     schema.TypeString,
// 			// 	Required: true,
// 			// },
// 			"environment": {
// 				Type:     schema.TypeString,
// 				Optional: true,
// 				Default:  "production",
// 			},
// 			"organization_id": {
// 				Type:     schema.TypeString,
// 				Required: true,
// 			},
// 			"token_key": {
// 				Type:     schema.TypeString,
// 				Required: true,
// 			},
// 			"token_secret": {
// 				Type:     schema.TypeString,
// 				Required: true,
// 			},
// 		},
// 		ResourcesMap: map[string]*schema.Resource{
// 			"clickhouse_service": initServiceAllocationSchema(),
// 		},
// 		DataSourcesMap:       map[string]*schema.Resource{},
// 		ConfigureContextFunc: providerContextConfigure,
// 	}
// }

// func readTokenFromFile(filePath string) (string, string) {
// 	return "avhj1U5QCdWAE9CA9", "4b1dROiHQEuSXJHlV8zHFd0S7WQj7CGxz5kGJeJnca"
// }

// func providerContextConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
// 	// Warning or errors can be collected in a slice type
// 	var diags diag.Diagnostics

// 	// tokenKey, tokenSecret := readTokenFromFile(d.Get("credentials_file").(string))
// 	env := d.Get("environment").(string)
// 	organizationId := d.Get("organization_id").(string)
// 	tokenKey := d.Get("token_key").(string)
// 	tokenSecret := d.Get("token_secret").(string)
// 	c, err := NewClient(env, organizationId, tokenKey, tokenSecret)
// 	if err != nil {
// 		return nil, diag.FromErr(err)
// 	}

// 	return c, diags
// }
