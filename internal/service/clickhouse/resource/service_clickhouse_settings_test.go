package resource

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

var testClickHouseSettingsSchema = []api.ClickHouseSettingSchema{
	{Name: "compatibility", Type: "string"},
	{Name: "max_query_size", Type: "integer"},
	{Name: "max_threads", Type: "integer"},
}

func clickHouseSettingsResourceWithSchema(t *testing.T) (*ServiceClickHouseSettingsResource, schema.Schema) {
	t.Helper()
	ctx := context.Background()
	r := NewServiceClickHouseSettingsResource().(*ServiceClickHouseSettingsResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", schemaResp.Diagnostics)
	}
	return r, schemaResp.Schema
}

func clickHouseSettingsMapValue(settings map[string]string) tftypes.Value {
	elements := make(map[string]tftypes.Value, len(settings))
	for name, value := range settings {
		elements[name] = tftypes.NewValue(tftypes.String, value)
	}
	return tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, elements)
}

// clickHouseSettingsRaw builds a tftypes.Value matching the resource schema.
// id is nil for a plan before create and the service ID in state.
func clickHouseSettingsRaw(ctx context.Context, sch schema.Schema, id any, serviceID any, settings tftypes.Value) tftypes.Value {
	return tftypes.NewValue(sch.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, id),
		"service_id": tftypes.NewValue(tftypes.String, serviceID),
		"settings":   settings,
	})
}

func clickHouseSettingsStateRaw(ctx context.Context, sch schema.Schema, serviceID string, settings map[string]string) tftypes.Value {
	return clickHouseSettingsRaw(ctx, sch, serviceID, serviceID, clickHouseSettingsMapValue(settings))
}

func clickHouseSettingsFromState(t *testing.T, state tfsdk.State) (models.ServiceClickHouseSettingsResourceModel, map[string]string) {
	t.Helper()
	ctx := context.Background()
	var model models.ServiceClickHouseSettingsResourceModel
	if d := state.Get(ctx, &model); d.HasError() {
		t.Fatalf("read state: %v", d)
	}
	settings := map[string]string{}
	if d := model.Settings.ElementsAs(ctx, &settings, false); d.HasError() {
		t.Fatalf("read settings: %v", d)
	}
	return model, settings
}

func diagnosticWith(diags diag.Diagnostics, summary string) diag.Diagnostic {
	for _, d := range diags {
		if d.Summary() == summary {
			return d
		}
	}
	return nil
}

func TestServiceClickHouseSettingsResource_Create(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc).
		GetClickHouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(testClickHouseSettingsSchema, nil).
		UpdateClickHouseSettingsMock.
		Expect(ctx, "svc-1", api.ClickHouseSettingsUpdate{Settings: map[string]api.ClickHouseSettingValue{
			"compatibility":    {Text: "26.2"},
			"max_query_size":   {Text: "262144", IsNumber: true},
			"unlisted_setting": {Text: "1"},
		}}).
		Return(&api.ClickHouseSettingsUpdateResult{Warnings: []api.ClickHouseSettingWarning{
			{Name: "compatibility", Message: "Changing the compatibility version can cause query failures."},
			{Message: "Some changes take effect after a restart."},
		}}, nil).
		ListClickHouseSettingsMock.
		Expect(ctx, "svc-1").
		Return([]api.ClickHouseSetting{
			{Name: "compatibility", Value: api.ClickHouseSettingValue{Text: "26.2"}},
			{Name: "max_query_size", Value: api.ClickHouseSettingValue{Text: "262144", IsNumber: true}},
			{Name: "unlisted_setting", Value: api.ClickHouseSettingValue{Text: "1"}},
		}, nil)
	r.client = client

	planned := map[string]string{"compatibility": "26.2", "max_query_size": "262144", "unlisted_setting": "1"}
	req := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: clickHouseSettingsRaw(ctx, sch, nil, "svc-1", clickHouseSettingsMapValue(planned))},
	}
	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}

	r.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics: %v", resp.Diagnostics)
	}
	if got := client.UpdateClickHouseSettingsAfterCounter(); got != 1 {
		t.Errorf("UpdateClickHouseSettings calls = %d; want 1", got)
	}

	warnings := resp.Diagnostics.Warnings()
	if len(warnings) != 2 {
		t.Fatalf("got %d warnings, want one per API warning: %v", len(warnings), warnings)
	}
	if got := warnings[0].Summary(); !strings.Contains(got, "compatibility") {
		t.Errorf("first warning summary = %q; want it to name compatibility", got)
	}
	if got := warnings[1].Detail(); got != "Some changes take effect after a restart." {
		t.Errorf("second warning detail = %q", got)
	}

	model, settings := clickHouseSettingsFromState(t, resp.State)
	if model.ID.ValueString() != "svc-1" {
		t.Errorf("state ID = %q; want svc-1", model.ID.ValueString())
	}
	if diff := cmp.Diff(planned, settings); diff != "" {
		t.Errorf("state settings mismatch (-want +got):\n%s", diff)
	}
}

// TestServiceClickHouseSettingsResource_Create_WarnsWhenReadBackDiffers keeps
// the planned values in state, as Terraform requires, and warns about a
// setting the service does not report as written.
func TestServiceClickHouseSettingsResource_Create_WarnsWhenReadBackDiffers(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetClickHouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(testClickHouseSettingsSchema, nil).
		UpdateClickHouseSettingsMock.
		Return(&api.ClickHouseSettingsUpdateResult{}, nil).
		ListClickHouseSettingsMock.
		Expect(ctx, "svc-1").
		Return([]api.ClickHouseSetting{
			{Name: "max_query_size", Value: api.ClickHouseSettingValue{Text: "262144", IsNumber: true}},
		}, nil)

	planned := map[string]string{"compatibility": "26.2", "max_query_size": "262144"}
	req := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: clickHouseSettingsRaw(ctx, sch, nil, "svc-1", clickHouseSettingsMapValue(planned))},
	}
	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}

	r.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics: %v", resp.Diagnostics)
	}
	warnings := resp.Diagnostics.Warnings()
	if len(warnings) != 1 || !strings.Contains(warnings[0].Detail(), "compatibility") {
		t.Fatalf("want one warning naming compatibility; got %v", warnings)
	}
	_, settings := clickHouseSettingsFromState(t, resp.State)
	if diff := cmp.Diff(planned, settings); diff != "" {
		t.Errorf("state settings mismatch (-want +got):\n%s", diff)
	}
}

// TestServiceClickHouseSettingsResource_Create_KeepsSettingsWhenResponseUnreadable
// keeps ownership of settings the API accepted when its response cannot be
// read: state holds the planned settings and the lost warnings are reported.
func TestServiceClickHouseSettingsResource_Create_KeepsSettingsWhenResponseUnreadable(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetClickHouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(testClickHouseSettingsSchema, nil).
		UpdateClickHouseSettingsMock.
		Return(nil, api.NewClickHouseSettingsResponseError(errors.New("json: cannot unmarshal object into Go struct field"))).
		ListClickHouseSettingsMock.
		Expect(ctx, "svc-1").
		Return([]api.ClickHouseSetting{
			{Name: "max_query_size", Value: api.ClickHouseSettingValue{Text: "262144", IsNumber: true}},
		}, nil)

	planned := map[string]string{"max_query_size": "262144"}
	req := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: clickHouseSettingsRaw(ctx, sch, nil, "svc-1", clickHouseSettingsMapValue(planned))},
	}
	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}

	r.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics: %v", resp.Diagnostics)
	}
	if d := diagnosticWith(resp.Diagnostics, "Unable to read the ClickHouse settings response"); d == nil || d.Severity() != diag.SeverityWarning {
		t.Fatalf("want a warning about the unreadable response; got %v", resp.Diagnostics)
	}
	_, settings := clickHouseSettingsFromState(t, resp.State)
	if diff := cmp.Diff(planned, settings); diff != "" {
		t.Errorf("state settings mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceClickHouseSettingsResource_Create_RejectsNonIntegerForIntegerSetting(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	// UpdateClickHouseSettingsMock is intentionally not set: nothing may be
	// written when a value does not match its schema type.
	r.client = api.NewClientMock(mc).
		GetClickHouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(testClickHouseSettingsSchema, nil)

	req := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: clickHouseSettingsRaw(ctx, sch, nil, "svc-1", clickHouseSettingsMapValue(map[string]string{
			"compatibility":  "26.2",
			"max_query_size": "256KiB",
		}))},
	}
	resp := &resource.CreateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}

	r.Create(ctx, req, resp)

	d := diagnosticWith(resp.Diagnostics, "Invalid ClickHouse setting value")
	if d == nil {
		t.Fatalf("Create should reject a non-integer value for an integer setting; got %v", resp.Diagnostics)
	}
	if !strings.Contains(d.Detail(), "max_query_size") {
		t.Errorf("diagnostic detail = %q; want it to name max_query_size", d.Detail())
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("Create should not write state when nothing was written")
	}
}

func TestServiceClickHouseSettingsResource_Create_SpecializedErrors(t *testing.T) {
	cases := []struct {
		name        string
		schemaErr   error
		updateErr   error
		wantSummary string
		wantDetail  string
	}{
		{
			name:        "403 on schema",
			schemaErr:   errors.New("status: 403, body: forbidden"),
			wantSummary: "ClickHouse settings API not available",
			wantDetail:  "enabled per organization",
		},
		{
			name:        "403 on update",
			updateErr:   errors.New("status: 403, body: forbidden"),
			wantSummary: "ClickHouse settings API not available",
			wantDetail:  "control-plane:service:manage",
		},
		{
			name:        "404 on update",
			updateErr:   errors.New("status: 404, body: not found"),
			wantSummary: serviceNotFoundSummary,
			wantDetail:  "svc-1",
		},
		{
			name:        "400 on update",
			updateErr:   errors.New("status: 400, body: unknown setting"),
			wantSummary: "Error updating ClickHouse settings",
			wantDetail:  "unknown setting",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r, sch := clickHouseSettingsResourceWithSchema(t)

			mc := minimock.NewController(t)
			client := api.NewClientMock(mc)
			if tc.schemaErr != nil {
				client.GetClickHouseSettingsSchemaMock.Expect(ctx, "svc-1").Return(nil, tc.schemaErr)
			} else {
				client.GetClickHouseSettingsSchemaMock.Expect(ctx, "svc-1").Return(testClickHouseSettingsSchema, nil)
				client.UpdateClickHouseSettingsMock.Return(nil, tc.updateErr)
			}
			r.client = client

			req := resource.CreateRequest{
				Plan: tfsdk.Plan{Schema: sch, Raw: clickHouseSettingsRaw(ctx, sch, nil, "svc-1", clickHouseSettingsMapValue(map[string]string{"compatibility": "26.2"}))},
			}
			resp := &resource.CreateResponse{
				State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
			}

			r.Create(ctx, req, resp)

			d := diagnosticWith(resp.Diagnostics, tc.wantSummary)
			if d == nil || d.Severity() != diag.SeverityError {
				t.Fatalf("want an error %q; got %v", tc.wantSummary, resp.Diagnostics)
			}
			if !strings.Contains(d.Detail(), tc.wantDetail) {
				t.Errorf("diagnostic detail = %q; want it to contain %q", d.Detail(), tc.wantDetail)
			}
		})
	}
}

// TestServiceClickHouseSettingsResource_Read_NarrowsToManagedSettings covers
// the ownership rule: unmanaged settings never reach state, a managed setting
// reset outside Terraform drops out so the next plan sets it again, a changed
// value shows as drift, and an integer the API returns as a number does not
// differ from the configured string.
func TestServiceClickHouseSettingsResource_Read_NarrowsToManagedSettings(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		ListClickHouseSettingsMock.
		Expect(ctx, "svc-1").
		Return([]api.ClickHouseSetting{
			{Name: "compatibility", Value: api.ClickHouseSettingValue{Text: "25.8"}},
			{Name: "max_query_size", Value: api.ClickHouseSettingValue{Text: "262144", IsNumber: true}},
			{Name: "max_threads", Value: api.ClickHouseSettingValue{Text: "8", IsNumber: true}},
			{Name: "set_by_cloud", Value: api.ClickHouseSettingValue{Text: "1", IsNumber: true}},
		}, nil)

	stateRaw := clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{
		"compatibility":     "26.2",
		"max_query_size":    "262144",
		"max_threads":       "08",
		"reset_out_of_band": "1",
	})
	req := resource.ReadRequest{State: tfsdk.State{Schema: sch, Raw: stateRaw}}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch, Raw: stateRaw}}

	r.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read returned diagnostics: %v", resp.Diagnostics)
	}
	_, settings := clickHouseSettingsFromState(t, resp.State)
	want := map[string]string{
		"compatibility":  "25.8",
		"max_query_size": "262144",
		"max_threads":    "08",
	}
	if diff := cmp.Diff(want, settings); diff != "" {
		t.Errorf("state settings mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceClickHouseSettingsResource_Read_ServiceGoneRemovesResource(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		ListClickHouseSettingsMock.
		Expect(ctx, "svc-1").
		Return(nil, errors.New("status: 404, body: not found"))

	stateRaw := clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2"})
	req := resource.ReadRequest{State: tfsdk.State{Schema: sch, Raw: stateRaw}}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: sch, Raw: stateRaw}}

	r.Read(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Read should not error on 404; got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Errorf("Read should remove resource from state on 404; State.Raw.IsNull() = false")
	}
}

// TestServiceClickHouseSettingsResource_Update sends changed and added
// settings in one PATCH and resets removed ones, leaving unchanged ones alone.
func TestServiceClickHouseSettingsResource_Update(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc).
		GetClickHouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(testClickHouseSettingsSchema, nil).
		UpdateClickHouseSettingsMock.
		Expect(ctx, "svc-1", api.ClickHouseSettingsUpdate{Settings: map[string]api.ClickHouseSettingValue{
			"compatibility": {Text: "26.2"},
			"max_threads":   {Text: "16", IsNumber: true},
		}}).
		Return(&api.ClickHouseSettingsUpdateResult{}, nil).
		ResetClickHouseSettingMock.
		Expect(ctx, "svc-1", "max_query_size").
		Return(nil).
		ListClickHouseSettingsMock.
		Expect(ctx, "svc-1").
		Return([]api.ClickHouseSetting{
			{Name: "compatibility", Value: api.ClickHouseSettingValue{Text: "26.2"}},
			{Name: "max_threads", Value: api.ClickHouseSettingValue{Text: "16", IsNumber: true}},
			{Name: "readonly", Value: api.ClickHouseSettingValue{Text: "0", IsNumber: true}},
		}, nil)
	r.client = client

	planned := map[string]string{"compatibility": "26.2", "max_threads": "16", "readonly": "0"}
	req := resource.UpdateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: clickHouseSettingsStateRaw(ctx, sch, "svc-1", planned)},
		State: tfsdk.State{Schema: sch, Raw: clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{
			"compatibility":  "25.8",
			"max_query_size": "262144",
			"readonly":       "0",
		})},
	}
	resp := &resource.UpdateResponse{State: req.State}

	r.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update returned diagnostics: %v", resp.Diagnostics)
	}
	if len(resp.Diagnostics) != 0 {
		t.Errorf("Update should not warn when the read-back matches; got %v", resp.Diagnostics)
	}
	if got := client.UpdateClickHouseSettingsAfterCounter(); got != 1 {
		t.Errorf("UpdateClickHouseSettings calls = %d; want 1", got)
	}
	_, settings := clickHouseSettingsFromState(t, resp.State)
	if diff := cmp.Diff(planned, settings); diff != "" {
		t.Errorf("state settings mismatch (-want +got):\n%s", diff)
	}
}

// TestServiceClickHouseSettingsResource_Update_ResetLoop drives the reset loop
// past a 404 and stops it at the first other failure. Settings not reset stay
// in state.
func TestServiceClickHouseSettingsResource_Update_ResetLoop(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.ResetClickHouseSettingMock.When(ctx, "svc-1", "a_setting").Then(nil)
	client.ResetClickHouseSettingMock.When(ctx, "svc-1", "b_setting").Then(errors.New("status: 404, body: not found"))
	client.ResetClickHouseSettingMock.When(ctx, "svc-1", "c_setting").Then(errors.New("status: 400, body: cannot reset"))
	r.client = client

	req := resource.UpdateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2"})},
		State: tfsdk.State{Schema: sch, Raw: clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{
			"a_setting":     "1",
			"b_setting":     "2",
			"c_setting":     "3",
			"compatibility": "26.2",
			"d_setting":     "4",
		})},
	}
	resp := &resource.UpdateResponse{State: req.State}

	r.Update(ctx, req, resp)

	d := diagnosticWith(resp.Diagnostics, "Error resetting ClickHouse setting")
	if d == nil {
		t.Fatalf("Update should report the failed reset; got %v", resp.Diagnostics)
	}
	if !strings.Contains(d.Detail(), "c_setting") || !strings.Contains(d.Detail(), "not reset: d_setting") {
		t.Errorf("diagnostic detail = %q; want it to name c_setting as failed and d_setting as not reset", d.Detail())
	}
	if got := client.ResetClickHouseSettingAfterCounter(); got != 3 {
		t.Errorf("ResetClickHouseSetting calls = %d; want 3", got)
	}
	_, settings := clickHouseSettingsFromState(t, resp.State)
	want := map[string]string{"c_setting": "3", "compatibility": "26.2", "d_setting": "4"}
	if diff := cmp.Diff(want, settings); diff != "" {
		t.Errorf("state settings mismatch (-want +got):\n%s", diff)
	}
}

// TestServiceClickHouseSettingsResource_Update_PatchFailureResetsNothing stops
// the update at a failed PATCH, before any reset, and keeps the prior state.
func TestServiceClickHouseSettingsResource_Update_PatchFailureResetsNothing(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	// ResetClickHouseSettingMock is intentionally not set: a reset after a
	// failed PATCH fails the test.
	r.client = api.NewClientMock(mc).
		GetClickHouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(testClickHouseSettingsSchema, nil).
		UpdateClickHouseSettingsMock.
		Return(nil, errors.New("status: 400, body: unknown setting"))

	prior := clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "25.8", "max_query_size": "262144"})
	req := resource.UpdateRequest{
		Plan:  tfsdk.Plan{Schema: sch, Raw: clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2"})},
		State: tfsdk.State{Schema: sch, Raw: prior},
	}
	resp := &resource.UpdateResponse{State: req.State}

	r.Update(ctx, req, resp)

	if d := diagnosticWith(resp.Diagnostics, "Error updating ClickHouse settings"); d == nil {
		t.Fatalf("Update should report the failed PATCH; got %v", resp.Diagnostics)
	}
	if !resp.State.Raw.Equal(prior) {
		t.Errorf("Update should keep the prior state when the PATCH fails")
	}
}

// TestServiceClickHouseSettingsResource_Update_ResetsAfterUnreadableResponse
// goes on to the resets when the API accepted the PATCH but its response
// cannot be read, and records the added setting in state.
func TestServiceClickHouseSettingsResource_Update_ResetsAfterUnreadableResponse(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc).
		GetClickHouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(testClickHouseSettingsSchema, nil).
		UpdateClickHouseSettingsMock.
		Return(nil, api.NewClickHouseSettingsResponseError(errors.New("unexpected end of JSON input"))).
		ResetClickHouseSettingMock.
		Expect(ctx, "svc-1", "max_query_size").
		Return(nil).
		ListClickHouseSettingsMock.
		Expect(ctx, "svc-1").
		Return([]api.ClickHouseSetting{
			{Name: "max_threads", Value: api.ClickHouseSettingValue{Text: "16", IsNumber: true}},
		}, nil)
	r.client = client

	planned := map[string]string{"compatibility": "26.2", "max_threads": "16"}
	req := resource.UpdateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: clickHouseSettingsStateRaw(ctx, sch, "svc-1", planned)},
		State: tfsdk.State{Schema: sch, Raw: clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{
			"compatibility":  "26.2",
			"max_query_size": "262144",
		})},
	}
	resp := &resource.UpdateResponse{State: req.State}

	r.Update(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update returned diagnostics: %v", resp.Diagnostics)
	}
	if d := diagnosticWith(resp.Diagnostics, "Unable to read the ClickHouse settings response"); d == nil {
		t.Fatalf("want a warning about the unreadable response; got %v", resp.Diagnostics)
	}
	if got := client.ResetClickHouseSettingAfterCounter(); got != 1 {
		t.Errorf("ResetClickHouseSetting calls = %d; want 1", got)
	}
	_, settings := clickHouseSettingsFromState(t, resp.State)
	if diff := cmp.Diff(planned, settings); diff != "" {
		t.Errorf("state settings mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceClickHouseSettingsResource_Delete(t *testing.T) {
	cases := []struct {
		name      string
		results   map[string]error
		wantError string
		wantCalls uint64
		wantState map[string]string
	}{
		{
			name:      "resets every managed setting",
			results:   map[string]error{"compatibility": nil, "max_query_size": nil},
			wantCalls: 2,
		},
		{
			name:      "continues past a 404",
			results:   map[string]error{"compatibility": errors.New("status: 404, body: not found"), "max_query_size": nil},
			wantCalls: 2,
		},
		{
			name:      "stops on any other failure",
			results:   map[string]error{"compatibility": errors.New("status: 403, body: forbidden")},
			wantError: "The remaining settings were not reset: max_query_size.",
			wantCalls: 1,
		},
		{
			name:      "keeps only the settings not reset",
			results:   map[string]error{"compatibility": nil, "max_query_size": errors.New("status: 403, body: forbidden")},
			wantError: "Could not reset ClickHouse setting max_query_size",
			wantCalls: 2,
			wantState: map[string]string{"max_query_size": "262144"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r, sch := clickHouseSettingsResourceWithSchema(t)

			mc := minimock.NewController(t)
			client := api.NewClientMock(mc)
			for name, err := range tc.results {
				client.ResetClickHouseSettingMock.When(ctx, "svc-1", name).Then(err)
			}
			r.client = client

			stateRaw := clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2", "max_query_size": "262144"})
			req := resource.DeleteRequest{State: tfsdk.State{Schema: sch, Raw: stateRaw}}
			resp := &resource.DeleteResponse{State: tfsdk.State{Schema: sch, Raw: stateRaw}}

			r.Delete(ctx, req, resp)

			if got := client.ResetClickHouseSettingAfterCounter(); got != tc.wantCalls {
				t.Errorf("ResetClickHouseSetting calls = %d; want %d", got, tc.wantCalls)
			}
			if tc.wantError == "" {
				if resp.Diagnostics.HasError() {
					t.Errorf("Delete returned diagnostics: %v", resp.Diagnostics)
				}
				return
			}
			d := diagnosticWith(resp.Diagnostics, "Error resetting ClickHouse setting")
			if d == nil {
				t.Fatalf("Delete should report the failed reset; got %v", resp.Diagnostics)
			}
			if !strings.Contains(d.Detail(), tc.wantError) || !strings.Contains(d.Detail(), "enabled per organization") {
				t.Errorf("diagnostic detail = %q; want it to contain %q and explain the 403", d.Detail(), tc.wantError)
			}
			if tc.wantState != nil {
				_, settings := clickHouseSettingsFromState(t, resp.State)
				if diff := cmp.Diff(tc.wantState, settings); diff != "" {
					t.Errorf("state settings mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestServiceClickHouseSettingsResource_ImportState_AdoptsEverySetting(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		ListClickHouseSettingsMock.
		Expect(ctx, "svc-1").
		Return([]api.ClickHouseSetting{
			{Name: "compatibility", Value: api.ClickHouseSettingValue{Text: "26.2"}},
			{Name: "max_query_size", Value: api.ClickHouseSettingValue{Text: "262144", IsNumber: true}},
		}, nil)

	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}

	r.ImportState(ctx, resource.ImportStateRequest{ID: "svc-1"}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diags: %v", resp.Diagnostics)
	}
	model, settings := clickHouseSettingsFromState(t, resp.State)
	if model.ID.ValueString() != "svc-1" || model.ServiceID.ValueString() != "svc-1" {
		t.Errorf("id, service_id = %q, %q; want svc-1, svc-1", model.ID.ValueString(), model.ServiceID.ValueString())
	}
	want := map[string]string{"compatibility": "26.2", "max_query_size": "262144"}
	if diff := cmp.Diff(want, settings); diff != "" {
		t.Errorf("state settings mismatch (-want +got):\n%s", diff)
	}
}

func TestServiceClickHouseSettingsResource_ImportState_ServiceNotFound(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		ListClickHouseSettingsMock.
		Expect(ctx, "missing-svc").
		Return(nil, errors.New("status: 404, body: not found"))

	resp := &resource.ImportStateResponse{
		State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)},
	}

	r.ImportState(ctx, resource.ImportStateRequest{ID: "missing-svc"}, resp)

	if d := diagnosticWith(resp.Diagnostics, serviceNotFoundSummary); d == nil {
		t.Fatalf("ImportState should error on 404; got %v", resp.Diagnostics)
	}
}

func TestServiceClickHouseSettingsResource_ModifyPlan(t *testing.T) {
	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	managed := map[string]string{"compatibility": "26.2", "max_query_size": "262144"}
	unknownMap := tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, tftypes.UnknownValue)
	unknownElement := func(settings map[string]string, name string) tftypes.Value {
		elements := map[string]tftypes.Value{name: tftypes.NewValue(tftypes.String, tftypes.UnknownValue)}
		for k, v := range settings {
			elements[k] = tftypes.NewValue(tftypes.String, v)
		}
		return tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, elements)
	}

	cases := []struct {
		name      string
		state     tftypes.Value
		plan      tftypes.Value
		wantReset []string
	}{
		{
			name: "create",
			plan: clickHouseSettingsRaw(ctx, sch, tftypes.UnknownValue, "svc-1", clickHouseSettingsMapValue(managed)),
		},
		{
			name:  "no reset",
			state: clickHouseSettingsStateRaw(ctx, sch, "svc-1", managed),
			plan:  clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2", "max_query_size": "1048576", "max_threads": "8"}),
		},
		{
			name:      "removed key",
			state:     clickHouseSettingsStateRaw(ctx, sch, "svc-1", managed),
			plan:      clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "262144"}),
			wantReset: []string{"compatibility"},
		},
		{
			name:      "removed key other than compatibility",
			state:     clickHouseSettingsStateRaw(ctx, sch, "svc-1", managed),
			plan:      clickHouseSettingsStateRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2"}),
			wantReset: []string{"max_query_size"},
		},
		{
			name:      "destroy",
			state:     clickHouseSettingsStateRaw(ctx, sch, "svc-1", managed),
			wantReset: []string{"compatibility", "max_query_size"},
		},
		{
			name:      "service_id change",
			state:     clickHouseSettingsStateRaw(ctx, sch, "svc-1", managed),
			plan:      clickHouseSettingsRaw(ctx, sch, tftypes.UnknownValue, "svc-2", clickHouseSettingsMapValue(managed)),
			wantReset: []string{"compatibility", "max_query_size"},
		},
		{
			name:      "service_id unknown",
			state:     clickHouseSettingsStateRaw(ctx, sch, "svc-1", managed),
			plan:      clickHouseSettingsRaw(ctx, sch, tftypes.UnknownValue, tftypes.UnknownValue, clickHouseSettingsMapValue(managed)),
			wantReset: []string{"compatibility", "max_query_size"},
		},
		{
			name:  "unknown map",
			state: clickHouseSettingsStateRaw(ctx, sch, "svc-1", managed),
			plan:  clickHouseSettingsRaw(ctx, sch, "svc-1", "svc-1", unknownMap),
		},
		{
			name:  "unknown element",
			state: clickHouseSettingsStateRaw(ctx, sch, "svc-1", managed),
			plan:  clickHouseSettingsRaw(ctx, sch, "svc-1", "svc-1", unknownElement(map[string]string{"max_query_size": "262144"}, "compatibility")),
		},
		{
			name:      "unknown element with a removed key",
			state:     clickHouseSettingsStateRaw(ctx, sch, "svc-1", managed),
			plan:      clickHouseSettingsRaw(ctx, sch, "svc-1", "svc-1", unknownElement(nil, "max_query_size")),
			wantReset: []string{"compatibility"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nullRaw := tftypes.NewValue(sch.Type().TerraformType(ctx), nil)
			stateRaw, planRaw := tc.state, tc.plan
			if stateRaw.Type() == nil {
				stateRaw = nullRaw
			}
			if planRaw.Type() == nil {
				planRaw = nullRaw
			}

			req := resource.ModifyPlanRequest{
				State: tfsdk.State{Schema: sch, Raw: stateRaw},
				Plan:  tfsdk.Plan{Schema: sch, Raw: planRaw},
			}
			resp := &resource.ModifyPlanResponse{Plan: req.Plan}

			r.ModifyPlan(ctx, req, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("ModifyPlan must never error; got %v", resp.Diagnostics)
			}
			warnings := resp.Diagnostics.Warnings()
			if len(tc.wantReset) == 0 {
				if len(warnings) != 0 {
					t.Errorf("want no warning; got %v", warnings)
				}
				return
			}
			if len(warnings) != 1 || warnings[0].Summary() != "ClickHouse settings will be reset" {
				t.Fatalf("want one reset warning; got %v", warnings)
			}
			detail := warnings[0].Detail()
			if !strings.Contains(detail, strings.Join(tc.wantReset, ", ")+".") {
				t.Errorf("warning detail = %q; want it to name exactly %v", detail, tc.wantReset)
			}
			if !strings.Contains(detail, "platform default") {
				t.Errorf("warning detail = %q; want it to say the settings return to their platform defaults", detail)
			}
			if got, want := strings.Contains(detail, "sets compatibility when it creates a service"), slices.Contains(tc.wantReset, "compatibility"); got != want {
				t.Errorf("warning detail = %q; compatibility note present = %v, want %v", detail, got, want)
			}
		})
	}
}

func TestServiceClickHouseSettingsSchema_ValidatorsRejectInvalidMaps(t *testing.T) {
	ctx := context.Background()
	_, sch := clickHouseSettingsResourceWithSchema(t)

	settingsAttr, ok := sch.Attributes["settings"].(schema.MapAttribute)
	if !ok {
		t.Fatalf("settings attribute is not MapAttribute")
	}

	cases := []struct {
		name        string
		value       types.Map
		wantInvalid bool
	}{
		{
			name:  "valid",
			value: types.MapValueMust(types.StringType, map[string]attr.Value{"compatibility": types.StringValue("26.2")}),
		},
		{
			name:        "empty key",
			value:       types.MapValueMust(types.StringType, map[string]attr.Value{"": types.StringValue("1")}),
			wantInvalid: true,
		},
		{
			name:        "null value",
			value:       types.MapValueMust(types.StringType, map[string]attr.Value{"compatibility": types.StringNull()}),
			wantInvalid: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validator.MapRequest{Path: path.Root("settings"), ConfigValue: tc.value}
			resp := &validator.MapResponse{}
			for _, v := range settingsAttr.Validators {
				v.ValidateMap(ctx, req, resp)
			}
			if resp.Diagnostics.HasError() != tc.wantInvalid {
				t.Errorf("validation hasError=%v; want %v (diags=%v)", resp.Diagnostics.HasError(), tc.wantInvalid, resp.Diagnostics)
			}
		})
	}
}

func TestServiceClickHouseSettingsSchema_ServiceIDMustBeUUID(t *testing.T) {
	_, sch := clickHouseSettingsResourceWithSchema(t)

	serviceID, ok := sch.Attributes["service_id"].(schema.StringAttribute)
	if !ok || len(serviceID.Validators) != 1 {
		t.Fatalf("service_id must have one UUID validator: %#v", serviceID)
	}
	assertStringValidatorAccepts(t, serviceID.Validators[0], "11111111-1111-1111-1111-111111111111")
	assertStringValidatorRejects(t, serviceID.Validators[0], "")
}

// The package-wide suppression in TestMain hides the notice; this opts back in
// to check it reaches Create, Update and ImportState but not Read.
func TestServiceClickHouseSettingsResource_BetaNotice(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "false")

	ctx := context.Background()
	r, sch := clickHouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		ListClickHouseSettingsMock.
		Expect(ctx, "svc-1").
		Return(nil, nil)

	empty := clickHouseSettingsStateRaw(ctx, sch, "svc-1", nil)
	nullRaw := tftypes.NewValue(sch.Type().TerraformType(ctx), nil)

	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: nullRaw}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: empty}}, createResp)

	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: empty}}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: sch, Raw: empty}, State: tfsdk.State{Schema: sch, Raw: empty}}, updateResp)

	importResp := &resource.ImportStateResponse{State: tfsdk.State{Schema: sch, Raw: nullRaw}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "svc-1"}, importResp)

	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: sch, Raw: empty}}
	r.Read(ctx, resource.ReadRequest{State: tfsdk.State{Schema: sch, Raw: empty}}, readResp)

	for name, diags := range map[string]diag.Diagnostics{
		"Create":      createResp.Diagnostics,
		"Update":      updateResp.Diagnostics,
		"ImportState": importResp.Diagnostics,
	} {
		if diagnosticWith(diags, "Beta Resource") == nil {
			t.Errorf("%s emitted no beta notice; diagnostics = %v", name, diags)
		}
	}
	if diagnosticWith(readResp.Diagnostics, "Beta Resource") != nil {
		t.Errorf("Read must not emit the beta notice; diagnostics = %v", readResp.Diagnostics)
	}
}
