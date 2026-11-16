package resource

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func mongoDBColumns(nameTypes ...string) types.List {
	elems := make([]attr.Value, 0, len(nameTypes)/2)
	for i := 0; i+1 < len(nameTypes); i += 2 {
		elems = append(elems, models.ClickPipeMongoDBColumnModel{
			Name: types.StringValue(nameTypes[i]),
			Type: types.StringValue(nameTypes[i+1]),
		}.ObjectValue())
	}
	return types.ListValueMust(models.ClickPipeMongoDBColumnModel{}.ObjectType(), elems)
}

func mongoDBStructuredIngestion(columns types.List) models.ClickPipeMongoDBStructuredIngestionModel {
	return models.ClickPipeMongoDBStructuredIngestionModel{
		DropUnexpectedValues: types.BoolNull(),
		Columns:              columns,
		ExcludedColumns:      types.SetNull(types.StringType),
		SortingKeys:          types.ListNull(types.StringType),
		PartitionByExpr:      types.StringNull(),
	}
}

func TestConvertMongoDBStructuredIngestionModelToAPI(t *testing.T) {
	ctx := context.Background()
	inferred := mongoDBStructuredIngestion(types.ListNull(models.ClickPipeMongoDBColumnModel{}.ObjectType()))
	provided := mongoDBStructuredIngestion(mongoDBColumns("name", "String", "inception_date", "DateTime64(9)"))
	provided.ExcludedColumns = stringSetValue([]string{"missions"})
	provided.SortingKeys = stringListValue([]string{"name"})
	provided.PartitionByExpr = types.StringValue("toYYYYMM(inception_date)")
	provided.DropUnexpectedValues = types.BoolValue(false)

	tests := []struct {
		name     string
		obj      types.Object
		expected string
	}{
		{
			name:     "null disables structured ingestion",
			obj:      types.ObjectNull(models.ClickPipeMongoDBStructuredIngestionModel{}.ObjectType().AttrTypes),
			expected: `null`,
		},
		{
			name:     "omitted columns ask the API to infer them",
			obj:      inferred.ObjectValue(),
			expected: `{"enabled":true}`,
		},
		{
			name: "provided columns and settings are sent as is",
			obj:  provided.ObjectValue(),
			expected: `{"enabled":true,"dropUnexpectedValues":false,` +
				`"columns":[{"name":"name","type":"String"},{"name":"inception_date","type":"DateTime64(9)"}],` +
				`"excludedColumns":["missions"],"sortingKeys":["name"],"partitionByExpr":"toYYYYMM(inception_date)"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnostics := diag.Diagnostics{}
			si := convertMongoDBStructuredIngestionModelToAPI(ctx, &diagnostics, tt.obj)
			require.False(t, diagnostics.HasError(), "%v", diagnostics)
			payload, err := json.Marshal(si)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(payload))
		})
	}
}

func TestMongoDBStructuredIngestionFromAPI(t *testing.T) {
	ctx := context.Background()
	f := false
	nullObj := types.ObjectNull(models.ClickPipeMongoDBStructuredIngestionModel{}.ObjectType().AttrTypes)

	unknownColumns := types.ListUnknown(models.ClickPipeMongoDBColumnModel{}.ObjectType())
	nameColumn := []api.ClickPipeMongoDBColumn{{Name: "name", Type: "String"}}

	inferredWithExclusion := mongoDBStructuredIngestion(unknownColumns)
	inferredWithExclusion.ExcludedColumns = stringSetValue([]string{"missions"})
	inferredWithExclusionExpected := mongoDBStructuredIngestion(mongoDBColumns("name", "String"))
	inferredWithExclusionExpected.ExcludedColumns = stringSetValue([]string{"missions"})

	explicitDefault := mongoDBStructuredIngestion(mongoDBColumns("name", "String"))
	explicitDefault.DropUnexpectedValues = types.BoolValue(false)
	explicitDefault.ExcludedColumns = types.SetValueMust(types.StringType, []attr.Value{})
	explicitDefault.SortingKeys = stringListValue([]string{"_id"})
	explicitDefault.PartitionByExpr = types.StringValue("")

	customSettings := mongoDBStructuredIngestion(mongoDBColumns("name", "String"))
	customSettings.ExcludedColumns = stringSetValue([]string{"missions"})
	customSettings.SortingKeys = stringListValue([]string{"name", "_id"})
	customSettings.PartitionByExpr = types.StringValue("toYYYYMM(inception_date)")
	recreatedWithDefaults := mongoDBStructuredIngestion(mongoDBColumns("name", "String"))
	recreatedWithDefaults.ExcludedColumns = types.SetValueMust(types.StringType, []attr.Value{})

	tests := []struct {
		name     string
		apiSI    *api.ClickPipeMongoDBStructuredIngestion
		state    types.Object
		expected types.Object
	}{
		{
			name:     "disabled in API is null",
			apiSI:    nil,
			state:    nullObj,
			expected: nullObj,
		},
		{
			name:     "inferred columns are read after create",
			apiSI:    &api.ClickPipeMongoDBStructuredIngestion{Enabled: true, DropUnexpectedValues: &f, Columns: nameColumn, ExcludedColumns: []string{"missions"}},
			state:    inferredWithExclusion.ObjectValue(),
			expected: inferredWithExclusionExpected.ObjectValue(),
		},
		{
			name:     "provided columns are echoed by the API",
			apiSI:    &api.ClickPipeMongoDBStructuredIngestion{Enabled: true, DropUnexpectedValues: &f, Columns: nameColumn},
			state:    mongoDBStructuredIngestion(mongoDBColumns("name", "String")).ObjectValue(),
			expected: mongoDBStructuredIngestion(mongoDBColumns("name", "String")).ObjectValue(),
		},
		{
			name:     "columns re-added outside Terraform are refreshed from the API",
			apiSI:    &api.ClickPipeMongoDBStructuredIngestion{Enabled: true, DropUnexpectedValues: &f, Columns: nameColumn},
			state:    mongoDBStructuredIngestion(mongoDBColumns("name", "Int64")).ObjectValue(),
			expected: mongoDBStructuredIngestion(mongoDBColumns("name", "String")).ObjectValue(),
		},
		{
			name:     "columns are read from the API on import",
			apiSI:    &api.ClickPipeMongoDBStructuredIngestion{Enabled: true, DropUnexpectedValues: &f, Columns: nameColumn},
			state:    nullObj,
			expected: mongoDBStructuredIngestion(mongoDBColumns("name", "String")).ObjectValue(),
		},
		{
			name:     "settings explicitly set to their defaults are kept",
			apiSI:    &api.ClickPipeMongoDBStructuredIngestion{Enabled: true, DropUnexpectedValues: &f, Columns: nameColumn},
			state:    explicitDefault.ObjectValue(),
			expected: explicitDefault.ObjectValue(),
		},
		{
			// The empty excluded_columns set is kept, as for Postgres table mappings.
			name:     "custom settings reset outside Terraform are refreshed as defaults",
			apiSI:    &api.ClickPipeMongoDBStructuredIngestion{Enabled: true, DropUnexpectedValues: &f, Columns: nameColumn},
			state:    customSettings.ObjectValue(),
			expected: recreatedWithDefaults.ObjectValue(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, diags := mongoDBStructuredIngestionFromAPI(ctx, tt.apiSI, tt.state)
			require.False(t, diags.HasError(), "%v", diags)
			assert.True(t, tt.expected.Equal(got), "expected %v, got %v", tt.expected, got)
		})
	}
}

func validateMongoDBStructuredIngestionSSH(t *testing.T, sshKeyResourceID types.String, si models.ClickPipeMongoDBStructuredIngestionModel) diag.Diagnostics {
	t.Helper()
	ctx := context.Background()

	model := buildMongoDBSSHKeyResourcePlan(sshKeyResourceID)
	var sourceModel models.ClickPipeSourceModel
	require.False(t, model.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{}).HasError())
	mongoAttrs := sourceModel.MongoDB.Attributes()
	mapping := mongoDBTableMappingModel(types.BoolNull())
	mapping.StructuredIngestion = si.ObjectValue()
	mongoAttrs["table_mappings"] = types.SetValueMust(
		models.ClickPipeMongoDBTableMappingModel{}.ObjectType(),
		[]attr.Value{mapping.ObjectValue()},
	)
	sourceModel.MongoDB = types.ObjectValueMust(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes, mongoAttrs)
	model.Source = sourceModel.ObjectValue()
	model.Scaling = types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes)
	model.Destination = types.ObjectNull(models.ClickPipeDestinationModel{}.ObjectType().AttrTypes)
	model.FieldMappings = types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType())
	model.Settings = types.DynamicNull()

	schemaResponse := &resource.SchemaResponse{}
	(&ClickPipeResource{}).Schema(ctx, resource.SchemaRequest{}, schemaResponse)
	require.False(t, schemaResponse.Diagnostics.HasError())

	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	require.False(t, plan.Set(ctx, &model).HasError())
	config := tfsdk.Config{Schema: schemaResponse.Schema, Raw: plan.Raw}

	resp := &resource.ValidateConfigResponse{}
	mongoDBStructuredIngestionValidator{}.ValidateResource(ctx, resource.ValidateConfigRequest{Config: config}, resp)
	return resp.Diagnostics
}

func TestMongoDBStructuredIngestionSSHValidator(t *testing.T) {
	inferred := mongoDBStructuredIngestion(types.ListNull(models.ClickPipeMongoDBColumnModel{}.ObjectType()))
	provided := mongoDBStructuredIngestion(mongoDBColumns("name", "String"))
	sshKey := types.StringValue("00000000-0000-0000-0000-000000000001")

	t.Run("inference with SSH key is rejected", func(t *testing.T) {
		diags := validateMongoDBStructuredIngestionSSH(t, sshKey, inferred)
		require.True(t, diags.HasError())
		assert.Contains(t, diags.Errors()[0].Detail(), "shop.orders")
	})
	t.Run("inference with an SSH key known only at apply is rejected", func(t *testing.T) {
		assert.True(t, validateMongoDBStructuredIngestionSSH(t, types.StringUnknown(), inferred).HasError())
	})
	t.Run("provided columns with SSH key are accepted", func(t *testing.T) {
		assert.False(t, validateMongoDBStructuredIngestionSSH(t, sshKey, provided).HasError())
	})
	t.Run("inference without SSH key is accepted", func(t *testing.T) {
		assert.False(t, validateMongoDBStructuredIngestionSSH(t, types.StringNull(), inferred).HasError())
	})
}

func TestMongoDBStructuredIngestionValidator_ExcludedProvidedColumns(t *testing.T) {
	overlapping := mongoDBStructuredIngestion(mongoDBColumns("name", "String", "internal_notes", "String"))
	overlapping.ExcludedColumns = stringSetValue([]string{"internal_notes"})
	diags := validateMongoDBStructuredIngestionSSH(t, types.StringNull(), overlapping)
	require.True(t, diags.HasError())
	assert.Contains(t, diags.Errors()[0].Detail(), "internal_notes")

	inferred := mongoDBStructuredIngestion(types.ListNull(models.ClickPipeMongoDBColumnModel{}.ObjectType()))
	inferred.ExcludedColumns = stringSetValue([]string{"internal_notes"})
	assert.False(t, validateMongoDBStructuredIngestionSSH(t, types.StringNull(), inferred).HasError())

	disjoint := mongoDBStructuredIngestion(mongoDBColumns("name", "String"))
	disjoint.ExcludedColumns = stringSetValue([]string{"internal_notes"})
	assert.False(t, validateMongoDBStructuredIngestionSSH(t, types.StringNull(), disjoint).HasError())
}

func mongoDBStructuredMapping(collection string, si models.ClickPipeMongoDBStructuredIngestionModel) models.ClickPipeMongoDBTableMappingModel {
	mapping := mongoDBTableMappingModel(types.BoolNull())
	mapping.SourceCollection = types.StringValue(collection)
	mapping.TargetTable = types.StringValue("shop_" + collection)
	mapping.StructuredIngestion = si.ObjectValue()
	return mapping
}

func mongoDBModelWithMappings(t *testing.T, mappings ...models.ClickPipeMongoDBTableMappingModel) models.ClickPipeResourceModel {
	t.Helper()
	model := buildMongoDBSSHKeyResourcePlan(types.StringNull())
	var sourceModel models.ClickPipeSourceModel
	require.False(t, model.Source.As(context.Background(), &sourceModel, basetypes.ObjectAsOptions{}).HasError())
	elements := make([]attr.Value, len(mappings))
	for i, mapping := range mappings {
		elements[i] = mapping.ObjectValue()
	}
	mongoAttrs := sourceModel.MongoDB.Attributes()
	mongoAttrs["table_mappings"] = types.SetValueMust(models.ClickPipeMongoDBTableMappingModel{}.ObjectType(), elements)
	sourceModel.MongoDB = types.ObjectValueMust(models.ClickPipeMongoDBSourceModel{}.ObjectType().AttrTypes, mongoAttrs)
	model.Source = sourceModel.ObjectValue()
	model.Scaling = types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes)
	model.Destination = types.ObjectNull(models.ClickPipeDestinationModel{}.ObjectType().AttrTypes)
	model.FieldMappings = types.ListNull(models.ClickPipeFieldMappingModel{}.ObjectType())
	model.Settings = types.DynamicNull()
	return model
}

func TestRepairMongoDBInferredColumnsPlan(t *testing.T) {
	ctx := context.Background()
	unknownColumns := types.ListUnknown(models.ClickPipeMongoDBColumnModel{}.ObjectType())
	inferredState := mongoDBStructuredIngestion(mongoDBColumns("name", "String"))
	provided := mongoDBStructuredIngestion(mongoDBColumns("model", "String"))

	state := mongoDBModelWithMappings(t,
		mongoDBStructuredMapping("replicants", inferredState),
		mongoDBStructuredMapping("missions", provided),
	)
	plan := mongoDBModelWithMappings(t,
		// existing inferred mapping: the framework marked its columns unknown
		mongoDBStructuredMapping("replicants", mongoDBStructuredIngestion(unknownColumns)),
		// existing provided mapping: planned from config
		mongoDBStructuredMapping("missions", provided),
		// new inferred mapping: read after apply
		mongoDBStructuredMapping("bounties", mongoDBStructuredIngestion(unknownColumns)),
	)

	repaired, diags := repairMongoDBInferredColumnsPlan(ctx, &plan, state)
	require.False(t, diags.HasError(), "%v", diags)
	assert.True(t, repaired)

	expected := mongoDBModelWithMappings(t,
		mongoDBStructuredMapping("replicants", inferredState),
		mongoDBStructuredMapping("missions", provided),
		mongoDBStructuredMapping("bounties", mongoDBStructuredIngestion(unknownColumns)),
	)
	assert.True(t, expected.Source.Equal(plan.Source), "expected %v, got %v", expected.Source, plan.Source)

	again, diags := repairMongoDBInferredColumnsPlan(ctx, &expected, state)
	require.False(t, diags.HasError())
	assert.False(t, again, "an already repaired plan is left as is")
}

// An unrelated change makes the framework plan the unconfigured columns as
// unknown; the repair must restore them before the "cannot modify" check.
func TestClickPipeResource_ModifyPlan_KeepsInferredMongoDBColumns(t *testing.T) {
	ctx := context.Background()
	state := mongoDBModelWithMappings(t,
		mongoDBStructuredMapping("replicants", mongoDBStructuredIngestion(mongoDBColumns("name", "String"))),
	)
	plan := mongoDBModelWithMappings(t,
		mongoDBStructuredMapping("replicants", mongoDBStructuredIngestion(types.ListUnknown(models.ClickPipeMongoDBColumnModel{}.ObjectType()))),
	)
	plan.Name = types.StringValue("renamed")

	diags := driveClickPipeModifyPlan(ctx, t, state, plan)
	assert.False(t, diags.HasError(), "unexpected errors: %v", diags.Errors())
}

// A mapping created with custom settings is removed and re-added outside
// Terraform with the defaults. Refresh must surface that drift, so that a
// config updated to match the live mapping plans cleanly, while the old
// config is still rejected as a change to an existing mapping.
func TestClickPipeResource_ModifyPlan_MongoDBStructuredIngestionResetOutsideTerraform(t *testing.T) {
	ctx := context.Background()
	f := false

	custom := mongoDBStructuredIngestion(mongoDBColumns("name", "String"))
	custom.ExcludedColumns = stringSetValue([]string{"missions"})
	custom.SortingKeys = stringListValue([]string{"name", "_id"})
	custom.PartitionByExpr = types.StringValue("toYYYYMM(inception_date)")

	refreshedSI, diags := mongoDBStructuredIngestionFromAPI(ctx,
		&api.ClickPipeMongoDBStructuredIngestion{Enabled: true, DropUnexpectedValues: &f, Columns: []api.ClickPipeMongoDBColumn{{Name: "name", Type: "String"}}},
		custom.ObjectValue(),
	)
	require.False(t, diags.HasError(), "%v", diags)
	var refreshed models.ClickPipeMongoDBStructuredIngestionModel
	require.False(t, refreshedSI.As(ctx, &refreshed, basetypes.ObjectAsOptions{}).HasError())
	state := mongoDBModelWithMappings(t, mongoDBStructuredMapping("replicants", refreshed))

	t.Run("config matching the live mapping plans cleanly", func(t *testing.T) {
		matching := mongoDBStructuredIngestion(mongoDBColumns("name", "String"))
		matching.ExcludedColumns = types.SetValueMust(types.StringType, []attr.Value{})
		plan := mongoDBModelWithMappings(t, mongoDBStructuredMapping("replicants", matching))
		diags := driveClickPipeModifyPlan(ctx, t, state, plan)
		assert.False(t, diags.HasError(), "unexpected errors: %v", diags.Errors())
	})

	t.Run("config with the old settings is a change to the mapping", func(t *testing.T) {
		plan := mongoDBModelWithMappings(t, mongoDBStructuredMapping("replicants", custom))
		diags := driveClickPipeModifyPlan(ctx, t, state, plan)
		assert.True(t, errorDetailContains(diags, "structured_ingestion"), "expected a cannot modify error, got %v", diags)
	})
}
