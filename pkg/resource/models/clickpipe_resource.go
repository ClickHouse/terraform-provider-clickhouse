//go:build alpha

package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ClickPipeScalingModel struct {
	Replicas types.Int64 `tfsdk:"replicas"`
}

func (m ClickPipeScalingModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"replicas": types.Int64Type,
		},
	}
}

func (m ClickPipeScalingModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"replicas": m.Replicas,
	})
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

type ClickPipeObjectStorageSourceModel struct {
	Type           types.String `tfsdk:"type"`
	Format         types.String `tfsdk:"format"`
	URL            types.String `tfsdk:"url"`
	Delimiter      types.String `tfsdk:"delimiter"`
	Compression    types.String `tfsdk:"compression"`
	IsContinuous   types.Bool   `tfsdk:"is_continuous"`
	Authentication types.String `tfsdk:"authentication"`
	AccessKey      types.Object `tfsdk:"access_key"`
	IAMRole        types.String `tfsdk:"iam_role"`
}

func (m ClickPipeObjectStorageSourceModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"type":           types.StringType,
			"format":         types.StringType,
			"url":            types.StringType,
			"delimiter":      types.StringType,
			"compression":    types.StringType,
			"is_continuous":  types.BoolType,
			"authentication": types.StringType,
			"access_key":     ClickPipeSourceAccessKeyModel{}.ObjectType(),
			"iam_role":       types.StringType,
		},
	}
}

func (m ClickPipeObjectStorageSourceModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"type":           m.Type,
		"format":         m.Format,
		"url":            m.URL,
		"delimiter":      m.Delimiter,
		"compression":    m.Compression,
		"is_continuous":  m.IsContinuous,
		"authentication": m.Authentication,
		"access_key":     m.AccessKey,
		"iam_role":       m.IAMRole,
	})
}

type ClickPipeSourceModel struct {
	Kafka         types.Object `tfsdk:"kafka"`
	ObjectStorage types.Object `tfsdk:"object_storage"`
}

func (m ClickPipeSourceModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"kafka":          ClickPipeKafkaSourceModel{}.ObjectType(),
			"object_storage": ClickPipeObjectStorageSourceModel{}.ObjectType(),
		},
	}
}

func (m ClickPipeSourceModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"kafka":          m.Kafka,
		"object_storage": m.ObjectStorage,
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
	Type types.String `tfsdk:"type"`
}

func (m ClickPipeDestinationTableEngineModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"type": types.StringType,
		},
	}
}

func (m ClickPipeDestinationTableEngineModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"type": m.Type,
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
	ID            types.String `tfsdk:"id"`
	ServiceID     types.String `tfsdk:"service_id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	Scaling       types.Object `tfsdk:"scaling"`
	State         types.String `tfsdk:"state"`
	Source        types.Object `tfsdk:"source"`
	Destination   types.Object `tfsdk:"destination"`
	FieldMappings types.List   `tfsdk:"field_mappings"`
}
