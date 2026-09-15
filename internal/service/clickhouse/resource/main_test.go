package resource

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

// Create and Update on the beta resources in this package prepend a beta
// notice, which would otherwise shift every diagnostic assertion below it.
// The notice itself is covered by TestBetaWarning in internal/utils.
func TestMain(m *testing.M) {
	os.Setenv(utils.SuppressBetaWarningsEnvVar, "1")
	os.Exit(m.Run())
}

// The package-wide suppression above means a test here sees no beta notice
// unless it opts back in, as this one does. It also covers the notice reaching
// Create/Update/ImportState in this package, which nothing else asserts (#696).
func TestBetaNoticeEmittedWhenNotSuppressed(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "false")

	ctx := context.Background()
	r := &UDFAttachmentResource{}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building resource schema failed: %v", schemaResp.Diagnostics.Errors())
	}
	sch := schemaResp.Schema
	nullRaw := tftypes.NewValue(sch.Type().TerraformType(ctx), nil)

	resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: nullRaw}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: nullRaw}}, resp)

	for _, d := range resp.Diagnostics.Warnings() {
		if d.Summary() == "Beta Resource" {
			return
		}
	}
	t.Errorf("Create emitted no beta notice; diagnostics = %v", resp.Diagnostics)
}
