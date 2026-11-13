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

// withDBSettingsAttr returns a copy of model with source.<postgres|mysql>.settings.<key>
// replaced by value. Used to vary a single settings attribute on the shared plan builders.
func withDBSettingsAttr(t *testing.T, model models.ClickPipeResourceModel, sourceKey, key string, value attr.Value) models.ClickPipeResourceModel {
	t.Helper()
	ctx := context.Background()

	var sourceModel models.ClickPipeSourceModel
	require.False(t, model.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())

	var dbObj types.Object
	var dbAttrTypes map[string]attr.Type
	switch sourceKey {
	case "postgres":
		dbObj = sourceModel.Postgres
		dbAttrTypes = models.ClickPipePostgresSourceModel{}.ObjectType().AttrTypes
	case "mysql":
		dbObj = sourceModel.MySQL
		dbAttrTypes = models.ClickPipeMySQLSourceModel{}.ObjectType().AttrTypes
	default:
		t.Fatalf("unsupported source %q", sourceKey)
	}

	dbAttrs := dbObj.Attributes()
	settingsObj, ok := dbAttrs["settings"].(types.Object)
	require.True(t, ok, "settings must be an object")
	settingsAttrs := settingsObj.Attributes()
	settingsAttrs[key] = value
	dbAttrs["settings"] = types.ObjectValueMust(settingsObj.AttributeTypes(ctx), settingsAttrs)

	rebuilt := types.ObjectValueMust(dbAttrTypes, dbAttrs)
	if sourceKey == "postgres" {
		sourceModel.Postgres = rebuilt
	} else {
		sourceModel.MySQL = rebuilt
	}
	model.Source = sourceModel.ObjectValue()
	return model
}

// dbDestination returns the destination block DB pipes use: only the database is set, the
// tables are managed through table_mappings.
func dbDestination() types.Object {
	return types.ObjectValueMust(
		models.ClickPipeDestinationModel{}.ObjectType().AttrTypes,
		map[string]attr.Value{
			"database":         types.StringValue("default"),
			"table":            types.StringNull(),
			"managed_table":    types.BoolNull(),
			"table_definition": types.ObjectNull(models.ClickPipeDestinationTableDefinitionModel{}.ObjectType().AttrTypes),
			"columns":          types.ListNull(models.ClickPipeDestinationColumnModel{}.ObjectType()),
			"roles":            types.ListNull(types.StringType),
		},
	)
}

func TestExtractSourceFromPlan_UseJsonNativeFormat(t *testing.T) {
	ctx := context.Background()
	r := &ClickPipeResource{}

	values := []struct {
		name     string
		value    types.Bool
		expected *bool
	}{
		{name: "unset is omitted from the payload", value: types.BoolNull(), expected: nil},
		{name: "unknown is omitted from the payload", value: types.BoolUnknown(), expected: nil},
		{name: "true is forwarded", value: types.BoolValue(true), expected: boolPtr(true)},
		{name: "false is forwarded", value: types.BoolValue(false), expected: boolPtr(false)},
	}

	t.Run("postgres", func(t *testing.T) {
		for _, tt := range values {
			t.Run(tt.name, func(t *testing.T) {
				plan := withDBSettingsAttr(t, buildPostgresSSHKeyResourcePlan(types.StringNull()), "postgres", "use_json_native_format", tt.value)
				diagnostics := diag.Diagnostics{}
				source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

				require.False(t, diagnostics.HasError(), "unexpected errors: %v", diagnostics.Errors())
				require.NotNil(t, source.Postgres)
				require.NotNil(t, source.Postgres.Settings)
				assert.Equal(t, tt.expected, source.Postgres.Settings.UseJsonNativeFormat)
			})
		}
	})

	t.Run("mysql", func(t *testing.T) {
		for _, tt := range values {
			t.Run(tt.name, func(t *testing.T) {
				plan := withDBSettingsAttr(t, buildMySQLSSHKeyResourcePlan(types.StringNull()), "mysql", "use_json_native_format", tt.value)
				diagnostics := diag.Diagnostics{}
				source := r.extractSourceFromPlan(ctx, &diagnostics, plan, nil, false)

				require.False(t, diagnostics.HasError(), "unexpected errors: %v", diagnostics.Errors())
				require.NotNil(t, source.MySQL)
				require.NotNil(t, source.MySQL.Settings)
				assert.Equal(t, tt.expected, source.MySQL.Settings.UseJsonNativeFormat)
			})
		}
	})
}

func TestClickPipeResource_syncClickPipeState_UseJsonNativeFormat(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name       string
		stateValue types.Bool
		apiValue   *bool
		expected   types.Bool
	}{
		{name: "true from API is read back", stateValue: types.BoolValue(false), apiValue: boolPtr(true), expected: types.BoolValue(true)},
		{name: "false from API is read back", stateValue: types.BoolValue(true), apiValue: boolPtr(false), expected: types.BoolValue(false)},
		{name: "omitted by API preserves the state default", stateValue: types.BoolValue(false), apiValue: nil, expected: types.BoolValue(false)},
		{name: "omitted by API stays null when unset", stateValue: types.BoolNull(), apiValue: nil, expected: types.BoolNull()},
	}

	t.Run("postgres", func(t *testing.T) {
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				state := withDBSettingsAttr(t, getPostgresInitialState(), "postgres", "use_json_native_format", tt.stateValue)
				response := &api.ClickPipe{
					ID:    "test-pipe-id",
					Name:  "test-pipe",
					State: "running",
					Source: api.ClickPipeSource{
						Postgres: &api.ClickPipePostgresSource{
							Host:     "postgres.example.com",
							Port:     5432,
							Database: "mydb",
							Settings: &api.ClickPipePostgresSettings{
								ReplicationMode:     "cdc",
								UseJsonNativeFormat: tt.apiValue,
							},
						},
					},
					Destination: api.ClickPipeDestination{Database: "default"},
				}

				mc := minimock.NewController(t)
				client := api.NewClientMock(mc).GetClickPipeMock.
					Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
					Return(response, nil)

				r := &ClickPipeResource{client: client}
				require.NoError(t, r.syncClickPipeState(ctx, &state))

				var sourceModel models.ClickPipeSourceModel
				require.False(t, state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())
				var pgModel models.ClickPipePostgresSourceModel
				require.False(t, sourceModel.Postgres.As(ctx, &pgModel, basetypes.ObjectAsOptions{}).HasError())
				var settings models.ClickPipePostgresSettingsModel
				require.False(t, pgModel.Settings.As(ctx, &settings, basetypes.ObjectAsOptions{}).HasError())
				assert.True(t, tt.expected.Equal(settings.UseJsonNativeFormat),
					"expected use_json_native_format %v, got %v", tt.expected, settings.UseJsonNativeFormat)
			})
		}
	})

	t.Run("mysql", func(t *testing.T) {
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				state := withDBSettingsAttr(t, buildMySQLSSHKeyResourcePlan(types.StringNull()), "mysql", "use_json_native_format", tt.stateValue)
				state.Destination = dbDestination()
				response := &api.ClickPipe{
					ID:    "test-pipe-id",
					Name:  "test-pipe",
					State: "running",
					Source: api.ClickPipeSource{
						MySQL: &api.ClickPipeMySQLSource{
							Host: "10.0.0.6",
							Port: 3306,
							Settings: &api.ClickPipeMySQLSettings{
								ReplicationMode:     "cdc",
								UseJsonNativeFormat: tt.apiValue,
							},
						},
					},
					Destination: api.ClickPipeDestination{Database: "default"},
				}

				mc := minimock.NewController(t)
				client := api.NewClientMock(mc).GetClickPipeMock.
					Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
					Return(response, nil)

				r := &ClickPipeResource{client: client}
				require.NoError(t, r.syncClickPipeState(ctx, &state))

				var sourceModel models.ClickPipeSourceModel
				require.False(t, state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())
				var mysqlModel models.ClickPipeMySQLSourceModel
				require.False(t, sourceModel.MySQL.As(ctx, &mysqlModel, basetypes.ObjectAsOptions{}).HasError())
				var settings models.ClickPipeMySQLSettingsModel
				require.False(t, mysqlModel.Settings.As(ctx, &settings, basetypes.ObjectAsOptions{}).HasError())
				assert.True(t, tt.expected.Equal(settings.UseJsonNativeFormat),
					"expected use_json_native_format %v, got %v", tt.expected, settings.UseJsonNativeFormat)
			})
		}
	})
}
