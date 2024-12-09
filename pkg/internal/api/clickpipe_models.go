package api

import "time"

type ClickPipeScaling struct {
	Replicas int64 `json:"replicas"`
}

type ClickPipeSourceCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ClickPipeKafkaSource struct {
	Type           string                     `json:"type"`
	Brokers        string                     `json:"brokers"`
	Topics         string                     `json:"topics"`
	ConsumerGroup  string                     `json:"consumerGroup"`
	Authentication string                     `json:"authentication"`
	Credentials    ClickPipeSourceCredentials `json:"credentials"`
}

type ClickPipeSourceSchema struct {
	Format string `json:"format"`
}

type ClickPipeSource struct {
	Kafka  *ClickPipeKafkaSource `json:"kafka,omitempty"`
	Schema ClickPipeSourceSchema `json:"schema"`
}

type ClickPipeDestinationColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ClickPipeDestination struct {
	Database     string                       `json:"database"`
	Table        string                       `json:"table"`
	ManagedTable bool                         `json:"managedTable"`
	Columns      []ClickPipeDestinationColumn `json:"columns"`
}

type ClickPipeFieldMapping struct {
	SourceField      string `json:"sourceField"`
	DestinationField string `json:"destinationField"`
}

type ClickPipe struct {
	ID            string                  `json:"id,omitempty"`
	ServiceID     string                  `json:"serviceId"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Scaling       *ClickPipeScaling       `json:"replicas,omitempty"`
	State         string                  `json:"state,omitempty"`
	Source        ClickPipeSource         `json:"source"`
	Destination   ClickPipeDestination    `json:"destination"`
	FieldMappings []ClickPipeFieldMapping `json:"fieldMappings"`
	CreatedAt     time.Time               `json:"createdAt,omitempty"`
	UpdatedAt     time.Time               `json:"updatedAt,omitempty"`
}
