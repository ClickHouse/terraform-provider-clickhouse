package resource

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/gojuno/minimock/v3"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
)

var (
	errResetSettingsUnsupported = errors.New(`status: 400, body: {"error":"BAD_REQUEST: request body property can't be validated: resetSettings","status":400}`)
	errServiceNotFound          = errors.New(`status: 404, body: {"error":"NOT_FOUND: svc-1","status":404}`)
	errSchemaUnavailable        = errors.New(`status: 503, body: {"error":"unavailable","status":503}`)
)

// recordSettingsWrites records each PATCH and DELETE, in order, as "PATCH" or "DELETE <name>".
func recordSettingsWrites(mock *api.ClientMock) *[]string {
	var calls []string
	mock.UpdateServiceClickhouseSettingsMock.Inspect(func(context.Context, string, map[string]any, []string) {
		calls = append(calls, "PATCH")
	})
	mock.DeleteServiceClickhouseSettingMock.Inspect(func(_ context.Context, _ string, name string) {
		calls = append(calls, "DELETE "+name)
	})
	return &calls
}

func assertSettingsWrites(t *testing.T, got *[]string, want ...string) {
	t.Helper()
	if !slices.Equal(*got, want) {
		t.Errorf("writes = %v; want %v", *got, want)
	}
}

func TestServiceClickhouseSettingsResource_Delete_ResetsAllInOnePatch(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("compatibility", "max_query_size"), nil).
		UpdateServiceClickhouseSettingsMock.
		Times(1).
		Expect(ctx, "svc-1", map[string]any{}, []string{"compatibility", "max_query_size"}).
		Return(&api.ServiceClickhouseSettingsUpdateResult{
			Warnings: []api.ServiceClickhouseSettingWarning{{Name: "compatibility", Message: "restarts the service"}},
		}, nil)

	resp := runClickhouseSettingsDelete(ctx, r, sch, clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"compatibility":  "26.2",
		"max_query_size": "262144",
	}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags: %v", resp.Diagnostics)
	}
	if len(resp.Diagnostics) != 1 || resp.Diagnostics[0].Detail() != "restarts the service" {
		t.Errorf("diagnostics = %v; want one warning with detail \"restarts the service\"", resp.Diagnostics)
	}
}

// A setting that left the allowlist would make the API reject the whole PATCH, so it is not sent.
func TestServiceClickhouseSettingsResource_Delete_SkipsRetiredSettings(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("max_query_size"), nil).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{}, []string{"max_query_size"}).
		Return(&api.ServiceClickhouseSettingsUpdateResult{}, nil)

	resp := runClickhouseSettingsDelete(ctx, r, sch, clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"a_removed_setting": "1",
		"max_query_size":    "262144",
	}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags: %v", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Delete_OnlyRetiredSettingsSendsNoPatch(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("max_query_size"), nil)

	resp := runClickhouseSettingsDelete(ctx, r, sch, clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"a_removed_setting": "1"}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags: %v", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Delete_ServiceGoneBeforeSchemaRead(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(nil, errServiceNotFound)

	resp := runClickhouseSettingsDelete(ctx, r, sch, clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "262144"}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags: %v", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Delete_ServiceGoneBeforePatch(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("max_query_size"), nil).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{}, []string{"max_query_size"}).
		Return(nil, errServiceNotFound)

	resp := runClickhouseSettingsDelete(ctx, r, sch, clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "262144"}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags: %v", resp.Diagnostics)
	}
}

// Without the schema there is no way to tell which names are retired, so every name is sent.
func TestServiceClickhouseSettingsResource_Delete_SchemaReadFailureSendsEveryName(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(nil, errSchemaUnavailable).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{}, []string{"compatibility", "max_query_size"}).
		Return(&api.ServiceClickhouseSettingsUpdateResult{}, nil)

	resp := runClickhouseSettingsDelete(ctx, r, sch, clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"compatibility":  "26.2",
		"max_query_size": "262144",
	}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags: %v", resp.Diagnostics)
	}
}

// An API without resetSettings gets today's per-name DELETE, for the names the schema still lists.
func TestServiceClickhouseSettingsResource_Delete_FallsBackToDeletePerName(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	mock := api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("compatibility", "max_query_size"), nil).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{}, []string{"compatibility", "max_query_size"}).
		Return(nil, errResetSettingsUnsupported)
	mock.DeleteServiceClickhouseSettingMock.When(ctx, "svc-1", "compatibility").Then(nil)
	mock.DeleteServiceClickhouseSettingMock.When(ctx, "svc-1", "max_query_size").Then(nil)
	writes := recordSettingsWrites(mock)
	r.client = mock

	resp := runClickhouseSettingsDelete(ctx, r, sch, clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"a_removed_setting": "1",
		"compatibility":     "26.2",
		"max_query_size":    "262144",
	}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags: %v", resp.Diagnostics)
	}
	assertSettingsWrites(t, writes, "PATCH", "DELETE compatibility", "DELETE max_query_size")
}

// Without the schema a retired name stays in the PATCH, so its 400 falls back to per-name DELETE, which treats a 404 as already reset.
func TestServiceClickhouseSettingsResource_Delete_UnfilteredUnknownSettingFallsBackToDeletePerName(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	mock := api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(nil, errSchemaUnavailable).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{}, []string{"a_removed_setting", "max_query_size"}).
		Return(nil, errors.New(`status: 400, body: {"error":"BAD_REQUEST: Unknown setting: \"a_removed_setting\". Allowed settings: max_query_size","status":400}`))
	mock.DeleteServiceClickhouseSettingMock.
		When(ctx, "svc-1", "a_removed_setting").
		Then(errors.New(`status: 404, body: {"error":"NOT_FOUND: Unknown setting: a_removed_setting","status":404}`))
	mock.DeleteServiceClickhouseSettingMock.When(ctx, "svc-1", "max_query_size").Then(nil)
	writes := recordSettingsWrites(mock)
	r.client = mock

	resp := runClickhouseSettingsDelete(ctx, r, sch, clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"a_removed_setting": "1",
		"max_query_size":    "262144",
	}))

	if resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags: %v", resp.Diagnostics)
	}
	assertSettingsWrites(t, writes, "PATCH", "DELETE a_removed_setting", "DELETE max_query_size")
}

func TestServiceClickhouseSettingsResource_Update_FallsBackToPatchThenDeletePerName(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	mock := api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("max_query_size", "keep_alive_timeout"), nil)
	mock.UpdateServiceClickhouseSettingsMock.
		When(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}, []string{"keep_alive_timeout"}).
		Then(nil, errResetSettingsUnsupported)
	mock.UpdateServiceClickhouseSettingsMock.
		When(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}, nil).
		Then(&api.ServiceClickhouseSettingsUpdateResult{
			Settings: map[string]string{"max_query_size": "524288"},
			Warnings: []api.ServiceClickhouseSettingWarning{{Name: "max_query_size", Message: "restarts the service"}},
		}, nil)
	mock.UpdateServiceClickhouseSettingsMock.Times(2)
	mock.DeleteServiceClickhouseSettingMock.
		Expect(ctx, "svc-1", "keep_alive_timeout").
		Return(nil)
	writes := recordSettingsWrites(mock)
	r.client = mock

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"max_query_size":     "262144",
		"keep_alive_timeout": "30",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "524288"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diags: %v", resp.Diagnostics)
	}
	assertSettingsWrites(t, writes, "PATCH", "PATCH", "DELETE keep_alive_timeout")
	if len(resp.Diagnostics) != 1 || resp.Diagnostics[0].Detail() != "restarts the service" {
		t.Errorf("diagnostics = %v; want one warning with detail \"restarts the service\"", resp.Diagnostics)
	}
}

// The DELETEs would leave the service half-updated if the settings PATCH failed, so none are sent.
func TestServiceClickhouseSettingsResource_Update_FallbackStopsWhenSettingsPatchFails(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	mock := api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("max_query_size", "keep_alive_timeout"), nil)
	mock.UpdateServiceClickhouseSettingsMock.
		When(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}, []string{"keep_alive_timeout"}).
		Then(nil, errResetSettingsUnsupported)
	mock.UpdateServiceClickhouseSettingsMock.
		When(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}, nil).
		Then(nil, errors.New(`status: 500, body: {"error":"internal","status":500}`))
	r.client = mock

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"max_query_size":     "262144",
		"keep_alive_timeout": "30",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "524288"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("Update diags = %v; want an error", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Update_RemovingOnlyRetiredSettingsSendsNoPatch(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("max_query_size"), nil)

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"max_query_size":    "262144",
		"a_removed_setting": "1",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "262144"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diags: %v", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Update_UnknownSettingAfterSchemaFilterDoesNotFallBack(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("keep_alive_timeout"), nil).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{}, []string{"keep_alive_timeout"}).
		Return(nil, errors.New(`status: 400, body: {"error":"BAD_REQUEST: Unknown setting: \"keep_alive_timeout\"","status":400}`))

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"max_query_size":     "262144",
		"keep_alive_timeout": "30",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "262144"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("Update diags = %v; want an error", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Update_ServiceNotFoundIsAnError(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(nil, errServiceNotFound)

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"max_query_size":     "262144",
		"keep_alive_timeout": "30",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "262144"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("Update diags = %v; want an error", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Update_UnfilteredUnknownSettingFallsBackToPatchThenDeletePerName(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	mock := api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(nil, errSchemaUnavailable)
	mock.UpdateServiceClickhouseSettingsMock.
		When(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}, []string{"a_removed_setting", "keep_alive_timeout"}).
		Then(nil, errors.New(`status: 400, body: {"error":"BAD_REQUEST: Unknown setting: \"a_removed_setting\". Allowed settings: keep_alive_timeout, max_query_size","status":400}`))
	mock.UpdateServiceClickhouseSettingsMock.
		When(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}, nil).
		Then(&api.ServiceClickhouseSettingsUpdateResult{Settings: map[string]string{"max_query_size": "524288"}}, nil)
	mock.DeleteServiceClickhouseSettingMock.
		When(ctx, "svc-1", "a_removed_setting").
		Then(errors.New(`status: 404, body: {"error":"NOT_FOUND: Unknown setting: a_removed_setting","status":404}`))
	mock.DeleteServiceClickhouseSettingMock.When(ctx, "svc-1", "keep_alive_timeout").Then(nil)
	writes := recordSettingsWrites(mock)
	r.client = mock

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"max_query_size":     "262144",
		"keep_alive_timeout": "30",
		"a_removed_setting":  "1",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "524288"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diags: %v", resp.Diagnostics)
	}
	assertSettingsWrites(t, writes, "PATCH", "PATCH", "DELETE a_removed_setting", "DELETE keep_alive_timeout")
}

func TestServiceClickhouseSettingsResource_Update_ResetOnlyInOnePatch(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("max_query_size", "keep_alive_timeout"), nil).
		UpdateServiceClickhouseSettingsMock.
		Times(1).
		Expect(ctx, "svc-1", map[string]any{}, []string{"keep_alive_timeout", "max_query_size"}).
		Return(&api.ServiceClickhouseSettingsUpdateResult{}, nil)

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"compatibility":      "26.2",
		"max_query_size":     "262144",
		"keep_alive_timeout": "30",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"compatibility": "26.2"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diags: %v", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Update_ChangeWithOnlyRetiredRemovalsSendsNoResets(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("max_query_size"), nil).
		UpdateServiceClickhouseSettingsMock.
		Times(1).
		Expect(ctx, "svc-1", map[string]any{"max_query_size": int64(524288)}, nil).
		Return(&api.ServiceClickhouseSettingsUpdateResult{}, nil)

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"max_query_size":    "262144",
		"a_removed_setting": "1",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "524288"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update diags: %v", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Update_PatchNotFoundIsAnError(t *testing.T) {
	t.Setenv("CLICKHOUSE_SUPPRESS_BETA_WARNINGS", "true")
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("keep_alive_timeout"), nil).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{}, []string{"keep_alive_timeout"}).
		Return(nil, errServiceNotFound)

	state := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{
		"max_query_size":     "262144",
		"keep_alive_timeout": "30",
	})
	plan := clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "262144"})

	resp := runClickhouseSettingsUpdate(ctx, r, sch, state, plan)

	if !resp.Diagnostics.HasError() {
		t.Fatalf("Update diags = %v; want an error", resp.Diagnostics)
	}
}

func TestServiceClickhouseSettingsResource_Delete_PatchServerErrorIsAnError(t *testing.T) {
	ctx := context.Background()
	r, sch := clickhouseSettingsResourceWithSchema(t)

	mc := minimock.NewController(t)
	r.client = api.NewClientMock(mc).
		GetServiceClickhouseSettingsSchemaMock.
		Expect(ctx, "svc-1").
		Return(clickhouseSettingsSchema("max_query_size"), nil).
		UpdateServiceClickhouseSettingsMock.
		Expect(ctx, "svc-1", map[string]any{}, []string{"max_query_size"}).
		Return(nil, errors.New(`status: 500, body: {"error":"internal","status":500}`))

	resp := runClickhouseSettingsDelete(ctx, r, sch, clickhouseSettingsRaw(ctx, sch, "svc-1", map[string]string{"max_query_size": "262144"}))

	if !resp.Diagnostics.HasError() {
		t.Fatalf("Delete diags = %v; want an error", resp.Diagnostics)
	}
}
