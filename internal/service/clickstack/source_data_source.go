package clickstack

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = (*sourceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*sourceDataSource)(nil)
)

// NewSourceDataSource is a helper to register the data source with the provider.
func NewSourceDataSource() datasource.DataSource {
	return &sourceDataSource{}
}

// sourceDataSource looks up a source by name and kind. Cloud creates its
// sources itself and they can't be managed as resources, so this is how a
// config gets their ids.
type sourceDataSource struct {
	client *client.Client
}

// sourceDataSourceModel maps the data source schema data.
type sourceDataSourceModel struct {
	Team         types.String `tfsdk:"team"`
	Name         types.String `tfsdk:"name"`
	Kind         types.String `tfsdk:"kind"`
	ID           types.String `tfsdk:"id"`
	ConnectionID types.String `tfsdk:"connection_id"`
	DatabaseName types.String `tfsdk:"database_name"`
	TableName    types.String `tfsdk:"table_name"`
}

func (d *sourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickstack_source"
}

func (d *sourceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a ClickStack source by name and, optionally, kind. Use it to reference " +
			"sources Terraform doesn't manage, such as the ones ClickHouse Cloud creates for a service.",
		Attributes: map[string]schema.Attribute{
			teamAttr: schema.StringAttribute{
				Optional:    true,
				Description: "Team ID to look the source up under, sent as the `x-hdx-team` header. Defaults to the API key's team.",
			},
			nameAttr: schema.StringAttribute{
				Required:    true,
				Description: "Name of the source to look up.",
			},
			"kind": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Kind of the source: one of `log`, `trace`, `metric`, `session`, `promql`. " +
					"Set it when sources of different kinds share a name.",
			},
			idAttr: schema.StringAttribute{
				Computed:    true,
				Description: "Identifier of the source.",
			},
			"connection_id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the ClickHouse connection the source reads through.",
			},
			"database_name": schema.StringAttribute{
				Computed:    true,
				Description: "ClickHouse database the source reads from.",
			},
			"table_name": schema.StringAttribute{
				Computed:    true,
				Description: "ClickHouse table the source reads from. Empty for `metric` sources.",
			},
		},
	}
}

func (d *sourceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("expected *service.ProviderData, got: %T. This is a bug in the provider.", req.ProviderData),
		)
		return
	}

	if providerData.ClickStack == nil {
		addNotConfiguredError(&resp.Diagnostics, "data source")
		return
	}
	d.client = providerData.ClickStack
}

func (d *sourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	utils.BetaWarning("clickhouse_clickstack_source", &resp.Diagnostics)
	var config sourceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sources, err := d.client.WithTeam(config.Team.ValueString()).ListSources(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Sources", err.Error())
		return
	}

	name, kind := config.Name.ValueString(), config.Kind.ValueString()
	var matches []client.Source
	for _, src := range sources {
		if src.Name == name && (kind == "" || src.Kind == kind) {
			matches = append(matches, src)
		}
	}

	switch len(matches) {
	case 0:
		resp.Diagnostics.AddError("Source Not Found", fmt.Sprintf("No source named %q%s was found.", name, kindSuffix(kind)))
		return
	case 1:
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		hint := "Set `kind` to pick one."
		if kind != "" {
			hint = "Rename one of them so the name is unique."
		}
		resp.Diagnostics.AddError(
			"Multiple Sources Found",
			fmt.Sprintf("Sources %s are all named %q%s. %s", strings.Join(ids, ", "), name, kindSuffix(kind), hint),
		)
		return
	}

	src := matches[0]
	config.ID = types.StringValue(src.ID)
	config.Kind = types.StringValue(src.Kind)
	config.ConnectionID = types.StringValue(src.Connection)
	config.DatabaseName = types.StringValue(src.From.DatabaseName)
	config.TableName = types.StringValue(src.From.TableName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func kindSuffix(kind string) string {
	if kind == "" {
		return ""
	}
	return fmt.Sprintf(" with kind %q", kind)
}
