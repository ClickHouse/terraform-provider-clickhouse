package datasource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func TestByocInfrastructuresDataSourceSchema(t *testing.T) {
	ctx := context.Background()
	d := NewByocInfrastructuresDataSource().(*byocInfrastructuresDataSource)
	resp := datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("invalid schema implementation: %v", diags)
	}
}

func TestByocInfrastructuresToListValue(t *testing.T) {
	list, diags := byocInfrastructuresToListValue([]api.ByocInfrastructure{
		{
			Id:            "byoc-1",
			State:         api.ByocStateReady,
			AccountId:     "123456789012",
			RegionId:      "us-east-1",
			CloudProvider: "aws",
			DisplayName:   "prod-byoc",
		},
		{
			Id:            "byoc-2",
			State:         api.ByocStateProvisioning,
			AccountId:     "gcp-project",
			RegionId:      "us-central1",
			CloudProvider: "gcp",
		},
	})
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if got := len(list.Elements()); got != 2 {
		t.Fatalf("infrastructures length = %d; want 2", got)
	}
}

func TestByocInfrastructuresToListValue_EmptyIsKnownEmptyList(t *testing.T) {
	list, diags := byocInfrastructuresToListValue(nil)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if list.IsNull() || list.IsUnknown() || len(list.Elements()) != 0 {
		t.Errorf("infrastructures = %v; want known empty list", list)
	}
}

func TestByocInfrastructuresDataSourceRead(t *testing.T) {
	ctx := context.Background()
	d := NewByocInfrastructuresDataSource().(*byocInfrastructuresDataSource)
	schemaResp := datasource.SchemaResponse{}
	d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", schemaResp.Diagnostics)
	}

	mc := minimock.NewController(t)
	d.client = api.NewClientMock(mc).
		GetOrganizationMock.
		Expect(ctx).
		Return(&api.OrgResult{
			ID: "org-1",
			ByocConfig: []api.ByocInfrastructure{
				{Id: "byoc-1", State: api.ByocStateReady, AccountId: "123456789012", RegionId: "us-east-1", CloudProvider: "aws", DisplayName: "prod-byoc"},
			},
		}, nil)

	resp := datasource.ReadResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
	}
	d.Read(ctx, datasource.ReadRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var got byocInfrastructuresDataSourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if n := len(got.Infrastructures.Elements()); n != 1 {
		t.Fatalf("infrastructures length = %d; want 1", n)
	}
}
