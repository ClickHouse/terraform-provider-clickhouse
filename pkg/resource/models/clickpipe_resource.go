package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ClickPipeScalingModel struct {
	Replicas types.Int64 `tfsdk:"replicas"`
}

func (m ClickPipeScalingModel) ObjectValue() types.Object {
	return types.ObjectValueMust(m.ObjectType().AttrTypes, map[string]attr.Value{
		"replicas": m.Replicas,
	})
}

func (m ClickPipeScalingModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"replicas": types.Int64Type,
		},
	}
}

type ClickPipeSourceCredentialsModel struct {
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

type ClickPipeKafkaSourceModel struct {
	Type           types.String `tfsdk:"type"`
	Brokers        types.String `tfsdk:"brokers"`
	Topics         types.String `tfsdk:"topics"`
	ConsumerGroup  types.String `tfsdk:"consumer_group"`
	Authentication types.String `tfsdk:"authentication"`
	Credentials    types.Object `tfsdk:"credentials"`
}

type ClickPipesSourceSchemaModel struct {
	Format types.String `tfsdk:"format"`
}

type ClickPipeSourceModel struct {
	Kafka  types.Object `tfsdk:"kafka"`
	Schema types.Object `tfsdk:"schema"`
}

type ClickPipeDestinationColumnModel struct {
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}

type ClickPipeDestinationModel struct {
	Database     types.String `tfsdk:"database"`
	Table        types.String `tfsdk:"table"`
	ManagedTable types.Bool   `tfsdk:"managed_table"`
	Columns      types.List   `tfsdk:"columns"`
}

type ClickPipeFieldMappingModel struct {
	SourceField      types.String `tfsdk:"source_field"`
	DestinationField types.String `tfsdk:"destination_field"`
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
