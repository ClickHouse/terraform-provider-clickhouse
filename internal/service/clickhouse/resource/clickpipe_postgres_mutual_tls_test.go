package resource

import (
	"context"
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

	tests := []struct {
		name      string
		mutualTLS types.Object
	}{
		{name: "preserved from state", mutualTLS: postgresMutualTLSValue("PEM-CERT", "PEM-KEY")},
		{name: "stays null when state has none", mutualTLS: postgresMutualTLSNull()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := postgresMutualTLSModel(ctx, t, tt.mutualTLS, "users")

			mock := api.NewClientMock(minimock.NewController(t))
			mock.GetClickPipeMock.Return(postgresAPIPipe(api.ClickPipeRunningState, "users"), nil)

			require.NoError(t, (&ClickPipeResource{client: mock}).syncClickPipeState(ctx, &state))

			pg := postgresSourceFromModel(ctx, t, state)
			assert.True(t, pg.MutualTLS.Equal(tt.mutualTLS), "mutual_tls must round-trip through a read unchanged, got %s", pg.MutualTLS)
		})
	}
}
