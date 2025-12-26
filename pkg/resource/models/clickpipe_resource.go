//go:build alpha

package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ClickPipeScalingModel struct {
	Replicas             types.Int64   `tfsdk:"replicas"`
	ReplicaCpuMillicores types.Int64   `tfsdk:"replica_cpu_millicores"`
	ReplicaMemoryGb      types.Float64 `tfsdk:"replica_memory_gb"`
}

func (m ClickPipeScalingModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"replicas":               types.Int64Type,
			"replica_cpu_millicores": types.Int64Type,
			"replica_memory_gb":      types.Float64Type,
		},
	}
}

func (m ClickPipeScalingModel) ObjectValue() types.Object {
	objValue, _ := types.ObjectValue(m.ObjectType().AttrTypes, map[string]attr.Value{
		"replicas":               m.Replicas,
		"replica_cpu_millicores": m.ReplicaCpuMillicores,
		"replica_memory_gb":      m.ReplicaMemoryGb,
	})
	return objValue
}

type ClickPipeKafkaOffsetModel struct {
	Strategy  types.String `tfsdk:"strategy"`
	Timestamp types.String `tfsdk:"timestamp"`
}

func (m ClickPipeKafkaOffsetModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"strategy":  types.StringType,
			"timestamp": types.StringType,
		},
	}
}

func (m ClickPipeKafkaOffsetModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"strategy":  m.Strategy,
		"timestamp": m.Timestamp,
	})
}

type ClickPipeKafkaSchemaRegistryModel struct {
	URL            types.String `tfsdk:"url"`
	Authentication types.String `tfsdk:"authentication"`
	Credentials    types.Object `tfsdk:"credentials"`
}

func (m ClickPipeKafkaSchemaRegistryModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"url":            types.StringType,
			"authentication": types.StringType,
			"credentials":    ClickPipeSourceCredentialsModel{}.ObjectType(),
		},
	}
}

func (m ClickPipeKafkaSchemaRegistryModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"url":            m.URL,
		"authentication": m.Authentication,
		"credentials":    m.Credentials,
	})
}

type ClickPipeSourceCredentialsModel struct {
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

func (m ClickPipeSourceCredentialsModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"username": types.StringType,
			"password": types.StringType,
		},
	}
}

func (m ClickPipeSourceCredentialsModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"username": m.Username,
		"password": m.Password,
	})
}

type ClickPipeSourceAccessKeyModel struct {
	AccessKeyID types.String `tfsdk:"access_key_id"`
	SecretKey   types.String `tfsdk:"secret_key"`
}

func (m ClickPipeSourceAccessKeyModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"access_key_id": types.StringType,
			"secret_key":    types.StringType,
		},
	}
}

func (m ClickPipeSourceAccessKeyModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"access_key_id": m.AccessKeyID,
		"secret_key":    m.SecretKey,
	})
}

type ClickPipeKafkaSourceCredentialsModel struct {
	// PLAIN and SCRAM
	ClickPipeSourceCredentialsModel

	// AWS IAM user credentials
	ClickPipeSourceAccessKeyModel

	// Azure EventHub connection string
	ConnectionString types.String `tfsdk:"connection_string"`
}

func (m ClickPipeKafkaSourceCredentialsModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"username":          types.StringType,
			"password":          types.StringType,
			"access_key_id":     types.StringType,
			"secret_key":        types.StringType,
			"connection_string": types.StringType,
		},
	}
}

func (m ClickPipeKafkaSourceCredentialsModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"username":          m.Username,
		"password":          m.Password,
		"access_key_id":     m.AccessKeyID,
		"secret_key":        m.SecretKey,
		"connection_string": m.ConnectionString,
	})
}

type ClickPipeKafkaSourceModel struct {
	Type   types.String `tfsdk:"type"`
	Format types.String `tfsdk:"format"`

	Brokers types.String `tfsdk:"brokers"`
	Topics  types.String `tfsdk:"topics"`

	ConsumerGroup types.String `tfsdk:"consumer_group"`

	Offset         types.Object `tfsdk:"offset"`
	SchemaRegistry types.Object `tfsdk:"schema_registry"`

	Authentication types.String `tfsdk:"authentication"`
	Credentials    types.Object `tfsdk:"credentials"`
	IAMRole        types.String `tfsdk:"iam_role"`
	CACertificate  types.String `tfsdk:"ca_certificate"`

	ReversePrivateEndpointIDs types.List `tfsdk:"reverse_private_endpoint_ids"`
}

func (m ClickPipeKafkaSourceModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"type":                         types.StringType,
			"format":                       types.StringType,
			"brokers":                      types.StringType,
			"topics":                       types.StringType,
			"consumer_group":               types.StringType,
			"offset":                       ClickPipeKafkaOffsetModel{}.ObjectType(),
			"schema_registry":              ClickPipeKafkaSchemaRegistryModel{}.ObjectType(),
			"authentication":               types.StringType,
			"credentials":                  ClickPipeKafkaSourceCredentialsModel{}.ObjectType(),
			"iam_role":                     types.StringType,
			"ca_certificate":               types.StringType,
			"reverse_private_endpoint_ids": types.ListType{ElemType: types.StringType},
		},
	}
}

func (m ClickPipeKafkaSourceModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"type":                         m.Type,
		"format":                       m.Format,
		"brokers":                      m.Brokers,
		"topics":                       m.Topics,
		"consumer_group":               m.ConsumerGroup,
		"offset":                       m.Offset,
		"schema_registry":              m.SchemaRegistry,
		"authentication":               m.Authentication,
		"credentials":                  m.Credentials,
		"iam_role":                     m.IAMRole,
		"ca_certificate":               m.CACertificate,
		"reverse_private_endpoint_ids": m.ReversePrivateEndpointIDs,
	})
}

type ClickPipeKinesisSourceModel struct {
	Format            types.String `tfsdk:"format"`
	StreamName        types.String `tfsdk:"stream_name"`
	Region            types.String `tfsdk:"region"`
	IteratorType      types.String `tfsdk:"iterator_type"`
	Timestamp         types.String `tfsdk:"timestamp"`
	UseEnhancedFanOut types.Bool   `tfsdk:"use_enhanced_fan_out"`
	Authentication    types.String `tfsdk:"authentication"`
	AccessKey         types.Object `tfsdk:"access_key"`
	IAMRole           types.String `tfsdk:"iam_role"`
}

func (m ClickPipeKinesisSourceModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"format":               types.StringType,
			"stream_name":          types.StringType,
			"region":               types.StringType,
			"iterator_type":        types.StringType,
			"timestamp":            types.StringType,
			"use_enhanced_fan_out": types.BoolType,
			"authentication":       types.StringType,
			"access_key":           ClickPipeSourceAccessKeyModel{}.ObjectType(),
			"iam_role":             types.StringType,
		},
	}
}

func (m ClickPipeKinesisSourceModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"format":               m.Format,
		"stream_name":          m.StreamName,
		"region":               m.Region,
		"iterator_type":        m.IteratorType,
		"timestamp":            m.Timestamp,
		"use_enhanced_fan_out": m.UseEnhancedFanOut,
		"authentication":       m.Authentication,
		"access_key":           m.AccessKey,
		"iam_role":             m.IAMRole,
	})
}

type ClickPipePostgresSettingsModel struct {
	SyncIntervalSeconds            types.Int64  `tfsdk:"sync_interval_seconds"`
	PullBatchSize                  types.Int64  `tfsdk:"pull_batch_size"`
	PublicationName                types.String `tfsdk:"publication_name"`
	ReplicationMode                types.String `tfsdk:"replication_mode"`
	ReplicationSlotName            types.String `tfsdk:"replication_slot_name"`
	AllowNullableColumns           types.Bool   `tfsdk:"allow_nullable_columns"`
	InitialLoadParallelism         types.Int64  `tfsdk:"initial_load_parallelism"`
	SnapshotNumRowsPerPartition    types.Int64  `tfsdk:"snapshot_num_rows_per_partition"`
	SnapshotNumberOfParallelTables types.Int64  `tfsdk:"snapshot_number_of_parallel_tables"`
	EnableFailoverSlots            types.Bool   `tfsdk:"enable_failover_slots"`
}

func (m ClickPipePostgresSettingsModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"sync_interval_seconds":              types.Int64Type,
			"pull_batch_size":                    types.Int64Type,
			"publication_name":                   types.StringType,
			"replication_mode":                   types.StringType,
			"replication_slot_name":              types.StringType,
			"allow_nullable_columns":             types.BoolType,
			"initial_load_parallelism":           types.Int64Type,
			"snapshot_num_rows_per_partition":    types.Int64Type,
			"snapshot_number_of_parallel_tables": types.Int64Type,
			"enable_failover_slots":              types.BoolType,
		},
	}
}

func (m ClickPipePostgresSettingsModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"sync_interval_seconds":              m.SyncIntervalSeconds,
		"pull_batch_size":                    m.PullBatchSize,
		"publication_name":                   m.PublicationName,
		"replication_mode":                   m.ReplicationMode,
		"replication_slot_name":              m.ReplicationSlotName,
		"allow_nullable_columns":             m.AllowNullableColumns,
		"initial_load_parallelism":           m.InitialLoadParallelism,
		"snapshot_num_rows_per_partition":    m.SnapshotNumRowsPerPartition,
		"snapshot_number_of_parallel_tables": m.SnapshotNumberOfParallelTables,
		"enable_failover_slots":              m.EnableFailoverSlots,
	})
}

type ClickPipePostgresTableMappingModel struct {
	SourceSchemaName    types.String `tfsdk:"source_schema_name"`
	SourceTable         types.String `tfsdk:"source_table"`
	TargetTable         types.String `tfsdk:"target_table"`
	ExcludedColumns     types.List   `tfsdk:"excluded_columns"`
	UseCustomSortingKey types.Bool   `tfsdk:"use_custom_sorting_key"`
	SortingKeys         types.List   `tfsdk:"sorting_keys"`
	TableEngine         types.String `tfsdk:"table_engine"`
	PartitionKey        types.String `tfsdk:"partition_key"`
}

func (m ClickPipePostgresTableMappingModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"source_schema_name":     types.StringType,
			"source_table":           types.StringType,
			"target_table":           types.StringType,
			"excluded_columns":       types.ListType{ElemType: types.StringType},
			"use_custom_sorting_key": types.BoolType,
			"sorting_keys":           types.ListType{ElemType: types.StringType},
			"table_engine":           types.StringType,
			"partition_key":          types.StringType,
		},
	}
}

func (m ClickPipePostgresTableMappingModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"source_schema_name":     m.SourceSchemaName,
		"source_table":           m.SourceTable,
		"target_table":           m.TargetTable,
		"excluded_columns":       m.ExcludedColumns,
		"use_custom_sorting_key": m.UseCustomSortingKey,
		"sorting_keys":           m.SortingKeys,
		"table_engine":           m.TableEngine,
		"partition_key":          m.PartitionKey,
	})
}

type ClickPipePostgresSourceModel struct {
	Host           types.String `tfsdk:"host"`
	Port           types.Int64  `tfsdk:"port"`
	Database       types.String `tfsdk:"database"`
	Authentication types.String `tfsdk:"authentication"`
	IAMRole        types.String `tfsdk:"iam_role"`
	TLSHost        types.String `tfsdk:"tls_host"`
	CACertificate  types.String `tfsdk:"ca_certificate"`
	Credentials    types.Object `tfsdk:"credentials"`
	Settings       types.Object `tfsdk:"settings"`
	TableMappings  types.Set    `tfsdk:"table_mappings"`
}

func (m ClickPipePostgresSourceModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"host":           types.StringType,
			"port":           types.Int64Type,
			"database":       types.StringType,
			"authentication": types.StringType,
			"iam_role":       types.StringType,
			"tls_host":       types.StringType,
			"ca_certificate": types.StringType,
			"credentials":    ClickPipeSourceCredentialsModel{}.ObjectType(),
			"settings":       ClickPipePostgresSettingsModel{}.ObjectType(),
			"table_mappings": types.SetType{ElemType: ClickPipePostgresTableMappingModel{}.ObjectType()},
		},
	}
}

func (m ClickPipePostgresSourceModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"host":           m.Host,
		"port":           m.Port,
		"database":       m.Database,
		"authentication": m.Authentication,
		"iam_role":       m.IAMRole,
		"tls_host":       m.TLSHost,
		"ca_certificate": m.CACertificate,
		"credentials":    m.Credentials,
		"settings":       m.Settings,
		"table_mappings": m.TableMappings,
	})
}

type ClickPipeObjectStorageSourceModel struct {
	Type           types.String `tfsdk:"type"`
	Format         types.String `tfsdk:"format"`
	URL            types.String `tfsdk:"url"`
	Delimiter      types.String `tfsdk:"delimiter"`
	Compression    types.String `tfsdk:"compression"`
	IsContinuous   types.Bool   `tfsdk:"is_continuous"`
	QueueURL       types.String `tfsdk:"queue_url"`
	Authentication types.String `tfsdk:"authentication"`
	AccessKey      types.Object `tfsdk:"access_key"`
	IAMRole        types.String `tfsdk:"iam_role"`

	// Azure Blob Storage specific fields
	ConnectionString   types.String `tfsdk:"connection_string"`
	Path               types.String `tfsdk:"path"`
	AzureContainerName types.String `tfsdk:"azure_container_name"`
}

func (m ClickPipeObjectStorageSourceModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"type":                 types.StringType,
			"format":               types.StringType,
			"url":                  types.StringType,
			"delimiter":            types.StringType,
			"compression":          types.StringType,
			"is_continuous":        types.BoolType,
			"queue_url":            types.StringType,
			"authentication":       types.StringType,
			"access_key":           ClickPipeSourceAccessKeyModel{}.ObjectType(),
			"iam_role":             types.StringType,
			"connection_string":    types.StringType,
			"path":                 types.StringType,
			"azure_container_name": types.StringType,
		},
	}
}

func (m ClickPipeObjectStorageSourceModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"type":                 m.Type,
		"format":               m.Format,
		"url":                  m.URL,
		"delimiter":            m.Delimiter,
		"compression":          m.Compression,
		"is_continuous":        m.IsContinuous,
		"queue_url":            m.QueueURL,
		"authentication":       m.Authentication,
		"access_key":           m.AccessKey,
		"iam_role":             m.IAMRole,
		"connection_string":    m.ConnectionString,
		"path":                 m.Path,
		"azure_container_name": m.AzureContainerName,
	})
}

type ClickPipeServiceAccountModel struct {
	ServiceAccountFile types.String `tfsdk:"service_account_file"`
}

func (m ClickPipeServiceAccountModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"service_account_file": types.StringType,
		},
	}
}

func (m ClickPipeServiceAccountModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"service_account_file": m.ServiceAccountFile,
	})
}

type ClickPipeBigQuerySettingsModel struct {
	ReplicationMode                types.String `tfsdk:"replication_mode"`
	AllowNullableColumns           types.Bool   `tfsdk:"allow_nullable_columns"`
	InitialLoadParallelism         types.Int64  `tfsdk:"initial_load_parallelism"`
	SnapshotNumRowsPerPartition    types.Int64  `tfsdk:"snapshot_num_rows_per_partition"`
	SnapshotNumberOfParallelTables types.Int64  `tfsdk:"snapshot_number_of_parallel_tables"`
}

func (m ClickPipeBigQuerySettingsModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"replication_mode":                   types.StringType,
			"allow_nullable_columns":             types.BoolType,
			"initial_load_parallelism":           types.Int64Type,
			"snapshot_num_rows_per_partition":    types.Int64Type,
			"snapshot_number_of_parallel_tables": types.Int64Type,
		},
	}
}

func (m ClickPipeBigQuerySettingsModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"replication_mode":                   m.ReplicationMode,
		"allow_nullable_columns":             m.AllowNullableColumns,
		"initial_load_parallelism":           m.InitialLoadParallelism,
		"snapshot_num_rows_per_partition":    m.SnapshotNumRowsPerPartition,
		"snapshot_number_of_parallel_tables": m.SnapshotNumberOfParallelTables,
	})
}

type ClickPipeBigQueryTableMappingModel struct {
	SourceDatasetName   types.String `tfsdk:"source_dataset_name"`
	SourceTable         types.String `tfsdk:"source_table"`
	TargetTable         types.String `tfsdk:"target_table"`
	ExcludedColumns     types.List   `tfsdk:"excluded_columns"`
	UseCustomSortingKey types.Bool   `tfsdk:"use_custom_sorting_key"`
	SortingKeys         types.List   `tfsdk:"sorting_keys"`
	TableEngine         types.String `tfsdk:"table_engine"`
}

func (m ClickPipeBigQueryTableMappingModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"source_dataset_name":    types.StringType,
			"source_table":           types.StringType,
			"target_table":           types.StringType,
			"excluded_columns":       types.ListType{ElemType: types.StringType},
			"use_custom_sorting_key": types.BoolType,
			"sorting_keys":           types.ListType{ElemType: types.StringType},
			"table_engine":           types.StringType,
		},
	}
}

func (m ClickPipeBigQueryTableMappingModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"source_dataset_name":    m.SourceDatasetName,
		"source_table":           m.SourceTable,
		"target_table":           m.TargetTable,
		"excluded_columns":       m.ExcludedColumns,
		"use_custom_sorting_key": m.UseCustomSortingKey,
		"sorting_keys":           m.SortingKeys,
		"table_engine":           m.TableEngine,
	})
}

type ClickPipeBigQuerySourceModel struct {
	SnapshotStagingPath types.String `tfsdk:"snapshot_staging_path"`
	Settings            types.Object `tfsdk:"settings"`
	TableMappings       types.List   `tfsdk:"table_mappings"`
	Credentials         types.Object `tfsdk:"credentials"`
}

func (m ClickPipeBigQuerySourceModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"snapshot_staging_path": types.StringType,
			"settings":              ClickPipeBigQuerySettingsModel{}.ObjectType(),
			"table_mappings":        types.ListType{ElemType: ClickPipeBigQueryTableMappingModel{}.ObjectType()},
			"credentials":           ClickPipeServiceAccountModel{}.ObjectType(),
		},
	}
}

func (m ClickPipeBigQuerySourceModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"snapshot_staging_path": m.SnapshotStagingPath,
		"settings":              m.Settings,
		"table_mappings":        m.TableMappings,
		"credentials":           m.Credentials,
	})
}

type ClickPipeSourceModel struct {
	Kafka         types.Object `tfsdk:"kafka"`
	ObjectStorage types.Object `tfsdk:"object_storage"`
	Kinesis       types.Object `tfsdk:"kinesis"`
	Postgres      types.Object `tfsdk:"postgres"`
	BigQuery      types.Object `tfsdk:"bigquery"`
}

func (m ClickPipeSourceModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"kafka":          ClickPipeKafkaSourceModel{}.ObjectType(),
			"object_storage": ClickPipeObjectStorageSourceModel{}.ObjectType(),
			"kinesis":        ClickPipeKinesisSourceModel{}.ObjectType(),
			"postgres":       ClickPipePostgresSourceModel{}.ObjectType(),
			"bigquery":       ClickPipeBigQuerySourceModel{}.ObjectType(),
		},
	}
}

func (m ClickPipeSourceModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"kafka":          m.Kafka,
		"object_storage": m.ObjectStorage,
		"kinesis":        m.Kinesis,
		"postgres":       m.Postgres,
		"bigquery":       m.BigQuery,
	})
}

type ClickPipeDestinationColumnModel struct {
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}

func (m ClickPipeDestinationColumnModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name": types.StringType,
			"type": types.StringType,
		},
	}
}

func (m ClickPipeDestinationColumnModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"name": m.Name,
		"type": m.Type,
	})
}

type ClickPipeDestinationTableEngineModel struct {
	Type            types.String `tfsdk:"type"`
	VersionColumnID types.String `tfsdk:"version_column_id"`
	ColumnIDs       types.List   `tfsdk:"column_ids"`
}

func (m ClickPipeDestinationTableEngineModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"type":              types.StringType,
			"version_column_id": types.StringType,
			"column_ids":        types.ListType{ElemType: types.StringType},
		},
	}
}

func (m ClickPipeDestinationTableEngineModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"type":              m.Type,
		"version_column_id": m.VersionColumnID,
		"column_ids":        m.ColumnIDs,
	})
}

type ClickPipeDestinationTableDefinitionModel struct {
	Engine      types.Object `tfsdk:"engine"`
	SortingKey  types.List   `tfsdk:"sorting_key"`
	PartitionBy types.String `tfsdk:"partition_by"`
	PrimaryKey  types.String `tfsdk:"primary_key"`
}

func (m ClickPipeDestinationTableDefinitionModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"engine":       ClickPipeDestinationTableEngineModel{}.ObjectType(),
			"sorting_key":  types.ListType{ElemType: types.StringType},
			"partition_by": types.StringType,
			"primary_key":  types.StringType,
		},
	}
}

func (m ClickPipeDestinationTableDefinitionModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"engine":       m.Engine,
		"sorting_key":  m.SortingKey,
		"partition_by": m.PartitionBy,
		"primary_key":  m.PrimaryKey,
	})
}

type ClickPipeDestinationModel struct {
	Database        types.String `tfsdk:"database"`
	Table           types.String `tfsdk:"table"`
	ManagedTable    types.Bool   `tfsdk:"managed_table"`
	TableDefinition types.Object `tfsdk:"table_definition"`
	Columns         types.List   `tfsdk:"columns"`
	Roles           types.List   `tfsdk:"roles"`
}

func (m ClickPipeDestinationModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"database":         types.StringType,
			"table":            types.StringType,
			"managed_table":    types.BoolType,
			"table_definition": ClickPipeDestinationTableDefinitionModel{}.ObjectType(),
			"columns":          types.ListType{ElemType: ClickPipeDestinationColumnModel{}.ObjectType()},
			"roles":            types.ListType{ElemType: types.StringType},
		},
	}
}

func (m ClickPipeDestinationModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"database":         m.Database,
		"table":            m.Table,
		"managed_table":    m.ManagedTable,
		"table_definition": m.TableDefinition,
		"columns":          m.Columns,
		"roles":            m.Roles,
	})
}

type ClickPipeFieldMappingModel struct {
	SourceField      types.String `tfsdk:"source_field"`
	DestinationField types.String `tfsdk:"destination_field"`
}

func (m ClickPipeFieldMappingModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"source_field":      types.StringType,
			"destination_field": types.StringType,
		},
	}
}

func (m ClickPipeFieldMappingModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"source_field":      m.SourceField,
		"destination_field": m.DestinationField,
	})
}

type ClickPipeResourceModel struct {
	ID            types.String  `tfsdk:"id"`
	ServiceID     types.String  `tfsdk:"service_id"`
	Name          types.String  `tfsdk:"name"`
	Scaling       types.Object  `tfsdk:"scaling"`
	State         types.String  `tfsdk:"state"`
	Stopped       types.Bool    `tfsdk:"stopped"`
	Source        types.Object  `tfsdk:"source"`
	Destination   types.Object  `tfsdk:"destination"`
	FieldMappings types.List    `tfsdk:"field_mappings"`
	Settings      types.Dynamic `tfsdk:"settings"`
	TriggerResync types.Bool    `tfsdk:"trigger_resync"`
}

type ClickPipeCdcInfrastructureModel struct {
	ServiceID            types.String  `tfsdk:"service_id"`
	ReplicaCpuMillicores types.Int64   `tfsdk:"replica_cpu_millicores"`
	ReplicaMemoryGb      types.Float64 `tfsdk:"replica_memory_gb"`
}
