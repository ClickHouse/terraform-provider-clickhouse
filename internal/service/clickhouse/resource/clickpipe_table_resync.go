package resource

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// Table-level resync of CDC pipes (Postgres, MySQL, MongoDB).
//
// A table is resynced when its mapping's resync_version changes. Like
// password_wo_version, the version is an intent shown in the plan that needs
// no reset after apply. Changing an existing mapping rebuilds its destination
// table from a fresh snapshot, so it requires changing the version too:
// otherwise the plan fails, as it did before resync existed.

const clickPipeResyncVersion = "resync_version"

// clickPipeResyncVersionAttribute is the resync_version attribute of a CDC
// table mapping. It is not sent to the API. The resync behavior is described
// once in the resource description (descriptions/clickpipe.md).
func clickPipeResyncVersionAttribute(tableNoun string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: fmt.Sprintf("Change this value to resync the %s. See *Table resync* above.", tableNoun),
		Optional:            true,
	}
}

// resyncVersionChanged reports whether a mapping's resync_version asks for a
// resync. An unknown planned version counts, as it is known at apply.
func resyncVersionChanged(prior, planned types.Int64) bool {
	return !planned.IsNull() && !planned.Equal(prior)
}

// sameStringSet and sameStringList treat null and empty as equal: the API
// stores both as unset, so switching between them changes nothing.
func isNullOrEmptySet(s types.Set) bool {
	return s.IsNull() || (!s.IsUnknown() && len(s.Elements()) == 0)
}

func isNullOrEmptyList(l types.List) bool {
	return l.IsNull() || (!l.IsUnknown() && len(l.Elements()) == 0)
}

func sameStringSet(a, b types.Set) bool {
	return a.Equal(b) || (isNullOrEmptySet(a) && isNullOrEmptySet(b))
}

func sameStringList(a, b types.List) bool {
	return a.Equal(b) || (isNullOrEmptyList(a) && isNullOrEmptyList(b))
}

// sqlTableMappingChange returns the first attribute that differs between the
// prior and planned mapping of the same Postgres or MySQL source table, or ""
// when none does. Both sources share the mapping attributes, so MySQL mappings
// are converted to the Postgres model, which has the same fields.
// resync_version is not compared: it decides whether a change is applied.
func sqlTableMappingChange(prior, planned models.ClickPipePostgresTableMappingModel) string {
	switch {
	case prior.TargetTable.ValueString() != planned.TargetTable.ValueString():
		return clickPipeTargetTable
	case !sameStringSet(prior.ExcludedColumns, planned.ExcludedColumns):
		return "excluded_columns"
	case !sameStringList(prior.SortingKeys, planned.SortingKeys):
		return "sorting_keys"
	case !prior.UseCustomSortingKey.Equal(planned.UseCustomSortingKey):
		return "use_custom_sorting_key"
	case !prior.TableEngine.Equal(planned.TableEngine):
		return "table_engine"
	case !prior.PartitionKey.Equal(planned.PartitionKey):
		return "partition_key"
	case !prior.PartitionByExpr.Equal(planned.PartitionByExpr):
		return "partition_by_expr"
	}
	return ""
}

// mongoDBTableMappingChange is sqlTableMappingChange for MongoDB mappings.
func mongoDBTableMappingChange(prior, planned models.ClickPipeMongoDBTableMappingModel) string {
	switch {
	case prior.TargetTable.ValueString() != planned.TargetTable.ValueString():
		return clickPipeTargetTable
	case !prior.TableEngine.Equal(planned.TableEngine):
		return "table_engine"
	case prior.PersistDeletedDocuments.ValueBool() != planned.PersistDeletedDocuments.ValueBool():
		return "persist_deleted_documents"
	case !prior.StructuredIngestion.Equal(planned.StructuredIngestion):
		return "structured_ingestion"
	}
	return ""
}

// validateTableMappingChange reports a planned change to an existing table
// mapping: an error unless its resync_version changed too, and a warning
// naming the table when it is resynced. A resynced table replaces its
// destination table under the same name, so target_table can never change.
func validateTableMappingChange(diagnostics *diag.Diagnostics, table, changed string, resync bool) {
	switch {
	case changed == clickPipeTargetTable:
		diagnostics.AddError(
			"Invalid table_mappings configuration",
			fmt.Sprintf("Cannot modify %s for existing table mapping '%s'. A resynced table keeps its destination table, so target_table cannot be changed even on resync: remove the mapping and add a new one.", changed, table),
		)
	case changed != "" && !resync:
		diagnostics.AddError(
			"Invalid table_mappings configuration",
			fmt.Sprintf("Cannot modify %s for existing table mapping '%s'. Change its %s in the same apply to resync the table with its new settings, which rebuilds its destination table from a fresh snapshot.", changed, table, clickPipeResyncVersion),
		)
	case resync:
		diagnostics.AddWarning(
			"Table will be resynced",
			fmt.Sprintf("%s changed for table mapping '%s'. Applying this plan copies the table again from the source with its current settings, then replaces its current destination table. Replication of the other tables is paused until the resync completes. Track the resync progress in the ClickHouse Cloud console.", clickPipeResyncVersion, table),
		)
	}
}

// validateMongoDBResyncColumns rejects a resync that turns on structured
// ingestion without columns: the API only infers columns for added
// collections. Existing structured ingestion mappings keep their inferred
// columns in the plan (see repairMongoDBInferredColumnsPlan), so this only
// fires when the columns are not known yet.
func validateMongoDBResyncColumns(ctx context.Context, diagnostics *diag.Diagnostics, table string, planned models.ClickPipeMongoDBTableMappingModel) {
	if planned.StructuredIngestion.IsNull() || planned.StructuredIngestion.IsUnknown() {
		return
	}
	var si models.ClickPipeMongoDBStructuredIngestionModel
	diagnostics.Append(planned.StructuredIngestion.As(ctx, &si, basetypes.ObjectAsOptions{})...)
	if diagnostics.HasError() {
		return
	}
	if si.Columns.IsNull() || si.Columns.IsUnknown() {
		diagnostics.AddError(
			"Invalid table_mappings configuration",
			fmt.Sprintf("structured_ingestion.columns must be set to resync collection '%s' with structured ingestion: columns are only inferred when a collection is added to the pipe.", table),
		)
	}
}

// tableMappingDeltas splits the difference between the prior and planned
// mappings into the tables to add, remove and resync. Mappings with the same
// key are the same table, and are resynced when resync says so. Other changes
// to them are rejected by ModifyPlan.
func tableMappingDeltas[M any](ctx context.Context, diagnostics *diag.Diagnostics, planSet, stateSet types.Set, key func(M) string, resync func(prior, planned M) bool) (toAdd, toRemove, toResync []M) {
	var planMappings, stateMappings []M
	diagnostics.Append(planSet.ElementsAs(ctx, &planMappings, false)...)
	diagnostics.Append(stateSet.ElementsAs(ctx, &stateMappings, false)...)
	if diagnostics.HasError() {
		return nil, nil, nil
	}

	stateByKey := make(map[string]M, len(stateMappings))
	for _, m := range stateMappings {
		stateByKey[key(m)] = m
	}
	planKeys := make(map[string]bool, len(planMappings))
	for _, planned := range planMappings {
		planKeys[key(planned)] = true
		prior, exists := stateByKey[key(planned)]
		switch {
		case !exists:
			toAdd = append(toAdd, planned)
		case resync(prior, planned):
			toResync = append(toResync, planned)
		}
	}
	for _, prior := range stateMappings {
		if !planKeys[key(prior)] {
			toRemove = append(toRemove, prior)
		}
	}
	return toAdd, toRemove, toResync
}

func convertAll[M, A any](mappings []M, convert func(M) A) []A {
	if len(mappings) == 0 {
		return nil
	}
	converted := make([]A, len(mappings))
	for i, m := range mappings {
		converted[i] = convert(m)
	}
	return converted
}

// sqlTableMappingKey identifies a Postgres or MySQL mapping by source and
// target table, so a changed target_table is an add plus a remove, which
// ModifyPlan rejects.
func sqlTableMappingKey(m models.ClickPipePostgresTableMappingModel) string {
	return fmt.Sprintf("%s.%s->%s", m.SourceSchemaName.ValueString(), m.SourceTable.ValueString(), m.TargetTable.ValueString())
}

func (c *ClickPipeResource) postgresTableMappingDeltas(ctx context.Context, diagnostics *diag.Diagnostics, planSet, stateSet types.Set) (toAdd, toRemove, toResync []api.ClickPipePostgresTableMapping) {
	resync := func(prior, planned models.ClickPipePostgresTableMappingModel) bool {
		return resyncVersionChanged(prior.ResyncVersion, planned.ResyncVersion)
	}
	add, remove, res := tableMappingDeltas(ctx, diagnostics, planSet, stateSet, sqlTableMappingKey, resync)
	toAPI := func(m models.ClickPipePostgresTableMappingModel) api.ClickPipePostgresTableMapping {
		return c.convertTableMappingModelToAPI(ctx, *diagnostics, m)
	}
	return convertAll(add, toAPI), convertAll(remove, toAPI), convertAll(res, toAPI)
}

func mySQLTableMappingDeltas(ctx context.Context, diagnostics *diag.Diagnostics, planSet, stateSet types.Set) (toAdd, toRemove, toResync []api.ClickPipeMySQLTableMapping) {
	key := func(m models.ClickPipeMySQLTableMappingModel) string {
		return sqlTableMappingKey(models.ClickPipePostgresTableMappingModel(m))
	}
	resync := func(prior, planned models.ClickPipeMySQLTableMappingModel) bool {
		return resyncVersionChanged(prior.ResyncVersion, planned.ResyncVersion)
	}
	add, remove, res := tableMappingDeltas(ctx, diagnostics, planSet, stateSet, key, resync)
	toAPI := func(m models.ClickPipeMySQLTableMappingModel) api.ClickPipeMySQLTableMapping {
		return convertMySQLTableMappingModelToAPI(ctx, diagnostics, m)
	}
	return convertAll(add, toAPI), convertAll(remove, toAPI), convertAll(res, toAPI)
}

// mongoDBTableMappingDeltas identifies collections by source only.
func mongoDBTableMappingDeltas(ctx context.Context, diagnostics *diag.Diagnostics, planSet, stateSet types.Set) (toAdd, toRemove, toResync []api.ClickPipeMongoDBTableMapping) {
	key := func(m models.ClickPipeMongoDBTableMappingModel) string {
		return fmt.Sprintf("%s.%s", m.SourceDatabaseName.ValueString(), m.SourceCollection.ValueString())
	}
	resync := func(prior, planned models.ClickPipeMongoDBTableMappingModel) bool {
		return resyncVersionChanged(prior.ResyncVersion, planned.ResyncVersion)
	}
	add, remove, res := tableMappingDeltas(ctx, diagnostics, planSet, stateSet, key, resync)
	// Additions and resyncs carry the whole mapping: the API replaces a resynced one entirely.
	toAPI := func(m models.ClickPipeMongoDBTableMappingModel) api.ClickPipeMongoDBTableMapping {
		return convertMongoDBTableMappingModelToAPI(ctx, diagnostics, m)
	}
	// A removal only identifies the collection: the API rejects mongoConfig and structuredIngestion here.
	toRemovalAPI := func(m models.ClickPipeMongoDBTableMappingModel) api.ClickPipeMongoDBTableMapping {
		mapping := toAPI(m)
		mapping.MongoConfig = nil
		mapping.StructuredIngestion = nil
		return mapping
	}
	return convertAll(add, toAPI), convertAll(remove, toRemovalAPI), convertAll(res, toAPI)
}
