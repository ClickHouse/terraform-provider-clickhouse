package resource

import (
	"context"
	"encoding/json"
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

func mongoDBTableMappingModel(persistDeleted types.Bool) models.ClickPipeMongoDBTableMappingModel {
	return models.ClickPipeMongoDBTableMappingModel{
		SourceDatabaseName:      types.StringValue("shop"),
		SourceCollection:        types.StringValue("orders"),
		TargetTable:             types.StringValue("shop_orders"),
		TableEngine:             types.StringNull(),
		PersistDeletedDocuments: persistDeleted,
		StructuredIngestion:     types.ObjectNull(models.ClickPipeMongoDBStructuredIngestionModel{}.ObjectType().AttrTypes),
	}
}

func TestConvertMongoDBTableMappingModelToAPI_PersistDeletedDocuments(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		value    types.Bool
		expected string
	}{
		{name: "unset sends no mongoConfig", value: types.BoolNull(), expected: ""},
		{name: "true", value: types.BoolValue(true), expected: `"mongoConfig":{"persistDeletedDocuments":true}`},
		{name: "false", value: types.BoolValue(false), expected: `"mongoConfig":{"persistDeletedDocuments":false}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostics := diag.Diagnostics{}
			mapping := convertMongoDBTableMappingModelToAPI(ctx, &diagnostics, mongoDBTableMappingModel(tt.value))
			require.False(t, diagnostics.HasError())

			payload, err := json.Marshal(mapping)
			require.NoError(t, err)
			if tt.expected == "" {
				assert.NotContains(t, string(payload), "mongoConfig")
			} else {
				assert.Contains(t, string(payload), tt.expected)
			}
		})
	}
}

func buildMongoDBPersistDeletedState(persistDeleted types.Bool) models.ClickPipeResourceModel {
	state := buildMongoDBSSHKeyResourcePlan(types.StringNull())
	var sourceModel models.ClickPipeSourceModel
	state.Source.As(context.Background(), &sourceModel, basetypes.ObjectAsOptions{})
	mongoAttrs := sourceModel.MongoDB.Attributes()
	mongoAttrs["table_mappings"] = types.SetValueMust(
		models.ClickPipeMongoDBTableMappingModel{}.ObjectType(),
		[]attr.Value{mongoDBTableMappingModel(persistDeleted).ObjectValue()},
	)
	sourceModel.MongoDB = types.ObjectValueMust(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes, mongoAttrs)
	state.Source = sourceModel.ObjectValue()
	return state
}

func mongoDBPersistDeletedAPIResponse(mongoConfig *api.ClickPipeMongoDBTableConfig) *api.ClickPipe {
	return &api.ClickPipe{
		ID:    "test-pipe-id",
		Name:  "test-pipe",
		State: "Running",
		Source: api.ClickPipeSource{
			MongoDB: &api.ClickPipeMongoDBSource{
				URI:            "mongodb+srv://cluster0.example.mongodb.net/mydb",
				ReadPreference: "secondaryPreferred",
				Settings:       &api.ClickPipeMongoDBSettings{ReplicationMode: "cdc"},
				Mappings: []api.ClickPipeMongoDBTableMapping{{
					SourceDatabaseName: "shop",
					SourceCollection:   "orders",
					TargetTable:        "shop_orders",
					MongoConfig:        mongoConfig,
				}},
			},
		},
		Destination: api.ClickPipeDestination{Database: "default"},
	}
}

func TestClickPipeResource_syncClickPipeState_MongoDBPersistDeletedDocuments(t *testing.T) {
	ctx := context.Background()
	enabled := true

	tests := []struct {
		name        string
		stateValue  types.Bool
		mongoConfig *api.ClickPipeMongoDBTableConfig
		expected    types.Bool
	}{
		{
			name:        "enabled in API is read back",
			stateValue:  types.BoolNull(),
			mongoConfig: &api.ClickPipeMongoDBTableConfig{PersistDeletedDocuments: &enabled},
			expected:    types.BoolValue(true),
		},
		{
			name:       "omitted by API stays null when unset",
			stateValue: types.BoolNull(),
			expected:   types.BoolNull(),
		},
		{
			name:       "omitted by API means false when explicitly disabled",
			stateValue: types.BoolValue(false),
			expected:   types.BoolValue(false),
		},
		{
			name:       "omitted by API surfaces drift when state was enabled",
			stateValue: types.BoolValue(true),
			expected:   types.BoolValue(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := buildMongoDBPersistDeletedState(tt.stateValue)

			mc := minimock.NewController(t)
			apiClientMock := api.NewClientMock(mc).
				GetClickPipeMock.
				Expect(ctx, state.ServiceID.ValueString(), state.ID.ValueString()).
				Return(mongoDBPersistDeletedAPIResponse(tt.mongoConfig), nil)

			r := &ClickPipeResource{client: apiClientMock}
			require.NoError(t, r.syncClickPipeState(ctx, &state))

			var sourceModel models.ClickPipeSourceModel
			require.False(t, state.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())
			var mongoModel models.ClickPipeMongoDBSourceModel
			require.False(t, sourceModel.MongoDB.As(ctx, &mongoModel, basetypes.ObjectAsOptions{}).HasError())
			var mappings []models.ClickPipeMongoDBTableMappingModel
			require.False(t, mongoModel.TableMappings.ElementsAs(ctx, &mappings, false).HasError())
			require.Len(t, mappings, 1)
			assert.True(t, tt.expected.Equal(mappings[0].PersistDeletedDocuments),
				"expected persist_deleted_documents %v, got %v", tt.expected, mappings[0].PersistDeletedDocuments)
		})
	}
}
