package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// A table_mappings-only edit must PATCH just the mapping deltas, not re-send the
// connection. Re-sending the uri/host while omitting unchanged credentials makes
// the control plane re-validate the source connection with no auth, which fails
// with AuthenticationFailed (issue #617).
func TestClickPipeResource_Update_MappingsOnlyOmitsConnection_Issue617(t *testing.T) {
	ctx := context.Background()
	// Only table_mappings differ (remove "orders"); host/port/database/credentials
	// /settings are identical in state and plan, so otherFieldsChanged is false.
	state := postgresUpdateModel(ctx, t, "users", "orders")
	plan := postgresUpdateModel(ctx, t, "users")

	mc := minimock.NewController(t)
	syncPipe := postgresAPIPipe(api.ClickPipeRunningState, "users")
	mock, calls := pauseEditClientMock(mc, syncPipe, func(int) (*api.ClickPipe, error) {
		return postgresAPIPipe(api.ClickPipePausedState, "users"), nil
	})

	// Capture the PATCH payload the provider builds.
	var captured api.ClickPipeUpdate
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
		*calls = append(*calls, "update")
		captured = update
		return postgresAPIPipe(api.ClickPipePausedState, "users"), nil
	})
	expectSyncRead(mock, calls, syncPipe)

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())

	require.NotNil(t, captured.Source)
	require.NotNil(t, captured.Source.Postgres)
	pg := captured.Source.Postgres

	// Connection fields must be omitted so no re-validation is triggered.
	assert.Empty(t, pg.Host, "host must not be re-sent on a mappings-only edit")
	assert.Zero(t, pg.Port, "port must not be re-sent on a mappings-only edit")
	assert.Empty(t, pg.Database, "database must not be re-sent on a mappings-only edit")
	assert.Nil(t, pg.Credentials, "credentials must not be re-sent on a mappings-only edit")
	assert.Nil(t, pg.Settings, "settings must not be re-sent on a mappings-only edit")

	// The mapping delta itself must still be carried.
	require.Len(t, pg.TableMappingsToRemove, 1, "the removed mapping must be sent")
	assert.Equal(t, "orders", pg.TableMappingsToRemove[0].SourceTable)
	assert.Empty(t, pg.TableMappingsToAdd)
}

// A connection change made in the same apply as a table_mappings change must NOT
// be dropped. The mappings-only shortcut applies only when nothing else changed;
// otherFieldsChanged now compares every non-mapping attribute (rather than an
// enumerated subset), so fields like host/authentication/tls_host can't slip
// through and get overwritten by a minimal payload (Copilot review on #618).
func TestClickPipeResource_Update_ConnectionChangeAlongsideMappingsNotDropped(t *testing.T) {
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users", "orders") // adds "orders"

	// Change a connection field in the same apply as the mapping add.
	var src models.ClickPipeSourceModel
	require.False(t, plan.Source.As(ctx, &src, basetypes.ObjectAsOptions{}).HasError())
	var pg models.ClickPipePostgresSourceModel
	require.False(t, src.Postgres.As(ctx, &pg, basetypes.ObjectAsOptions{}).HasError())
	pg.Host = types.StringValue("new-host.example.com")
	src.Postgres = pg.ObjectValue()
	plan.Source = src.ObjectValue()

	mc := minimock.NewController(t)
	syncPipe := postgresAPIPipe(api.ClickPipeRunningState, "users", "orders")
	mock, calls := pauseEditClientMock(mc, syncPipe, func(int) (*api.ClickPipe, error) {
		return postgresAPIPipe(api.ClickPipePausedState, "users", "orders"), nil
	})

	var captured api.ClickPipeUpdate
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
		*calls = append(*calls, "update")
		captured = update
		return postgresAPIPipe(api.ClickPipePausedState, "users", "orders"), nil
	})
	expectSyncRead(mock, calls, syncPipe)

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())

	require.NotNil(t, captured.Source)
	require.NotNil(t, captured.Source.Postgres)
	// The connection change must be carried, not dropped by the mappings-only shortcut.
	assert.Equal(t, "new-host.example.com", captured.Source.Postgres.Host)
	// And the mapping delta is still present.
	require.Len(t, captured.Source.Postgres.TableMappingsToAdd, 1)
	assert.Equal(t, "orders", captured.Source.Postgres.TableMappingsToAdd[0].SourceTable)
}

// A table_mappings-only edit on a pipe using write-only credentials
// (password_wo/password_wo_version) must still take the mappings-only shortcut.
// password_wo is always populated in plan (from config) and always null in state
// (per the write-only contract), even when nothing about the credentials actually
// changed. sourceFieldsChangedIgnoringMappings must not treat that expected
// divergence as a "connection changed" signal, or it permanently disables the
// shortcut for every write-only-credential pipe and reopens #617 for them.
func TestClickPipeResource_Update_MappingsOnlyWithWriteOnlyCredentials(t *testing.T) {
	ctx := context.Background()
	state := postgresUpdateModel(ctx, t, "users")
	plan := postgresUpdateModel(ctx, t, "users", "orders") // adds "orders" only

	setSteadyStateWriteOnlyCreds := func(m *models.ClickPipeResourceModel, passwordWOInPlan bool) {
		var src models.ClickPipeSourceModel
		require.False(t, m.Source.As(ctx, &src, basetypes.ObjectAsOptions{}).HasError())
		var pg models.ClickPipePostgresSourceModel
		require.False(t, src.Postgres.As(ctx, &pg, basetypes.ObjectAsOptions{}).HasError())
		var cred models.ClickPipeSourceCredentialsModel
		require.False(t, pg.Credentials.As(ctx, &cred, basetypes.ObjectAsOptions{}).HasError())
		cred.Password = types.StringNull()
		if passwordWOInPlan {
			cred.PasswordWO = types.StringValue("supersecret")
		} else {
			cred.PasswordWO = types.StringNull()
		}
		// password_wo_version is not write-only; it stays the same in plan and
		// state when there is no rotation, exactly like this mappings-only edit.
		cred.PasswordWOVersion = types.Int64Value(1)
		pg.Credentials = cred.ObjectValue()
		src.Postgres = pg.ObjectValue()
		m.Source = src.ObjectValue()
	}
	setSteadyStateWriteOnlyCreds(&state, false) // state: password_wo always nulled out
	setSteadyStateWriteOnlyCreds(&plan, true)   // plan: password_wo always carries the config value

	mc := minimock.NewController(t)
	syncPipe := postgresAPIPipe(api.ClickPipeRunningState, "users", "orders")
	mock, calls := pauseEditClientMock(mc, syncPipe, func(int) (*api.ClickPipe, error) {
		return postgresAPIPipe(api.ClickPipePausedState, "users", "orders"), nil
	})

	var captured api.ClickPipeUpdate
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
		*calls = append(*calls, "update")
		captured = update
		return postgresAPIPipe(api.ClickPipePausedState, "users", "orders"), nil
	})
	expectSyncRead(mock, calls, syncPipe)

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())

	require.NotNil(t, captured.Source)
	require.NotNil(t, captured.Source.Postgres)
	pg := captured.Source.Postgres

	// The write-only credentials divergence must not defeat the shortcut: the
	// connection must still be omitted, exactly as in the stateful-password case.
	assert.Empty(t, pg.Host, "host must not be re-sent on a mappings-only edit, even with write-only credentials")
	assert.Nil(t, pg.Credentials, "credentials must not be re-sent on a mappings-only edit")
	assert.Nil(t, pg.Settings, "settings must not be re-sent on a mappings-only edit")

	require.Len(t, pg.TableMappingsToAdd, 1, "the added mapping must be sent")
	assert.Equal(t, "orders", pg.TableMappingsToAdd[0].SourceTable)
}
