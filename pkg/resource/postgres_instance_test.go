//go:build alpha

package resource

import (
	"context"
	"fmt"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/test"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func getInitialPostgresState() models.PostgresInstanceResourceModel {
	pgConfigValues := map[string]attr.Value{
		"wal_level": types.StringValue("logical"),
	}
	pgConfig, _ := types.MapValue(types.StringType, pgConfigValues)

	tagsValues := map[string]attr.Value{
		"env": types.StringValue("test"),
	}
	tags, _ := types.MapValue(types.StringType, tagsValues)

	return models.PostgresInstanceResourceModel{
		ID:               types.StringValue("pg123"),
		Name:             types.StringValue("test-pg"),
		CloudProvider:    types.StringValue("aws"),
		Region:           types.StringValue("us-east-1"),
		PostgresVersion:  types.StringValue("17"),
		Size:             types.StringValue("m6gd.medium"),
		StorageSize:      types.Int64Value(118),
		HAType:           types.StringValue("none"),
		State:            types.StringValue("running"),
		IsPrimary:        types.BoolValue(true),
		Hostname:         types.StringValue("test-pg.ubicloud.com"),
		ConnectionString: types.StringValue("postgres://postgres@test-pg.ubicloud.com:5432/postgres"),
		Username:         types.StringValue("postgres"),
		PgConfig:         pgConfig,
		PgBouncerConfig:  types.MapNull(types.StringType),
		Tags:             tags,
	}
}

func getBasePostgresResponse(id string) api.PostgresInstance {
	return api.PostgresInstance{
		ID:               id,
		Name:             "test-pg",
		Provider:         "aws",
		Region:           "us-east-1",
		PostgresVersion:  "17",
		Size:             "m6gd.medium",
		StorageSize:      118,
		HAType:           "none",
		State:            "running",
		IsPrimary:        true,
		Hostname:         "test-pg.ubicloud.com",
		ConnectionString: "postgres://postgres@test-pg.ubicloud.com:5432/postgres",
		Username:         "postgres",
		PgConfig:         map[string]string{"wal_level": "logical"},
		PgBouncerConfig:  nil,
		Tags:             []api.Tag{{Key: "env", Value: "test"}},
	}
}

func TestPostgresInstanceResource_syncPostgresState(t *testing.T) {
	ctx := context.Background()
	state := getInitialPostgresState()

	tests := []struct {
		name         string
		state        models.PostgresInstanceResourceModel
		response     *api.PostgresInstance
		responseErr  error
		desiredState models.PostgresInstanceResourceModel
		wantErr      bool
	}{
		{
			name:  "Updates name field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Name = "new-pg-name"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.Name = types.StringValue("new-pg-name")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates cloud_provider field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Provider = "gcp"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.CloudProvider = types.StringValue("gcp")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates region field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Region = "eu-west-1"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.Region = types.StringValue("eu-west-1")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates postgres_version field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.PostgresVersion = "16"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.PostgresVersion = types.StringValue("16")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates size field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Size = "m6gd.xlarge"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.Size = types.StringValue("m6gd.xlarge")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates storage_size field with int to int64 conversion",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.StorageSize = 256
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.StorageSize = types.Int64Value(256)
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates ha_type field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.HAType = "async"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.HAType = types.StringValue("async")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates state field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.State = "stopped"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.State = types.StringValue("stopped")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates is_primary field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.IsPrimary = false
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.IsPrimary = types.BoolValue(false)
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates hostname field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Hostname = "new-pg.ubicloud.com"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.Hostname = types.StringValue("new-pg.ubicloud.com")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates connection_string field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.ConnectionString = "postgres://postgres@new-pg.ubicloud.com:5432/postgres"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.ConnectionString = types.StringValue("postgres://postgres@new-pg.ubicloud.com:5432/postgres")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates username field",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Username = "admin"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.Username = types.StringValue("admin")
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates pg_config with new values",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.PgConfig = map[string]string{"wal_level": "replica", "max_connections": "200"}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				pgConfigValues := map[string]attr.Value{
					"wal_level":       types.StringValue("replica"),
					"max_connections": types.StringValue("200"),
				}
				src.PgConfig, _ = types.MapValue(types.StringType, pgConfigValues)
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Handles nil pgConfig returns MapNull",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.PgConfig = nil
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.PgConfig = types.MapNull(types.StringType)
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Handles non-nil pgBouncerConfig with values",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.PgBouncerConfig = map[string]string{"pool_mode": "transaction", "max_client_conn": "500"}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				pgBouncerValues := map[string]attr.Value{
					"pool_mode":       types.StringValue("transaction"),
					"max_client_conn": types.StringValue("500"),
				}
				src.PgBouncerConfig, _ = types.MapValue(types.StringType, pgBouncerValues)
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Handles nil pgBouncerConfig returns MapNull",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.PgBouncerConfig = nil
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.PgBouncerConfig = types.MapNull(types.StringType)
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Updates tags with new values",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Tags = []api.Tag{{Key: "cost-center", Value: "business-a"}}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				tagsMap := map[string]attr.Value{
					"cost-center": types.StringValue("business-a"),
				}
				src.Tags, _ = types.MapValue(types.StringType, tagsMap)
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Handles empty tags preserves empty map when state had tags",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Tags = []api.Tag{}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				src.Tags, _ = types.MapValue(types.StringType, map[string]attr.Value{})
			}).Get(),
			wantErr: false,
		},
		{
			name: "Preserves empty map when tags were previously non-null",
			state: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				// State has tags = {} (empty map, not null) — user wrote tags = {} in config
				src.Tags, _ = types.MapValue(types.StringType, map[string]attr.Value{})
			}).Get(),
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Tags = []api.Tag{} // API returns empty tags
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				// Should stay as empty map, NOT become null — avoids perpetual diff
				src.Tags, _ = types.MapValue(types.StringType, map[string]attr.Value{})
			}).Get(),
			wantErr: false,
		},
		{
			name: "Null tags stay null when API returns no tags",
			state: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				// State has tags = null — user did not set tags in config
				src.Tags = types.MapNull(types.StringType)
			}).Get(),
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Tags = nil // API returns nil tags
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				// Should stay null
				src.Tags = types.MapNull(types.StringType)
			}).Get(),
			wantErr: false,
		},
		{
			name:  "Handles multiple tags",
			state: state,
			response: test.NewUpdater(getBasePostgresResponse(state.ID.ValueString())).Update(func(src *api.PostgresInstance) {
				src.Tags = []api.Tag{
					{Key: "env", Value: "production"},
					{Key: "team", Value: "backend"},
					{Key: "owner", Value: "alice"},
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				tagsMap := map[string]attr.Value{
					"env":   types.StringValue("production"),
					"team":  types.StringValue("backend"),
					"owner": types.StringValue("alice"),
				}
				src.Tags, _ = types.MapValue(types.StringType, tagsMap)
			}).Get(),
			wantErr: false,
		},
		{
			name:        "Returns error on API failure",
			state:       state,
			response:    nil,
			responseErr: fmt.Errorf("API error"),
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				// State should remain unchanged on error
			}).Get(),
			wantErr: true,
		},
		{
			name:        "Propagates not-found error when resource deleted outside Terraform",
			state:       state,
			response:    nil,
			responseErr: fmt.Errorf("status: 404, body: not found"),
			desiredState: test.NewUpdater(state).Update(func(src *models.PostgresInstanceResourceModel) {
				// State should remain unchanged on error
			}).Get(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)

			apiClientMock := api.NewClientMock(mc).
				GetPostgresInstanceMock.
				Expect(context.Background(), tt.state.ID.ValueString()).
				Return(tt.response, tt.responseErr)

			r := &PostgresInstanceResource{
				client: apiClientMock,
			}

			err := r.syncPostgresState(ctx, &tt.state)

			if (err != nil) != tt.wantErr {
				t.Errorf("%s error does not match:\ngot  = %v\nwant = %v", tt.name, err, tt.wantErr)
			}

			if !tt.state.Equals(tt.desiredState) {
				t.Errorf("%s state does not match:\ngot  = %v\nwant = %v\n", tt.name, tt.state, tt.desiredState)
			}
		})
	}
}

// getPostgresSchema returns the schema for the PostgresInstanceResource.
func getPostgresSchema() resource.SchemaResponse {
	r := &PostgresInstanceResource{}
	schemaResp := resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)
	return schemaResp
}

// makePostgresState creates a tfsdk.State populated with the given model.
func makePostgresState(t *testing.T, model models.PostgresInstanceResourceModel) tfsdk.State {
	t.Helper()
	schemaResp := getPostgresSchema()
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("failed to set state: %s", diags.Errors())
	}
	return state
}

// makePostgresPlan creates a tfsdk.Plan populated with the given model.
func makePostgresPlan(t *testing.T, model models.PostgresInstanceResourceModel) tfsdk.Plan {
	t.Helper()
	schemaResp := getPostgresSchema()
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(context.Background(), model)
	if diags.HasError() {
		t.Fatalf("failed to set plan: %s", diags.Errors())
	}
	return plan
}

func TestPostgresInstanceResource_ModifyPlan_StorageShrinkRejected(t *testing.T) {
	tests := []struct {
		name          string
		stateStorage  int64
		planStorage   int64
		expectError   bool
		errorContains string
	}{
		{
			name:         "allows storage increase",
			stateStorage: 118,
			planStorage:  256,
			expectError:  false,
		},
		{
			name:         "allows storage unchanged",
			stateStorage: 118,
			planStorage:  118,
			expectError:  false,
		},
		{
			name:          "rejects storage decrease",
			stateStorage:  256,
			planStorage:   118,
			expectError:   true,
			errorContains: "can only be increased",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateModel := getInitialPostgresState()
			stateModel.StorageSize = types.Int64Value(tt.stateStorage)

			planModel := getInitialPostgresState()
			planModel.StorageSize = types.Int64Value(tt.planStorage)

			r := &PostgresInstanceResource{}
			req := resource.ModifyPlanRequest{
				State: makePostgresState(t, stateModel),
				Plan:  makePostgresPlan(t, planModel),
			}
			resp := &resource.ModifyPlanResponse{}

			r.ModifyPlan(context.Background(), req, resp)

			if tt.expectError {
				if !resp.Diagnostics.HasError() {
					t.Errorf("expected error containing %q, but got no errors", tt.errorContains)
				} else {
					found := false
					for _, d := range resp.Diagnostics.Errors() {
						if contains(d.Detail(), tt.errorContains) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected error containing %q, got: %v", tt.errorContains, resp.Diagnostics.Errors())
					}
				}
			} else {
				if resp.Diagnostics.HasError() {
					t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
				}
			}
		})
	}
}

// contains checks if s contains substr (simple helper to avoid importing strings).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
