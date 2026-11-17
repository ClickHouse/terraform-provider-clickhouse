package resource

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/test"
)

func TestDeletionProtectionFromAPI(t *testing.T) {
	tests := []struct {
		name     string
		current  types.Bool
		apiValue *bool
		want     types.Bool
	}{
		{name: "true from the API fills an unset attribute", current: types.BoolNull(), apiValue: boolPtr(true), want: types.BoolValue(true)},
		{name: "true from the API overrides a stored false", current: types.BoolValue(false), apiValue: boolPtr(true), want: types.BoolValue(true)},
		{name: "false from the API leaves an unset attribute unset", current: types.BoolNull(), apiValue: boolPtr(false), want: types.BoolNull()},
		{name: "false from the API overrides a stored true", current: types.BoolValue(true), apiValue: boolPtr(false), want: types.BoolValue(false)},
		{name: "false from the API keeps a stored false", current: types.BoolValue(false), apiValue: boolPtr(false), want: types.BoolValue(false)},
		{name: "omitted field turns a stored true into false", current: types.BoolValue(true), apiValue: nil, want: types.BoolValue(false)},
		{name: "omitted field keeps a stored false", current: types.BoolValue(false), apiValue: nil, want: types.BoolValue(false)},
		{name: "omitted field keeps an unset attribute unset", current: types.BoolNull(), apiValue: nil, want: types.BoolNull()},
		{name: "omitted field settles an unknown value to null", current: types.BoolUnknown(), apiValue: nil, want: types.BoolNull()},
		{name: "false from the API settles an unknown value to null", current: types.BoolUnknown(), apiValue: boolPtr(false), want: types.BoolNull()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deletionProtectionFromAPI(tt.current, tt.apiValue); !got.Equal(tt.want) {
				t.Errorf("deletionProtectionFromAPI(%v, %v) = %v; want %v", tt.current, tt.apiValue, got, tt.want)
			}
		})
	}
}

func TestServiceResource_Create_deletionProtection(t *testing.T) {
	ctx := context.Background()
	r := &ServiceResource{}
	sch := buildServiceSchema(t, ctx, r)

	tests := []struct {
		name      string
		planned   types.Bool
		apiValue  *bool
		wantSent  *bool
		wantState types.Bool
		wantErr   bool
		syncValue *bool
	}{
		{name: "true is sent on create", planned: types.BoolValue(true), apiValue: boolPtr(true), wantSent: boolPtr(true), wantState: types.BoolValue(true)},
		{name: "false is sent on create", planned: types.BoolValue(false), apiValue: boolPtr(false), wantSent: boolPtr(false), wantState: types.BoolValue(false)},
		{name: "unset is omitted from the create request", planned: types.BoolNull(), apiValue: nil, wantSent: nil, wantState: types.BoolNull()},
		{name: "unset stays unset when the API reports false", planned: types.BoolNull(), apiValue: boolPtr(false), wantSent: nil, wantState: types.BoolNull()},
		{name: "unset stays unset when the API reports true", planned: types.BoolNull(), apiValue: boolPtr(true), wantSent: nil, wantState: types.BoolNull()},
		{name: "a sent true the API did not apply fails and records the API's false", planned: types.BoolValue(true), apiValue: boolPtr(false), wantSent: boolPtr(true), wantState: types.BoolValue(false), wantErr: true},
		{name: "a sent true the API did not report back fails and records unset", planned: types.BoolValue(true), apiValue: nil, wantSent: boolPtr(true), wantState: types.BoolNull(), wantErr: true},
		{name: "a sent false the API did not report back stays false", planned: types.BoolValue(false), apiValue: nil, wantSent: boolPtr(false), wantState: types.BoolValue(false)},
		{name: "defensive: an unknown planned value takes the API's value", planned: types.BoolUnknown(), apiValue: boolPtr(true), wantSent: nil, wantState: types.BoolValue(true)},
		{name: "a sent false the API reports as true fails and records true", planned: types.BoolValue(false), apiValue: boolPtr(true), wantSent: boolPtr(false), wantState: types.BoolValue(true), wantErr: true},
		{name: "the create response, not the follow-up read, confirms the sent value", planned: types.BoolValue(true), apiValue: boolPtr(true), syncValue: boolPtr(false), wantSent: boolPtr(true), wantState: types.BoolValue(true)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createResp := getBaseResponse("svc-new")
			createResp.State = api.StateRunning
			createResp.DeletionProtection = tt.apiValue
			syncResp := createResp
			if tt.syncValue != nil {
				syncResp.DeletionProtection = tt.syncValue
			}

			var sent *bool
			mc := minimock.NewController(t)
			r.client = api.NewClientMock(mc).
				CreateServiceMock.Set(func(_ context.Context, s api.Service) (*api.Service, string, error) {
				sent = s.DeletionProtection
				return &createResp, "api-generated-secret", nil
			}).
				WaitForServiceStateMock.Return(nil).
				GetServiceMock.Return(&syncResp, nil)

			plan := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
				s.DeletionProtection = tt.planned
				s.BackupConfiguration = types.ObjectNull(models.BackupConfiguration{}.ObjectType().AttrTypes)
				s.GeneratedPassword = types.StringUnknown()
			}).Get()

			planVal := tfsdk.Plan{Schema: sch}
			if d := planVal.Set(ctx, &plan); d.HasError() {
				t.Fatalf("encoding plan: %v", d.Errors())
			}
			resp := &resource.CreateResponse{State: tfsdk.State{Schema: sch}}
			r.Create(ctx, resource.CreateRequest{Plan: planVal, Config: tfsdk.Config{Schema: sch, Raw: planVal.Raw}}, resp)
			assertProtectionNotAppliedError(t, resp.Diagnostics, tt.wantErr)
			if tt.wantErr && !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "terraform untaint") {
				t.Errorf("create error detail should tell the user to untaint the new service: %q", resp.Diagnostics.Errors()[0].Detail())
			}

			if !reflect.DeepEqual(sent, tt.wantSent) {
				t.Errorf("deletionProtection sent = %v; want %v", sent, tt.wantSent)
			}

			var out models.ServiceResourceModel
			if d := resp.State.Get(ctx, &out); d.HasError() {
				t.Fatalf("decoding post-apply state: %v", d.Errors())
			}
			if !out.DeletionProtection.Equal(tt.wantState) {
				t.Errorf("deletion_protection in state = %v; want %v", out.DeletionProtection, tt.wantState)
			}
		})
	}
}

func TestServiceResource_Update_deletionProtection(t *testing.T) {
	ctx := context.Background()
	r := &ServiceResource{}
	sch := buildServiceSchema(t, ctx, r)

	withProtection := func(v types.Bool) models.ServiceResourceModel {
		return test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.BackupConfiguration = types.ObjectNull(models.BackupConfiguration{}.ObjectType().AttrTypes)
			s.DeletionProtection = v
		}).Get()
	}

	tests := []struct {
		name       string
		state      types.Bool
		plan       types.Bool
		apiValue   *bool
		wantCalled bool
		wantSent   *bool
		wantState  types.Bool
		wantErr    bool
		syncValue  *bool
	}{
		{name: "turning protection on sends true", state: types.BoolNull(), plan: types.BoolValue(true), apiValue: boolPtr(true), wantCalled: true, wantSent: boolPtr(true), wantState: types.BoolValue(true)},
		{name: "turning protection off sends false", state: types.BoolValue(true), plan: types.BoolValue(false), apiValue: boolPtr(false), wantCalled: true, wantSent: boolPtr(false), wantState: types.BoolValue(false)},
		{name: "unchanged value sends nothing", state: types.BoolValue(true), plan: types.BoolValue(true), apiValue: boolPtr(true), wantCalled: false, wantState: types.BoolValue(true)},
		{name: "unset value sends nothing", state: types.BoolNull(), plan: types.BoolNull(), apiValue: nil, wantCalled: false, wantState: types.BoolNull()},
		{name: "unset value stays unset when protection was turned on outside Terraform", state: types.BoolNull(), plan: types.BoolNull(), apiValue: boolPtr(true), wantCalled: false, wantState: types.BoolNull()},
		{name: "stored true stays true when protection was turned off outside Terraform", state: types.BoolValue(true), plan: types.BoolValue(true), apiValue: boolPtr(false), wantCalled: false, wantState: types.BoolValue(true)},
		{name: "stored false stays false when protection was turned on outside Terraform", state: types.BoolValue(false), plan: types.BoolValue(false), apiValue: boolPtr(true), wantCalled: false, wantState: types.BoolValue(false)},
		{name: "setting false on an unset attribute sends false", state: types.BoolNull(), plan: types.BoolValue(false), apiValue: boolPtr(false), wantCalled: true, wantSent: boolPtr(false), wantState: types.BoolValue(false)},
		{name: "a sent true the API did not apply fails and records the API's false", state: types.BoolNull(), plan: types.BoolValue(true), apiValue: boolPtr(false), wantCalled: true, wantSent: boolPtr(true), wantState: types.BoolValue(false), wantErr: true},
		{name: "a sent true the API did not report back fails and records unset", state: types.BoolValue(false), plan: types.BoolValue(true), apiValue: nil, wantCalled: true, wantSent: boolPtr(true), wantState: types.BoolNull(), wantErr: true},
		{name: "a sent false the API did not report back stays false", state: types.BoolValue(true), plan: types.BoolValue(false), apiValue: nil, wantCalled: true, wantSent: boolPtr(false), wantState: types.BoolValue(false)},
		{name: "a sent false the API reports as true fails and records true", state: types.BoolValue(true), plan: types.BoolValue(false), apiValue: boolPtr(true), wantCalled: true, wantSent: boolPtr(false), wantState: types.BoolValue(true), wantErr: true},
		{name: "the update response, not the follow-up read, confirms the sent value", state: types.BoolNull(), plan: types.BoolValue(true), apiValue: boolPtr(true), syncValue: boolPtr(false), wantCalled: true, wantSent: boolPtr(true), wantState: types.BoolValue(true)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncResp := getBaseResponse(encodableInitialState().ID.ValueString())
			syncResp.State = api.StateRunning
			syncResp.DeletionProtection = tt.apiValue
			patchResp := syncResp
			if tt.syncValue != nil {
				syncResp.DeletionProtection = tt.syncValue
			}

			var called bool
			var sent *bool
			mc := minimock.NewController(t)
			r.client = api.NewClientMock(mc).
				UpdateServiceMock.Optional().Set(func(_ context.Context, _ string, s api.ServiceUpdate) (*api.Service, error) {
				called = true
				sent = s.DeletionProtection
				return &patchResp, nil
			}).
				UpdateReplicaScalingMock.Optional().Return(&syncResp, nil).
				GetServiceMock.Return(&syncResp, nil)

			state := withProtection(tt.state)
			plan := withProtection(tt.plan)
			stateVal := tfsdk.State{Schema: sch}
			if d := stateVal.Set(ctx, &state); d.HasError() {
				t.Fatalf("encoding state: %v", d.Errors())
			}
			planVal := tfsdk.Plan{Schema: sch}
			if d := planVal.Set(ctx, &plan); d.HasError() {
				t.Fatalf("encoding plan: %v", d.Errors())
			}
			resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
			r.Update(ctx, resource.UpdateRequest{State: stateVal, Plan: planVal, Config: tfsdk.Config{Schema: sch, Raw: planVal.Raw}}, resp)
			assertProtectionNotAppliedError(t, resp.Diagnostics, tt.wantErr)

			if called != tt.wantCalled {
				t.Fatalf("UpdateService called = %v; want %v", called, tt.wantCalled)
			}
			if !reflect.DeepEqual(sent, tt.wantSent) {
				t.Errorf("deletionProtection sent = %v; want %v", sent, tt.wantSent)
			}

			var out models.ServiceResourceModel
			if d := resp.State.Get(ctx, &out); d.HasError() {
				t.Fatalf("decoding post-apply state: %v", d.Errors())
			}
			if !out.DeletionProtection.Equal(tt.wantState) {
				t.Errorf("deletion_protection in state = %v; want %v", out.DeletionProtection, tt.wantState)
			}
		})
	}
}

func assertProtectionNotAppliedError(t *testing.T, diags diag.Diagnostics, wantErr bool) {
	t.Helper()
	if !wantErr {
		if diags.HasError() {
			t.Fatalf("unexpected errors: %v", diags.Errors())
		}
		return
	}
	errs := diags.Errors()
	if len(errs) != 1 || errs[0].Summary() != "ClickHouse Service Deletion Protection Not Applied" {
		t.Fatalf("errors = %v; want one deletion protection not applied error", errs)
	}
}

func TestServiceResource_Delete_deletionProtection(t *testing.T) {
	ctx := context.Background()
	r := &ServiceResource{}
	sch := buildServiceSchema(t, ctx, r)

	runDelete := func(t *testing.T, deleteErr error) diag.Diagnostics {
		t.Helper()
		mc := minimock.NewController(t)
		r.client = api.NewClientMock(mc).DeleteServiceMock.Return(nil, deleteErr)

		state := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.BackupConfiguration = types.ObjectNull(models.BackupConfiguration{}.ObjectType().AttrTypes)
			s.DeletionProtection = types.BoolValue(true)
		}).Get()
		stateVal := tfsdk.State{Schema: sch}
		if d := stateVal.Set(ctx, &state); d.HasError() {
			t.Fatalf("encoding state: %v", d.Errors())
		}
		resp := &resource.DeleteResponse{State: stateVal}
		r.Delete(ctx, resource.DeleteRequest{State: stateVal}, resp)
		return resp.Diagnostics
	}

	t.Run("protected service tells the user how to turn protection off", func(t *testing.T) {
		diags := runDelete(t, api.ErrServiceDeletionProtected)
		if !diags.HasError() {
			t.Fatal("Delete should fail for a protected service")
		}
		if !strings.Contains(diags.Errors()[0].Summary(), "Protected From Deletion") {
			t.Errorf("summary = %q; want it to say the service is protected", diags.Errors()[0].Summary())
		}
		for _, want := range []string{"Deletion protection is enabled", "deletion_protection = false", "its own apply"} {
			if !detailContains(diags, want) {
				t.Errorf("detail %q does not mention %q", diags.Errors()[0].Detail(), want)
			}
		}
		if detailContains(diags, "now stopped") {
			t.Errorf("detail %q says the service was stopped, but the delete was refused before any stop", diags.Errors()[0].Detail())
		}
	})

	t.Run("refusal after the stop tells the user the service is stopped", func(t *testing.T) {
		diags := runDelete(t, fmt.Errorf("%w: %w: status: 409, body: CONFLICT", api.ErrServiceDeletionProtected, api.ErrDeleteRefusedAfterStop))
		if !diags.HasError() {
			t.Fatal("Delete should fail for a protected service")
		}
		if !detailContains(diags, "now stopped") {
			t.Errorf("detail %q does not say the service was stopped", diags.Errors()[0].Detail())
		}
		if detailContains(diags, "status: 409") {
			t.Errorf("detail %q repeats the raw API error", diags.Errors()[0].Detail())
		}
	})

	t.Run("other errors keep the generic message", func(t *testing.T) {
		diags := runDelete(t, fmt.Errorf("status: 500, body: boom"))
		if !diags.HasError() {
			t.Fatal("Delete should fail")
		}
		if detailContains(diags, "deletion_protection") {
			t.Errorf("detail %q should not mention deletion_protection for an unrelated error", diags.Errors()[0].Detail())
		}
	})
}
