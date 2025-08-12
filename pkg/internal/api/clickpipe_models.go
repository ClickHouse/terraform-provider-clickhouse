package api

import (
	"strconv"
	"strings"
	"time"
)

type ClickPipeScaling struct {
	Replicas             *int64      `json:"replicas,omitempty"`
	ReplicaCpuMillicores interface{} `json:"replicaCpuMillicores,omitempty"`
	ReplicaMemoryGb      interface{} `json:"replicaMemoryGb,omitempty"`
}

// This accounts for both the string from from kubernetes and the int input
func (s *ClickPipeScaling) GetCpuMillicores() *int64 {
	if s.ReplicaCpuMillicores == nil {
		return nil
	}
	switch v := s.ReplicaCpuMillicores.(type) {
	case string:
		// Handle string with 'm' suffix (e.g., "125m")
		str := strings.TrimSuffix(v, "m")
		val, _ := strconv.ParseInt(str, 10, 64)
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
		// Handle string with Kubernetes memory units
		var val float64

		if strings.HasSuffix(v, "Gi") {
			// Gibibytes - already in GB
			str := strings.TrimSuffix(v, "Gi")
			val, _ = strconv.ParseFloat(str, 64)
		} else if strings.HasSuffix(v, "Mi") {
			// Mebibytes - convert to GB (1024 Mi = 1 Gi)
			str := strings.TrimSuffix(v, "Mi")
			mebibytes, parseErr := strconv.ParseFloat(str, 64)
			if parseErr != nil {
				_ = parseErr
			} else {
				val = mebibytes / 1024.0 // Convert MiB to GiB
			}
		} else if strings.HasSuffix(v, "M") {
			// Megabytes - convert to GB (1000 M = 1 G)
			str := strings.TrimSuffix(v, "M")
			megabytes, parseErr := strconv.ParseFloat(str, 64)
			if parseErr != nil {
				_ = parseErr
			} else {
				val = megabytes / 1000.0 // Convert MB to GB
			}
		} else {
			// Plain number - assume GB
			val, _ = strconv.ParseFloat(v, 64)
		}

		return &val
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

	IsContinuous bool `json:"isContinuous"`

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

type ClickPipeSource struct {
	Kafka           *ClickPipeKafkaSource         `json:"kafka,omitempty"`
	ObjectStorage   *ClickPipeObjectStorageSource `json:"objectStorage,omitempty"`
	Kinesis         *ClickPipeKinesisSource       `json:"kinesis,omitempty"`
	ValidateSamples bool                          `json:"validateSamples,omitempty"`
}

type ClickPipeDestinationColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ClickPipeDestinationTableEngine struct {
	Type string `json:"type"`
}

type ClickPipeDestinationTableDefinition struct {
	Engine      ClickPipeDestinationTableEngine `json:"engine"`
	SortingKey  []string                        `json:"sortingKey"`
	PartitionBy *string                         `json:"partitionBy,omitempty"`
	PrimaryKey  *string                         `json:"primaryKey,omitempty"`
}

type ClickPipeDestination struct {
	Database        string                               `json:"database"`
	Table           string                               `json:"table"`
	ManagedTable    bool                                 `json:"managedTable"`
	TableDefinition *ClickPipeDestinationTableDefinition `json:"tableDefinition,omitempty"`
	Columns         []ClickPipeDestinationColumn         `json:"columns"`
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
	FieldMappings []ClickPipeFieldMapping `json:"fieldMappings"`
	CreatedAt     *time.Time              `json:"createdAt,omitempty"`
	UpdatedAt     *time.Time              `json:"updatedAt,omitempty"`
}

type ClickPipeUpdate struct {
	Name          *string                     `json:"name,omitempty"`
	Source        *ClickPipeSource            `json:"source,omitempty"`
	Destination   *ClickPipeDestinationUpdate `json:"destination,omitempty"`
	FieldMappings []ClickPipeFieldMapping     `json:"fieldMappings,omitempty"`
}
