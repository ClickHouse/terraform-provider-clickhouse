package datasource

import (
	"context"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

func contextBoolPtr(value bool) *bool       { return &value }
func contextStringPtr(value string) *string { return &value }

func clickPipesServiceContextConfig(waitForIdentity *bool, readyTimeout *string) tftypes.Value {
	objectType := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"service_id":            tftypes.String,
		"wait_for_identity":     tftypes.Bool,
		"ready_timeout":         tftypes.String,
		"gcp_workload_identity": tftypes.Object{AttributeTypes: map[string]tftypes.Type{"supported": tftypes.Bool, "ready": tftypes.Bool, "principal": tftypes.String}},
	}}
	values := map[string]tftypes.Value{
		"service_id":            tftypes.NewValue(tftypes.String, "svc-1"),
		"wait_for_identity":     tftypes.NewValue(tftypes.Bool, waitForIdentity),
		"ready_timeout":         tftypes.NewValue(tftypes.String, readyTimeout),
		"gcp_workload_identity": tftypes.NewValue(objectType.AttributeTypes["gcp_workload_identity"], nil),
	}
	return tftypes.NewValue(objectType, values)
}

func readClickPipesServiceContextDataSource(t *testing.T, d *clickPipesServiceContextDataSource, config tftypes.Value) (*fwdatasource.ReadResponse, clickPipesServiceContextDataSourceModel) {
	t.Helper()
	schemaResp := &fwdatasource.SchemaResponse{}
	d.Schema(context.Background(), fwdatasource.SchemaRequest{}, schemaResp)
	resp := &fwdatasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	d.Read(context.Background(), fwdatasource.ReadRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: config}}, resp)
	var state clickPipesServiceContextDataSourceModel
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Get(context.Background(), &state)...)
	}
	return resp, state
}

func TestClickPipesServiceContextDataSource_DefaultWaitsThirtySeconds(t *testing.T) {
	mc := minimock.NewController(t)
	mock := api.NewClientMock(mc)
	mock.WaitForClickPipesGCPWorkloadIdentityMock.Set(func(_ context.Context, serviceID string, timeout time.Duration) (*api.ClickPipesGCPWorkloadIdentityContext, error) {
		if serviceID != "svc-1" || timeout != 30*time.Second {
			t.Fatalf("serviceID=%q timeout=%s", serviceID, timeout)
		}
		return &api.ClickPipesGCPWorkloadIdentityContext{Supported: true, Ready: contextBoolPtr(true), Principal: contextStringPtr("tenant@example.com")}, nil
	})

	resp, state := readClickPipesServiceContextDataSource(t, &clickPipesServiceContextDataSource{client: mock}, clickPipesServiceContextConfig(nil, nil))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
	}
	if !state.WaitForIdentity.ValueBool() || state.ReadyTimeout.ValueString() != "30s" {
		t.Errorf("defaults: wait_for_identity=%v ready_timeout=%q", state.WaitForIdentity.ValueBool(), state.ReadyTimeout.ValueString())
	}
	attrs := state.GCPWorkloadIdentity.Attributes()
	if got := attrs["principal"].(types.String).ValueString(); got != "tenant@example.com" {
		t.Errorf("principal=%q", got)
	}
}

func TestClickPipesServiceContextDataSource_PreservesConfiguredTimeout(t *testing.T) {
	mc := minimock.NewController(t)
	mock := api.NewClientMock(mc)
	mock.WaitForClickPipesGCPWorkloadIdentityMock.Set(func(_ context.Context, _ string, timeout time.Duration) (*api.ClickPipesGCPWorkloadIdentityContext, error) {
		if timeout != time.Minute {
			t.Fatalf("timeout=%s, want 1m", timeout)
		}
		return &api.ClickPipesGCPWorkloadIdentityContext{Supported: true, Ready: contextBoolPtr(true), Principal: contextStringPtr("tenant@example.com")}, nil
	})
	timeout := "60s"

	resp, state := readClickPipesServiceContextDataSource(t, &clickPipesServiceContextDataSource{client: mock}, clickPipesServiceContextConfig(nil, &timeout))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
	}
	if state.ReadyTimeout.ValueString() != "60s" {
		t.Errorf("ready_timeout=%q, want configured value 60s", state.ReadyTimeout.ValueString())
	}
}

func TestClickPipesServiceContextDataSource_SnapshotPreservesNullableFields(t *testing.T) {
	mc := minimock.NewController(t)
	mock := api.NewClientMock(mc)
	mock.GetClickPipesServiceContextMock.Expect(context.Background(), "svc-1").Return(&api.ClickPipesServiceContext{
		GCPWorkloadIdentity: api.ClickPipesGCPWorkloadIdentityContext{Supported: true},
	}, nil)
	wait := false

	resp, state := readClickPipesServiceContextDataSource(t, &clickPipesServiceContextDataSource{client: mock}, clickPipesServiceContextConfig(&wait, nil))
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics.Errors())
	}
	attrs := state.GCPWorkloadIdentity.Attributes()
	if !attrs["ready"].(types.Bool).IsNull() || !attrs["principal"].(types.String).IsNull() {
		t.Errorf("nullable fields were not preserved: %v", attrs)
	}
}

func TestClickPipesServiceContextDataSource_RejectsInvalidTimeout(t *testing.T) {
	mc := minimock.NewController(t)
	badTimeout := "immediately"
	resp, _ := readClickPipesServiceContextDataSource(t, &clickPipesServiceContextDataSource{client: api.NewClientMock(mc)}, clickPipesServiceContextConfig(nil, &badTimeout))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid timeout diagnostic")
	}
}
