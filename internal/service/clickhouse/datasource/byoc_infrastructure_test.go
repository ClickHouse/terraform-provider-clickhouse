package datasource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

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
