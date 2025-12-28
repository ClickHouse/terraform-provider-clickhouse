package datasource

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
)

const (
	CloudProviderAWS   = "aws"
	CloudProviderGCP   = "gcp"
	CloudProviderAzure = "azure"
)

// Ensure the implementation satisfies the desired interfaces.
var _ datasource.DataSource = &privateEndpointConfigDataSource{}

// NewPrivateEndpointConfigDataSource is a helper function to simplify the provider implementation.
func NewPrivateEndpointConfigDataSource() datasource.DataSource {
	return &privateEndpointConfigDataSource{}
}

type privateEndpointConfigDataSource struct {
	client api.Client
}

func (d *privateEndpointConfigDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Access DataSourceData from the provider configuration
	if req.ProviderData == nil {
		return
	}
	d.client = req.ProviderData.(api.Client)
}

func (d *privateEndpointConfigDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "clickhouse_private_endpoint_config"
}

type privateEndpointConfigDataSourceModel struct {
	CloudProvider     types.String `tfsdk:"cloud_provider"`
	Region            types.String `tfsdk:"region"`
	EndpointServiceID types.String `tfsdk:"endpoint_service_id"`
}

func (d *privateEndpointConfigDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		DeprecationMessage: "This resource is deprecated since version 3.2.0. Please refer to [the docs](https://github.com/ClickHouse/terraform-provider-clickhouse?tab=readme-ov-file#breaking-changes-and-deprecations) for migration steps.",
		Attributes: map[string]schema.Attribute{
			"cloud_provider": schema.StringAttribute{
				Description: "The cloud provider for the private endpoint. Valid values are 'aws', 'gcp', or 'azure'.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf(CloudProviderAWS, CloudProviderGCP, CloudProviderAzure),
				},
			},
			"region": schema.StringAttribute{
				Description: "The region for the private endpoint. Valid values are specific to the cloud provider i.e. us-east-2",
				Required:    true,
			},
			"endpoint_service_id": schema.StringAttribute{
				Description: "The ID of the private endpoint that is used to securely connect to ClickHouse. This is a read-only attribute.",
				Computed:    true,
			},
		},
	}
}

func (d *privateEndpointConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data privateEndpointConfigDataSourceModel
	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	cloudProvider := data.CloudProvider.ValueString()
	region := data.Region.ValueString()

	// Make the API request to get the private endpoint config
	privateEndpointConfig, err := d.client.GetOrgPrivateEndpointConfig(ctx, cloudProvider, region)
	if err != nil {
		resp.Diagnostics.AddError("failed get", fmt.Sprintf("error getting privateEndpointConfig: %v", err))
		return
	}
	data.EndpointServiceID = types.StringValue(privateEndpointConfig.EndpointServiceId)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
