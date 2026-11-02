package resource

import (
	"context"
	"testing"
	"time"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

type postgresWriteOnlyModelArgs struct {
	username          string
	passwordWO        types.String
	passwordWOVersion int64
	caCertificate     types.String
}

// postgresWriteOnlyModel returns the Postgres fixture using write-only credentials.
// password_wo is only ever populated in plan (from config); state always holds null for it.
func postgresWriteOnlyModel(ctx context.Context, t *testing.T, args postgresWriteOnlyModelArgs) models.ClickPipeResourceModel {
	t.Helper()

	m := postgresUpdateModel(ctx, t, "users")
	var src models.ClickPipeSourceModel
	require.False(t, m.Source.As(ctx, &src, basetypes.ObjectAsOptions{}).HasError())
	var pg models.ClickPipePostgresSourceModel
	require.False(t, src.Postgres.As(ctx, &pg, basetypes.ObjectAsOptions{}).HasError())
	var cred models.ClickPipeSourceCredentialsModel
	require.False(t, pg.Credentials.As(ctx, &cred, basetypes.ObjectAsOptions{}).HasError())

	cred.Username = types.StringValue(args.username)
	cred.Password = types.StringNull()
	cred.PasswordWO = args.passwordWO
	cred.PasswordWOVersion = types.Int64Value(args.passwordWOVersion)
	pg.Credentials = cred.ObjectValue()
	pg.CACertificate = args.caCertificate
	src.Postgres = pg.ObjectValue()
	m.Source = src.ObjectValue()
	return m
}

func TestClickPipeUpdate_PostgresWriteOnlyCredentialsAndCACertificate(t *testing.T) {
	ctx := context.Background()
	state := postgresWriteOnlyModel(ctx, t, postgresWriteOnlyModelArgs{
		username:          "old-user",
		passwordWO:        types.StringNull(),
		passwordWOVersion: 1,
		caCertificate:     types.StringNull(),
	})
	plan := postgresWriteOnlyModel(ctx, t, postgresWriteOnlyModelArgs{
		username:          "new-user",
		passwordWO:        types.StringValue("new-secret"),
		passwordWOVersion: 2,
		caCertificate:     types.StringValue("PEM-CA"),
	})

	apiPipe := postgresAPIPipe(api.ClickPipeRunningState, "users")
	var captured []api.ClickPipeUpdate
	mock := api.NewClientMock(minimock.NewController(t))
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
		captured = append(captured, update)
		return apiPipe, nil
	})
	mock.WaitForClickPipeStateMock.Set(func(_ context.Context, _, _ string, _ func(string) bool, _ time.Duration) (*api.ClickPipe, error) {
		return apiPipe, nil
	})
	mock.GetClickPipeMock.Set(func(_ context.Context, _, _ string) (*api.ClickPipe, error) {
		return apiPipe, nil
	})

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update failed: %v", resp.Diagnostics.Errors())

	require.Len(t, captured, 1, "all changes must go in one PATCH")
	require.NotNil(t, captured[0].Source)
	pg := captured[0].Source.Postgres
	require.NotNil(t, pg, "update payload carries no postgres source")
	require.NotNil(t, pg.Credentials, "changed credentials must be sent")
	assert.Equal(t, "new-user", pg.Credentials.Username)
	assert.Equal(t, "new-secret", pg.Credentials.Password, "password_wo from config must be sent as password")
	require.NotNil(t, pg.CACertificate, "the ca_certificate change must be sent")
	assert.Equal(t, "PEM-CA", *pg.CACertificate)
}
