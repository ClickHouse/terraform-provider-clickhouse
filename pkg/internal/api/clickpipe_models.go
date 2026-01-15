package api

import (
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

type ClickPipeScaling struct {
	Replicas             *int64      `json:"replicas,omitempty"`
	ReplicaCpuMillicores interface{} `json:"replicaCpuMillicores,omitempty"`
	ReplicaMemoryGb      interface{} `json:"replicaMemoryGb,omitempty"`
}

// This accounts for both the string from kubernetes and the int input
func (s *ClickPipeScaling) GetCpuMillicores() *int64 {
	if s.ReplicaCpuMillicores == nil {
		return nil
	}
	switch v := s.ReplicaCpuMillicores.(type) {
	case string:
		// Parse using Kubernetes resource.Quantity
		quantity, err := resource.ParseQuantity(v)
		if err != nil {
			return nil
		}
		val := quantity.MilliValue()
		return &val
	case float64:
		val := int64(v)
		return &val
	default:
	}
	return nil
}

// This accounts for both the string from kubernetes and the float input
func (s *ClickPipeScaling) GetMemoryGb() *float64 {
	if s.ReplicaMemoryGb == nil {
		return nil
	}
	switch v := s.ReplicaMemoryGb.(type) {
	case string:
		// Parse using Kubernetes resource.Quantity
		quantity, err := resource.ParseQuantity(v)
		if err != nil {
			return nil
		}
		// Convert to bytes, then to GB (1 GiB = 1073741824 bytes)
		bytes := quantity.Value()
		gb := float64(bytes) / 1073741824.0 // Convert bytes to GiB
		return &gb
	case float64:
		return &v
	default:
	}
	return nil
}

type ClickPipeSourceCredentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type ClickPipeSourceAccessKey struct {
	AccessKeyID string `json:"accessKeyId,omitempty"`
	SecretKey   string `json:"secretKey,omitempty"`
}

type ClickPipeKafkaSourceCredentials struct {
	*ClickPipeSourceCredentials
	*ClickPipeSourceAccessKey

	ConnectionString *string `json:"connectionString,omitempty"`
}

type ClickPipeKafkaOffset struct {
	Strategy  string  `json:"strategy"`
	Timestamp *string `json:"timestamp,omitempty"`
}

type ClickPipeKafkaSchemaRegistry struct {
	URL            string                      `json:"url"`
	Authentication string                      `json:"authentication"`
	Credentials    *ClickPipeSourceCredentials `json:"credentials,omitempty"`
}

type ClickPipeKafkaSource struct {
	Type   string `json:"type,omitempty"`
	Format string `json:"format,omitempty"`

	Brokers string `json:"brokers,omitempty"`
	Topics  string `json:"topics,omitempty"`

	ConsumerGroup *string `json:"consumerGroup,omitempty"`

	Offset *ClickPipeKafkaOffset `json:"offset,omitempty"`

	SchemaRegistry *ClickPipeKafkaSchemaRegistry `json:"schemaRegistry,omitempty"`

	Authentication string                           `json:"authentication,omitempty"`
	Credentials    *ClickPipeKafkaSourceCredentials `json:"credentials,omitempty"`
	IAMRole        *string                          `json:"iamRole,omitempty"`
	CACertificate  *string                          `json:"caCertificate,omitempty"`

	ReversePrivateEndpointIDs []string `json:"reversePrivateEndpointIds,omitempty"`
}

type ClickPipeObjectStorageSource struct {
	Type   string `json:"type"`
	Format string `json:"format"`

	URL         string  `json:"url,omitempty"`
	Delimiter   *string `json:"delimiter,omitempty"`
	Compression *string `json:"compression,omitempty"`

	IsContinuous bool    `json:"isContinuous"`
	QueueURL     *string `json:"queueUrl,omitempty"`

	Authentication *string                   `json:"authentication,omitempty"`
	AccessKey      *ClickPipeSourceAccessKey `json:"accessKey,omitempty"`
	IAMRole        *string                   `json:"iamRole,omitempty"`

	// Azure Blob Storage specific fields
	ConnectionString   *string `json:"connectionString,omitempty"`
	Path               *string `json:"path,omitempty"`
	AzureContainerName *string `json:"azureContainerName,omitempty"`
}

type ClickPipeKinesisSource struct {
	Format string `json:"format"`

	StreamName string `json:"streamName"`
	Region     string `json:"region"`

	IteratorType string  `json:"iteratorType"`
	Timestamp    *string `json:"timestamp,omitempty"`

	UseEnhancedFanOut bool `json:"useEnhancedFanOut,omitempty"`

	Authentication string                    `json:"authentication"`
	AccessKey      *ClickPipeSourceAccessKey `json:"accessKey,omitempty"`
	IAMRole        *string                   `json:"iamRole,omitempty"`
}

type ClickPipePostgresSource struct {
	Host                  string                          `json:"host,omitempty"`
	Port                  int                             `json:"port,omitempty"`
	Database              string                          `json:"database,omitempty"`
	Authentication        *string                         `json:"authentication,omitempty"`
	IAMRole               *string                         `json:"iamRole,omitempty"`
	TLSHost               *string                         `json:"tlsHost,omitempty"`
	CACertificate         *string                         `json:"caCertificate,omitempty"`
	Credentials           *ClickPipeSourceCredentials     `json:"credentials,omitempty"`
	Settings              *ClickPipePostgresSettings      `json:"settings,omitempty"`
	Mappings              []ClickPipePostgresTableMapping `json:"tableMappings,omitempty"`
	TableMappingsToRemove []ClickPipePostgresTableMapping `json:"tableMappingsToRemove,omitempty"`
	TableMappingsToAdd    []ClickPipePostgresTableMapping `json:"tableMappingsToAdd,omitempty"`
}

type ClickPipePostgresSettings struct {
	SyncIntervalSeconds            *int    `json:"syncIntervalSeconds,omitempty"`
	PullBatchSize                  *int    `json:"pullBatchSize,omitempty"`
	PublicationName                *string `json:"publicationName,omitempty"`
	ReplicationMode                string  `json:"replicationMode,omitempty"`
	ReplicationSlotName            *string `json:"replicationSlotName,omitempty"`
	AllowNullableColumns           *bool   `json:"allowNullableColumns,omitempty"`
	InitialLoadParallelism         *int    `json:"initialLoadParallelism,omitempty"`
	SnapshotNumRowsPerPartition    *int    `json:"snapshotNumRowsPerPartition,omitempty"`
	SnapshotNumberOfParallelTables *int    `json:"snapshotNumberOfParallelTables,omitempty"`
	EnableFailoverSlots            *bool   `json:"enableFailoverSlots,omitempty"`
	DeleteOnMerge                  *bool   `json:"deleteOnMerge,omitempty"`
}

type ClickPipePostgresTableMapping struct {
	SourceSchemaName    string   `json:"sourceSchemaName"`
	SourceTable         string   `json:"sourceTable"`
	TargetTable         string   `json:"targetTable"`
	ExcludedColumns     []string `json:"excludedColumns,omitempty"`
	UseCustomSortingKey *bool    `json:"useCustomSortingKey,omitempty"`
	SortingKeys         []string `json:"sortingKeys,omitempty"`
	TableEngine         *string  `json:"tableEngine,omitempty"`
	PartitionKey        *string  `json:"partitionKey,omitempty"`
}

type ClickPipeServiceAccount struct {
	ServiceAccountFile string `json:"serviceAccountFile,omitempty"`
}

type ClickPipeBigQuerySettings struct {
	ReplicationMode                string `json:"replicationMode,omitempty"`
	AllowNullableColumns           *bool  `json:"allowNullableColumns,omitempty"`
	InitialLoadParallelism         *int   `json:"initialLoadParallelism,omitempty"`
	SnapshotNumRowsPerPartition    *int   `json:"snapshotNumRowsPerPartition,omitempty"`
	SnapshotNumberOfParallelTables *int   `json:"snapshotNumberOfParallelTables,omitempty"`
}

type ClickPipeBigQueryTableMapping struct {
	SourceDatasetName   string   `json:"sourceDatasetName"`
	SourceTable         string   `json:"sourceTable"`
	TargetTable         string   `json:"targetTable"`
	ExcludedColumns     []string `json:"excludedColumns,omitempty"`
	UseCustomSortingKey *bool    `json:"useCustomSortingKey,omitempty"`
	SortingKeys         []string `json:"sortingKeys,omitempty"`
	TableEngine         *string  `json:"tableEngine,omitempty"`
}

type ClickPipeBigQuerySource struct {
	SnapshotStagingPath   string                          `json:"snapshotStagingPath,omitempty"`
	Settings              ClickPipeBigQuerySettings       `json:"settings"`
	Mappings              []ClickPipeBigQueryTableMapping `json:"tableMappings"`
	TableMappingsToRemove []ClickPipeBigQueryTableMapping `json:"tableMappingsToRemove,omitempty"`
	TableMappingsToAdd    []ClickPipeBigQueryTableMapping `json:"tableMappingsToAdd,omitempty"`
	Credentials           *ClickPipeServiceAccount        `json:"credentials,omitempty"`
}

type ClickPipeSource struct {
	Kafka           *ClickPipeKafkaSource         `json:"kafka,omitempty"`
	ObjectStorage   *ClickPipeObjectStorageSource `json:"objectStorage,omitempty"`
	Kinesis         *ClickPipeKinesisSource       `json:"kinesis,omitempty"`
	Postgres        *ClickPipePostgresSource      `json:"postgres,omitempty"`
	BigQuery        *ClickPipeBigQuerySource      `json:"bigquery,omitempty"`
	ValidateSamples bool                          `json:"validateSamples,omitempty"`
}

type ClickPipeDestinationColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ClickPipeDestinationTableEngine struct {
	Type            string   `json:"type"`
	VersionColumnID *string  `json:"versionColumnId,omitempty"`
	ColumnIDs       []string `json:"columnIds,omitempty"`
}

type ClickPipeDestinationTableDefinition struct {
	Engine      ClickPipeDestinationTableEngine `json:"engine"`
	SortingKey  []string                        `json:"sortingKey"`
	PartitionBy *string                         `json:"partitionBy,omitempty"`
	PrimaryKey  *string                         `json:"primaryKey,omitempty"`
}

type ClickPipeDestination struct {
	Database        string                               `json:"database"`
	Table           *string                              `json:"table,omitempty"`
	ManagedTable    *bool                                `json:"managedTable,omitempty"`
	TableDefinition *ClickPipeDestinationTableDefinition `json:"tableDefinition,omitempty"`
	Columns         []ClickPipeDestinationColumn         `json:"columns,omitempty"`
	Roles           []string                             `json:"roles,omitempty"`
}

type ClickPipeDestinationUpdate struct {
	Columns []ClickPipeDestinationColumn `json:"columns"`
}

type ClickPipeFieldMapping struct {
	SourceField      string `json:"sourceField"`
	DestinationField string `json:"destinationField"`
}

type ClickPipe struct {
	ID            string                  `json:"id,omitempty"`
	Name          string                  `json:"name"`
	Scaling       *ClickPipeScaling       `json:"scaling,omitempty"`
	State         string                  `json:"state,omitempty"`
	Source        ClickPipeSource         `json:"source"`
	Destination   ClickPipeDestination    `json:"destination"`
	FieldMappings []ClickPipeFieldMapping `json:"fieldMappings,omitempty"`
	Settings      map[string]any          `json:"settings,omitempty"`
	CreatedAt     *time.Time              `json:"createdAt,omitempty"`
	UpdatedAt     *time.Time              `json:"updatedAt,omitempty"`
}

type ClickPipeUpdate struct {
	Name          *string                     `json:"name,omitempty"`
	Source        *ClickPipeSource            `json:"source,omitempty"`
	Destination   *ClickPipeDestinationUpdate `json:"destination,omitempty"`
	FieldMappings []ClickPipeFieldMapping     `json:"fieldMappings,omitempty"`
	Settings      map[string]any              `json:"settings,omitempty"`
}
