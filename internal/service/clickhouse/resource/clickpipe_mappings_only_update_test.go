package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
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
