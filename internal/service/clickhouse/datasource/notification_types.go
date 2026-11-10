package datasource

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

//go:embed descriptions/notification_types.md
var notificationTypesDataSourceDescription string

const notificationTypesDataSourceName = "clickhouse_notification_types"

var _ datasource.DataSource = &notificationTypesDataSource{}

func NewNotificationTypesDataSource() datasource.DataSource {
	return &notificationTypesDataSource{}
}

type notificationTypesDataSource struct {
	client api.Client
}

type notificationTypesDataSourceModel struct {
	Types types.List `tfsdk:"types"`
}

var notificationTypeAttrTypes = map[string]attr.Type{
	"name":            types.StringType,
	"title":           types.StringType,
	"description":     types.StringType,
	"category":        types.StringType,
	"severity":        types.StringType,
	"resource_type":   types.StringType,
	"is_configurable": types.BoolType,
	"is_available":    types.BoolType,
}

func (d *notificationTypesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*service.ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("expected *service.ProviderData, got %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	if providerData.API == nil {
		resp.Diagnostics.AddError("ClickHouse Cloud API not configured",
			"This resource requires ClickHouse Cloud credentials. Set organization_id, token_key and token_secret on the provider (or the corresponding CLICKHOUSE_* environment variables).")
		return
	}
	d.client = providerData.API
}

func (d *notificationTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_types"
}

func (d *notificationTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: notificationTypesDataSourceDescription,
		Attributes: map[string]schema.Attribute{
			"types": schema.ListNestedAttribute{
				Description: "Notification types the organization is entitled to.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Stable identifier of the notification type, as used in a clickhouse_notification_settings rule. Never renamed or reused.",
							Computed:    true,
						},
						"title": schema.StringAttribute{
							Description: "Human-readable name of the notification type.",
							Computed:    true,
						},
						"description": schema.StringAttribute{
							Description: "Longer explanation of what the notification reports. Empty for types that have none.",
							Computed:    true,
						},
						"category": schema.StringAttribute{
							Description: "Broad grouping the notification type belongs to, such as 'billing' or 'service'. New values may be added over time.",
							Computed:    true,
						},
						"severity": schema.StringAttribute{
							Description: "Relative importance of notifications of this type: 'info', 'warning' or 'critical'.",
							Computed:    true,
						},
						"resource_type": schema.StringAttribute{
							Description: "Kind of resource a notification of this type is about, which is the kind of ID a rule naming this type takes. New values may be added over time.",
							Computed:    true,
						},
						"is_configurable": schema.BoolAttribute{
							Description: "Whether the delivery default can be changed. A rule naming a type that is not configurable is rejected. False does not mean the notification is always delivered.",
							Computed:    true,
						},
						"is_available": schema.BoolAttribute{
							Description: "False when the API key reading this data source lacks the permission the type requires. The type is still listed, just flagged.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *notificationTypesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	utils.BetaWarning(notificationTypesDataSourceName, &resp.Diagnostics)

	var data notificationTypesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	notificationTypes, err := d.client.ListNotificationTypes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing notification types", "Could not list notification types: "+err.Error())
		return
	}

	typeValues := make([]attr.Value, len(notificationTypes))
	for i, notificationType := range notificationTypes {
		obj, diags := types.ObjectValue(notificationTypeAttrTypes, map[string]attr.Value{
			"name":            types.StringValue(notificationType.Name),
			"title":           types.StringValue(notificationType.Title),
			"description":     types.StringValue(notificationType.Description),
			"category":        types.StringValue(notificationType.Category),
			"severity":        types.StringValue(notificationType.Severity),
			"resource_type":   types.StringValue(notificationType.ResourceType),
			"is_configurable": types.BoolValue(notificationType.IsConfigurable),
			"is_available":    types.BoolValue(notificationType.IsAvailable),
		})
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		typeValues[i] = obj
	}

	typesList, diags := types.ListValue(types.ObjectType{AttrTypes: notificationTypeAttrTypes}, typeValues)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Types = typesList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
