package resource

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

func byocReadySetup(t *testing.T) (*ByocInfrastructureReadyResource, resource.SchemaResponse, tfsdk.Plan) {
	t.Helper()
	r := NewByocInfrastructureReadyResource().(*ByocInfrastructureReadyResource)
	schemaResp := resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", schemaResp.Diagnostics)
	}
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	model := byocInfrastructureReadyModel{
		ID:     types.StringUnknown(),
		ByocID: types.StringValue(byocInfraID),
		State:  types.StringUnknown(),
	}
	if diags := plan.Set(context.Background(), &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}
	return r, schemaResp, plan
}

func TestByocReadyWaitDone(t *testing.T) {
	for state, want := range map[string]bool{
		api.ByocStateAwaitingConnector: false,
		api.ByocStateProvisioning:      false,
		api.ByocStateUpgrading:         false,
		"infra-some-future-state":      false,
		api.ByocStateReady:             true,
		api.ByocStateDegraded:          true,
		api.ByocStateTerminating:       true,
		api.ByocStateTerminated:        true,
	} {
		if got := byocReadyWaitDone(state); got != want {
			t.Errorf("byocReadyWaitDone(%q) = %v; want %v", state, got, want)
		}
	}
}

func TestByocInfrastructureReadyCreateWaitsForReady(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp, plan := byocReadySetup(t)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.WaitForByocInfrastructureStateMock.
		Set(func(_ context.Context, byocId string, _ func(string) bool, maxWaitSeconds int) error {
			if byocId != byocInfraID || maxWaitSeconds != byocReadyWaitSeconds {
				t.Errorf("wait(%q, %d); want (%q, %d)", byocId, maxWaitSeconds, byocInfraID, byocReadyWaitSeconds)
			}
			return nil
		})
	client.GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(byocRestrictedDetails(api.ByocStateReady), nil)
	r.client = client

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	var state byocInfrastructureReadyModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.ID.ValueString() != byocInfraID || state.State.ValueString() != api.ByocStateReady {
		t.Errorf("state = %+v; want id = byoc_id and state ready", state)
	}
}

func TestByocInfrastructureReadyCreateFailsOnDegraded(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp, plan := byocReadySetup(t)

	details := byocRestrictedDetails(api.ByocStateDegraded)
	details.Connector = &api.ByocConnector{PlatformDetail: strPtr("grant the executor role access to the cluster")}
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.WaitForByocInfrastructureStateMock.Return(nil)
	client.GetByocInfrastructureMock.Return(details, nil)
	r.client = client

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create should fail when the infrastructure degrades")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, api.ByocStateDegraded) || !strings.Contains(detail, "grant the executor role") {
		t.Errorf("detail = %q; want the state and the connector platform detail", detail)
	}
}

func TestByocInfrastructureReadyCreateReportsWaitTimeout(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp, plan := byocReadySetup(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		WaitForByocInfrastructureStateMock.
		Return(errors.New("BYOC infrastructure is in state infra-awaiting-connector"))

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create should fail when the wait times out")
	}
	if detail := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(detail, "enrollment token") {
		t.Errorf("detail = %q; want the connector hint", detail)
	}
}
