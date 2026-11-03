package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

const mongoIAMRoleARN = "arn:aws:iam::123456789012:role/docdb-reader"

func mongoCredentials(password string) types.Object {
	return types.ObjectValueMust(models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes, map[string]attr.Value{
		"username":            types.StringValue("mongo_user"),
		"password":            types.StringValue(password),
		"password_wo":         types.StringNull(),
		"password_wo_version": types.Int64Null(),
	})
}

func noMongoCredentials() types.Object {
	return types.ObjectNull(models.ClickPipeSourceCredentialsModel{}.ObjectType().AttrTypes)
}

// mongoSourceModel returns a MongoDB-only source with the given auth settings.
func mongoSourceModel(authentication string, iamRole types.String, credentials types.Object, readPreference string) models.ClickPipeSourceModel {
	source := workloadIdentitySourceModel()
	source.MongoDB = types.ObjectValueMust(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes, map[string]attr.Value{
		"uri":                    types.StringValue("mongodb://docdb.cluster-abc.us-east-1.docdb.amazonaws.com:27017"),
		"read_preference":        types.StringValue(readPreference),
		"authentication":         types.StringValue(authentication),
		"iam_role":               iamRole,
		"tls_host":               types.StringNull(),
		"ca_certificate":         types.StringNull(),
		"disable_tls":            types.BoolValue(false),
		"skip_cert_verification": types.BoolValue(false),
		"credentials":            credentials,
		"settings":               models.ClickPipeMongoDBSettingsModel{ReplicationMode: types.StringValue("cdc")}.ObjectValue(),
		"table_mappings":         types.SetValueMust(models.ClickPipeMongoDBTableMappingModel{}.ObjectType(), []attr.Value{}),
		"ssh_key_resource_id":    types.StringNull(),
	})
	return source
}

// mongoUpdateModel reuses the Postgres pause-before-edit fixture for everything but the source.
func mongoUpdateModel(ctx context.Context, t *testing.T, source models.ClickPipeSourceModel) models.ClickPipeResourceModel {
	t.Helper()
	m := postgresUpdateModel(ctx, t)
	m.Source = source.ObjectValue()
	return m
}

func mongoAPIPipe(state, authentication, iamRole string) *api.ClickPipe {
	pipe := &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-pipe",
		State: state,
		Source: api.ClickPipeSource{MongoDB: &api.ClickPipeMongoDBSource{
			URI:            "mongodb://docdb.cluster-abc.us-east-1.docdb.amazonaws.com:27017",
			ReadPreference: "secondaryPreferred",
			Authentication: &authentication,
			Settings:       &api.ClickPipeMongoDBSettings{ReplicationMode: "cdc"},
		}},
		Destination: api.ClickPipeDestination{Database: "default"},
	}
	if iamRole != "" {
		pipe.Source.MongoDB.IAMRole = &iamRole
	}
	return pipe
}

func TestExtractSourceFromPlan_MongoDBAuthentication(t *testing.T) {
	tests := []struct {
		name               string
		source             models.ClickPipeSourceModel
		wantAuthentication *string
		wantIAMRole        *string
	}{
		{
			name:               "IAM role create sends the role and no credentials",
			source:             mongoSourceModel(api.ClickPipeAuthenticationIAMRole, types.StringValue(mongoIAMRoleARN), noMongoCredentials(), "secondaryPreferred"),
			wantAuthentication: new(api.ClickPipeAuthenticationIAMRole),
			wantIAMRole:        new(mongoIAMRoleARN),
		},
		{
			// Older API versions reject these fields on MongoDB, so basic creates leave them out.
			name:   "basic create omits the auth fields",
			source: mongoSourceModel(clickPipeAuthBasic, types.StringNull(), mongoCredentials("secret"), "secondaryPreferred"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostics := diag.Diagnostics{}
			plan := models.ClickPipeResourceModel{Source: tt.source.ObjectValue()}

			got := (&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, false)

			require.False(t, diagnostics.HasError(), "unexpected diagnostics: %v", diagnostics.Errors())
			require.NotNil(t, got.MongoDB)
			assert.Equal(t, tt.wantAuthentication, got.MongoDB.Authentication)
			assert.Equal(t, tt.wantIAMRole, got.MongoDB.IAMRole)
			if tt.wantAuthentication != nil {
				assert.Nil(t, got.MongoDB.Credentials)
			}
		})
	}
}

func TestExtractSourceFromPlan_MongoDBAuthenticationRejectsConflicts(t *testing.T) {
	tests := []struct {
		name   string
		source models.ClickPipeSourceModel
	}{
		{"IAM role without a role ARN", mongoSourceModel(api.ClickPipeAuthenticationIAMRole, types.StringNull(), noMongoCredentials(), "secondaryPreferred")},
		{"IAM role with credentials", mongoSourceModel(api.ClickPipeAuthenticationIAMRole, types.StringValue(mongoIAMRoleARN), mongoCredentials("secret"), "secondaryPreferred")},
		{"basic with a role ARN", mongoSourceModel(clickPipeAuthBasic, types.StringValue(mongoIAMRoleARN), mongoCredentials("secret"), "secondaryPreferred")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostics := diag.Diagnostics{}
			plan := models.ClickPipeResourceModel{Source: tt.source.ObjectValue()}

			(&ClickPipeResource{}).extractSourceFromPlan(context.Background(), &diagnostics, plan, nil, false)

			assert.True(t, diagnostics.HasError(), "expected a configuration error")
		})
	}
}

func TestSyncClickPipeState_MongoDBAuthentication(t *testing.T) {
	tests := []struct {
		name               string
		apiAuthentication  string
		apiIAMRole         string
		wantAuthentication string
		wantIAMRole        types.String
	}{
		{"IAM role", api.ClickPipeAuthenticationIAMRole, mongoIAMRoleARN, api.ClickPipeAuthenticationIAMRole, types.StringValue(mongoIAMRoleARN)},
		{"API without an auth value reads as basic", "", "", clickPipeAuthBasic, types.StringNull()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			state := mongoUpdateModel(ctx, t, mongoSourceModel(clickPipeAuthBasic, types.StringNull(), noMongoCredentials(), "secondaryPreferred"))
			mc := minimock.NewController(t)
			client := api.NewClientMock(mc).
				GetClickPipeMock.
				Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
				Return(mongoAPIPipe(api.ClickPipeRunningState, tt.apiAuthentication, tt.apiIAMRole), nil)

			require.NoError(t, (&ClickPipeResource{client: client}).syncClickPipeState(ctx, &state))

			var source models.ClickPipeSourceModel
			require.False(t, state.Source.As(ctx, &source, basetypes.ObjectAsOptions{}).HasError())
			var mongo models.ClickPipeMongoDBSourceModel
			require.False(t, source.MongoDB.As(ctx, &mongo, basetypes.ObjectAsOptions{}).HasError())
			assert.Equal(t, tt.wantAuthentication, mongo.Authentication.ValueString())
			assert.Equal(t, tt.wantIAMRole, mongo.IAMRole)
		})
	}
}

func TestClickPipeResource_Update_MongoDBAuthentication(t *testing.T) {
	basic := mongoSourceModel(clickPipeAuthBasic, types.StringNull(), mongoCredentials("secret"), "secondaryPreferred")
	iam := mongoSourceModel(api.ClickPipeAuthenticationIAMRole, types.StringValue(mongoIAMRoleARN), noMongoCredentials(), "secondaryPreferred")
	tests := []struct {
		name               string
		state, plan        models.ClickPipeSourceModel
		wantAuthentication *string
		wantIAMRole        *string
		wantCredentials    bool
	}{
		{
			name:               "password to IAM role in place",
			state:              basic,
			plan:               iam,
			wantAuthentication: new(api.ClickPipeAuthenticationIAMRole),
			wantIAMRole:        new(mongoIAMRoleARN),
		},
		{
			name:               "IAM role to password with credentials",
			state:              iam,
			plan:               mongoSourceModel(clickPipeAuthBasic, types.StringNull(), mongoCredentials("new-secret"), "secondaryPreferred"),
			wantAuthentication: new(clickPipeAuthBasic),
			wantCredentials:    true,
		},
		{
			name:               "new role on an IAM pipe",
			state:              iam,
			plan:               mongoSourceModel(api.ClickPipeAuthenticationIAMRole, types.StringValue(mongoIAMRoleARN+"-v2"), noMongoCredentials(), "secondaryPreferred"),
			wantAuthentication: new(api.ClickPipeAuthenticationIAMRole),
			wantIAMRole:        new(mongoIAMRoleARN + "-v2"),
		},
		{
			// Unchanged auth stays out of the PATCH, so the edit works against older API versions.
			name:  "other edits of an IAM pipe omit the auth fields",
			state: iam,
			plan:  mongoSourceModel(api.ClickPipeAuthenticationIAMRole, types.StringValue(mongoIAMRoleARN), noMongoCredentials(), "primary"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			syncPipe := mongoAPIPipe(api.ClickPipeRunningState, clickPipeAuthBasic, "")
			mc := minimock.NewController(t)
			mock, calls := pauseEditClientMock(mc, syncPipe, func(int) (*api.ClickPipe, error) {
				return mongoAPIPipe(api.ClickPipePausedState, clickPipeAuthBasic, ""), nil
			})
			var captured api.ClickPipeUpdate
			mock.UpdateClickPipeMock.Set(func(_ context.Context, _, _ string, update api.ClickPipeUpdate) (*api.ClickPipe, error) {
				*calls = append(*calls, "update")
				captured = update
				return mongoAPIPipe(api.ClickPipePausedState, clickPipeAuthBasic, ""), nil
			})
			expectSyncRead(mock, calls, syncPipe)

			resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, mongoUpdateModel(ctx, t, tt.state), mongoUpdateModel(ctx, t, tt.plan))

			require.False(t, resp.Diagnostics.HasError(), "update must succeed: %v", resp.Diagnostics.Errors())
			require.NotNil(t, captured.Source)
			require.NotNil(t, captured.Source.MongoDB)
			assert.Equal(t, tt.wantAuthentication, captured.Source.MongoDB.Authentication)
			assert.Equal(t, tt.wantIAMRole, captured.Source.MongoDB.IAMRole)
			assert.Equal(t, tt.wantCredentials, captured.Source.MongoDB.Credentials != nil)
		})
	}
}

func TestClickPipeResource_Update_MongoDBSwitchToBasicNeedsPassword(t *testing.T) {
	ctx := context.Background()
	state := mongoUpdateModel(ctx, t, mongoSourceModel(api.ClickPipeAuthenticationIAMRole, types.StringValue(mongoIAMRoleARN), noMongoCredentials(), "secondaryPreferred"))
	plan := mongoUpdateModel(ctx, t, mongoSourceModel(clickPipeAuthBasic, types.StringNull(), noMongoCredentials(), "secondaryPreferred"))
	// A bare mock fails the test on any API call: the change is rejected before the pipe is paused or patched.
	mock := api.NewClientMock(minimock.NewController(t))

	resp := driveClickPipeUpdate(ctx, t, &ClickPipeResource{client: mock}, state, plan)

	require.True(t, resp.Diagnostics.HasError(), "switching to basic without a password must fail")
}
