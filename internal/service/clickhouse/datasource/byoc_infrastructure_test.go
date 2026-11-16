package datasource

import (
	"context"
	"errors"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func boolPointer(v bool) *bool       { return &v }
func stringPointer(v string) *string { return &v }

func TestByocInfrastructureDataSourceSchema(t *testing.T) {
	ctx := context.Background()
	d := NewByocInfrastructureDataSource().(*byocInfrastructureDataSource)
	resp := datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid schema implementation: %v", diags)
	}
}

func TestByocInfrastructureDetailsToModel_MapsAllFields(t *testing.T) {
	details := &api.ByocInfrastructureDetails{
		Id:                           "byoc-1",
		State:                        api.ByocStateReady,
		AccountId:                    "123456789012",
		RegionId:                     "us-east-1",
		CloudProvider:                "aws",
		DisplayName:                  "prod-byoc",
		EnablePrivateLink:            boolPointer(true),
		EnablePrivateLoadBalancer:    boolPointer(false),
		GcpPscSubnetId:               stringPointer("psc-subnet"),
		VpcCidrRange:                 stringPointer("10.0.0.0/16"),
		VpcAvailabilityZoneList:      []string{"us-east-1a", "us-east-1b"},
		IsByoVpc:                     boolPointer(true),
		ByoVpcId:                     stringPointer("vpc-123"),
		ByoVpcPrivateSubnetIds:       []string{"subnet-1", "subnet-2"},
		ByoVpcSharedVpcHostProjectId: stringPointer("host-project"),
	}

	var model byocInfrastructureDataSourceModel
	diags := byocInfrastructureDetailsToModel(context.Background(), details, map[string]string{"env": "prod"}, &model)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}

	if got := model.ID.ValueString(); got != "byoc-1" {
		t.Errorf("id = %q; want byoc-1", got)
	}
	if got := model.State.ValueString(); got != api.ByocStateReady {
		t.Errorf("state = %q; want %q", got, api.ByocStateReady)
	}
	if !model.EnablePrivateLink.ValueBool() {
		t.Error("enable_private_link should be true")
	}
	if model.EnablePrivateLoadBalancer.ValueBool() {
		t.Error("enable_private_load_balancer should be false")
	}
	if !model.EnablePublicLoadBalancer.IsNull() {
		t.Error("enable_public_load_balancer should be null when absent")
	}
	if got := model.VpcCidrRange.ValueString(); got != "10.0.0.0/16" {
		t.Errorf("vpc_cidr_range = %q", got)
	}
	if got := len(model.VpcAvailabilityZoneList.Elements()); got != 2 {
		t.Errorf("vpc_availability_zone_list length = %d; want 2", got)
	}
	if got := model.ByoVpcID.ValueString(); got != "vpc-123" {
		t.Errorf("byo_vpc_id = %q", got)
	}
	if got := len(model.ByoVpcPrivateSubnetIDs.Elements()); got != 2 {
		t.Errorf("byo_vpc_private_subnet_ids length = %d; want 2", got)
	}
	if model.ByoVpcPodCidrRangeNames.IsNull() || len(model.ByoVpcPodCidrRangeNames.Elements()) != 0 {
		t.Errorf("byo_vpc_pod_cidr_range_names = %v; want known empty list", model.ByoVpcPodCidrRangeNames)
	}
	if got := len(model.Tags.Elements()); got != 1 {
		t.Errorf("tags length = %d; want 1", got)
	}
}

func TestByocInfrastructureDetailsToModel_NilTagsIsKnownEmptyMap(t *testing.T) {
	details := &api.ByocInfrastructureDetails{Id: "byoc-1", State: api.ByocStateProvisioning}

	var model byocInfrastructureDataSourceModel
	diags := byocInfrastructureDetailsToModel(context.Background(), details, nil, &model)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if model.Tags.IsNull() || model.Tags.IsUnknown() || len(model.Tags.Elements()) != 0 {
		t.Errorf("tags = %v; want known empty map", model.Tags)
	}
	if !model.EnablePrivateLink.IsNull() {
		t.Error("enable_private_link should be null when absent")
	}
}

func byocInfraReadSetup(t *testing.T, privateLink bool) (*byocInfrastructureDataSource, datasource.ReadRequest, *datasource.ReadResponse, *api.ClientMock) {
	t.Helper()
	ctx := context.Background()
	d := NewByocInfrastructureDataSource().(*byocInfrastructureDataSource)
	schemaResp := datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", schemaResp.Diagnostics)
	}

	cfgModel := byocInfrastructureDataSourceModel{
		ID:                       types.StringValue("byoc-1"),
		VpcAvailabilityZoneList:  types.ListNull(types.StringType),
		ByoVpcPrivateSubnetIDs:   types.ListNull(types.StringType),
		ByoVpcPodCidrRangeNames:  types.ListNull(types.StringType),
		Tags:                     types.MapNull(types.StringType),
		PrivateEndpointConfig:    types.ObjectNull(byocPrivateEndpointConfigObjectType().AttrTypes),
		CustomerAuthorizedAccess: types.ObjectNull(byocCustomerAuthorizedAccessObjectType().AttrTypes),
	}
	cfgState := tfsdk.State{Schema: schemaResp.Schema}
	if diags := cfgState.Set(ctx, &cfgModel); diags.HasError() {
		t.Fatalf("set config: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetByocInfrastructureMock.
		Expect(ctx, "byoc-1").
		Return(&api.ByocInfrastructureDetails{
			Id:                "byoc-1",
			State:             api.ByocStateReady,
			AccountId:         "123456789012",
			RegionId:          "us-east-1",
			CloudProvider:     "aws",
			DisplayName:       "prod-byoc",
			EnablePrivateLink: boolPointer(privateLink),
		}, nil)
	client.GetByocInfrastructureTagsMock.
		Expect(ctx, "byoc-1").
		Return(map[string]string{}, nil)
	client.GetByocInfrastructureCustomerAuthorizedAccessMock.
		Optional().
		Expect(ctx, "byoc-1").
		Return(nil, errors.New("status: 404, body: BYOC infrastructure not found"))
	d.client = client

	req := datasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: cfgState.Raw}}
	resp := &datasource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	return d, req, resp, client
}

func TestByocInfrastructureDataSourceReadSkipsEndpointConfigWhenPrivateLinkDisabled(t *testing.T) {
	ctx := context.Background()
	d, req, resp, client := byocInfraReadSetup(t, false)

	d.Read(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	if n := len(client.GetByocInfrastructurePrivateEndpointConfigMock.Calls()); n != 0 {
		t.Errorf("private endpoint config calls = %d; want none when private link is disabled", n)
	}

	var got byocInfrastructureDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if !got.PrivateEndpointConfig.IsNull() {
		t.Errorf("private_endpoint_config = %v; want null", got.PrivateEndpointConfig)
	}
}

func TestByocInfrastructureDataSourceReadFetchesEndpointConfigWhenPrivateLinkEnabled(t *testing.T) {
	ctx := context.Background()
	d, req, resp, client := byocInfraReadSetup(t, true)
	client.GetByocInfrastructurePrivateEndpointConfigMock.
		Expect(ctx, "byoc-1").
		Return(&api.ByocInfrastructurePrivateEndpointConfig{
			EndpointName:       "com.amazonaws.vpce.us-east-1.vpce-svc-1",
			PrivateDnsHostname: "byoc-1.us-east-1.vpce.clickhouse.cloud",
		}, nil)

	d.Read(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var got byocInfrastructureDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	attrs := got.PrivateEndpointConfig.Attributes()
	if got := attrs["endpoint_name"].(types.String).ValueString(); got != "com.amazonaws.vpce.us-east-1.vpce-svc-1" {
		t.Errorf("endpoint_name = %q", got)
	}
	if got := attrs["private_dns_hostname"].(types.String).ValueString(); got != "byoc-1.us-east-1.vpce.clickhouse.cloud" {
		t.Errorf("private_dns_hostname = %q", got)
	}
}

func TestByocInfrastructureDataSourceReadTreatsEndpointConfig404AsNull(t *testing.T) {
	ctx := context.Background()
	d, req, resp, client := byocInfraReadSetup(t, true)
	client.GetByocInfrastructurePrivateEndpointConfigMock.
		Expect(ctx, "byoc-1").
		Return(nil, errors.New("status: 404, body: not found"))

	d.Read(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 during provisioning must not be an error: %v", resp.Diagnostics)
	}

	var got byocInfrastructureDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if !got.PrivateEndpointConfig.IsNull() {
		t.Errorf("private_endpoint_config = %v; want null on provisioning 404", got.PrivateEndpointConfig)
	}
}

func TestByocInfrastructureDataSourceReadTreatsCustomerAccess404AsNull(t *testing.T) {
	ctx := context.Background()
	d, req, resp, _ := byocInfraReadSetup(t, false)

	d.Read(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 for a disabled feature must not be an error: %v", resp.Diagnostics)
	}

	var got byocInfrastructureDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if !got.CustomerAuthorizedAccess.IsNull() {
		t.Errorf("customer_authorized_access = %v; want null when the feature is off", got.CustomerAuthorizedAccess)
	}
}

func TestByocInfrastructureDataSourceReadPopulatesCustomerAccess(t *testing.T) {
	ctx := context.Background()
	d, req, resp, client := byocInfraReadSetup(t, false)
	start := "2026-06-01T12:00:00.000Z"
	end := "2026-06-01T16:00:00.000Z"
	client.GetByocInfrastructureCustomerAuthorizedAccessMock.
		Expect(ctx, "byoc-1").
		Return(&api.ByocInfrastructureCustomerAuthorizedAccess{
			Enabled:            true,
			DurationSeconds:    14400,
			StartTimestamp:     &start,
			Allowed:            true,
			WindowEndTimestamp: &end,
		}, nil)

	d.Read(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diags: %v", resp.Diagnostics)
	}

	var got byocInfrastructureDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	attrs := got.CustomerAuthorizedAccess.Attributes()
	if got := attrs["enabled"].(types.Bool).ValueBool(); !got {
		t.Error("enabled = false; want true")
	}
	if got := attrs["duration_seconds"].(types.Int64).ValueInt64(); got != 14400 {
		t.Errorf("duration_seconds = %d; want 14400", got)
	}
	if got := attrs["start_timestamp"].(types.String).ValueString(); got != start {
		t.Errorf("start_timestamp = %q; want %q", got, start)
	}
	if got := attrs["allowed"].(types.Bool).ValueBool(); !got {
		t.Error("allowed = false; want true")
	}
	if got := attrs["window_end_timestamp"].(types.String).ValueString(); got != end {
		t.Errorf("window_end_timestamp = %q; want %q", got, end)
	}
}

func TestByocCustomerAuthorizedAccessToObjectValueNullsAbsentTimestamps(t *testing.T) {
	obj, diags := byocCustomerAuthorizedAccessToObjectValue(&api.ByocInfrastructureCustomerAuthorizedAccess{Enabled: false})
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	attrs := obj.Attributes()
	if !attrs["start_timestamp"].IsNull() || !attrs["window_end_timestamp"].IsNull() {
		t.Errorf("timestamps should be null when absent, got %v", attrs)
	}
}

func TestByocPrivateEndpointConfigToObjectValue(t *testing.T) {
	obj, diags := byocPrivateEndpointConfigToObjectValue(nil)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if !obj.IsNull() {
		t.Error("object should be null for nil config")
	}

	obj, diags = byocPrivateEndpointConfigToObjectValue(&api.ByocInfrastructurePrivateEndpointConfig{
		EndpointName:       "com.amazonaws.vpce.us-east-1.vpce-svc-1",
		PrivateDnsHostname: "byoc-1.us-east-1.vpce.clickhouse.cloud",
	})
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	attrs := obj.Attributes()
	if got := attrs["endpoint_name"].(types.String).ValueString(); got != "com.amazonaws.vpce.us-east-1.vpce-svc-1" {
		t.Errorf("endpoint_name = %q", got)
	}
	if got := attrs["private_dns_hostname"].(types.String).ValueString(); got != "byoc-1.us-east-1.vpce.clickhouse.cloud" {
		t.Errorf("private_dns_hostname = %q", got)
	}
}
