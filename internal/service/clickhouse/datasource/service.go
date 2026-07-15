package datasource

import (
	"context"
	_ "embed"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

//go:embed descriptions/service.md
var serviceDataSourceDescription string

var _ datasource.DataSource = &serviceDataSource{}

// ---- shared types ---------------------------------------------------------

func endpointObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"protocol": types.StringType,
		"host":     types.StringType,
		"port":     types.Int64Type,
	}}
}

func ipAccessObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"source":      types.StringType,
		"description": types.StringType,
	}}
}

// serviceObjectType is the shared object type for both data sources.
func serviceObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"id":                                 types.StringType,
		"name":                               types.StringType,
		"cloud_provider":                     types.StringType,
		"region":                             types.StringType,
		"tier":                               types.StringType,
		"state":                              types.StringType,
		"clickhouse_version":                 types.StringType,
		"created_at":                         types.StringType,
		"release_channel":                    types.StringType,
		"is_primary":                         types.BoolType,
		"readonly":                           types.BoolType,
		"compliance_type":                    types.StringType,
		"byoc_id":                            types.StringType,
		"warehouse_id":                       types.StringType,
		"num_replicas":                       types.Int64Type,
		"min_replicas":                       types.Int64Type,
		"max_replicas":                       types.Int64Type,
		"autoscaling_mode":                   types.StringType,
		"min_total_memory_gb":                types.Int64Type,
		"max_total_memory_gb":                types.Int64Type,
		"min_replica_memory_gb":              types.Int64Type,
		"max_replica_memory_gb":              types.Int64Type,
		"idle_scaling":                       types.BoolType,
		"idle_timeout_minutes":               types.Int64Type,
		"iam_role":                           types.StringType,
		"ip_access":                          types.ListType{ElemType: ipAccessObjectType()},
		"private_endpoint_ids":               types.ListType{ElemType: types.StringType},
		"encryption_key":                     types.StringType,
		"encryption_assumed_role_identifier": types.StringType,
		"has_transparent_data_encryption":    types.BoolType,
		"transparent_data_encryption_key_id": types.StringType,
		"encryption_role_id":                 types.StringType,
		"enable_core_dumps":                  types.BoolType,
		"endpoints":                          types.ListType{ElemType: endpointObjectType()},
		"tags":                               types.MapType{ElemType: types.StringType},
	}}
}

// ---- pointer helpers ------------------------------------------------------

func int64PtrOrNull(p *int) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*p))
}

func boolPtrOrNull(p *bool) types.Bool {
	if p == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*p)
}

func strPtrOrNull(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return strOrNull(*p)
}

// ---- mapping --------------------------------------------------------------

// serviceToObjectValue maps an api.Service to the shared object value. It is the
// single mapping used by both the singular and plural data sources.
func serviceToObjectValue(ctx context.Context, svc api.Service) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	// endpoints
	epElems := make([]attr.Value, 0, len(svc.Endpoints))
	for _, e := range svc.Endpoints {
		o, d := types.ObjectValue(endpointObjectType().AttrTypes, map[string]attr.Value{
			"protocol": types.StringValue(e.Protocol),
			"host":     types.StringValue(e.Host),
			"port":     types.Int64Value(int64(e.Port)),
		})
		diags.Append(d...)
		epElems = append(epElems, o)
	}
	endpoints, d := types.ListValue(endpointObjectType(), epElems)
	diags.Append(d...)

	// ip access
	ipElems := make([]attr.Value, 0, len(svc.IpAccessList))
	for _, ip := range svc.IpAccessList {
		o, d := types.ObjectValue(ipAccessObjectType().AttrTypes, map[string]attr.Value{
			"source":      types.StringValue(ip.Source),
			"description": types.StringValue(ip.Description),
		})
		diags.Append(d...)
		ipElems = append(ipElems, o)
	}
	ipAccess, d := types.ListValue(ipAccessObjectType(), ipElems)
	diags.Append(d...)

	// private endpoint ids
	pidElems := make([]attr.Value, 0, len(svc.PrivateEndpointIds))
	for _, id := range svc.PrivateEndpointIds {
		pidElems = append(pidElems, types.StringValue(id))
	}
	privateEndpointIds, d := types.ListValue(types.StringType, pidElems)
	diags.Append(d...)

	// tags (reuse postgres helper)
	tags, d := apiTagsToStringMap(svc.Tags)
	diags.Append(d...)

	obj, d := types.ObjectValue(serviceObjectType().AttrTypes, map[string]attr.Value{
		"id":                                 types.StringValue(svc.Id),
		"name":                               types.StringValue(svc.Name),
		"cloud_provider":                     types.StringValue(svc.Provider),
		"region":                             types.StringValue(svc.Region),
		"tier":                               strOrNull(svc.Tier),
		"state":                              strOrNull(svc.State),
		"clickhouse_version":                 strOrNull(svc.ClickHouseVersion),
		"created_at":                         strOrNull(svc.CreatedAt),
		"release_channel":                    strOrNull(svc.ReleaseChannel),
		"is_primary":                         boolPtrOrNull(svc.IsPrimary),
		"readonly":                           types.BoolValue(svc.ReadOnly),
		"compliance_type":                    strPtrOrNull(svc.ComplianceType),
		"byoc_id":                            strPtrOrNull(svc.BYOCId),
		"warehouse_id":                       strPtrOrNull(svc.DataWarehouseId),
		"num_replicas":                       int64PtrOrNull(svc.NumReplicas),
		"min_replicas":                       int64PtrOrNull(svc.MinReplicas),
		"max_replicas":                       int64PtrOrNull(svc.MaxReplicas),
		"autoscaling_mode":                   strOrNull(svc.AutoscalingMode),
		"min_total_memory_gb":                int64PtrOrNull(svc.MinTotalMemoryGb),
		"max_total_memory_gb":                int64PtrOrNull(svc.MaxTotalMemoryGb),
		"min_replica_memory_gb":              int64PtrOrNull(svc.MinReplicaMemoryGb),
		"max_replica_memory_gb":              int64PtrOrNull(svc.MaxReplicaMemoryGb),
		"idle_scaling":                       types.BoolValue(svc.IdleScaling),
		"idle_timeout_minutes":               int64PtrOrNull(svc.IdleTimeoutMinutes),
		"iam_role":                           strOrNull(svc.IAMRole),
		"ip_access":                          ipAccess,
		"private_endpoint_ids":               privateEndpointIds,
		"encryption_key":                     strOrNull(svc.EncryptionKey),
		"encryption_assumed_role_identifier": strOrNull(svc.EncryptionAssumedRoleIdentifier),
		"has_transparent_data_encryption":    types.BoolValue(svc.HasTransparentDataEncryption),
		"transparent_data_encryption_key_id": strOrNull(svc.TransparentEncryptionDataKeyID),
		"encryption_role_id":                 strOrNull(svc.EncryptionRoleID),
		"enable_core_dumps":                  boolPtrOrNull(svc.EnableCoreDumps),
		"endpoints":                          endpoints,
		"tags":                               tags,
	})
	diags.Append(d...)
	return obj, diags
}

// serviceComputedAttributes returns the computed attribute schema shared by the
// singular data source (top level) and the plural data source (nested).
func serviceComputedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name":                  schema.StringAttribute{Description: "Human-readable name of the service.", Computed: true},
		"cloud_provider":        schema.StringAttribute{Description: "Cloud provider hosting the service ('aws', 'gcp', or 'azure').", Computed: true},
		"region":                schema.StringAttribute{Description: "Cloud region the service runs in.", Computed: true},
		"tier":                  schema.StringAttribute{Description: "Service tier.", Computed: true},
		"state":                 schema.StringAttribute{Description: "Current service state (e.g. 'running', 'idle', 'stopped').", Computed: true},
		"clickhouse_version":    schema.StringAttribute{Description: "ClickHouse version running on the service.", Computed: true},
		"created_at":            schema.StringAttribute{Description: "RFC3339 creation timestamp.", Computed: true},
		"release_channel":       schema.StringAttribute{Description: "Release channel ('default' or 'fast').", Computed: true},
		"is_primary":            schema.BoolAttribute{Description: "True for a primary service; false for a read replica.", Computed: true},
		"readonly":              schema.BoolAttribute{Description: "Whether the service is read-only.", Computed: true},
		"compliance_type":       schema.StringAttribute{Description: "Compliance regime the service was created under, if any (e.g. 'hipaa', 'pci').", Computed: true},
		"byoc_id":               schema.StringAttribute{Description: "Identifier of the BYOC (Bring Your Own Cloud) infrastructure hosting the service, if any.", Computed: true},
		"warehouse_id":          schema.StringAttribute{Description: "Identifier of the data warehouse this service belongs to, if any.", Computed: true},
		"num_replicas":          schema.Int64Attribute{Description: "Number of replicas.", Computed: true},
		"min_replicas":          schema.Int64Attribute{Description: "Minimum number of replicas (horizontal autoscaling).", Computed: true},
		"max_replicas":          schema.Int64Attribute{Description: "Maximum number of replicas (horizontal autoscaling).", Computed: true},
		"autoscaling_mode":      schema.StringAttribute{Description: "Autoscaling mode ('vertical' or 'horizontal').", Computed: true},
		"min_total_memory_gb":   schema.Int64Attribute{Description: "Minimum total memory across all replicas, in GB.", Computed: true},
		"max_total_memory_gb":   schema.Int64Attribute{Description: "Maximum total memory across all replicas, in GB.", Computed: true},
		"min_replica_memory_gb": schema.Int64Attribute{Description: "Minimum memory per replica, in GB.", Computed: true},
		"max_replica_memory_gb": schema.Int64Attribute{Description: "Maximum memory per replica, in GB.", Computed: true},
		"idle_scaling":          schema.BoolAttribute{Description: "Whether the service scales down when idle.", Computed: true},
		"idle_timeout_minutes":  schema.Int64Attribute{Description: "Minutes of inactivity before the service idles.", Computed: true},
		"iam_role":              schema.StringAttribute{Description: "AWS IAM role the service assumes to access external resources.", Computed: true},
		"ip_access": schema.ListNestedAttribute{Description: "IP allow-list entries permitted to connect to the service.", Computed: true, NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"source":      schema.StringAttribute{Description: "CIDR or IP address allowed to connect.", Computed: true},
				"description": schema.StringAttribute{Description: "Description of the allow-list entry.", Computed: true},
			},
		}},
		"private_endpoint_ids":               schema.ListAttribute{Description: "IDs of private endpoints attached to the service.", Computed: true, ElementType: types.StringType},
		"encryption_key":                     schema.StringAttribute{Description: "Customer-managed encryption key (CMK) protecting the service, if any.", Computed: true},
		"encryption_assumed_role_identifier": schema.StringAttribute{Description: "IAM role assumed to use the customer-managed encryption key, if any.", Computed: true},
		"has_transparent_data_encryption":    schema.BoolAttribute{Description: "Whether Transparent Data Encryption (TDE) is enabled.", Computed: true},
		"transparent_data_encryption_key_id": schema.StringAttribute{Description: "Key ID used for Transparent Data Encryption, if enabled.", Computed: true},
		"encryption_role_id":                 schema.StringAttribute{Description: "Role ID used for encryption, if any.", Computed: true},
		"enable_core_dumps":                  schema.BoolAttribute{Description: "Whether core dumps are enabled for debugging.", Computed: true},
		"endpoints": schema.ListNestedAttribute{Description: "Network endpoints exposed by the service.", Computed: true, NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"protocol": schema.StringAttribute{Description: "Endpoint protocol (e.g. 'nativesecure', 'https', 'mysql').", Computed: true},
				"host":     schema.StringAttribute{Description: "Endpoint host.", Computed: true},
				"port":     schema.Int64Attribute{Description: "Endpoint port.", Computed: true},
			},
		}},
		"tags": schema.MapAttribute{Description: "User-defined key/value tags on the service.", Computed: true, ElementType: types.StringType},
	}
}

// serviceComputedAttributesWithID is the shared computed attribute set plus a
// computed id, used by the plural data source's nested list objects and by the
// schema/object-type consistency test.
func serviceComputedAttributesWithID() map[string]schema.Attribute {
	attrs := serviceComputedAttributes()
	attrs["id"] = schema.StringAttribute{Description: "Unique identifier of the service.", Computed: true}
	return attrs
}

// ---- singular data source -------------------------------------------------

func NewServiceDataSource() datasource.DataSource { return &serviceDataSource{} }

type serviceDataSource struct{ client api.Client }

func (d *serviceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *serviceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

func (d *serviceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := serviceComputedAttributes()
	attrs["id"] = schema.StringAttribute{Description: "Unique identifier of the service to look up.", Required: true}
	resp.Schema = schema.Schema{
		MarkdownDescription: serviceDataSourceDescription,
		Attributes:          attrs,
	}
}

// serviceDataSourceModel mirrors the shared object type as a struct so state is
// set via the battle-tested resp.State.Set idiom (matching postgres_service.go),
// not a per-attribute SetAttribute loop. Populated from the shared object via
// obj.As, so the mapping stays DRY (single serviceToObjectValue).
type serviceDataSourceModel struct {
	ID                             types.String `tfsdk:"id"`
	Name                           types.String `tfsdk:"name"`
	CloudProvider                  types.String `tfsdk:"cloud_provider"`
	Region                         types.String `tfsdk:"region"`
	Tier                           types.String `tfsdk:"tier"`
	State                          types.String `tfsdk:"state"`
	ClickHouseVersion              types.String `tfsdk:"clickhouse_version"`
	CreatedAt                      types.String `tfsdk:"created_at"`
	ReleaseChannel                 types.String `tfsdk:"release_channel"`
	IsPrimary                      types.Bool   `tfsdk:"is_primary"`
	ReadOnly                       types.Bool   `tfsdk:"readonly"`
	ComplianceType                 types.String `tfsdk:"compliance_type"`
	BYOCID                         types.String `tfsdk:"byoc_id"`
	WarehouseID                    types.String `tfsdk:"warehouse_id"`
	NumReplicas                    types.Int64  `tfsdk:"num_replicas"`
	MinReplicas                    types.Int64  `tfsdk:"min_replicas"`
	MaxReplicas                    types.Int64  `tfsdk:"max_replicas"`
	AutoscalingMode                types.String `tfsdk:"autoscaling_mode"`
	MinTotalMemoryGb               types.Int64  `tfsdk:"min_total_memory_gb"`
	MaxTotalMemoryGb               types.Int64  `tfsdk:"max_total_memory_gb"`
	MinReplicaMemoryGb             types.Int64  `tfsdk:"min_replica_memory_gb"`
	MaxReplicaMemoryGb             types.Int64  `tfsdk:"max_replica_memory_gb"`
	IdleScaling                    types.Bool   `tfsdk:"idle_scaling"`
	IdleTimeoutMinutes             types.Int64  `tfsdk:"idle_timeout_minutes"`
	IAMRole                        types.String `tfsdk:"iam_role"`
	IpAccess                       types.List   `tfsdk:"ip_access"`
	PrivateEndpointIds             types.List   `tfsdk:"private_endpoint_ids"`
	EncryptionKey                  types.String `tfsdk:"encryption_key"`
	EncryptionAssumedRoleID        types.String `tfsdk:"encryption_assumed_role_identifier"`
	HasTransparentDataEncryption   types.Bool   `tfsdk:"has_transparent_data_encryption"`
	TransparentDataEncryptionKeyID types.String `tfsdk:"transparent_data_encryption_key_id"`
	EncryptionRoleID               types.String `tfsdk:"encryption_role_id"`
	EnableCoreDumps                types.Bool   `tfsdk:"enable_core_dumps"`
	Endpoints                      types.List   `tfsdk:"endpoints"`
	Tags                           types.Map    `tfsdk:"tags"`
}

func (d *serviceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg serviceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	svc, err := d.client.GetServiceBase(ctx, cfg.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading service", "Could not read service "+cfg.ID.ValueString()+": "+err.Error())
		return
	}

	obj, diags := serviceToObjectValue(ctx, *svc)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var data serviceDataSourceModel
	resp.Diagnostics.Append(obj.As(ctx, &data, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
