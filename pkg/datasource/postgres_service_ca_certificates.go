package datasource

import (
	"context"
	_ "embed"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

//go:embed descriptions/postgres_service_ca_certificates.md
var postgresCaCertsDataSourceDescription string

var _ datasource.DataSource = &postgresCaCertificatesDataSource{}

// NewPostgresServiceCaCertificatesDataSource fetches the PEM-encoded CA chain
// for a Managed Postgres service (for clients that pin the CA).
func NewPostgresServiceCaCertificatesDataSource() datasource.DataSource {
	return &postgresCaCertificatesDataSource{}
}

type postgresCaCertificatesDataSource struct {
	client api.Client
}

type postgresCaCertificatesDataSourceModel struct {
	ServiceID   types.String `tfsdk:"service_id"`
	Certificate types.String `tfsdk:"certificate"`
}

func (d *postgresCaCertificatesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *postgresCaCertificatesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_postgres_service_ca_certificates"
}

func (d *postgresCaCertificatesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: postgresCaCertsDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				Description: "ID of the Postgres service whose CA certificate chain to fetch.",
				Required:    true,
			},
			"certificate": schema.StringAttribute{
				Description: "PEM-encoded CA certificate chain.",
				Computed:    true,
			},
		},
	}
}

func (d *postgresCaCertificatesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data postgresCaCertificatesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pem, err := d.client.GetPostgresCaCertificates(ctx, data.ServiceID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading Postgres CA certificates",
			"Could not fetch CA certificates for Postgres service "+data.ServiceID.ValueString()+": "+err.Error(),
		)
		return
	}

	data.Certificate = types.StringValue(string(pem))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
