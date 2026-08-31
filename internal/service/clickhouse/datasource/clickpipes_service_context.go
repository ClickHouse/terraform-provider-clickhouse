package datasource

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

const clickPipesServiceContextDefaultReadyTimeout = 30 * time.Second

//go:embed descriptions/clickpipes_service_context.md
var clickPipesServiceContextDataSourceDescription string

var _ datasource.DataSource = &clickPipesServiceContextDataSource{}

// NewClickPipesServiceContextDataSource creates a ClickPipes service context data source.
func NewClickPipesServiceContextDataSource() datasource.DataSource {
	return &clickPipesServiceContextDataSource{}
}

type clickPipesServiceContextDataSource struct {
	client api.Client
}

type clickPipesServiceContextDataSourceModel struct {
	ServiceID           types.String `tfsdk:"service_id"`
	WaitForIdentity     types.Bool   `tfsdk:"wait_for_identity"`
	ReadyTimeout        types.String `tfsdk:"ready_timeout"`
	GCPWorkloadIdentity types.Object `tfsdk:"gcp_workload_identity"`
}

func clickPipesGCPWorkloadIdentityObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"supported": types.BoolType,
		"ready":     types.BoolType,
		"principal": types.StringType,
	}}
}

func (d *clickPipesServiceContextDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickpipes_service_context"
}

func (d *clickPipesServiceContextDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: clickPipesServiceContextDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Cloud service ID whose ClickPipes context should be retrieved.",
				Required:    true,
			},
			"wait_for_identity": schema.BoolAttribute{
				Description: "Wait until GCP workload identity is ready and has a principal. Defaults to true.",
				Optional:    true,
				Computed:    true,
			},
			"ready_timeout": schema.StringAttribute{
				Description: "Maximum duration to wait for GCP workload identity readiness. Defaults to 30s. Uses Go duration syntax, for example 2m or 1m30s.",
				Optional:    true,
				Computed:    true,
			},
			"gcp_workload_identity": schema.SingleNestedAttribute{
				Description: "GCP workload identity capability and tenant service account principal.",
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					"supported": schema.BoolAttribute{Description: "Whether the ClickPipes deployment supports GCP workload identity.", Computed: true},
					"ready":     schema.BoolAttribute{Description: "Whether the service tenant identity is ready.", Computed: true},
					"principal": schema.StringAttribute{Description: "GCP service account to grant access to source resources.", Computed: true},
				},
			},
		},
	}
}

func (d *clickPipesServiceContextDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError("ClickHouse Cloud API not configured", "This data source requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).")
		return
	}
	d.client = providerData.API
}

func (d *clickPipesServiceContextDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	utils.BetaWarning("clickhouse_clickpipes_service_context", &resp.Diagnostics)

	var data clickPipesServiceContextDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	waitForIdentity := true
	if !data.WaitForIdentity.IsNull() && !data.WaitForIdentity.IsUnknown() {
		waitForIdentity = data.WaitForIdentity.ValueBool()
	}
	readyTimeout := clickPipesServiceContextDefaultReadyTimeout
	readyTimeoutValue := clickPipesServiceContextDefaultReadyTimeout.String()
	if !data.ReadyTimeout.IsNull() && !data.ReadyTimeout.IsUnknown() {
		parsed, err := time.ParseDuration(data.ReadyTimeout.ValueString())
		if err != nil || parsed <= 0 {
			resp.Diagnostics.AddError("Invalid ready timeout", fmt.Sprintf("ready_timeout must be a positive Go duration such as 30s or 2m; got %q", data.ReadyTimeout.ValueString()))
			return
		}
		readyTimeout = parsed
		readyTimeoutValue = data.ReadyTimeout.ValueString()
	}

	var identity *api.ClickPipesGCPWorkloadIdentityContext
	var err error
	if waitForIdentity {
		identity, err = d.client.WaitForClickPipesGCPWorkloadIdentity(ctx, data.ServiceID.ValueString(), readyTimeout)
	} else {
		var serviceContext *api.ClickPipesServiceContext
		serviceContext, err = d.client.GetClickPipesServiceContext(ctx, data.ServiceID.ValueString())
		if serviceContext != nil {
			identity = &serviceContext.GCPWorkloadIdentity
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading ClickPipes service context", err.Error())
		return
	}

	data.WaitForIdentity = types.BoolValue(waitForIdentity)
	data.ReadyTimeout = types.StringValue(readyTimeoutValue)
	data.GCPWorkloadIdentity = types.ObjectValueMust(clickPipesGCPWorkloadIdentityObjectType().AttrTypes, map[string]attr.Value{
		"supported": types.BoolValue(identity.Supported),
		"ready":     types.BoolPointerValue(identity.Ready),
		"principal": types.StringPointerValue(identity.Principal),
	})
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
