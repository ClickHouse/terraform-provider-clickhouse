package resource

import (
	"context"
	"errors"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

func byocCustomerAccessObject(t *testing.T, enabled bool, duration int64, start types.String) types.Object {
	t.Helper()
	obj, diags := types.ObjectValue(byocCustomerAccessAttrTypes(), map[string]attr.Value{
		"enabled":          types.BoolValue(enabled),
		"duration_seconds": types.Int64Value(duration),
		"start_timestamp":  start,
	})
	if diags.HasError() {
		t.Fatalf("build object: %v", diags)
	}
	return obj
}

func TestByocValidateCustomerAccessRequiresStartForWindow(t *testing.T) {
	ctx := context.Background()

	var diags diag.Diagnostics
	byocValidateCustomerAccess(ctx, byocCustomerAccessObject(t, true, 3600, types.StringNull()), &diags)
	if !diags.HasError() {
		t.Error("a window without start_timestamp must be rejected")
	}

	for name, obj := range map[string]types.Object{
		"null block":           types.ObjectNull(byocCustomerAccessAttrTypes()),
		"unknown block":        types.ObjectUnknown(byocCustomerAccessAttrTypes()),
		"always allowed":       byocCustomerAccessObject(t, true, 0, types.StringNull()),
		"denied with window":   byocCustomerAccessObject(t, false, 3600, types.StringNull()),
		"window with start":    byocCustomerAccessObject(t, true, 3600, types.StringValue("2026-06-01T12:00:00Z")),
		"window unknown start": byocCustomerAccessObject(t, true, 3600, types.StringUnknown()),
	} {
		var d diag.Diagnostics
		byocValidateCustomerAccess(ctx, obj, &d)
		if d.HasError() {
			t.Errorf("%s: unexpected diags: %v", name, d)
		}
	}
}

func TestRfc3339ValidatorRejectsSubMillisecondPrecision(t *testing.T) {
	for value, wantErr := range map[string]bool{
		"2026-06-01T12:00:00Z":             false,
		"2026-06-01T12:00:00.1Z":           false,
		"2026-06-01T12:00:00.123Z":         false,
		"2026-06-01T14:00:00.123+02:00":    false,
		"2026-06-01T12:00:00.1230Z":        true,
		"2026-06-01T12:00:00.123456Z":      true,
		"2026-06-01T14:00:00.123456+02:00": true,
		"not a timestamp":                  true,
	} {
		resp := &validator.StringResponse{}
		rfc3339Validator{}.ValidateString(context.Background(), validator.StringRequest{ConfigValue: types.StringValue(value)}, resp)
		if got := resp.Diagnostics.HasError(); got != wantErr {
			t.Errorf("%q: error = %v; want %v", value, got, wantErr)
		}
	}
}

func TestByocCustomerAccessObjectFromAPIKeepsConfiguredStartFormat(t *testing.T) {
	ctx := context.Background()
	apiStart := "2026-06-01T12:00:00.000Z"
	access := &api.ByocInfrastructureCustomerAuthorizedAccess{Enabled: true, DurationSeconds: 3600, StartTimestamp: &apiStart}

	configured := byocCustomerAccessObject(t, true, 3600, types.StringValue("2026-06-01T14:00:00+02:00"))
	obj, diags := byocCustomerAccessObjectFromAPI(ctx, access, configured)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if got := obj.Attributes()["start_timestamp"].(types.String).ValueString(); got != "2026-06-01T14:00:00+02:00" {
		t.Errorf("start_timestamp = %q; want the configured spelling of the same instant", got)
	}

	differing := byocCustomerAccessObject(t, true, 3600, types.StringValue("2026-06-02T12:00:00Z"))
	obj, diags = byocCustomerAccessObjectFromAPI(ctx, access, differing)
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if got := obj.Attributes()["start_timestamp"].(types.String).ValueString(); got != apiStart {
		t.Errorf("start_timestamp = %q; want the API value when the instant differs", got)
	}

	obj, diags = byocCustomerAccessObjectFromAPI(ctx, &api.ByocInfrastructureCustomerAuthorizedAccess{Enabled: false}, types.ObjectNull(byocCustomerAccessAttrTypes()))
	if diags.HasError() {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if !obj.Attributes()["start_timestamp"].IsNull() {
		t.Error("start_timestamp should be null when the API returns none")
	}
}

func TestByocInfrastructureResourceUpdateAppliesCustomerAccess(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	model := byocInfraModel()
	model.ID = types.StringValue(byocInfraID)
	model.State = types.StringValue(api.ByocStateReady)
	model.CloudProvider = types.StringValue("aws")
	model.DisplayName = types.StringValue("byoc-us-east-1")
	model.CustomerAuthorizedAccess = byocCustomerAccessObject(t, true, 3600, types.StringValue("2026-06-01T12:00:00Z"))
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	apiStart := "2026-06-01T12:00:00.000Z"
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.UpdateByocInfrastructureCustomerAuthorizedAccessMock.
		Expect(ctx, byocInfraID, api.ByocInfrastructureCustomerAuthorizedAccessUpdateRequest{
			Enabled:         true,
			DurationSeconds: 3600,
			StartTimestamp:  strPtr("2026-06-01T12:00:00Z"),
		}).
		Return(&api.ByocInfrastructureCustomerAuthorizedAccess{Enabled: true, DurationSeconds: 3600, StartTimestamp: &apiStart, Allowed: true}, nil)
	client.GetByocInfrastructureMock.
		Expect(ctx, byocInfraID).
		Return(byocInfraDetails(api.ByocStateReady), nil)
	client.GetByocInfrastructureTagsMock.
		Expect(ctx, byocInfraID).
		Return(map[string]string{}, nil)
	r.client = client

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if n := len(client.UpdateByocInfrastructureMock.Calls()); n != 0 {
		t.Errorf("infrastructure PATCH called %d times; the customer access change alone must not patch the infrastructure", n)
	}

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	attrs := got.CustomerAuthorizedAccess.Attributes()
	if got := attrs["start_timestamp"].(types.String).ValueString(); got != "2026-06-01T12:00:00Z" {
		t.Errorf("start_timestamp = %q; want the configured value", got)
	}
	if got := attrs["duration_seconds"].(types.Int64).ValueInt64(); got != 3600 {
		t.Errorf("duration_seconds = %d; want 3600", got)
	}
}

func TestByocInfrastructureResourceUpdateLeavesCustomerAccessWhenBlockRemoved(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)

	stateModel := byocInfraModel()
	stateModel.ID = types.StringValue(byocInfraID)
	stateModel.State = types.StringValue(api.ByocStateReady)
	stateModel.CloudProvider = types.StringValue("aws")
	stateModel.DisplayName = types.StringValue("byoc-us-east-1")
	stateModel.CustomerAuthorizedAccess = byocCustomerAccessObject(t, false, 0, types.StringNull())
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	planModel := stateModel
	planModel.DisplayName = types.StringValue("renamed-byoc")
	planModel.CustomerAuthorizedAccess = types.ObjectNull(byocCustomerAccessAttrTypes())
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}

	details := byocInfraDetails(api.ByocStateReady)
	details.DisplayName = "renamed-byoc"
	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.UpdateByocInfrastructureMock.
		Expect(ctx, byocInfraID, api.ByocInfrastructureUpdateRequest{DisplayName: strPtr("renamed-byoc")}).
		Return(&api.ByocInfrastructure{Id: byocInfraID, State: api.ByocStateReady}, nil)
	client.GetByocInfrastructureMock.Expect(ctx, byocInfraID).Return(details, nil)
	client.GetByocInfrastructureTagsMock.Expect(ctx, byocInfraID).Return(map[string]string{}, nil)
	r.client = client

	resp := resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", resp.Diagnostics)
	}
	if n := len(client.UpdateByocInfrastructureCustomerAuthorizedAccessMock.Calls()); n != 0 {
		t.Errorf("removing the block must not touch the setting, got %d calls", n)
	}

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if !got.CustomerAuthorizedAccess.IsNull() {
		t.Errorf("customer_authorized_access = %v; want null once unmanaged", got.CustomerAuthorizedAccess)
	}
}

func TestByocInfrastructureResourceReadRefreshesManagedCustomerAccess(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)

	stateModel := byocInfraModel()
	stateModel.ID = types.StringValue(byocInfraID)
	stateModel.State = types.StringValue(api.ByocStateReady)
	stateModel.CloudProvider = types.StringValue("aws")
	stateModel.DisplayName = types.StringValue("byoc-us-east-1")
	stateModel.CustomerAuthorizedAccess = byocCustomerAccessObject(t, true, 0, types.StringNull())
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetByocInfrastructureMock.Expect(ctx, byocInfraID).Return(byocInfraDetails(api.ByocStateReady), nil)
	client.GetByocInfrastructureTagsMock.Expect(ctx, byocInfraID).Return(map[string]string{}, nil)
	client.GetByocInfrastructureCustomerAuthorizedAccessMock.
		Expect(ctx, byocInfraID).
		Return(&api.ByocInfrastructureCustomerAuthorizedAccess{Enabled: false, Allowed: false}, nil)
	r.client = client

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.CustomerAuthorizedAccess.Attributes()["enabled"].(types.Bool).ValueBool() {
		t.Error("enabled should reflect the console-side change to false")
	}
}

func TestByocInfrastructureResourceReadSkipsUnmanagedCustomerAccess(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)
	state := byocInfraState(t, schemaResp)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetByocInfrastructureMock.Expect(ctx, byocInfraID).Return(byocInfraDetails(api.ByocStateReady), nil)
	client.GetByocInfrastructureTagsMock.Expect(ctx, byocInfraID).Return(map[string]string{}, nil)
	r.client = client

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	if n := len(client.GetByocInfrastructureCustomerAuthorizedAccessMock.Calls()); n != 0 {
		t.Errorf("unmanaged customer access must not be read, got %d calls", n)
	}
}

func TestByocInfrastructureResourceReadKeepsCustomerAccessOn404(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocInfraSchema(t)

	stateModel := byocInfraModel()
	stateModel.ID = types.StringValue(byocInfraID)
	stateModel.State = types.StringValue(api.ByocStateReady)
	stateModel.CloudProvider = types.StringValue("aws")
	stateModel.DisplayName = types.StringValue("byoc-us-east-1")
	stateModel.CustomerAuthorizedAccess = byocCustomerAccessObject(t, false, 0, types.StringNull())
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.GetByocInfrastructureMock.Expect(ctx, byocInfraID).Return(byocInfraDetails(api.ByocStateReady), nil)
	client.GetByocInfrastructureTagsMock.Expect(ctx, byocInfraID).Return(map[string]string{}, nil)
	client.GetByocInfrastructureCustomerAuthorizedAccessMock.
		Expect(ctx, byocInfraID).
		Return(nil, errors.New("status: 404, body: BYOC infrastructure not found"))
	r.client = client

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("a 404 for a disabled feature must not fail the read: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() == 0 {
		t.Error("expected a warning about the unavailable setting")
	}

	var got models.ByocInfrastructureResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.CustomerAuthorizedAccess.IsNull() {
		t.Error("the last applied setting must survive a 404 refresh")
	}
}
