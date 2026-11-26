package resource

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/utils"
)

func byocEnrollmentTokenSchema(t *testing.T) (*ByocEnrollmentTokenResource, resource.SchemaResponse) {
	t.Helper()
	r := NewByocEnrollmentTokenResource().(*ByocEnrollmentTokenResource)
	resp := resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema diagnostics: %v", resp.Diagnostics)
	}
	return r, resp
}

func byocEnrollmentTokenPlan(t *testing.T, schemaResp resource.SchemaResponse, ttl types.Int64) tfsdk.Plan {
	t.Helper()
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	model := byocEnrollmentTokenModel{
		ID:         types.StringUnknown(),
		ByocID:     types.StringValue(byocInfraID),
		TTLSeconds: ttl,
		Token:      types.StringUnknown(),
		ExpiresAt:  types.StringUnknown(),
	}
	if diags := plan.Set(context.Background(), &model); diags.HasError() {
		t.Fatalf("set plan: %v", diags)
	}
	return plan
}

func shortenByocEnrollmentTokenWait(t *testing.T, wait time.Duration) {
	t.Helper()
	prevWait, prevInterval := byocEnrollmentTokenConflictWait, byocEnrollmentTokenConflictInterval
	byocEnrollmentTokenConflictWait, byocEnrollmentTokenConflictInterval = wait, time.Millisecond
	t.Cleanup(func() {
		byocEnrollmentTokenConflictWait, byocEnrollmentTokenConflictInterval = prevWait, prevInterval
	})
}

func TestByocEnrollmentTokenSchemaMarksTokenSensitive(t *testing.T) {
	_, schemaResp := byocEnrollmentTokenSchema(t)
	attr, ok := schemaResp.Schema.Attributes["token"]
	if !ok || !attr.IsSensitive() {
		t.Fatal("token must be a sensitive attribute")
	}
}

func TestByocEnrollmentTokenCreateRetriesUntilTrustAdmitted(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	shortenByocEnrollmentTokenWait(t, time.Minute)
	ctx := context.Background()
	r, schemaResp := byocEnrollmentTokenSchema(t)
	plan := byocEnrollmentTokenPlan(t, schemaResp, types.Int64Value(7200))

	calls := 0
	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateByocInfrastructureEnrollmentTokenMock.
		Set(func(_ context.Context, byocId string, req api.ByocEnrollmentTokenRequest) (*api.ByocEnrollmentToken, error) {
			calls++
			if byocId != byocInfraID {
				t.Errorf("byocId = %q; want %q", byocId, byocInfraID)
			}
			if req.TtlSeconds == nil || *req.TtlSeconds != 7200 {
				t.Errorf("ttlSeconds = %v; want 7200", req.TtlSeconds)
			}
			if calls < 3 {
				return nil, errors.New("status: 409, body: trust not admitted")
			}
			return &api.ByocEnrollmentToken{Token: "enroll-secret", ExpiresAt: "2026-01-01T02:00:00Z"}, nil
		})

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
	if calls != 3 {
		t.Errorf("mint calls = %d; want 3 (two conflicts, then success)", calls)
	}
	var state byocEnrollmentTokenModel
	if diags := resp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if state.ID.ValueString() != byocInfraID || state.Token.ValueString() != "enroll-secret" || state.ExpiresAt.ValueString() != "2026-01-01T02:00:00Z" {
		t.Errorf("state = %+v", state)
	}
}

func TestByocEnrollmentTokenCreateOmitsUnsetTTL(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocEnrollmentTokenSchema(t)
	plan := byocEnrollmentTokenPlan(t, schemaResp, types.Int64Null())

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateByocInfrastructureEnrollmentTokenMock.
		Expect(minimock.AnyContext, byocInfraID, api.ByocEnrollmentTokenRequest{}).
		Return(&api.ByocEnrollmentToken{Token: "enroll-secret", ExpiresAt: "2026-01-01T04:00:00Z"}, nil)

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", resp.Diagnostics)
	}
}

func TestByocEnrollmentTokenCreateFailsFastOnOtherErrors(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	shortenByocEnrollmentTokenWait(t, time.Minute)
	ctx := context.Background()
	r, schemaResp := byocEnrollmentTokenSchema(t)
	plan := byocEnrollmentTokenPlan(t, schemaResp, types.Int64Null())

	mc := minimock.NewController(t)
	client := api.NewClientMock(mc)
	client.CreateByocInfrastructureEnrollmentTokenMock.
		Return(nil, errors.New("status: 400, body: infrastructure is not restricted-access"))
	r.client = client

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create should fail on a 400")
	}
	if n := len(client.CreateByocInfrastructureEnrollmentTokenMock.Calls()); n != 1 {
		t.Errorf("mint calls = %d; only 409 is retried", n)
	}
}

func TestByocEnrollmentTokenCreateGivesUpAfterConflictWait(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	shortenByocEnrollmentTokenWait(t, 50*time.Millisecond)
	ctx := context.Background()
	r, schemaResp := byocEnrollmentTokenSchema(t)
	plan := byocEnrollmentTokenPlan(t, schemaResp, types.Int64Null())

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		CreateByocInfrastructureEnrollmentTokenMock.
		Return(nil, errors.New("status: 409, body: trust not admitted"))

	resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Create should fail once the conflict wait is exhausted")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(detail, "status: 409") || !strings.Contains(detail, "cluster trust") {
		t.Errorf("detail = %q; want the last conflict and the trust hint", detail)
	}
}

func TestByocEnrollmentTokenReadNeverMints(t *testing.T) {
	ctx := context.Background()
	r, schemaResp := byocEnrollmentTokenSchema(t)
	state := tfsdk.State{Schema: schemaResp.Schema}
	want := byocEnrollmentTokenModel{
		ID:         types.StringValue(byocInfraID),
		ByocID:     types.StringValue(byocInfraID),
		TTLSeconds: types.Int64Null(),
		Token:      types.StringValue("enroll-secret"),
		ExpiresAt:  types.StringValue("2026-01-01T04:00:00Z"),
	}
	if diags := state.Set(ctx, &want); diags.HasError() {
		t.Fatalf("set state: %v", diags)
	}
	// No expectations: any API call fails the test.
	r.client = api.NewClientMock(minimock.NewController(t))

	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", resp.Diagnostics)
	}
	var got byocEnrollmentTokenModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got != want {
		t.Errorf("state = %+v; want unchanged %+v", got, want)
	}
}

func TestByocEnrollmentTokenImportLeavesTokenNull(t *testing.T) {
	t.Setenv(utils.SuppressBetaWarningsEnvVar, "true")
	ctx := context.Background()
	r, schemaResp := byocEnrollmentTokenSchema(t)
	resp := resource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: byocInfraID}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("ImportState diagnostics: %v", resp.Diagnostics)
	}
	var byocID, token types.String
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("byoc_id"), &byocID)...)
	resp.Diagnostics.Append(resp.State.GetAttribute(ctx, path.Root("token"), &token)...)
	if byocID.ValueString() != byocInfraID {
		t.Errorf("byoc_id = %v; want %q", byocID, byocInfraID)
	}
	if !token.IsNull() {
		t.Errorf("token = %v; the API never returns an existing token", token)
	}
}

func TestByocWholeMinutesValidator(t *testing.T) {
	for value, wantErr := range map[int64]bool{3600: false, 7260: false, 3601: true} {
		resp := validator.Int64Response{}
		byocWholeMinutesValidator{}.ValidateInt64(context.Background(), validator.Int64Request{
			Path:        path.Root("ttl_seconds"),
			ConfigValue: types.Int64Value(value),
		}, &resp)
		if resp.Diagnostics.HasError() != wantErr {
			t.Errorf("ttl_seconds = %d: error = %v; want %v", value, resp.Diagnostics.HasError(), wantErr)
		}
	}
}
