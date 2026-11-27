package resource

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func postgresMutualTLSNull() types.Object {
	return types.ObjectNull(models.ClickPipePostgresMutualTLSModel{}.ObjectType().AttrTypes)
}

func postgresMutualTLSValue(certificate, privateKey string) types.Object {
	return models.ClickPipePostgresMutualTLSModel{
		Certificate: types.StringValue(certificate),
		PrivateKey:  types.StringValue(privateKey),
	}.ObjectValue()
}

// withPostgresSource returns m with mutate applied to its Postgres source.
func withPostgresSource(ctx context.Context, t *testing.T, m models.ClickPipeResourceModel, mutate func(pg *models.ClickPipePostgresSourceModel)) models.ClickPipeResourceModel {
	t.Helper()

	var src models.ClickPipeSourceModel
	require.False(t, m.Source.As(ctx, &src, basetypes.ObjectAsOptions{}).HasError())
	var pg models.ClickPipePostgresSourceModel
	require.False(t, src.Postgres.As(ctx, &pg, basetypes.ObjectAsOptions{}).HasError())
	mutate(&pg)
	src.Postgres = pg.ObjectValue()
	m.Source = src.ObjectValue()
	return m
}

// postgresMutualTLSModel returns the Postgres update fixture with the given
// mutual_tls block.
func postgresMutualTLSModel(ctx context.Context, t *testing.T, mutualTLS types.Object, tables ...string) models.ClickPipeResourceModel {
	t.Helper()

	return withPostgresSource(ctx, t, postgresUpdateModel(ctx, t, tables...), func(pg *models.ClickPipePostgresSourceModel) {
		pg.MutualTLS = mutualTLS
	})
}

func postgresSourceFromModel(ctx context.Context, t *testing.T, m models.ClickPipeResourceModel) models.ClickPipePostgresSourceModel {
	t.Helper()

	var src models.ClickPipeSourceModel
	require.False(t, m.Source.As(ctx, &src, basetypes.ObjectAsOptions{}).HasError())
	var pg models.ClickPipePostgresSourceModel
	require.False(t, src.Postgres.As(ctx, &pg, basetypes.ObjectAsOptions{}).HasError())
	return pg
}

func TestClickPipeSchema_PostgresMutualTLS(t *testing.T) {
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	(&ClickPipeResource{}).Schema(ctx, resource.SchemaRequest{}, schemaResp)
	require.False(t, schemaResp.Diagnostics.HasError())

	source := schemaResp.Schema.Attributes["source"].(schema.SingleNestedAttribute)
	postgres := source.Attributes["postgres"].(schema.SingleNestedAttribute)
	mutualTLS, ok := postgres.Attributes["mutual_tls"].(schema.SingleNestedAttribute)
	require.True(t, ok, "postgres.mutual_tls must be a single nested attribute")

	assert.True(t, mutualTLS.Optional)
	assert.False(t, mutualTLS.Computed, "mutual_tls is never returned by the API, so it must not be computed")
	for _, name := range []string{"certificate", "private_key"} {
		attribute, ok := mutualTLS.Attributes[name].(schema.StringAttribute)
		require.True(t, ok, "mutual_tls.%s must be a string attribute", name)
		assert.True(t, attribute.Required, "mutual_tls.%s must be required", name)
		assert.True(t, attribute.Sensitive, "mutual_tls.%s must be sensitive", name)
	}

	// The model's object type must match the schema, or state encoding panics.
	assert.Equal(t,
		postgres.GetType(),
		models.ClickPipePostgresSourceModel{}.ObjectType(),
	)
}

// A zero-value model (mutual_tls never assigned) must build a valid object with
// a null mutual_tls rather than panic.
func TestClickPipePostgresSourceModel_ObjectValueDefaultsMutualTLSToNull(t *testing.T) {
	ctx := context.Background()
	pg := postgresSourceFromModel(ctx, t, postgresUpdateModel(ctx, t, "users"))
	pg.MutualTLS = types.Object{}

	var decoded models.ClickPipePostgresSourceModel
	require.False(t, pg.ObjectValue().As(ctx, &decoded, basetypes.ObjectAsOptions{}).HasError())
	assert.True(t, decoded.MutualTLS.IsNull())
	assert.Equal(t, postgresMutualTLSNull(), decoded.MutualTLS)
}

func TestExtractSourceFromPlan_PostgresMutualTLS(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	t.Run("set on create is sent", func(t *testing.T) {
		plan := postgresMutualTLSModel(ctx, t, postgresMutualTLSValue("PEM-CERT", "PEM-KEY"), "users")

		diagnostics := diag.Diagnostics{}
		source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

		require.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
		require.NotNil(t, source.Postgres)
		require.NotNil(t, source.Postgres.MutualTLS)
		require.False(t, source.Postgres.MutualTLS.IsNull())
		assert.Equal(t, api.ClickPipePostgresMutualTLS{Certificate: "PEM-CERT", PrivateKey: "PEM-KEY"}, *source.Postgres.MutualTLS.Value)
	})

	t.Run("unset on create is omitted", func(t *testing.T) {
		plan := postgresMutualTLSModel(ctx, t, postgresMutualTLSNull(), "users")

		diagnostics := diag.Diagnostics{}
		source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

		require.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
		require.NotNil(t, source.Postgres)
		assert.Nil(t, source.Postgres.MutualTLS, "an unset mutual_tls must be omitted, not sent as null")
	})

	t.Run("rejected with disable_tls", func(t *testing.T) {
		plan := withPostgresSource(ctx, t, postgresMutualTLSModel(ctx, t, postgresMutualTLSValue("PEM-CERT", "PEM-KEY"), "users"), func(pg *models.ClickPipePostgresSourceModel) {
			pg.DisableTLS = types.BoolValue(true)
		})

		diagnostics := diag.Diagnostics{}
		source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

		assert.Nil(t, source)
		require.True(t, diagnostics.HasError())
		assert.Contains(t, diagnostics.Errors()[0].Detail(), "disable_tls")
	})
}

func TestValidatePostgresMutualTLSConfiguration(t *testing.T) {
	ctx := context.Background()
	mutualTLS := postgresMutualTLSValue("PEM-CERT", "PEM-KEY")

	tests := []struct {
		name      string
		mutate    func(pg *models.ClickPipePostgresSourceModel)
		wantError string
	}{
		{
			name:   "no mutual_tls with disable_tls is valid",
			mutate: func(pg *models.ClickPipePostgresSourceModel) { pg.DisableTLS = types.BoolValue(true) },
		},
		{
			name:   "mutual_tls with TLS enabled is valid",
			mutate: func(pg *models.ClickPipePostgresSourceModel) { pg.MutualTLS = mutualTLS },
		},
		{
			name: "mutual_tls with explicit disable_tls false is valid",
			mutate: func(pg *models.ClickPipePostgresSourceModel) {
				pg.MutualTLS = mutualTLS
				pg.DisableTLS = types.BoolValue(false)
			},
		},
		{
			name: "mutual_tls with disable_tls is rejected",
			mutate: func(pg *models.ClickPipePostgresSourceModel) {
				pg.MutualTLS = mutualTLS
				pg.DisableTLS = types.BoolValue(true)
			},
			wantError: "cannot be set when disable_tls is true",
		},
		{
			name: "mutual_tls with unknown disable_tls is deferred",
			mutate: func(pg *models.ClickPipePostgresSourceModel) {
				pg.MutualTLS = mutualTLS
				pg.DisableTLS = types.BoolUnknown()
			},
		},
		{
			name: "unknown mutual_tls with disable_tls is deferred",
			mutate: func(pg *models.ClickPipePostgresSourceModel) {
				pg.MutualTLS = types.ObjectUnknown(models.ClickPipePostgresMutualTLSModel{}.ObjectType().AttrTypes)
				pg.DisableTLS = types.BoolValue(true)
			},
		},
		{
			name: "mutual_tls with workload identity is rejected",
			mutate: func(pg *models.ClickPipePostgresSourceModel) {
				pg.MutualTLS = mutualTLS
				pg.Authentication = types.StringValue(api.ClickPipeAuthenticationServiceAccountWorkloadIdentity)
			},
			wantError: "SERVICE_ACCOUNT_WORKLOAD_IDENTITY",
		},
		{
			name: "mutual_tls with IAM role authentication is valid",
			mutate: func(pg *models.ClickPipePostgresSourceModel) {
				pg.MutualTLS = mutualTLS
				pg.Authentication = types.StringValue(api.ClickPipeAuthenticationIAMRole)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := withPostgresSource(ctx, t, postgresUpdateModel(ctx, t, "users"), tt.mutate)

			diagnostics := diag.Diagnostics{}
			validatePostgresMutualTLSConfiguration(ctx, plan.Source, &diagnostics)

			if tt.wantError == "" {
				assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
				return
			}
			require.Len(t, diagnostics.Errors(), 1)
			assert.Equal(t, "Invalid attribute combination", diagnostics.Errors()[0].Summary())
			assert.Contains(t, diagnostics.Errors()[0].Detail(), tt.wantError)
		})
	}

	t.Run("non-postgres and null sources are ignored", func(t *testing.T) {
		diagnostics := diag.Diagnostics{}
		validatePostgresMutualTLSConfiguration(ctx, types.ObjectNull(models.ClickPipeSourceModel{}.ObjectType().AttrTypes), &diagnostics)
		validatePostgresMutualTLSConfiguration(ctx, buildMySQLServerIDPlan(types.Int64Null()).Source, &diagnostics)
		assert.False(t, diagnostics.HasError(), "expected no errors, got: %v", diagnostics.Errors())
	})
}

// The API never returns mutualTls, so a read must carry the block over from
// prior state and leave it null when state has none (e.g. after an import).
func TestClickPipeResource_syncClickPipeState_PostgresMutualTLS(t *testing.T) {
	ctx := context.Background()

	stored := postgresMutualTLSValue("PEM-CERT", "PEM-KEY")
	tests := []struct {
		name      string
		state     types.Object
		apiStored bool
		want      types.Object
	}{
		{name: "preserved from state while the API reports one", state: stored, apiStored: true, want: stored},
		{name: "dropped when it was removed outside Terraform", state: stored, apiStored: false, want: postgresMutualTLSNull()},
		{name: "stays null when state has none", state: postgresMutualTLSNull(), apiStored: false, want: postgresMutualTLSNull()},
		{name: "stays null after an import, since it cannot be read back", state: postgresMutualTLSNull(), apiStored: true, want: postgresMutualTLSNull()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := postgresMutualTLSModel(ctx, t, tt.state, "users")

			mock := api.NewClientMock(minimock.NewController(t))
			mock.GetClickPipeMock.Return(withStoredMutualTLS(postgresAPIPipe(api.ClickPipeRunningState, "users"), tt.apiStored), nil)

			require.NoError(t, (&ClickPipeResource{client: mock}).syncClickPipeState(ctx, &state))

			pg := postgresSourceFromModel(ctx, t, state)
			assert.True(t, pg.MutualTLS.Equal(tt.want), "want mutual_tls %s, got %s", tt.want, pg.MutualTLS)
		})
	}
}

// withStoredMutualTLS makes a mock API pipe report a stored client certificate
// the way the API does, with {"configured": true}.
func withStoredMutualTLS(pipe *api.ClickPipe, stored bool) *api.ClickPipe {
	if stored {
		pipe.Source.Postgres.MutualTLS = api.NullableValue(api.ClickPipePostgresMutualTLS{Configured: true})
	}
	return pipe
}

// drivePostgresMutualTLSUpdate runs Update from state to plan against a mock
// API and returns every PATCH payload the provider sent.
func drivePostgresMutualTLSUpdate(ctx context.Context, t *testing.T, state, plan models.ClickPipeResourceModel, tables ...string) []api.ClickPipeUpdate {
	t.Helper()

	// The API reports a stored certificate whenever the plan ends with one.
	stored := !postgresSourceFromModel(ctx, t, plan).MutualTLS.IsNull()
	syncPipe := withStoredMutualTLS(postgresAPIPipe(api.ClickPipeRunningState, tables...), stored)
	mock, calls := pauseEditClientMock(minimock.NewController(t), syncPipe, func(int) (*api.ClickPipe, error) {
		return withStoredMutualTLS(postgresAPIPipe(api.ClickPipePausedState, tables...), stored), nil
	})
	var captured []api.ClickPipeUpdate
	mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
		*calls = append(*calls, "update")
		captured = append(captured, update)
		return withStoredMutualTLS(postgresAPIPipe(api.ClickPipePausedState, tables...), stored), nil
	})
	expectSyncRead(mock, calls, syncPipe)

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)
	require.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())

	// The write-only block must survive the post-update read exactly as planned.
	var finalState models.ClickPipeResourceModel
	require.False(t, resp.State.Get(ctx, &finalState).HasError())
	wantMutualTLS := postgresSourceFromModel(ctx, t, plan).MutualTLS
	assert.True(t, postgresSourceFromModel(ctx, t, finalState).MutualTLS.Equal(wantMutualTLS), "state must hold the planned mutual_tls")

	return captured
}

// The mutualTls PATCH contract: an object replaces the stored client
// certificate, an explicit null removes it and omitting the field keeps it.
func TestClickPipeUpdate_PostgresMutualTLS(t *testing.T) {
	ctx := context.Background()
	oldCert := postgresMutualTLSValue("OLD-CERT", "OLD-KEY")
	newCert := postgresMutualTLSValue("NEW-CERT", "NEW-KEY")

	// requireSinglePostgresPatch returns the Postgres source of the only PATCH.
	requireSinglePostgresPatch := func(t *testing.T, captured []api.ClickPipeUpdate) *api.ClickPipePostgresSource {
		t.Helper()
		require.Len(t, captured, 1, "the change must go in one PATCH")
		require.NotNil(t, captured[0].Source)
		require.NotNil(t, captured[0].Source.Postgres, "update payload carries no postgres source")
		return captured[0].Source.Postgres
	}

	t.Run("added is sent", func(t *testing.T) {
		state := postgresMutualTLSModel(ctx, t, postgresMutualTLSNull(), "users")
		plan := postgresMutualTLSModel(ctx, t, newCert, "users")

		pg := requireSinglePostgresPatch(t, drivePostgresMutualTLSUpdate(ctx, t, state, plan, "users"))

		require.NotNil(t, pg.MutualTLS, "a mutual_tls-only change must be detected as a source change")
		require.False(t, pg.MutualTLS.IsNull())
		assert.Equal(t, api.ClickPipePostgresMutualTLS{Certificate: "NEW-CERT", PrivateKey: "NEW-KEY"}, *pg.MutualTLS.Value)
		assert.Nil(t, pg.Credentials, "unchanged credentials must not be re-sent")
	})

	t.Run("changed is sent", func(t *testing.T) {
		state := postgresMutualTLSModel(ctx, t, oldCert, "users")
		plan := postgresMutualTLSModel(ctx, t, newCert, "users")

		pg := requireSinglePostgresPatch(t, drivePostgresMutualTLSUpdate(ctx, t, state, plan, "users"))

		require.NotNil(t, pg.MutualTLS)
		require.False(t, pg.MutualTLS.IsNull())
		assert.Equal(t, api.ClickPipePostgresMutualTLS{Certificate: "NEW-CERT", PrivateKey: "NEW-KEY"}, *pg.MutualTLS.Value)
	})

	t.Run("only the private key changed is sent", func(t *testing.T) {
		state := postgresMutualTLSModel(ctx, t, postgresMutualTLSValue("CERT", "OLD-KEY"), "users")
		plan := postgresMutualTLSModel(ctx, t, postgresMutualTLSValue("CERT", "NEW-KEY"), "users")

		pg := requireSinglePostgresPatch(t, drivePostgresMutualTLSUpdate(ctx, t, state, plan, "users"))

		require.NotNil(t, pg.MutualTLS)
		assert.Equal(t, api.ClickPipePostgresMutualTLS{Certificate: "CERT", PrivateKey: "NEW-KEY"}, *pg.MutualTLS.Value)
	})

	t.Run("removed is sent as an explicit null", func(t *testing.T) {
		state := postgresMutualTLSModel(ctx, t, oldCert, "users")
		plan := postgresMutualTLSModel(ctx, t, postgresMutualTLSNull(), "users")

		captured := drivePostgresMutualTLSUpdate(ctx, t, state, plan, "users")
		pg := requireSinglePostgresPatch(t, captured)

		require.NotNil(t, pg.MutualTLS, "a removed mutual_tls must be sent, not omitted")
		assert.True(t, pg.MutualTLS.IsNull())

		payload, err := json.Marshal(captured[0])
		require.NoError(t, err)
		assert.Contains(t, string(payload), `"mutualTls":null`)
	})

	t.Run("unchanged is omitted when another source field changes", func(t *testing.T) {
		state := postgresMutualTLSModel(ctx, t, oldCert, "users")
		plan := withPostgresSource(ctx, t, postgresMutualTLSModel(ctx, t, oldCert, "users"), func(pg *models.ClickPipePostgresSourceModel) {
			pg.Host = types.StringValue("new-host.example.com")
		})

		captured := drivePostgresMutualTLSUpdate(ctx, t, state, plan, "users")
		pg := requireSinglePostgresPatch(t, captured)

		assert.Equal(t, "new-host.example.com", pg.Host)
		assert.Nil(t, pg.MutualTLS, "an unchanged mutual_tls must not be re-sent")

		payload, err := json.Marshal(captured[0])
		require.NoError(t, err)
		assert.NotContains(t, string(payload), "mutualTls")
	})

	t.Run("never set stays omitted when another source field changes", func(t *testing.T) {
		state := postgresMutualTLSModel(ctx, t, postgresMutualTLSNull(), "users")
		plan := withPostgresSource(ctx, t, postgresMutualTLSModel(ctx, t, postgresMutualTLSNull(), "users"), func(pg *models.ClickPipePostgresSourceModel) {
			pg.Host = types.StringValue("new-host.example.com")
		})

		pg := requireSinglePostgresPatch(t, drivePostgresMutualTLSUpdate(ctx, t, state, plan, "users"))

		assert.Nil(t, pg.MutualTLS, "a null-to-null mutual_tls must be omitted, not sent as null")
	})

	t.Run("mappings-only edit omits it", func(t *testing.T) {
		state := postgresMutualTLSModel(ctx, t, oldCert, "users", "orders")
		plan := postgresMutualTLSModel(ctx, t, oldCert, "users")

		pg := requireSinglePostgresPatch(t, drivePostgresMutualTLSUpdate(ctx, t, state, plan, "users"))

		assert.Nil(t, pg.MutualTLS, "mutual_tls must not be re-sent on a mappings-only edit")
		assert.Empty(t, pg.Host, "the mappings-only shortcut must still apply with mutual_tls configured")
		require.Len(t, pg.TableMappingsToRemove, 1)
		assert.Equal(t, "orders", pg.TableMappingsToRemove[0].SourceTable)
	})

	t.Run("changed alongside mappings is sent with the mapping delta", func(t *testing.T) {
		state := postgresMutualTLSModel(ctx, t, oldCert, "users")
		plan := postgresMutualTLSModel(ctx, t, newCert, "users", "orders")

		pg := requireSinglePostgresPatch(t, drivePostgresMutualTLSUpdate(ctx, t, state, plan, "users", "orders"))

		require.NotNil(t, pg.MutualTLS)
		assert.Equal(t, api.ClickPipePostgresMutualTLS{Certificate: "NEW-CERT", PrivateKey: "NEW-KEY"}, *pg.MutualTLS.Value)
		require.Len(t, pg.TableMappingsToAdd, 1)
		assert.Equal(t, "orders", pg.TableMappingsToAdd[0].SourceTable)
	})
}

func TestPostgresMutualTLSForUpdate(t *testing.T) {
	cert := postgresMutualTLSValue("CERT", "KEY")
	unknown := types.ObjectUnknown(models.ClickPipePostgresMutualTLSModel{}.ObjectType().AttrTypes)
	planned := api.NullableValue(api.ClickPipePostgresMutualTLS{Certificate: "CERT", PrivateKey: "KEY"})

	assert.Nil(t, postgresMutualTLSForUpdate(cert, cert, planned), "unchanged must be omitted")
	assert.Nil(t, postgresMutualTLSForUpdate(postgresMutualTLSNull(), postgresMutualTLSNull(), nil), "null to null must be omitted")
	assert.Same(t, planned, postgresMutualTLSForUpdate(cert, postgresMutualTLSNull(), planned), "added must be sent")

	removed := postgresMutualTLSForUpdate(postgresMutualTLSNull(), cert, nil)
	require.NotNil(t, removed)
	assert.True(t, removed.IsNull(), "removed must be an explicit null")

	// An unknown plan converts to nil, so the stored certificate is left alone
	// rather than removed.
	assert.Nil(t, postgresMutualTLSForUpdate(unknown, cert, nil))
}
