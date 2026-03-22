package datasource

import (
	"context"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &postgresInstanceDataSource{}

func NewPostgresInstanceDataSource() datasource.DataSource {
	return &postgresInstanceDataSource{}
}

type postgresInstanceDataSource struct {
	client api.Client
}

type postgresInstanceDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	CloudProvider    types.String `tfsdk:"cloud_provider"`
	Region           types.String `tfsdk:"region"`
	PostgresVersion  types.String `tfsdk:"postgres_version"`
	Size             types.String `tfsdk:"size"`
	StorageSize      types.Int64  `tfsdk:"storage_size"`
	HAType           types.String `tfsdk:"ha_type"`
	State            types.String `tfsdk:"state"`
	IsPrimary        types.Bool   `tfsdk:"is_primary"`
	Hostname         types.String `tfsdk:"hostname"`
	ConnectionString types.String `tfsdk:"connection_string"`
	Username         types.String `tfsdk:"username"`
	PgConfig         types.Map    `tfsdk:"pg_config"`
	PgBouncerConfig  types.Map    `tfsdk:"pg_bouncer_config"`
	Tags             types.Map    `tfsdk:"tags"`
}

func (d *postgresInstanceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *postgresInstanceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_instance"
}

func (d *postgresInstanceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up information about an existing ClickHouse Cloud managed Postgres instance.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the Postgres instance.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "User defined identifier for the Postgres instance.",
				Computed:    true,
			},
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider in which the Postgres instance is deployed.",
				Computed:    true,
			},
			"region": schema.StringAttribute{
				Description: "Region within the cloud provider in which the Postgres instance is deployed.",
				Computed:    true,
			},
			"postgres_version": schema.StringAttribute{
				Description: "Postgres major version.",
				Computed:    true,
			},
			"size": schema.StringAttribute{
				Description: "Size of the Postgres instance (e.g. 'standard-2').",
				Computed:    true,
			},
			"storage_size": schema.Int64Attribute{
				Description: "Storage size in GiB for the Postgres instance.",
				Computed:    true,
			},
			"ha_type": schema.StringAttribute{
				Description: "High availability type: 'none' (0 standbys), 'async' (1 standby), or 'sync' (2 standbys).",
				Computed:    true,
			},
			"pg_config": schema.MapAttribute{
				Description: "Custom Postgres configuration parameters.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"pg_bouncer_config": schema.MapAttribute{
				Description: "Custom PgBouncer configuration parameters.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"tags": schema.MapAttribute{
				Description: "Tags assigned to the Postgres instance.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"state": schema.StringAttribute{
				Description: "Current state of the Postgres instance.",
				Computed:    true,
			},
			"is_primary": schema.BoolAttribute{
				Description: "Whether this is a primary Postgres instance.",
				Computed:    true,
			},
			"hostname": schema.StringAttribute{
				Description: "Hostname of the Postgres instance.",
				Computed:    true,
			},
			"connection_string": schema.StringAttribute{
				Description: "Connection string for the Postgres instance.",
				Computed:    true,
				Sensitive:   true,
			},
			"username": schema.StringAttribute{
				Description: "Default username for the Postgres instance.",
				Computed:    true,
			},
		},
	}
}

func (d *postgresInstanceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data postgresInstanceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	instance, err := d.client.GetPostgresInstance(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Postgres Instance",
			"Could not read Postgres instance id "+data.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	data.ID = types.StringValue(instance.ID)
	data.Name = types.StringValue(instance.Name)
	data.CloudProvider = types.StringValue(instance.Provider)
	data.Region = types.StringValue(instance.Region)
	data.PostgresVersion = types.StringValue(instance.PostgresVersion)
	data.Size = types.StringValue(instance.Size)
	data.StorageSize = types.Int64Value(int64(instance.StorageSize))
	data.HAType = types.StringValue(instance.HAType)
	data.State = types.StringValue(instance.State)
	data.IsPrimary = types.BoolValue(instance.IsPrimary)
	data.Hostname = types.StringValue(instance.Hostname)
	data.ConnectionString = types.StringValue(instance.ConnectionString)
	data.Username = types.StringValue(instance.Username)

	// Map pgConfig
	if instance.PgConfig == nil {
		data.PgConfig = types.MapNull(types.StringType)
	} else {
		pgConfigValues := make(map[string]attr.Value, len(instance.PgConfig))
		for k, v := range instance.PgConfig {
			pgConfigValues[k] = types.StringValue(v)
		}
		data.PgConfig, _ = types.MapValue(types.StringType, pgConfigValues)
	}

	// Map pgBouncerConfig
	if instance.PgBouncerConfig == nil {
		data.PgBouncerConfig = types.MapNull(types.StringType)
	} else {
		pgBouncerConfigValues := make(map[string]attr.Value, len(instance.PgBouncerConfig))
		for k, v := range instance.PgBouncerConfig {
			pgBouncerConfigValues[k] = types.StringValue(v)
		}
		data.PgBouncerConfig, _ = types.MapValue(types.StringType, pgBouncerConfigValues)
	}

	// Map tags
	if len(instance.Tags) == 0 {
		data.Tags = types.MapNull(types.StringType)
	} else {
		tagsValues := make(map[string]attr.Value, len(instance.Tags))
		for _, tag := range instance.Tags {
			tagsValues[tag.Key] = types.StringValue(tag.Value)
		}
		data.Tags, _ = types.MapValue(types.StringType, tagsValues)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
