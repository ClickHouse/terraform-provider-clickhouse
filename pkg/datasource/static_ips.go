package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces
var (
	_ datasource.DataSource              = &staticIPsDataSource{}
	_ datasource.DataSourceWithConfigure = &staticIPsDataSource{}
)

// NewStaticIPsDataSource creates a new data source instance
func NewStaticIPsDataSource() datasource.DataSource {
	return &staticIPsDataSource{}
}

// staticIPsDataSource is the data source implementation
type staticIPsDataSource struct{}

// staticIPsDataSourceModel maps the data source schema data
type staticIPsDataSourceModel struct {
	ID            types.String   `tfsdk:"id"`
	CloudProvider types.String   `tfsdk:"cloud_provider"`
	Region        types.String   `tfsdk:"region"`
	EgressIPs     []types.String `tfsdk:"egress_ips"`
	IngressIPs    []types.String `tfsdk:"ingress_ips"`
	S3Endpoints   []types.String `tfsdk:"s3_endpoints"`
}

// staticIPsResponse represents the structure of the API response
type staticIPsResponse map[string][]regionData

// regionData represents the data for a specific region
type regionData struct {
	Region      string   `json:"region"`
	EgressIPs   []string `json:"egress_ips"`
	IngressIPs  []string `json:"ingress_ips"`
	S3Endpoints []string `json:"s3_endpoints,omitempty"`
}

// Metadata returns the data source type name
func (d *staticIPsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_static_ips"
}

// Schema defines the schema for the data source
func (d *staticIPsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Fetches static IPs from ClickHouse Cloud API filtered by cloud provider and region.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier for this data source.",
				Computed:    true,
			},
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider to filter IPs for (aws, azure, gcp).",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("aws", "gcp", "azure"),
				},
			},
			"region": schema.StringAttribute{
				Description: "Region to filter IPs for (e.g., us-east-1 for AWS).",
				Required:    true,
			},
			"egress_ips": schema.ListAttribute{
				Description: "List of egress (outbound) static IPs for the specified region.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"ingress_ips": schema.ListAttribute{
				Description: "List of ingress (inbound) static IPs for the specified region.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"s3_endpoints": schema.ListAttribute{
				Description: "List of S3 endpoints for the specified region (AWS only).",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

// Configure adds the provider configured client to the data source
func (d *staticIPsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// No configuration needed for this data source
}

// Read refreshes the Terraform state with the latest data
func (d *staticIPsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state staticIPsDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate cloud provider value
	cloudProvider := state.CloudProvider.ValueString()
	if cloudProvider == "" {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"cloud_provider must be specified",
		)
		return
	}

	// Validate region value
	region := state.Region.ValueString()
	if region == "" {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"region must be specified",
		)
		return
	}

	// Fetch the data from the API
	jsonData, err := fetchStaticIPsJSON(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Read ClickHouse Static IPs",
			err.Error(),
		)
		return
	}

	// Parse the JSON
	var apiResponse staticIPsResponse
	if err := json.Unmarshal(jsonData, &apiResponse); err != nil {
		resp.Diagnostics.AddError(
			"Unable to Parse ClickHouse Static IPs",
			fmt.Sprintf("Error parsing JSON: %s", err.Error()),
		)
		return
	}

	// Find the region data for the specified cloud provider and region
	var regionInfo *regionData

	cloudProviderInfo, found := apiResponse[cloudProvider]
	if !found {
		foundCloudProviders := make([]string, 0, len(apiResponse))
		for cp := range apiResponse {
			foundCloudProviders = append(foundCloudProviders, cp)
		}

		resp.Diagnostics.AddError(
			"Invalid Cloud Provider",
			fmt.Sprintf("Cloud provider '%s' is not supported. Must be one of: %s", cloudProvider, strings.Join(foundCloudProviders, ", ")),
		)
		return
	}

	for _, r := range cloudProviderInfo {
		if r.Region == region {
			regionInfo = &r
			break
		}
	}

	// If no matching region found, report an error
	if regionInfo == nil {
		resp.Diagnostics.AddError(
			"Region Not Found",
			fmt.Sprintf("No data found for region '%s' in cloud provider '%s'", region, cloudProvider),
		)
		return
	}

	// Set the ID based on cloud provider and region
	state.ID = types.StringValue(fmt.Sprintf("%s-%s", cloudProvider, region))

	// Convert egress IPs to Terraform framework types
	egressIPs := make([]types.String, 0, len(regionInfo.EgressIPs))
	for _, ip := range regionInfo.EgressIPs {
		egressIPs = append(egressIPs, types.StringValue(ip))
	}
	state.EgressIPs = egressIPs

	// Convert ingress IPs to Terraform framework types
	ingressIPs := make([]types.String, 0, len(regionInfo.IngressIPs))
	for _, ip := range regionInfo.IngressIPs {
		ingressIPs = append(ingressIPs, types.StringValue(ip))
	}
	state.IngressIPs = ingressIPs

	// Convert S3 endpoints to Terraform framework types (if available)
	s3Endpoints := make([]types.String, 0, len(regionInfo.S3Endpoints))
	for _, endpoint := range regionInfo.S3Endpoints {
		s3Endpoints = append(s3Endpoints, types.StringValue(endpoint))
	}
	state.S3Endpoints = s3Endpoints

	// Save the data into the Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// fetchStaticIPsJSON fetches static IPs JSON from the ClickHouse Cloud API
func fetchStaticIPsJSON(ctx context.Context) ([]byte, error) {
	// Create a new HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.clickhouse.cloud/static-ips.json", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Check the response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned non-OK status: %d", resp.StatusCode)
	}

	// Read the response body
	jsonData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	return jsonData, nil
}
