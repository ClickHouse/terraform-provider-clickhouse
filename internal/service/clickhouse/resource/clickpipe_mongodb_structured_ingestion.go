package resource

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// convertMongoDBStructuredIngestionModelToAPI builds the structuredIngestion
// payload of a table mapping. A null object means structured ingestion is off,
// and null columns ask the API to infer them.
func convertMongoDBStructuredIngestionModelToAPI(ctx context.Context, diagnostics *diag.Diagnostics, obj types.Object) *api.ClickPipeMongoDBStructuredIngestion {
	if obj.IsNull() || obj.IsUnknown() {
		return nil
	}
	var model models.ClickPipeMongoDBStructuredIngestionModel
	diagnostics.Append(obj.As(ctx, &model, basetypes.ObjectAsOptions{})...)
	if diagnostics.HasError() {
		return nil
	}

	si := &api.ClickPipeMongoDBStructuredIngestion{Enabled: true}
	if !model.DropUnexpectedValues.IsNull() && !model.DropUnexpectedValues.IsUnknown() {
		si.DropUnexpectedValues = model.DropUnexpectedValues.ValueBoolPointer()
	}
	if !model.Columns.IsNull() && !model.Columns.IsUnknown() {
		var columns []models.ClickPipeMongoDBColumnModel
		diagnostics.Append(model.Columns.ElementsAs(ctx, &columns, false)...)
		si.Columns = make([]api.ClickPipeMongoDBColumn, len(columns))
		for i, column := range columns {
			si.Columns[i] = api.ClickPipeMongoDBColumn{Name: column.Name.ValueString(), Type: column.Type.ValueString()}
		}
	}
	if !model.ExcludedColumns.IsNull() && !model.ExcludedColumns.IsUnknown() {
		diagnostics.Append(model.ExcludedColumns.ElementsAs(ctx, &si.ExcludedColumns, false)...)
	}
	if !model.SortingKeys.IsNull() && !model.SortingKeys.IsUnknown() {
		diagnostics.Append(model.SortingKeys.ElementsAs(ctx, &si.SortingKeys, false)...)
	}
	if !model.PartitionByExpr.IsNull() && !model.PartitionByExpr.IsUnknown() {
		si.PartitionByExpr = model.PartitionByExpr.ValueStringPointer()
	}
	return si
}

// mongoDBDocumentKeyColumn is the MongoDB document key, the default sorting key.
const mongoDBDocumentKeyColumn = "_id"

// mongoDBStructuredIngestionFromAPI builds the structured_ingestion state of a
// table mapping from the GET response, reconciled with prior state.
//
// columns are always taken from the API: they are the replicated columns,
// either as provided (which cannot overlap excluded_columns, so the API echoes
// them unchanged) or as inferred when the mapping was created. Prior state is
// only used to keep settings that were explicitly set to their default values.
func mongoDBStructuredIngestionFromAPI(ctx context.Context, apiSI *api.ClickPipeMongoDBStructuredIngestion, stateObj types.Object) (types.Object, diag.Diagnostics) {
	var diagnostics diag.Diagnostics
	nullObj := types.ObjectNull(models.ClickPipeMongoDBStructuredIngestionModel{}.ObjectType().AttrTypes)
	if apiSI == nil || !apiSI.Enabled {
		return nullObj, diagnostics
	}

	var state models.ClickPipeMongoDBStructuredIngestionModel
	hasState := !stateObj.IsNull() && !stateObj.IsUnknown()
	if hasState {
		diagnostics.Append(stateObj.As(ctx, &state, basetypes.ObjectAsOptions{})...)
		if diagnostics.HasError() {
			return nullObj, diagnostics
		}
	}

	model := models.ClickPipeMongoDBStructuredIngestionModel{}

	dropUnexpected := apiSI.DropUnexpectedValues != nil && *apiSI.DropUnexpectedValues
	if dropUnexpected || (hasState && !state.DropUnexpectedValues.IsNull()) {
		model.DropUnexpectedValues = types.BoolValue(dropUnexpected)
	} else {
		model.DropUnexpectedValues = types.BoolNull()
	}

	columnType := models.ClickPipeMongoDBColumnModel{}.ObjectType()
	if len(apiSI.Columns) > 0 {
		columns := make([]attr.Value, len(apiSI.Columns))
		for i, column := range apiSI.Columns {
			columns[i] = models.ClickPipeMongoDBColumnModel{
				Name: types.StringValue(column.Name),
				Type: types.StringValue(column.Type),
			}.ObjectValue()
		}
		model.Columns = types.ListValueMust(columnType, columns)
	} else {
		model.Columns = types.ListNull(columnType)
	}

	// When the API omits a setting, prior state is kept only if it holds the
	// default the API omitted, so an explicit default does not cause a diff.
	// Non-default state values are dropped, so a mapping recreated outside
	// Terraform with defaults shows up as drift instead of being hidden.
	if len(apiSI.ExcludedColumns) > 0 {
		model.ExcludedColumns = stringSetValue(apiSI.ExcludedColumns)
	} else if hasState && !state.ExcludedColumns.IsNull() && !state.ExcludedColumns.IsUnknown() {
		model.ExcludedColumns = types.SetValueMust(types.StringType, []attr.Value{})
	} else {
		model.ExcludedColumns = types.SetNull(types.StringType)
	}

	if len(apiSI.SortingKeys) > 0 {
		model.SortingKeys = stringListValue(apiSI.SortingKeys)
	} else if hasState && state.SortingKeys.Equal(stringListValue([]string{mongoDBDocumentKeyColumn})) {
		model.SortingKeys = state.SortingKeys
	} else {
		model.SortingKeys = types.ListNull(types.StringType)
	}

	if apiSI.PartitionByExpr != nil && *apiSI.PartitionByExpr != "" {
		model.PartitionByExpr = types.StringValue(*apiSI.PartitionByExpr)
	} else if hasState && state.PartitionByExpr.Equal(types.StringValue("")) {
		model.PartitionByExpr = state.PartitionByExpr
	} else {
		model.PartitionByExpr = types.StringNull()
	}

	return model.ObjectValue(), diagnostics
}

func stringSetValue(values []string) types.Set {
	elems := make([]attr.Value, len(values))
	for i, v := range values {
		elems[i] = types.StringValue(v)
	}
	return types.SetValueMust(types.StringType, elems)
}

func stringListValue(values []string) types.List {
	elems := make([]attr.Value, len(values))
	for i, v := range values {
		elems[i] = types.StringValue(v)
	}
	return types.ListValueMust(types.StringType, elems)
}

// repairMongoDBInferredColumnsPlan keeps the inferred columns of existing
// MongoDB structured ingestion mappings in the plan.
//
// columns is Optional+Computed, so when it is not configured the framework
// marks it unknown whenever anything in the resource changes, and
// UseStateForUnknown cannot be used because set elements cannot be matched
// with their prior state by path. Mappings are matched here by source
// database and collection instead:
// - An unknown or null planned value takes the state value.
// - Configured columns are never touched.
// - New mappings stay unknown so the inferred columns are read after apply.
func repairMongoDBInferredColumnsPlan(ctx context.Context, plan *models.ClickPipeResourceModel, state models.ClickPipeResourceModel) (bool, diag.Diagnostics) {
	var diagnostics diag.Diagnostics
	if plan.Source.IsNull() || plan.Source.IsUnknown() || state.Source.IsNull() || state.Source.IsUnknown() {
		return false, diagnostics
	}

	var planSource, stateSource models.ClickPipeSourceModel
	diagnostics.Append(plan.Source.As(ctx, &planSource, basetypes.ObjectAsOptions{})...)
	diagnostics.Append(state.Source.As(ctx, &stateSource, basetypes.ObjectAsOptions{})...)
	if diagnostics.HasError() || planSource.MongoDB.IsNull() || planSource.MongoDB.IsUnknown() || stateSource.MongoDB.IsNull() {
		return false, diagnostics
	}

	var planMongo, stateMongo models.ClickPipeMongoDBSourceModel
	diagnostics.Append(planSource.MongoDB.As(ctx, &planMongo, basetypes.ObjectAsOptions{})...)
	diagnostics.Append(stateSource.MongoDB.As(ctx, &stateMongo, basetypes.ObjectAsOptions{})...)
	if diagnostics.HasError() || planMongo.TableMappings.IsNull() || planMongo.TableMappings.IsUnknown() || stateMongo.TableMappings.IsNull() {
		return false, diagnostics
	}

	type mappingKey struct{ database, collection string }
	stateColumns := map[mappingKey]types.List{}
	var stateMappings []models.ClickPipeMongoDBTableMappingModel
	diagnostics.Append(stateMongo.TableMappings.ElementsAs(ctx, &stateMappings, false)...)
	for _, mapping := range stateMappings {
		if mapping.StructuredIngestion.IsNull() {
			continue
		}
		var si models.ClickPipeMongoDBStructuredIngestionModel
		diagnostics.Append(mapping.StructuredIngestion.As(ctx, &si, basetypes.ObjectAsOptions{})...)
		if !si.Columns.IsNull() && !si.Columns.IsUnknown() {
			stateColumns[mappingKey{mapping.SourceDatabaseName.ValueString(), mapping.SourceCollection.ValueString()}] = si.Columns
		}
	}

	var planMappings []models.ClickPipeMongoDBTableMappingModel
	diagnostics.Append(planMongo.TableMappings.ElementsAs(ctx, &planMappings, false)...)
	if diagnostics.HasError() {
		return false, diagnostics
	}

	repaired := false
	elements := make([]attr.Value, len(planMappings))
	for i, mapping := range planMappings {
		elements[i] = mapping.ObjectValue()
		if mapping.StructuredIngestion.IsNull() || mapping.StructuredIngestion.IsUnknown() {
			continue
		}
		var si models.ClickPipeMongoDBStructuredIngestionModel
		diagnostics.Append(mapping.StructuredIngestion.As(ctx, &si, basetypes.ObjectAsOptions{})...)
		if !si.Columns.IsNull() && !si.Columns.IsUnknown() {
			continue
		}
		columns, ok := stateColumns[mappingKey{mapping.SourceDatabaseName.ValueString(), mapping.SourceCollection.ValueString()}]
		if !ok {
			continue
		}
		si.Columns = columns
		mapping.StructuredIngestion = si.ObjectValue()
		elements[i] = mapping.ObjectValue()
		repaired = true
	}
	if !repaired || diagnostics.HasError() {
		return false, diagnostics
	}

	tableMappings, diags := types.SetValue(models.ClickPipeMongoDBTableMappingModel{}.ObjectType(), elements)
	diagnostics.Append(diags...)
	if diagnostics.HasError() {
		return false, diagnostics
	}
	planMongo.TableMappings = tableMappings
	planSource.MongoDB = planMongo.ObjectValue()
	plan.Source = planSource.ObjectValue()
	return true, diagnostics
}
