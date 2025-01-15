package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource               = &ClickPipeResource{}
	_ resource.ResourceWithModifyPlan = &ClickPipeResource{}
	_ resource.ResourceWithConfigure  = &ClickPipeResource{}
)

type ClickPipeResource struct {
	client api.Client
}

func NewClickPipeResource() resource.Resource {
	return &ClickPipeResource{}
}

func (c *ClickPipeResource) Configure(_ context.Context, request resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	c.client = request.ProviderData.(api.Client)
}

func (c *ClickPipeResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_clickpipe"
}

func (c *ClickPipeResource) Schema(_ context.Context, _ resource.SchemaRequest, response *resource.SchemaResponse) {
	wrapStringsWithBackticksAndJoinCommaSeparated := func(s []string) string {
		wrapped := make([]string, len(s))
		for i, v := range s {
			wrapped[i] = "`" + v + "`"
		}
		return strings.Join(wrapped, ", ")
	}

	response.Schema = schema.Schema{
		Description: "The ClickPipe resource allows you to create and manage ClickPipes data ingestion in ClickHouse Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of the ClickPipe. Generated by the ClickHouse Cloud.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": schema.StringAttribute{
				Description: "The ID of the service to which the ClickPipe belongs.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the ClickPipe.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "The description of the ClickPipe.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"scaling": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"replicas": schema.Int64Attribute{
						Description: "The number of desired replicas for the ClickPipe. Default is 1. The maximum value is 10.",
						Optional:    true,
						Computed:    true,
						Validators: []validator.Int64{
							int64validator.Between(1, 10),
						},
					},
				},
				Optional: true,
				Computed: true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: "The state of the ClickPipe. (`Running`, `Stopped`). Default is `Running`. Whenever the pipe state changes, the Terraform provider will try to ensure the actual state matches the planned value. If pipe is `Failed` and plan is `Running`, the provider will try to resume the pipe. If plan is `Stopped`, the provider will try to stop the pipe. If the pipe is `InternalError`, no action will be taken.",
				Optional:            true,
				Default:             stringdefault.StaticString(api.ClickPipeRunningState),
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(api.ClickPipeRunningState, api.ClickPipeStoppedState),
				},
			},
			"source": schema.SingleNestedAttribute{
				Description: "The data source for the ClickPipe.",
				Attributes: map[string]schema.Attribute{
					"kafka": schema.SingleNestedAttribute{
						Optional: true,
						Attributes: map[string]schema.Attribute{
							"type": schema.StringAttribute{
								MarkdownDescription: fmt.Sprintf(
									"The type of the Kafka source. (%s). Default is `%s`.",
									wrapStringsWithBackticksAndJoinCommaSeparated(api.ClickPipeKafkaSourceTypes),
									api.ClickPipeKafkaSourceType,
								),
								Computed: true,
								Optional: true,
								Default:  stringdefault.StaticString(api.ClickPipeKafkaSourceType),
								Validators: []validator.String{
									stringvalidator.OneOf(api.ClickPipeKafkaSourceTypes...),
								},
							},
							"format": schema.StringAttribute{
								MarkdownDescription: fmt.Sprintf(
									"The format of the Kafka source. (%s)",
									wrapStringsWithBackticksAndJoinCommaSeparated(api.ClickPipeKafkaFormats),
								),
								Required: true,
								Validators: []validator.String{
									stringvalidator.OneOf(api.ClickPipeKafkaFormats...),
								},
							},
							"brokers": schema.StringAttribute{
								Description: "The list of Kafka bootstrap brokers. (comma separated)",
								Required:    true,
							},
							"topics": schema.StringAttribute{
								Description: "The list of Kafka topics. (comma separated)",
								Required:    true,
							},
							"consumer_group": schema.StringAttribute{
								MarkdownDescription: "Consumer group of the Kafka source. If not provided `clickpipes-<ID>` will be used.",
								Computed:            true,
								Optional:            true,
							},
							"offset": schema.SingleNestedAttribute{
								MarkdownDescription: "The Kafka offset.",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"strategy": schema.StringAttribute{
										MarkdownDescription: fmt.Sprintf(
											"The offset strategy for the Kafka source. (%s)",
											wrapStringsWithBackticksAndJoinCommaSeparated(api.ClickPipeKafkaOffsetStrategies),
										),
										Required: true,
										Validators: []validator.String{
											stringvalidator.OneOf(api.ClickPipeKafkaOffsetStrategies...),
										},
									},
									"timestamp": schema.StringAttribute{
										MarkdownDescription: fmt.Sprintf(
											"The timestamp for the Kafka offset. Use with `%s` offset strategy.",
											api.ClickPipeKafkaOffsetFromTimestampStrategy,
										),
										Optional: true,
									},
								},
							},
							"schema_registry": schema.SingleNestedAttribute{
								MarkdownDescription: "The schema registry for the Kafka source.",
								Optional:            true,
								Attributes: map[string]schema.Attribute{
									"url": schema.StringAttribute{
										Description: "The URL of the schema registry.",
										Required:    true,
									},
									"authentication": schema.StringAttribute{
										Description: "The authentication method for the Schema Registry. Only supported is `PLAIN`.",
										Required:    true,
										Validators: []validator.String{
											stringvalidator.OneOf("PLAIN"),
										},
									},
									"credentials": schema.SingleNestedAttribute{
										MarkdownDescription: "The credentials for the Schema Registry.",
										Required:            true,
										Attributes: map[string]schema.Attribute{
											"username": schema.StringAttribute{
												Description: "The username for the Schema Registry.",
												Required:    true,
												Sensitive:   true,
											},
											"password": schema.StringAttribute{
												Description: "The password for the Schema Registry.",
												Required:    true,
												Sensitive:   true,
											},
										},
									},
								},
							},
							"authentication": schema.StringAttribute{
								MarkdownDescription: fmt.Sprintf(
									"The authentication method for the Kafka source. (%s). Default is `%s`.",
									wrapStringsWithBackticksAndJoinCommaSeparated(api.ClickPipeKafkaAuthenticationMethods),
									api.ClickPipeKafkaAuthenticationPlain,
								),
								Computed: true,
								Optional: true,
								Default:  stringdefault.StaticString("PLAIN"),
								Validators: []validator.String{
									stringvalidator.OneOf(api.ClickPipeKafkaAuthenticationMethods...),
								},
							},
							"credentials": schema.SingleNestedAttribute{
								MarkdownDescription: "The credentials for the Kafka source.",
								Attributes: map[string]schema.Attribute{
									"username": schema.StringAttribute{
										Description: "The username for the Kafka source.",
										Optional:    true,
										Sensitive:   true,
									},
									"password": schema.StringAttribute{
										Description: "The password for the Kafka source.",
										Optional:    true,
										Sensitive:   true,
									},
									"access_key_id": schema.StringAttribute{
										Description: "The access key ID for the Kafka source. Use with `IAM_USER` authentication.",
										Optional:    true,
										Sensitive:   true,
									},
									"secret_key": schema.StringAttribute{
										Description: "The secret key for the Kafka source. Use with `IAM_USER` authentication.",
										Optional:    true,
										Sensitive:   true,
									},
									"connection_string": schema.StringAttribute{
										Description: "The connection string for the Kafka source. Use with `azureeventhub` Kafka source type. Use with `PLAIN` authentication.",
										Optional:    true,
										Sensitive:   true,
									},
								},
								Required: true,
							},
							"iam_role": schema.StringAttribute{
								MarkdownDescription: "The IAM role for the Kafka source. Use with `IAM_ROLE` authentication. Read more in [ClickPipes documentation page](https://clickhouse.com/docs/en/integrations/clickpipes/kafka#iam)",
								Optional:            true,
							},
							"ca_certificate": schema.StringAttribute{
								MarkdownDescription: "PEM encoded CA certificates to validate the broker's certificate.",
								Optional:            true,
							},
						},
					},
				},
				Required: true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"destination": schema.SingleNestedAttribute{
				Description: "The destination for the ClickPipe.",
				Attributes: map[string]schema.Attribute{
					"database": schema.StringAttribute{
						MarkdownDescription: "The name of the ClickHouse database. Default is `default`.",
						Default:             stringdefault.StaticString("default"),
						Computed:            true,
						Optional:            true,
					},
					"table": schema.StringAttribute{
						Description: "The name of the ClickHouse table.",
						Required:    true,
					},
					"managed_table": schema.BoolAttribute{
						MarkdownDescription: "Whether the table is managed by ClickHouse Cloud. If `false`, the table must exist in the database. Default is `true`.",
						Default:             booldefault.StaticBool(true),
						Computed:            true,
						Optional:            true,
					},
					"table_definition": schema.SingleNestedAttribute{
						MarkdownDescription: "Definition of the destination table. Required for ClickPipes managed tables.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"engine": schema.SingleNestedAttribute{
								MarkdownDescription: "The engine of the ClickHouse table.",
								Required:            true,
								Attributes: map[string]schema.Attribute{
									"type": schema.StringAttribute{
										MarkdownDescription: "The type of the engine. Only `MergeTree` is supported.",
										Required:            true,
										Validators: []validator.String{
											stringvalidator.OneOf("MergeTree"),
										},
									},
								},
							},
							"sorting_key": schema.ListAttribute{
								MarkdownDescription: "The list of columns for the sorting key.",
								Optional:            true,
								ElementType:         types.StringType,
							},
							"partition_by": schema.StringAttribute{
								MarkdownDescription: "The column to partition the table by.",
								Optional:            true,
							},
							"primary_key": schema.StringAttribute{
								MarkdownDescription: "The primary key of the table.",
								Optional:            true,
							},
						},
					},
					"columns": schema.ListNestedAttribute{
						Description: "The list of columns for the ClickHouse table.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"name": schema.StringAttribute{
									Description: "The name of the column.",
									Required:    true,
								},
								"type": schema.StringAttribute{
									Description: "The type of the column.",
									Required:    true,
								},
							},
						},
						Required: true,
					},
					"roles": schema.ListAttribute{
						MarkdownDescription: "ClickPipe will create a ClickHouse user with these roles. Add your custom roles here if required.",
						ElementType:         types.StringType,
						Optional:            true,
					},
				},
				Required: true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
			},
			"field_mappings": schema.ListNestedAttribute{
				Description: "Field mapping between source and destination table.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"source_field": schema.StringAttribute{
							Description: "The name of the source field.",
							Required:    true,
						},
						"destination_field": schema.StringAttribute{
							Description: "The name of the column in destination table.",
							Required:    true,
						},
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (c *ClickPipeResource) ModifyPlan(ctx context.Context, request resource.ModifyPlanRequest, response *resource.ModifyPlanResponse) {
	if request.Plan.Raw.IsNull() {
		// If the entire plan is null, the resource is planned for destruction.
		return
	}

	var plan, state, config models.ClickPipeResourceModel
	diags := request.Plan.Get(ctx, &plan)
	response.Diagnostics.Append(diags...)
	if !request.State.Raw.IsNull() {
		diags = request.State.Get(ctx, &state)
		response.Diagnostics.Append(diags...)

		// ignore source.kafka.consumer_group if already exists in state, and is unknown in the plan
		if kafkaAttr, ok := state.Source.Attributes()["kafka"].(types.Object); ok && !kafkaAttr.IsNull() {
			plan.Source.Attributes()["kafka"].(types.Object).Attributes()["consumer_group"] = kafkaAttr.Attributes()["consumer_group"]
		}
	}
	if response.Diagnostics.HasError() {
		return
	}

	if !request.Config.Raw.IsNull() {
		diags = request.Config.Get(ctx, &config)
		response.Diagnostics.Append(diags...)
	}
	if response.Diagnostics.HasError() {
		return
	}

	if !request.State.Raw.IsNull() {
		if !plan.ServiceID.IsNull() && plan.ServiceID != state.ServiceID {
			response.Diagnostics.AddAttributeError(
				path.Root("service_id"),
				"Invalid Update",
				"ClickPipe cannot be moved between services. Please delete and recreate the ClickPipe.",
			)
		}
	}
}

func (c *ClickPipeResource) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var plan models.ClickPipeResourceModel
	diags := request.Plan.Get(ctx, &plan)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()

	clickPipe := api.ClickPipe{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}

	sourceModel := models.ClickPipeSourceModel{}
	response.Diagnostics.Append(plan.Source.As(ctx, &sourceModel, basetypes.ObjectAsOptions{})...)

	if !sourceModel.Kafka.IsNull() {
		kafkaModel := models.ClickPipeKafkaSourceModel{}
		credentialsModel := models.ClickPipeKafkaSourceCredentialsModel{}
		response.Diagnostics.Append(sourceModel.Kafka.As(ctx, &kafkaModel, basetypes.ObjectAsOptions{})...)
		response.Diagnostics.Append(kafkaModel.Credentials.As(ctx, &credentialsModel, basetypes.ObjectAsOptions{})...)

		var consumerGroup *string
		if !kafkaModel.ConsumerGroup.IsUnknown() {
			consumerGroup = kafkaModel.ConsumerGroup.ValueStringPointer()
		}

		clickPipe.Source.Kafka = &api.ClickPipeKafkaSource{
			Type:           kafkaModel.Type.ValueString(),
			Format:         kafkaModel.Format.ValueString(),
			Brokers:        kafkaModel.Brokers.ValueString(),
			Topics:         kafkaModel.Topics.ValueString(),
			ConsumerGroup:  consumerGroup,
			Authentication: kafkaModel.Authentication.ValueString(),
			Credentials: &api.ClickPipeKafkaSourceCredentials{
				ClickPipeSourceCredentials: &api.ClickPipeSourceCredentials{
					Username: credentialsModel.Username.ValueString(),
					Password: credentialsModel.Password.ValueString(),
				},
			},
		}

		if !kafkaModel.SchemaRegistry.IsNull() {
			schemaRegistryModel := models.ClickPipeKafkaSchemaRegistryModel{}
			response.Diagnostics.Append(kafkaModel.SchemaRegistry.As(ctx, &schemaRegistryModel, basetypes.ObjectAsOptions{})...)
			credentialsModel := models.ClickPipeSourceCredentialsModel{}
			response.Diagnostics.Append(schemaRegistryModel.Credentials.As(ctx, &credentialsModel, basetypes.ObjectAsOptions{})...)

			clickPipe.Source.Kafka.SchemaRegistry = &api.ClickPipeKafkaSchemaRegistry{
				URL: schemaRegistryModel.URL.ValueString(),
				Credentials: &api.ClickPipeSourceCredentials{
					Username: credentialsModel.Username.ValueString(),
					Password: credentialsModel.Password.ValueString(),
				},
			}
		}

		if !kafkaModel.Offset.IsNull() {
			offsetModel := models.ClickPipeKafkaOffsetModel{}
			response.Diagnostics.Append(kafkaModel.Offset.As(ctx, &offsetModel, basetypes.ObjectAsOptions{})...)

			var timestamp *string
			if !offsetModel.Timestamp.IsUnknown() {
				timestamp = offsetModel.Timestamp.ValueStringPointer()
			}

			clickPipe.Source.Kafka.Offset = &api.ClickPipeKafkaOffset{
				Strategy:  offsetModel.Strategy.ValueString(),
				Timestamp: timestamp,
			}
		}
	}

	destinationModel := models.ClickPipeDestinationModel{}
	response.Diagnostics.Append(plan.Destination.As(ctx, &destinationModel, basetypes.ObjectAsOptions{})...)
	destinationColumnsModels := make([]models.ClickPipeDestinationColumnModel, len(destinationModel.Columns.Elements()))
	response.Diagnostics.Append(destinationModel.Columns.ElementsAs(ctx, &destinationColumnsModels, false)...)

	clickPipe.Destination = api.ClickPipeDestination{
		Database:     destinationModel.Database.ValueString(),
		Table:        destinationModel.Table.ValueString(),
		ManagedTable: destinationModel.ManagedTable.ValueBool(),
		Columns:      make([]api.ClickPipeDestinationColumn, len(destinationColumnsModels)),
	}

	if destinationModel.ManagedTable.ValueBool() {
		if destinationModel.TableDefinition.IsNull() {
			response.Diagnostics.AddError(
				"Error Creating ClickPipe",
				"Managed table requires table definition",
			)
			return
		}

		tableDefinitionModel := models.ClickPipeDestinationTableDefinitionModel{}
		response.Diagnostics.Append(destinationModel.TableDefinition.As(ctx, &tableDefinitionModel, basetypes.ObjectAsOptions{})...)

		sortingKey := make([]string, len(tableDefinitionModel.SortingKey.Elements()))

		for i, sortingKeyModel := range tableDefinitionModel.SortingKey.Elements() {
			sortingKey[i] = sortingKeyModel.String()
		}

		tableEngineModel := models.ClickPipeDestinationTableEngineModel{}
		response.Diagnostics.Append(tableDefinitionModel.Engine.As(ctx, &tableEngineModel, basetypes.ObjectAsOptions{})...)

		clickPipe.Destination.TableDefinition = &api.ClickPipeDestinationTableDefinition{
			Engine:      api.ClickPipeDestinationTableEngine{Type: tableEngineModel.Type.ValueString()},
			PartitionBy: tableDefinitionModel.PartitionBy.ValueStringPointer(),
			PrimaryKey:  tableDefinitionModel.PrimaryKey.ValueStringPointer(),
			SortingKey:  sortingKey,
		}
	}

	for i, columnModel := range destinationColumnsModels {
		clickPipe.Destination.Columns[i] = api.ClickPipeDestinationColumn{
			Name: columnModel.Name.ValueString(),
			Type: columnModel.Type.ValueString(),
		}
	}

	fieldMappingsModels := make([]models.ClickPipeFieldMappingModel, len(plan.FieldMappings.Elements()))
	response.Diagnostics.Append(plan.FieldMappings.ElementsAs(ctx, &fieldMappingsModels, false)...)
	clickPipe.FieldMappings = make([]api.ClickPipeFieldMapping, len(fieldMappingsModels))
	for i, fieldMappingModel := range fieldMappingsModels {
		clickPipe.FieldMappings[i] = api.ClickPipeFieldMapping{
			SourceField:      fieldMappingModel.SourceField.ValueString(),
			DestinationField: fieldMappingModel.DestinationField.ValueString(),
		}
	}

	createdClickPipe, err := c.client.CreateClickPipe(ctx, serviceID, clickPipe)
	if err != nil {
		response.Diagnostics.AddError(
			"Error Creating ClickPipe",
			"Could not create ClickPipe, unexpected error: "+err.Error(),
		)
		return
	}

	if !plan.Scaling.IsNull() {
		replicasModel := models.ClickPipeScalingModel{}
		response.Diagnostics.Append(plan.Scaling.As(ctx, &replicasModel, basetypes.ObjectAsOptions{})...)

		var desiredReplicas, desiredConcurrency *int64
		if !replicasModel.Replicas.IsNull() && createdClickPipe.Scaling.Replicas != nil && *createdClickPipe.Scaling.Replicas != replicasModel.Replicas.ValueInt64() {
			desiredReplicas = replicasModel.Replicas.ValueInt64Pointer()
		}

		//if !replicasModel.Concurrency.IsNull() && createdClickPipe.Scaling.Concurrency != nil && *createdClickPipe.Scaling.Concurrency != replicasModel.Concurrency.ValueInt64() {
		//	desiredConcurrency = replicasModel.Concurrency.ValueInt64Pointer()
		//}

		if desiredReplicas != nil || desiredConcurrency != nil {
			scalingRequest := api.ClickPipeScaling{
				Replicas:    desiredReplicas,
				Concurrency: desiredConcurrency,
			}

			if createdClickPipe, err = c.client.ScalingClickPipe(ctx, serviceID, createdClickPipe.ID, scalingRequest); err != nil {
				response.Diagnostics.AddError(
					"Error Scaling ClickPipe",
					"Could not scale ClickPipe, unexpected error: "+err.Error(),
				)
				return
			}
		}
	}

	if plan.State.ValueString() == api.ClickPipeStoppedState {
		if _, err := c.client.ChangeClickPipeState(ctx, serviceID, createdClickPipe.ID, api.ClickPipeStoppedState); err != nil {
			response.Diagnostics.AddError(
				"Error Stopping ClickPipe",
				"Could not stop ClickPipe, unexpected error: "+err.Error(),
			)
			return
		}
	}

	if _, err := c.client.WaitForClickPipeState(ctx, serviceID, createdClickPipe.ID, func(state string) bool {
		return state == plan.State.ValueString() // we expect the state to be the same as planned: "Running" or "Stopped"
	}, 60); err != nil {
		response.Diagnostics.AddError(
			"Error retrieving ClickPipe state",
			"Could not retrieve ClickPipe state, unexpected error: "+err.Error(),
		)
		return
	}

	plan.ID = types.StringValue(createdClickPipe.ID)

	if err := c.syncClickPipeState(ctx, &plan); err != nil {
		response.Diagnostics.AddError(
			"Error reading ClickPipe",
			"Could not read ClickPipe, unexpected error: "+err.Error(),
		)
		return
	}

	diags = response.State.Set(ctx, plan)
	response.Diagnostics.Append(diags...)
}

func (c *ClickPipeResource) syncClickPipeState(ctx context.Context, state *models.ClickPipeResourceModel) error {
	if state.ID.IsNull() {
		return fmt.Errorf("ClickPipe ID is required to sync state")
	}

	clickPipe, err := c.client.GetClickPipe(ctx, state.ServiceID.ValueString(), state.ID.ValueString())
	if api.IsNotFound(err) {
		// ClickPipe does not exist, deleted outside Terraform
		state.ID = types.StringNull()
		return nil
	} else if err != nil {
		return err
	}

	state.ID = types.StringValue(clickPipe.ID)
	state.Name = types.StringValue(clickPipe.Name)
	state.Description = types.StringValue(clickPipe.Description)
	state.State = types.StringValue(clickPipe.State)

	if clickPipe.Scaling != nil {
		scalingModel := models.ClickPipeScalingModel{
			Replicas: types.Int64PointerValue(clickPipe.Scaling.Replicas),
			//Concurrency: types.Int64PointerValue(clickPipe.Scaling.Concurrency),
		}

		state.Scaling = scalingModel.ObjectValue()
	} else {
		state.Scaling = types.ObjectNull(models.ClickPipeScalingModel{}.ObjectType().AttrTypes)
	}

	stateSourceModel := models.ClickPipeSourceModel{}
	if diags := state.Source.As(ctx, &stateSourceModel, basetypes.ObjectAsOptions{}); diags.HasError() {
		return fmt.Errorf("error reading ClickPipe source: %v", diags)
	}

	sourceModel := models.ClickPipeSourceModel{}
	if clickPipe.Source.Kafka != nil {
		stateKafkaModel := models.ClickPipeKafkaSourceModel{}
		if diags := stateSourceModel.Kafka.As(ctx, &stateKafkaModel, basetypes.ObjectAsOptions{}); diags.HasError() {
			return fmt.Errorf("error reading ClickPipe Kafka source: %v", diags)
		}

		credentialsModel := models.ClickPipeKafkaSourceCredentialsModel{}
		if diags := stateKafkaModel.Credentials.As(ctx, &credentialsModel, basetypes.ObjectAsOptions{}); diags.HasError() {
			return fmt.Errorf("error reading ClickPipe Kafka source credentials: %v", diags)
		}

		var consumerGroup string
		if clickPipe.Source.Kafka.ConsumerGroup != nil {
			consumerGroup = *clickPipe.Source.Kafka.ConsumerGroup
		}

		kafkaModel := models.ClickPipeKafkaSourceModel{
			Type:           types.StringValue(clickPipe.Source.Kafka.Type),
			Format:         types.StringValue(clickPipe.Source.Kafka.Format),
			Brokers:        types.StringValue(clickPipe.Source.Kafka.Brokers),
			Topics:         types.StringValue(clickPipe.Source.Kafka.Topics),
			ConsumerGroup:  types.StringValue(consumerGroup),
			Authentication: types.StringValue(clickPipe.Source.Kafka.Authentication),
			Credentials:    credentialsModel.ObjectValue(),
			CACertificate:  types.StringPointerValue(clickPipe.Source.Kafka.CACertificate),
			IAMRole:        types.StringPointerValue(clickPipe.Source.Kafka.IAMRole),
		}

		if clickPipe.Source.Kafka.SchemaRegistry != nil {
			schemaRegistryModel := models.ClickPipeKafkaSchemaRegistryModel{
				URL: types.StringValue(clickPipe.Source.Kafka.SchemaRegistry.URL),
			}

			kafkaModel.SchemaRegistry = schemaRegistryModel.ObjectValue()
		} else {
			kafkaModel.SchemaRegistry = types.ObjectNull(models.ClickPipeKafkaSchemaRegistryModel{}.ObjectType().AttrTypes)
		}

		if clickPipe.Source.Kafka.Offset != nil {
			offsetModel := models.ClickPipeKafkaOffsetModel{
				Strategy:  types.StringValue(clickPipe.Source.Kafka.Offset.Strategy),
				Timestamp: types.StringPointerValue(clickPipe.Source.Kafka.Offset.Timestamp),
			}

			kafkaModel.Offset = offsetModel.ObjectValue()
		} else {
			kafkaModel.Offset = types.ObjectNull(models.ClickPipeKafkaOffsetModel{}.ObjectType().AttrTypes)
		}

		sourceModel.Kafka = kafkaModel.ObjectValue()
	} else {
		sourceModel.Kafka = types.ObjectNull(models.ClickPipeKafkaSourceModel{}.ObjectType().AttrTypes)
	}

	state.Source = sourceModel.ObjectValue()

	return nil
}

func (c *ClickPipeResource) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var state models.ClickPipeResourceModel
	diags := request.State.Get(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	if err := c.syncClickPipeState(ctx, &state); err != nil {
		response.Diagnostics.AddError(
			"Error Reading ClickPipe",
			"Could not read ClickPipe, unexpected error: "+err.Error(),
		)
		return
	}

	if state.ID.IsNull() {
		// ClickPipe does not exist, removed outside Terraform
		response.State.RemoveResource(ctx)
		return
	}

	diags = response.State.Set(ctx, state)
	response.Diagnostics.Append(diags...)
}

func (c *ClickPipeResource) Update(ctx context.Context, req resource.UpdateRequest, response *resource.UpdateResponse) {
	var plan, state models.ClickPipeResourceModel
	diags := req.Plan.Get(ctx, &plan)
	response.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	response.Diagnostics.Append(diags...)

	if response.Diagnostics.HasError() {
		return
	}

	if !plan.State.Equal(state.State) {
		var command string

		switch plan.State.ValueString() {
		case api.ClickPipeRunningState:
			command = api.ClickPipeStateStart
		case api.ClickPipeStoppedState:
			command = api.ClickPipeStateStop
		}

		if _, err := c.client.ChangeClickPipeState(ctx, state.ServiceID.ValueString(), state.ID.ValueString(), command); err != nil {
			response.Diagnostics.AddError(
				"Error Changing ClickPipe State",
				"Could not change ClickPipe state, unexpected error: "+err.Error(),
			)
			return
		}

		if _, err := c.client.WaitForClickPipeState(ctx, state.ServiceID.ValueString(), state.ID.ValueString(), func(state string) bool {
			return state == plan.State.ValueString()
		}, 60); err != nil {
			response.Diagnostics.AddError(
				"Error Retrieving ClickPipe State",
				"Could not retrieve ClickPipe state, unexpected error: "+err.Error(),
			)
			return
		}
	}

	if !plan.Scaling.Equal(state.Scaling) {
		replicasModel := models.ClickPipeScalingModel{}
		response.Diagnostics.Append(plan.Scaling.As(ctx, &replicasModel, basetypes.ObjectAsOptions{})...)

		scalingRequest := api.ClickPipeScaling{
			Replicas: replicasModel.Replicas.ValueInt64Pointer(),
		}

		if _, err := c.client.ScalingClickPipe(ctx, state.ServiceID.ValueString(), state.ID.ValueString(), scalingRequest); err != nil {
			response.Diagnostics.AddError(
				"Error Scaling ClickPipe",
				"Could not scale ClickPipe, unexpected error: "+err.Error(),
			)
			return
		}
	}

	if err := c.syncClickPipeState(ctx, &plan); err != nil {
		response.Diagnostics.AddError(
			"Error Reading ClickPipe",
			"Could not read ClickPipe, unexpected error: "+err.Error(),
		)
		return
	}

	diags = response.State.Set(ctx, plan)
	response.Diagnostics.Append(diags...)
}

func (c *ClickPipeResource) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var state models.ClickPipeResourceModel
	diags := request.State.Get(ctx, &state)
	response.Diagnostics.Append(diags...)
	if response.Diagnostics.HasError() {
		return
	}

	if err := c.client.DeleteClickPipe(ctx, state.ServiceID.ValueString(), state.ID.ValueString()); err != nil {
		response.Diagnostics.AddError(
			"Error Deleting ClickPipe",
			"Could not delete ClickPipe, unexpected error: "+err.Error(),
		)
	}
}
