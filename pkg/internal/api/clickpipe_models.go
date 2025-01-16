package api

import "time"

type ClickPipeScaling struct {
	Replicas    *int64 `json:"replicas,omitempty"`
	Concurrency *int64 `json:"-"` // Concurrency is not available in API yet
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
	Timestamp *string `json:"timestamp"`
}

type ClickPipeKafkaSchemaRegistry struct {
	URL            string                      `json:"url"`
	Authentication string                      `json:"authentication"`
	Credentials    *ClickPipeSourceCredentials `json:"credentials,omitempty"`
}

type ClickPipeKafkaSource struct {
	Type   string `json:"type"`
	Format string `json:"format"`

	Brokers string `json:"brokers"`
	Topics  string `json:"topics"`

	ConsumerGroup *string `json:"consumerGroup,omitempty"`

	Offset *ClickPipeKafkaOffset `json:"offset,omitempty"`

	SchemaRegistry *ClickPipeKafkaSchemaRegistry `json:"schemaRegistry,omitempty"`

	Authentication string                           `json:"authentication"`
	Credentials    *ClickPipeKafkaSourceCredentials `json:"credentials,omitempty"`
	IAMRole        *string                          `json:"iamRole,omitempty"`
	CACertificate  *string                          `json:"caCertificate,omitempty"`
}

type ClickPipeObjectStorageSource struct {
	Type   string `json:"type"`
	Format string `json:"format"`

	URL         string  `json:"url"`
	Delimiter   *string `json:"delimiter,omitempty"`
	Compression *string `json:"compression,omitempty"`

	IsContinuous bool `json:"isContinuous"`

	Authentication string                    `json:"authentication"`
	AccessKey      *ClickPipeSourceAccessKey `json:"accessKey,omitempty"`
	IAMRole        *string                   `json:"iamRole,omitempty"`
}

type ClickPipeSource struct {
	Kafka           *ClickPipeKafkaSource         `json:"kafka,omitempty"`
	ObjectStorage   *ClickPipeObjectStorageSource `json:"objectStorage,omitempty"`
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
}

type ClickPipeFieldMapping struct {
	SourceField      string `json:"sourceField"`
	DestinationField string `json:"destinationField"`
}

type ClickPipe struct {
	ID            string                  `json:"id,omitempty"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Scaling       *ClickPipeScaling       `json:"scaling,omitempty"`
	State         string                  `json:"state,omitempty"`
	Source        ClickPipeSource         `json:"source"`
	Destination   ClickPipeDestination    `json:"destination"`
	FieldMappings []ClickPipeFieldMapping `json:"fieldMappings"`
	CreatedAt     *time.Time              `json:"createdAt,omitempty"`
	UpdatedAt     *time.Time              `json:"updatedAt,omitempty"`
}
