package datasource

import (
	"context"
	_ "embed"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

//go:embed descriptions/postgres_service.md
var postgresServiceDataSourceDescription string

// postgresDefaultPort mirrors the resource's hardcoded listening port.
const postgresDefaultPort int64 = 5432

var _ datasource.DataSource = &postgresServiceDataSource{}

// NewPostgresServiceDataSource fetches a single Managed Postgres service by ID.
func NewPostgresServiceDataSource() datasource.DataSource {
	return &postgresServiceDataSource{}
}

type postgresServiceDataSource struct {
	client api.Client
}

type postgresServiceDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	CloudProvider    types.String `tfsdk:"cloud_provider"`
	Region           types.String `tfsdk:"region"`
	PostgresVersion  types.String `tfsdk:"postgres_version"`
	Size             types.String `tfsdk:"size"`
	HaType           types.String `tfsdk:"ha_type"`
	State            types.String `tfsdk:"state"`
	CreatedAt        types.String `tfsdk:"created_at"`
	IsPrimary        types.Bool   `tfsdk:"is_primary"`
	Hostname         types.String `tfsdk:"hostname"`
	Port             types.Int64  `tfsdk:"port"`
	Username         types.String `tfsdk:"username"`
	ConnectionString types.String `tfsdk:"connection_string"`
	Tags             types.Map    `tfsdk:"tags"`
	PgConfig         types.Map    `tfsdk:"pg_config"`
	PgBouncerConfig  types.Map    `tfsdk:"pgbouncer_config"`
}

func (d *postgresServiceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *postgresServiceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_service"
}

func (d *postgresServiceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: postgresServiceDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"id":                schema.StringAttribute{Description: "Unique identifier of the Postgres service to look up.", Required: true},
			"name":              schema.StringAttribute{Description: "Human-readable name.", Computed: true},
			"cloud_provider":    schema.StringAttribute{Description: "Cloud provider hosting the instance.", Computed: true},
			"region":            schema.StringAttribute{Description: "Cloud region.", Computed: true},
			"postgres_version":  schema.StringAttribute{Description: "Major Postgres version.", Computed: true},
			"size":              schema.StringAttribute{Description: "Instance size (VM SKU).", Computed: true},
			"ha_type":           schema.StringAttribute{Description: "High-availability mode ('none', 'async', 'sync').", Computed: true},
			"state":             schema.StringAttribute{Description: "Server-reported state.", Computed: true},
			"created_at":        schema.StringAttribute{Description: "RFC3339 creation timestamp.", Computed: true},
			"is_primary":        schema.BoolAttribute{Description: "True for a primary; false for a read replica.", Computed: true},
			"hostname":          schema.StringAttribute{Description: "Network hostname for client connections.", Computed: true},
			"port":              schema.Int64Attribute{Description: "TCP port for client connections.", Computed: true},
			"username":          schema.StringAttribute{Description: "Default superuser name.", Computed: true},
			"connection_string": schema.StringAttribute{Description: "Full connection URI (embeds the password). Sensitive.", Computed: true, Sensitive: true},
			"tags": schema.MapAttribute{
				Description: "User tags. Read-only; a string map.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"pg_config": schema.MapAttribute{
				Description: "Postgres server parameters currently set on the instance. Read-only; a string map.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"pgbouncer_config": schema.MapAttribute{
				Description: "PgBouncer parameters currently set on the instance. Read-only; a string map.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *postgresServiceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data postgresServiceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pg, err := d.client.GetPostgres(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Postgres service", "Could not read Postgres service "+data.ID.ValueString()+": "+err.Error())
		return
	}
	cfg, err := d.client.GetPostgresConfig(ctx, data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading Postgres configuration", "Could not read config for Postgres service "+data.ID.ValueString()+": "+err.Error())
		return
	}

	data.ID = types.StringValue(pg.Id)
	data.Name = types.StringValue(pg.Name)
	data.CloudProvider = types.StringValue(pg.Provider)
	data.Region = types.StringValue(pg.Region)
	data.PostgresVersion = types.StringValue(pg.PostgresVersion)
	data.Size = types.StringValue(pg.Size)
	if pg.HaType != "" {
		data.HaType = types.StringValue(pg.HaType)
	} else {
		data.HaType = types.StringValue("none")
	}
	data.State = types.StringValue(pg.State)
	data.CreatedAt = types.StringValue(pg.CreatedAt)
	data.IsPrimary = types.BoolValue(pg.IsPrimary)
	data.Hostname = strOrNull(pg.Hostname)
	data.Port = types.Int64Value(postgresDefaultPort)
	data.Username = strOrNull(pg.Username)
	data.ConnectionString = strOrNull(pg.ConnectionString)

	tags, diags := apiTagsToStringMap(pg.Tags)
	resp.Diagnostics.Append(diags...)
	pgCfg, diags := pgConfigToStringMap(cfg.PgConfig)
	resp.Diagnostics.Append(diags...)
	pbCfg, diags := pgConfigToStringMap(cfg.PgBouncerConfig)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Tags = tags
	data.PgConfig = pgCfg
	data.PgBouncerConfig = pbCfg

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// --- helpers ---------------------------------------------------------------

func strOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// apiTagsToStringMap converts api tags to a string map, dropping empty-value
// tags. Empty input returns a known empty map (not null), matching the
// resource layer's apiTagsToMapValue so a data-source result interpolated into
// a resource attribute doesn't produce a spurious empty-vs-null diff.
func apiTagsToStringMap(tags []api.Tag) (types.Map, diag.Diagnostics) {
	m := make(map[string]attr.Value, len(tags))
	for _, t := range tags {
		if t.Value == "" {
			continue
		}
		m[t.Key] = types.StringValue(t.Value)
	}
	return types.MapValue(types.StringType, m)
}

// pgConfigToStringMap converts an api.PgConfigMap to a string map. Empty input
// returns a known empty map (not null), matching the resource layer's
// apiConfigToMapValue.
func pgConfigToStringMap(c api.PgConfigMap) (types.Map, diag.Diagnostics) {
	m := make(map[string]attr.Value, len(c))
	for k, v := range c {
		m[k] = types.StringValue(v)
	}
	return types.MapValue(types.StringType, m)
}
