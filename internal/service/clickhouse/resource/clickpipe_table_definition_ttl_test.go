package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func TestClickPipeResource_TableDefinitionTTLSchema(t *testing.T) {
	ctx := context.Background()
	schemaResponse := &resource.SchemaResponse{}
	(&ClickPipeResource{}).Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError(), "building resource schema failed: %v", schemaResponse.Diagnostics.Errors())

	destinationAttribute := schemaResponse.Schema.Attributes["destination"]
	require.IsType(t, resourceschema.SingleNestedAttribute{}, destinationAttribute)
	tableDefinitionAttribute := destinationAttribute.(resourceschema.SingleNestedAttribute).Attributes["table_definition"]
	require.IsType(t, resourceschema.SingleNestedAttribute{}, tableDefinitionAttribute)
	tableDefinitionNestedAttribute := tableDefinitionAttribute.(resourceschema.SingleNestedAttribute)
	ttlAttribute := tableDefinitionNestedAttribute.Attributes["ttl"]
	require.IsType(t, resourceschema.StringAttribute{}, ttlAttribute)
	ttlStringAttribute := ttlAttribute.(resourceschema.StringAttribute)

	assert.True(t, ttlStringAttribute.Optional)
	assert.False(t, ttlStringAttribute.Required)
	assert.False(t, ttlStringAttribute.Computed)
	assert.Len(t, ttlStringAttribute.Validators, 1)
	assert.Len(t, tableDefinitionNestedAttribute.PlanModifiers, 1, "table_definition must keep its RequiresReplace plan modifier")
}

func TestClickPipeResource_SyncTableDefinitionTTL(t *testing.T) {
	ctx := context.Background()
	ttl := "toDateTime(id) + INTERVAL 30 DAY"

	tests := []struct {
		name        string
		apiTTL      *string
		expectedTTL types.String
	}{
		{name: "maps ttl returned by the API into state", apiTTL: &ttl, expectedTTL: types.StringValue(ttl)},
		{name: "leaves ttl null when the API returns none", apiTTL: nil, expectedTTL: types.StringNull()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := buildObjectStoragePlan(types.BoolNull(), types.StringNull())
			apiClickPipe := &api.ClickPipe{
				ID:    "test-pipe-id",
				Name:  "test-object-storage-pipe",
				State: api.ClickPipeRunningState,
				Source: api.ClickPipeSource{ObjectStorage: &api.ClickPipeObjectStorageSource{
					Type:           api.ClickPipeObjectStorageS3Type,
					Format:         "JSONEachRow",
					URL:            "https://test-bucket.s3.us-east-1.amazonaws.com/data/*.json",
					Authentication: strPtr(api.ClickPipeAuthenticationIAMUser),
				}},
				Destination: api.ClickPipeDestination{
					Database:     "default",
					Table:        strPtr("data"),
					ManagedTable: boolPtr(true),
					TableDefinition: &api.ClickPipeDestinationTableDefinition{
						Engine: api.ClickPipeDestinationTableEngine{Type: "MergeTree"},
						TTL:    test.apiTTL,
					},
					Columns: []api.ClickPipeDestinationColumn{{Name: "id", Type: "Int32"}},
				},
			}
			mockController := minimock.NewController(t)
			apiClient := api.NewClientMock(mockController).
				GetClickPipeMock.
				Expect(ctx, "service-123", "test-pipe-id").
				Return(apiClickPipe, nil)

			err := (&ClickPipeResource{client: apiClient}).syncClickPipeState(ctx, &state)

			require.NoError(t, err)
			var destinationModel models.ClickPipeDestinationModel
			require.False(t, state.Destination.As(ctx, &destinationModel, basetypes.ObjectAsOptions{}).HasError())
			var tableDefinitionModel models.ClickPipeDestinationTableDefinitionModel
			require.False(t, destinationModel.TableDefinition.As(ctx, &tableDefinitionModel, basetypes.ObjectAsOptions{}).HasError())
			assert.Equal(t, test.expectedTTL, tableDefinitionModel.TTL)
		})
	}
}
