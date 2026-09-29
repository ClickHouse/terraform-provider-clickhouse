package clickstack

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The beta notice moved out of ValidateConfig, which no test could miss because
// it ran on every plan, into the three operations that bring a beta resource
// under management or change it. Nothing else asserts it reaches them, so a
// dropped call would otherwise leave the suite green (#696).
func TestBetaNoticeEmittedByMutatingOperations(t *testing.T) {
	ctx := context.Background()
	r := &sourceResource{}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building resource schema failed: %v", schemaResp.Diagnostics.Errors())
	}
	sch := schemaResp.Schema
	nullRaw := tftypes.NewValue(sch.Type().TerraformType(ctx), nil)

	// Each operation is driven with a null plan/state so it errors out right
	// after the notice; the notice is emitted before any client call, so no
	// ClickStack API is needed to observe it.
	for name, emit := range map[string]func() []string{
		"Create": func() []string {
			resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: nullRaw}}
			r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: nullRaw}}, resp)
			return warningSummaries(resp.Diagnostics)
		},
		"Update": func() []string {
			resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: nullRaw}}
			r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: nullRaw}}, resp)
			return warningSummaries(resp.Diagnostics)
		},
		"ImportState": func() []string {
			resp := &resource.ImportStateResponse{State: tfsdk.State{Schema: sch, Raw: nullRaw}}
			r.ImportState(ctx, resource.ImportStateRequest{ID: "source-1"}, resp)
			return warningSummaries(resp.Diagnostics)
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := emit()
			for _, summary := range got {
				if summary == "Beta Resource" {
					return
				}
			}
			t.Errorf("%s emitted no beta notice; warnings = %v", name, got)
		})
	}
}

func warningSummaries(diags diag.Diagnostics) []string {
	summaries := make([]string, 0, len(diags.Warnings()))
	for _, d := range diags.Warnings() {
		summaries = append(summaries, d.Summary())
	}
	return summaries
}
