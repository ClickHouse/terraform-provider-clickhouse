package resource

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

func TestApplyUpgradeWindowToState_PopulatesAllFields(t *testing.T) {
	window := &api.UpgradeWindow{Weekday: 3, StartHourUtc: 12, Duration: 6}

	state := &models.ServiceUpgradeWindowResourceModel{
		ServiceID: types.StringValue("svc-1"),
	}

	applyUpgradeWindowToState(window, state)

	if state.Weekday.ValueInt64() != 3 {
		t.Errorf("Weekday = %d; want 3", state.Weekday.ValueInt64())
	}
	if state.StartHourUtc.ValueInt64() != 12 {
		t.Errorf("StartHourUtc = %d; want 12", state.StartHourUtc.ValueInt64())
	}
	if state.Duration.ValueInt64() != 6 {
		t.Errorf("Duration = %d; want 6", state.Duration.ValueInt64())
	}
}

// TestServiceUpgradeWindowResource_ImportState verifies that the custom
// ImportState handler writes both `id` and `service_id` from the user-supplied
// import ID.
func TestServiceUpgradeWindowResource_ImportState(t *testing.T) {
	ctx := context.Background()
	r := NewServiceUpgradeWindowResource().(*ServiceUpgradeWindowResource)

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", schemaResp.Diagnostics)
	}
	sch := schemaResp.Schema

	req := resource.ImportStateRequest{ID: "uw-import-1"}
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: sch,
			Raw:    tftypes.NewValue(sch.Type().TerraformType(ctx), nil),
		},
	}

	r.ImportState(ctx, req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diags: %v", resp.Diagnostics)
	}

	var id, serviceID types.String
	if d := resp.State.GetAttribute(ctx, path.Root("id"), &id); d.HasError() {
		t.Fatalf("read id: %v", d)
	}
	if d := resp.State.GetAttribute(ctx, path.Root("service_id"), &serviceID); d.HasError() {
		t.Fatalf("read service_id: %v", d)
	}
	if id.ValueString() != "uw-import-1" {
		t.Errorf("id = %q; want uw-import-1", id.ValueString())
	}
	if serviceID.ValueString() != "uw-import-1" {
		t.Errorf("service_id = %q; want uw-import-1", serviceID.ValueString())
	}
}

// TestServiceUpgradeWindowSchema_ValidatorsRejectInvalidValues exercises the
// attribute validators declared on the schema so the public-API contract
// (weekday 0-6, start_hour_utc in {0,6,12,18}) is enforced before any API call.
func TestServiceUpgradeWindowSchema_ValidatorsRejectInvalidValues(t *testing.T) {
	ctx := context.Background()
	r := NewServiceUpgradeWindowResource().(*ServiceUpgradeWindowResource)

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", schemaResp.Diagnostics)
	}

	weekdayAttr, ok := schemaResp.Schema.Attributes["weekday"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("weekday attribute is not Int64Attribute")
	}
	startHourAttr, ok := schemaResp.Schema.Attributes["start_hour_utc"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("start_hour_utc attribute is not Int64Attribute")
	}

	cases := []struct {
		name        string
		validators  []validator.Int64
		attrPath    path.Path
		value       int64
		wantInvalid bool
	}{
		{name: "weekday=-1", validators: weekdayAttr.Validators, attrPath: path.Root("weekday"), value: -1, wantInvalid: true},
		{name: "weekday=0", validators: weekdayAttr.Validators, attrPath: path.Root("weekday"), value: 0, wantInvalid: false},
		{name: "weekday=6", validators: weekdayAttr.Validators, attrPath: path.Root("weekday"), value: 6, wantInvalid: false},
		{name: "weekday=7", validators: weekdayAttr.Validators, attrPath: path.Root("weekday"), value: 7, wantInvalid: true},

		{name: "start_hour=0", validators: startHourAttr.Validators, attrPath: path.Root("start_hour_utc"), value: 0, wantInvalid: false},
		{name: "start_hour=3", validators: startHourAttr.Validators, attrPath: path.Root("start_hour_utc"), value: 3, wantInvalid: true},
		{name: "start_hour=6", validators: startHourAttr.Validators, attrPath: path.Root("start_hour_utc"), value: 6, wantInvalid: false},
		{name: "start_hour=18", validators: startHourAttr.Validators, attrPath: path.Root("start_hour_utc"), value: 18, wantInvalid: false},
		{name: "start_hour=24", validators: startHourAttr.Validators, attrPath: path.Root("start_hour_utc"), value: 24, wantInvalid: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.validators) == 0 {
				t.Fatalf("no validators on %s", tc.attrPath)
			}

			req := validator.Int64Request{
				Path:        tc.attrPath,
				ConfigValue: types.Int64Value(tc.value),
			}
			resp := &validator.Int64Response{}
			for _, v := range tc.validators {
				v.ValidateInt64(ctx, req, resp)
			}
			if resp.Diagnostics.HasError() != tc.wantInvalid {
				t.Errorf("validation hasError=%v; want %v (diags=%v)", resp.Diagnostics.HasError(), tc.wantInvalid, resp.Diagnostics)
			}
		})
	}
}

// TestServiceUpgradeWindowResource_Create_RefusesToClobber drives Create end to
// end with a mocked API client to lock in the documented behavior: if a window
// already exists for the service, Create must surface an "import it" diagnostic
// and never issue a PUT. This is the only branch of the resource where the
// provider intentionally deviates from PUT-as-upsert.
func TestServiceUpgradeWindowResource_Create_RefusesToClobber(t *testing.T) {
	ctx := context.Background()
	r := NewServiceUpgradeWindowResource().(*ServiceUpgradeWindowResource)

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", schemaResp.Diagnostics)
	}
	sch := schemaResp.Schema

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetUpgradeWindowMock.
		Expect(ctx, "svc-1").
		Return(&api.UpgradeWindow{Weekday: 0, StartHourUtc: 0, Duration: 6}, nil)
	// UpdateUpgradeWindowMock is intentionally not set — minimock fails the test
	// if Update is called, proving the clobber-guard short-circuits.

	planRaw := tftypes.NewValue(sch.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"service_id":     tftypes.NewValue(tftypes.String, "svc-1"),
		"weekday":        tftypes.NewValue(tftypes.Number, big.NewFloat(3)),
		"start_hour_utc": tftypes.NewValue(tftypes.Number, big.NewFloat(12)),
		"duration":       tftypes.NewValue(tftypes.Number, nil),
	})

	req := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: planRaw},
	}
	resp := &resource.CreateResponse{
		State: tfsdk.State{
			Schema: sch,
			Raw:    tftypes.NewValue(sch.Type().TerraformType(ctx), nil),
		},
	}

	r.Create(ctx, req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("Create should have produced an error diagnostic; got %v", resp.Diagnostics)
	}
	if got := resp.Diagnostics[0].Summary(); got != "Upgrade window already exists for this service" {
		t.Errorf("diagnostic summary = %q; want \"Upgrade window already exists for this service\"", got)
	}
}

// TestServiceUpgradeWindowResource_Create_404OnGetProceedsWithPut covers the
// happy path: GET returns 404 (no existing window), so Create issues a PUT and
// writes the returned window into state.
func TestServiceUpgradeWindowResource_Create_404OnGetProceedsWithPut(t *testing.T) {
	ctx := context.Background()
	r := NewServiceUpgradeWindowResource().(*ServiceUpgradeWindowResource)

	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema: %v", schemaResp.Diagnostics)
	}
	sch := schemaResp.Schema

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetUpgradeWindowMock.
		Expect(ctx, "svc-1").
		Return(nil, errors.New("status: 404, body: not found")).
		UpdateUpgradeWindowMock.
		Expect(ctx, "svc-1", api.UpgradeWindowUpdate{Weekday: 3, StartHourUtc: 12}).
		Return(&api.UpgradeWindow{Weekday: 3, StartHourUtc: 12, Duration: 6}, nil)

	planRaw := tftypes.NewValue(sch.Type().TerraformType(ctx), map[string]tftypes.Value{
		"id":             tftypes.NewValue(tftypes.String, nil),
		"service_id":     tftypes.NewValue(tftypes.String, "svc-1"),
		"weekday":        tftypes.NewValue(tftypes.Number, big.NewFloat(3)),
		"start_hour_utc": tftypes.NewValue(tftypes.Number, big.NewFloat(12)),
		"duration":       tftypes.NewValue(tftypes.Number, nil),
	})

	req := resource.CreateRequest{
		Plan: tfsdk.Plan{Schema: sch, Raw: planRaw},
	}
	resp := &resource.CreateResponse{
		State: tfsdk.State{
			Schema: sch,
			Raw:    tftypes.NewValue(sch.Type().TerraformType(ctx), nil),
		},
	}

	r.Create(ctx, req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Create returned diagnostics: %v", resp.Diagnostics)
	}

	var got models.ServiceUpgradeWindowResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.ID.ValueString() != "svc-1" {
		t.Errorf("state ID = %q; want svc-1", got.ID.ValueString())
	}
	if got.Weekday.ValueInt64() != 3 || got.StartHourUtc.ValueInt64() != 12 || got.Duration.ValueInt64() != 6 {
		t.Errorf("state values = (weekday=%d, start=%d, duration=%d); want (3, 12, 6)",
			got.Weekday.ValueInt64(), got.StartHourUtc.ValueInt64(), got.Duration.ValueInt64())
	}
}
