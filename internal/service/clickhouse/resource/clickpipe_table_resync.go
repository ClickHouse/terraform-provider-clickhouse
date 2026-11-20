package resource

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

// Table-level resync of CDC pipes (Postgres, MySQL, MongoDB).
//
// Changing an existing table mapping rebuilds its destination table from a
// fresh snapshot, so it is opt-in with the pipe's resync_tables_on_change.
// Without it the plan fails, as it did before resync existed: a plan warning
// alone is not enough for pipelines that apply with -auto-approve.

const clickPipeResyncTablesOnChange = "resync_tables_on_change"

// clickPipeResyncTablesOnChangeAttribute is the resync_tables_on_change
// attribute of a ClickPipe. It is not sent to the API, it only decides how a
// change to an existing table mapping is applied.
func clickPipeResyncTablesOnChangeAttribute() schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: "Postgres, MySQL and MongoDB pipes only. When `true`, changing an existing table mapping in `table_mappings`, other than its `target_table`, resyncs that table: " +
			"it is copied again from the source with the new settings, then replaces its current destination table. Replication of the other tables is paused until the resync completes. " +
			"Track the resync progress in the ClickHouse Cloud console. " +
			"When `false`, such a change fails at plan time. Changing only this attribute resyncs nothing. Defaults to `false`.",
		Optional: true,
		Computed: true,
		Default:  booldefault.StaticBool(false),
	}
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

// postgresTableMappingChange returns the first attribute that differs between
// the prior and planned mapping of the same source table, or "" when none
// does. MySQL mappings have the same attributes, so they are converted and
// compared here too.
func postgresTableMappingChange(prior, planned models.ClickPipePostgresTableMappingModel) string {
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

// mongoDBTableMappingChange is postgresTableMappingChange for MongoDB mappings.
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
// mapping: an error unless the pipe opted into table resync, a warning naming
// the table to resync otherwise. A resynced table replaces its destination table
// under the same name, so target_table can never change.
func validateTableMappingChange(diagnostics *diag.Diagnostics, table, changed string, resyncTablesOnChange bool) {
	switch {
	case changed == "":
	case changed == clickPipeTargetTable:
		diagnostics.AddError(
			"Invalid table_mappings configuration",
			fmt.Sprintf("Cannot modify %s for existing table mapping '%s'. A resynced table keeps its destination table, so target_table cannot be changed even with %s: remove the mapping and add a new one.", changed, table, clickPipeResyncTablesOnChange),
		)
	case !resyncTablesOnChange:
		diagnostics.AddError(
			"Invalid table_mappings configuration",
			fmt.Sprintf("Cannot modify %s for existing table mapping '%s'. Set %s = true on the pipe to resync the table with its new settings, which rebuilds its destination table from a fresh snapshot.", changed, table, clickPipeResyncTablesOnChange),
		)
	default:
		diagnostics.AddWarning(
			"Table will be resynced",
			fmt.Sprintf("%s changed for table mapping '%s' and the pipe has %s = true. Applying this plan copies the table again from the source with its new settings, then replaces its current destination table. Replication of the other tables is paused until the resync completes. Track the resync progress in the ClickHouse Cloud console.", changed, table, clickPipeResyncTablesOnChange),
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
// key are the same table. A planned mapping that changed is only resynced when
// resync is true; ModifyPlan rejects it otherwise.
func tableMappingDeltas[M any](ctx context.Context, diagnostics *diag.Diagnostics, planSet, stateSet types.Set, resync bool, key func(M) string, change func(prior, planned M) string) (toAdd, toRemove, toResync []M) {
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
		case resync && change(prior, planned) != "":
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

// postgresTableMappingKey identifies a Postgres or MySQL mapping by source and
// target table, so a changed target_table is an add plus a remove, which
// ModifyPlan rejects.
func postgresTableMappingKey(m models.ClickPipePostgresTableMappingModel) string {
	return fmt.Sprintf("%s.%s->%s", m.SourceSchemaName.ValueString(), m.SourceTable.ValueString(), m.TargetTable.ValueString())
}

func (c *ClickPipeResource) postgresTableMappingDeltas(ctx context.Context, diagnostics *diag.Diagnostics, planSet, stateSet types.Set, resync bool) (toAdd, toRemove, toResync []api.ClickPipePostgresTableMapping) {
	add, remove, res := tableMappingDeltas(ctx, diagnostics, planSet, stateSet, resync, postgresTableMappingKey, postgresTableMappingChange)
	toAPI := func(m models.ClickPipePostgresTableMappingModel) api.ClickPipePostgresTableMapping {
		return c.convertTableMappingModelToAPI(ctx, *diagnostics, m)
	}
	return convertAll(add, toAPI), convertAll(remove, toAPI), convertAll(res, toAPI)
}

func mySQLTableMappingDeltas(ctx context.Context, diagnostics *diag.Diagnostics, planSet, stateSet types.Set, resync bool) (toAdd, toRemove, toResync []api.ClickPipeMySQLTableMapping) {
	key := func(m models.ClickPipeMySQLTableMappingModel) string {
		return postgresTableMappingKey(models.ClickPipePostgresTableMappingModel(m))
	}
	change := func(prior, planned models.ClickPipeMySQLTableMappingModel) string {
		return postgresTableMappingChange(models.ClickPipePostgresTableMappingModel(prior), models.ClickPipePostgresTableMappingModel(planned))
	}
	add, remove, res := tableMappingDeltas(ctx, diagnostics, planSet, stateSet, resync, key, change)
	toAPI := func(m models.ClickPipeMySQLTableMappingModel) api.ClickPipeMySQLTableMapping {
		return convertMySQLTableMappingModelToAPI(ctx, diagnostics, m)
	}
	return convertAll(add, toAPI), convertAll(remove, toAPI), convertAll(res, toAPI)
}

// mongoDBTableMappingDeltas identifies collections by source only.
func mongoDBTableMappingDeltas(ctx context.Context, diagnostics *diag.Diagnostics, planSet, stateSet types.Set, resync bool) (toAdd, toRemove, toResync []api.ClickPipeMongoDBTableMapping) {
	key := func(m models.ClickPipeMongoDBTableMappingModel) string {
		return fmt.Sprintf("%s.%s", m.SourceDatabaseName.ValueString(), m.SourceCollection.ValueString())
	}
	add, remove, res := tableMappingDeltas(ctx, diagnostics, planSet, stateSet, resync, key, mongoDBTableMappingChange)
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
