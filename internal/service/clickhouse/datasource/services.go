package datasource

import (
	"context"
	_ "embed"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

//go:embed descriptions/services.md
var servicesDataSourceDescription string

var _ datasource.DataSource = &servicesDataSource{}

func NewServicesDataSource() datasource.DataSource { return &servicesDataSource{} }

type servicesDataSource struct{ client api.Client }

type servicesDataSourceModel struct {
	Tags     types.Map  `tfsdk:"tags"`
	Services types.List `tfsdk:"services"`
}

// tagFiltersFromMap builds ClickHouse API tag filters ("tag:Key=Value") from a
// Terraform tags map. Returns nil for an empty/absent map. Sorted for
// determinism.
func tagFiltersFromMap(tags map[string]string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for k, v := range tags {
		out = append(out, fmt.Sprintf("tag:%s=%s", k, v))
	}
	sort.Strings(out)
	return out
}

// servicesToListValue maps a slice of api.Service into the shared list value
// used by the plural data source's "services" attribute. Extracted from Read
// so the mapping is independently testable. A nil/empty slice yields a known
// empty list, not a null one.
func servicesToListValue(ctx context.Context, items []api.Service) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	elems := make([]attr.Value, 0, len(items))
	for _, it := range items {
		obj, d := serviceToObjectValue(ctx, it)
		diags.Append(d...)
		elems = append(elems, obj)
	}
	list, d := types.ListValue(serviceObjectType(), elems)
	diags.Append(d...)
	return list, diags
}

func (d *servicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Data Source Configure Type", "Expected api.Client, got something else. Please report this issue to the provider developers.")
		return
	}
	d.client = client
}

func (d *servicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_services"
}

func (d *servicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: servicesDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"tags": schema.MapAttribute{
				Description: "Optional tag filter. Each key/value becomes an API filter `tag:Key=Value`. Only services matching all tags are returned.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"services": schema.ListNestedAttribute{
				Description:  "All services in the organization (optionally tag-filtered).",
				Computed:     true,
				NestedObject: schema.NestedAttributeObject{Attributes: serviceComputedAttributesWithID()},
			},
		},
	}
}

func (d *servicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg servicesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tagMap map[string]string
	if !cfg.Tags.IsNull() {
		resp.Diagnostics.Append(cfg.Tags.ElementsAs(ctx, &tagMap, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	items, err := d.client.ListServices(ctx, tagFiltersFromMap(tagMap))
	if err != nil {
		resp.Diagnostics.AddError("Error listing services", "Could not list services: "+err.Error())
		return
	}

	list, diags := servicesToListValue(ctx, items)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &servicesDataSourceModel{Tags: cfg.Tags, Services: list})...)
}
