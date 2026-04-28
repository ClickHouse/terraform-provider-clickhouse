package datasource

import (
	"context"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &postgresInstanceCACertificateDataSource{}

func NewPostgresInstanceCACertificateDataSource() datasource.DataSource {
	return &postgresInstanceCACertificateDataSource{}
}

type postgresInstanceCACertificateDataSource struct {
	client api.Client
}

type postgresInstanceCACertificateModel struct {
	PostgresInstanceID types.String `tfsdk:"postgres_instance_id"`
	PEM                types.String `tfsdk:"pem"`
}

func (d *postgresInstanceCACertificateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *postgresInstanceCACertificateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_instance_ca_certificate"
}

func (d *postgresInstanceCACertificateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to retrieve the CA certificate for a ClickHouse Cloud managed Postgres instance.",
		Attributes: map[string]schema.Attribute{
			"postgres_instance_id": schema.StringAttribute{
				Description: "ID of the Postgres instance.",
				Required:    true,
			},
			"pem": schema.StringAttribute{
				Description: "CA certificate in PEM format.",
				Computed:    true,
				Sensitive:   true,
			},
		},
	}
}

func (d *postgresInstanceCACertificateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data postgresInstanceCACertificateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pem, err := d.client.GetPostgresInstanceCACertificate(ctx, data.PostgresInstanceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Postgres Instance CA Certificate",
			"Could not read CA certificate for Postgres instance "+data.PostgresInstanceID.ValueString()+": "+err.Error(),
		)
		return
	}

	data.PEM = types.StringValue(pem)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
