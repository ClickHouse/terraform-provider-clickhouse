package resource

import (
	"context"
	"crypto/sha1" // nolint:gosec
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &ServiceResource{}
	_ resource.ResourceWithConfigure   = &ServiceResource{}
	_ resource.ResourceWithImportState = &ServiceResource{}
)

// NewServiceResource is a helper function to simplify the provider implementation.
func NewServiceResource() resource.Resource {
	return &ServiceResource{}
}

// ServiceResource is the resource implementation.
type ServiceResource struct {
	client api.Client
}

// Metadata returns the resource type name.
func (r *ServiceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

// Schema defines the schema for the resource.
func (r *ServiceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "ID of the created service. Generated by ClickHouse Cloud.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "User defined identifier for the service.",
				Required:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password for the default user. One of either `password` or `password_hash` must be specified.",
				Optional:    true,
				Sensitive:   true,
			},
			"password_hash": schema.StringAttribute{
				Description: "SHA256 hash of password for the default user. One of either `password` or `password_hash` must be specified.",
				Optional:    true,
				Sensitive:   true,
			},
			"double_sha1_password_hash": schema.StringAttribute{
				Description: "Double SHA1 hash of password for connecting with the MySQL protocol. Cannot be specified if `password` is specified.",
				Optional:    true,
				Sensitive:   true,
			},
			"cloud_provider": schema.StringAttribute{
				Description: "Cloud provider ('aws', 'gcp', or 'azure') in which the service is deployed in.",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "Region within the cloud provider in which the service is deployed in.",
				Required:    true,
			},
			"tier": schema.StringAttribute{
				Description: "Tier of the service: 'development', 'production'. Production services scale, Development are fixed size.",
				Required:    true,
			},
			"idle_scaling": schema.BoolAttribute{
				Description: "When set to true the service is allowed to scale down to zero when idle.",
				Optional:    true,
			},
			"ip_access": schema.ListNestedAttribute{
				Description: "List of IP addresses allowed to access the service.",
				Required:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"source": schema.StringAttribute{
							Description: "IP address allowed to access the service. In case you want to set the ip_access to anywhere you should set source to 0.0.0.0/0",
							Required:    true,
						},
						"description": schema.StringAttribute{
							Description: "Description of the IP address.",
							Optional:    true,
						},
					},
				},
			},
			"endpoints": schema.ListNestedAttribute{
				Description: "List of public endpoints.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"protocol": schema.StringAttribute{
							Description: "Endpoint protocol: https or nativesecure",
							Computed:    true,
						},
						"host": schema.StringAttribute{
							Description: "Endpoint host.",
							Computed:    true,
						},
						"port": schema.Int64Attribute{
							Description: "Endpoint port.",
							Computed:    true,
						},
					},
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"min_total_memory_gb": schema.Int64Attribute{
				Description: "Minimum total memory of all workers during auto-scaling in Gb. Available only for 'production' services. Must be a multiple of 12 and greater than 24.",
				Optional:    true,
			},
			"max_total_memory_gb": schema.Int64Attribute{
				Description: "Maximum total memory of all workers during auto-scaling in Gb. Available only for 'production' services. Must be a multiple of 12 and lower than 360 for non paid services or 720 for paid services.",
				Optional:    true,
			},
			"num_replicas": schema.Int64Attribute{
				Description: "Number of replicas for the service. Available only for 'production' services. Must be between 3 and 20. Contact support to enable this feature.",
				Optional:    true,
			},
			"idle_timeout_minutes": schema.Int64Attribute{
				Description: "Set minimum idling timeout (in minutes). Must be greater than or equal to 5 minutes. Must be set if idle_scaling is enabled",
				Optional:    true,
			},
			"iam_role": schema.StringAttribute{
				Description: "IAM role used for accessing objects in s3.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"private_endpoint_config": schema.SingleNestedAttribute{
				Description: "Service config for private endpoints",
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					"endpoint_service_id": schema.StringAttribute{
						Description: "Unique identifier of the interface endpoint you created in your VPC with the AWS(Service Name) or GCP(Target Service) resource",
						Computed:    true,
					},
					"private_dns_hostname": schema.StringAttribute{
						Description: "Private DNS Hostname of the VPC you created",
						Computed:    true,
					},
				},
				DeprecationMessage: "Please use the `clickhouse_private_endpoint_config` data source instead.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
			},
			"encryption_key": schema.StringAttribute{
				Description: "Custom encryption key arn",
				Optional:    true,
			},
			"encryption_assumed_role_identifier": schema.StringAttribute{
				Description: "Custom role identifier arn ",
				Optional:    true,
			},
		},
		MarkdownDescription: `You can use the *clickhouse_service* resource to deploy ClickHouse cloud instances on supported cloud providers.`,
	}
}

// Configure adds the provider configured client to the resource.
func (r *ServiceResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(api.Client)
}

func (r *ServiceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		// If the entire plan is null, the resource is planned for destruction.
		return
	}

	var plan, state models.ServiceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if !req.State.Raw.IsNull() {
		diags = req.State.Get(ctx, &state)
		resp.Diagnostics.Append(diags...)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	if !req.State.Raw.IsNull() {
		// Validations for updates.
		if !plan.CloudProvider.IsNull() && plan.CloudProvider != state.CloudProvider {
			resp.Diagnostics.AddAttributeError(
				path.Root("cloud_provider"),
				"Invalid Update",
				"ClickHouse does not support changing service cloud providers",
			)
		}

		if !plan.Region.IsNull() && plan.Region != state.Region {
			resp.Diagnostics.AddAttributeError(
				path.Root("region"),
				"Invalid Update",
				"ClickHouse does not support changing service regions",
			)
		}

		if !plan.Tier.IsNull() && plan.Tier != state.Tier {
			resp.Diagnostics.AddAttributeError(
				path.Root("tier"),
				"Invalid Update",
				"ClickHouse does not support changing service tiers",
			)
		}

		if !plan.EncryptionKey.IsNull() && plan.EncryptionKey != state.EncryptionKey {
			resp.Diagnostics.AddAttributeError(
				path.Root("encryption_key"),
				"Invalid Update",
				"ClickHouse does not support changing encryption_key",
			)
		}

		if !plan.EncryptionAssumedRoleIdentifier.IsNull() && plan.EncryptionAssumedRoleIdentifier != state.EncryptionAssumedRoleIdentifier {
			resp.Diagnostics.AddAttributeError(
				path.Root("encryption_assumed_role_identifier"),
				"Invalid Update",
				"ClickHouse does not support changing encryption_assumed_role_identifier",
			)
		}
	}

	if plan.Tier.ValueString() == api.TierDevelopment {
		if !plan.MinTotalMemoryGb.IsNull() || !plan.MaxTotalMemoryGb.IsNull() || !plan.NumReplicas.IsNull() {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				"min_total_memory_gb, max_total_memory_gb and num_replicas cannot be defined if the service tier is development",
			)
		}

		if !plan.EncryptionKey.IsNull() || !plan.EncryptionAssumedRoleIdentifier.IsNull() {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				"custom managed encryption cannot be defined if the service tier is development",
			)
		}
	} else if plan.Tier.ValueString() == api.TierProduction {
		if plan.MinTotalMemoryGb.IsNull() || plan.MaxTotalMemoryGb.IsNull() {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				"min_total_memory_gb and max_total_memory_gb must be defined if the service tier is production",
			)
		}

		if !plan.EncryptionAssumedRoleIdentifier.IsNull() && plan.EncryptionKey.IsNull() {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				"encryption_assumed_role_identifier cannot be defined without encryption_key as well",
			)
		}

		if !plan.EncryptionKey.IsNull() && strings.Compare(plan.CloudProvider.ValueString(), "aws") != 0 {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				"encryption_key and the encryption_assumed_role_identifier is only available for aws services",
			)
		}
	}

	if plan.IdleTimeoutMinutes.IsNull() && plan.IdleScaling.ValueBool() {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"idle_timeout_minutes should be defined if idle_scaling is enabled",
		)
	}

	if !plan.IdleTimeoutMinutes.IsNull() && !plan.IdleScaling.ValueBool() {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"idle_timeout_minutes must be null if idle_scaling is disabled",
		)
	}

	if !plan.Password.IsNull() && !plan.PasswordHash.IsNull() {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"Only one of either password or password_hash may be specified",
		)
	}

	if plan.Password.IsNull() && plan.PasswordHash.IsNull() {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"One of either password or password_hash must be specified",
		)
	}

	if !plan.Password.IsNull() && !plan.DoubleSha1PasswordHash.IsNull() {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"`double_sha1_password_hash` cannot be specified if `password` specified",
		)
	}

	if !plan.DoubleSha1PasswordHash.IsNull() && plan.PasswordHash.IsNull() {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			"`double_sha1_password_hash` cannot be specified without `password_hash`",
		)
	}

	if !plan.DoubleSha1PasswordHash.IsNull() {
		match, _ := regexp.MatchString("^[0-9a-fA-F]{40}$", plan.DoubleSha1PasswordHash.ValueString())
		if !match {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				"`double_sha1_password_hash` is not a double sha1 hash",
			)
		}
	}

	if !plan.PasswordHash.IsNull() {
		match, _ := regexp.MatchString("^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$", plan.PasswordHash.ValueString())
		if !match {
			resp.Diagnostics.AddError(
				"Invalid Configuration",
				"`password_hash` is not a base64 encoded hash",
			)
		}
	}
}

// Create a new resource
func (r *ServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan models.ServiceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request body from plan
	service := api.Service{
		Name:     plan.Name.ValueString(),
		Provider: plan.CloudProvider.ValueString(),
		Region:   plan.Region.ValueString(),
		Tier:     plan.Tier.ValueString(),
	}

	if service.Tier == api.TierProduction {
		minTotalMemoryGb := int(plan.MinTotalMemoryGb.ValueInt64())
		service.MinTotalMemoryGb = &minTotalMemoryGb
		maxTotalMemoryGb := int(plan.MaxTotalMemoryGb.ValueInt64())
		service.MaxTotalMemoryGb = &maxTotalMemoryGb

		if !plan.EncryptionKey.IsNull() {
			service.EncryptionKey = plan.EncryptionKey.ValueString()
		}
		if !plan.EncryptionAssumedRoleIdentifier.IsNull() {
			service.EncryptionAssumedRoleIdentifier = plan.EncryptionAssumedRoleIdentifier.ValueString()
		}
	}

	service.IdleScaling = plan.IdleScaling.ValueBool()
	if !plan.IdleTimeoutMinutes.IsNull() {
		idleTimeoutMinutes := int(plan.IdleTimeoutMinutes.ValueInt64())
		service.IdleTimeoutMinutes = &idleTimeoutMinutes
	}

	ipAccessModels := make([]models.IPAccessList, 0, len(plan.IpAccessList.Elements()))
	plan.IpAccessList.ElementsAs(ctx, &ipAccessModels, false)
	ipAccessLists := make([]api.IpAccess, 0, len(ipAccessModels))
	for _, ipAccessModel := range ipAccessModels {
		ipAccessLists = append(ipAccessLists, api.IpAccess{
			Source:      ipAccessModel.Source.ValueString(),
			Description: ipAccessModel.Description.ValueString(),
		})
	}
	service.IpAccessList = ipAccessLists

	// Create new service
	s, _, err := r.client.CreateService(service)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating service",
			"Could not create service, unexpected error: "+err.Error(),
		)
		return
	}

	numErrors := 0
	id := s.Id
	for {
		s, err = r.client.GetService(id)
		if err != nil {
			numErrors++
			if numErrors > api.MaxRetry {
				resp.Diagnostics.AddError(
					"Error retrieving service state",
					"Could not retrieve service state after creation, unexpected error: "+err.Error(),
				)
				return
			}
			time.Sleep(time.Second * 5)
			continue
		}

		if s.State != "provisioning" {
			break
		}

		time.Sleep(time.Second * 5)
	}

	// Update service password if provided explicitly
	planPassword := plan.Password.ValueString()
	if len(planPassword) > 0 {
		_, err := r.client.UpdateServicePassword(s.Id, servicePasswordUpdateFromPlainPassword(planPassword))
		if err != nil {
			resp.Diagnostics.AddError(
				"Error setting service password",
				"Could not set service password after creation, unexpected error: "+err.Error(),
			)
			return
		}
	}

	// Update hashed service password if provided explicitly
	if passwordHash, doubleSha1PasswordHash := plan.PasswordHash.ValueString(), plan.DoubleSha1PasswordHash.ValueString(); len(passwordHash) > 0 || len(doubleSha1PasswordHash) > 0 {
		passwordUpdate := api.ServicePasswordUpdate{
			NewPasswordHash: passwordHash,
		}

		if len(doubleSha1PasswordHash) > 0 {
			passwordUpdate.NewDoubleSha1Hash = doubleSha1PasswordHash
		}

		_, err := r.client.UpdateServicePassword(s.Id, passwordUpdate)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error setting service password",
				"Could not set service password after creation, unexpected error: "+err.Error(),
			)
			return
		}
	}

	// Map response body to schema and populate Computed attribute values
	plan.ID = types.StringValue(s.Id)
	err = r.syncServiceState(ctx, &plan, true)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading ClickHouse Service",
			"Could not read ClickHouse service id "+plan.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Read refreshes the Terraform state with the latest data.
func (r *ServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ServiceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.syncServiceState(ctx, &state, false)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading ClickHouse Service",
			"Could not read ClickHouse service id "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	if state.ID.IsNull() {
		// Resource was deleted outside terraform
		resp.State.RemoveResource(ctx)
	} else {
		// Set refreshed state
		diags = resp.State.Set(ctx, state)
		resp.Diagnostics.Append(diags...)
	}
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *ServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan, state models.ServiceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request body from plan
	serviceId := state.ID.ValueString()
	service := api.ServiceUpdate{
		Name:         "",
		IpAccessList: nil,
	}
	serviceChange := false

	if plan.Name != state.Name {
		service.Name = plan.Name.ValueString()
		serviceChange = true
	}

	if !plan.IpAccessList.Equal(state.IpAccessList) {
		serviceChange = true
		var currentIPAccessList, desiredIPAccessList []models.IPAccessList
		state.IpAccessList.ElementsAs(ctx, &currentIPAccessList, false)
		plan.IpAccessList.ElementsAs(ctx, &desiredIPAccessList, false)

		var add, remove []api.IpAccess

		for _, ipAccess := range currentIPAccessList {
			remove = append(remove, api.IpAccess{
				Source:      ipAccess.Source.ValueString(),
				Description: ipAccess.Description.ValueString(),
			})
		}

		for _, ipAccess := range desiredIPAccessList {
			add = append(add, api.IpAccess{
				Source:      ipAccess.Source.ValueString(),
				Description: ipAccess.Description.ValueString(),
			})
		}

		service.IpAccessList = &api.IpAccessUpdate{
			Add:    add,
			Remove: remove,
		}
	}

	// Update existing service
	if serviceChange {
		var err error
		_, err = r.client.UpdateService(serviceId, service)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating ClickHouse Service",
				"Could not update service, unexpected error: "+err.Error(),
			)
			return
		}
	}

	scalingChange := false
	serviceScaling := api.ServiceScalingUpdate{
		IdleScaling: state.IdleScaling.ValueBoolPointer(),
	}

	if plan.IdleScaling != state.IdleScaling {
		scalingChange = true
		idleScaling := new(bool)
		*idleScaling = plan.IdleScaling.ValueBool()
		serviceScaling.IdleScaling = idleScaling
	}
	if plan.MinTotalMemoryGb != state.MinTotalMemoryGb {
		scalingChange = true
		if !plan.MinTotalMemoryGb.IsNull() {
			minTotalMemoryGb := int(plan.MinTotalMemoryGb.ValueInt64())
			serviceScaling.MinTotalMemoryGb = &minTotalMemoryGb
		}
	}
	if plan.MaxTotalMemoryGb != state.MaxTotalMemoryGb {
		scalingChange = true
		if !plan.MaxTotalMemoryGb.IsNull() {
			maxTotalMemoryGb := int(plan.MaxTotalMemoryGb.ValueInt64())
			serviceScaling.MaxTotalMemoryGb = &maxTotalMemoryGb
		}
	}
	if plan.NumReplicas != state.NumReplicas {
		scalingChange = true
		if !plan.NumReplicas.IsNull() {
			numReplicas := int(plan.NumReplicas.ValueInt64())
			serviceScaling.NumReplicas = &numReplicas
		}
	}
	if plan.IdleTimeoutMinutes != state.IdleTimeoutMinutes {
		scalingChange = true
		if !plan.IdleTimeoutMinutes.IsNull() {
			idleTimeoutMinutes := int(plan.IdleTimeoutMinutes.ValueInt64())
			serviceScaling.IdleTimeoutMinutes = &idleTimeoutMinutes
		}
	}

	if scalingChange {
		var err error
		_, err = r.client.UpdateServiceScaling(serviceId, serviceScaling)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating ClickHouse Service Scaling",
				"Could not update service scaling, unexpected error: "+err.Error(),
			)
			return
		}
	}

	password := plan.Password.ValueString()
	if len(password) > 0 && plan.Password != state.Password {
		password = plan.Password.ValueString()
		_, err := r.client.UpdateServicePassword(serviceId, servicePasswordUpdateFromPlainPassword(password))
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating ClickHouse Service Password",
				"Could not update service password, unexpected error: "+err.Error(),
			)
			return
		}
	} else if !plan.PasswordHash.IsNull() || !plan.DoubleSha1PasswordHash.IsNull() {
		passwordUpdate := api.ServicePasswordUpdate{}

		if !plan.PasswordHash.IsNull() { // change in password hash
			passwordUpdate.NewPasswordHash = plan.PasswordHash.ValueString()
		} else { // no change in password hash, use state value
			passwordUpdate.NewPasswordHash = state.PasswordHash.ValueString()
		}

		if !plan.DoubleSha1PasswordHash.IsNull() { // change in double sha1 password hash
			passwordUpdate.NewDoubleSha1Hash = plan.DoubleSha1PasswordHash.ValueString()
		}

		_, err := r.client.UpdateServicePassword(serviceId, passwordUpdate)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Updating ClickHouse Service Password",
				"Could not update service password, unexpected error: "+err.Error(),
			)
			return
		}
	}

	err := r.syncServiceState(ctx, &plan, true)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error syncing service state",
			"Could not sync service state, unexpected error: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *ServiceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state models.ServiceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete existing order
	_, err := r.client.DeleteService(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting ClickHouse Service",
			"Could not delete service, unexpected error: "+err.Error(),
		)
		return
	}
}

func (r *ServiceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Retrieve import ID and save to id attribute
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// syncServiceState fetches the latest state ClickHouse Cloud API and updates the Terraform state.
func (r *ServiceResource) syncServiceState(ctx context.Context, state *models.ServiceResourceModel, updateTimestamp bool) error {
	if state.ID.IsNull() {
		return errors.New("service ID must be set to fetch the service")
	}

	// Get latest service value from ClickHouse OpenAPI
	service, err := r.client.GetService(state.ID.ValueString())
	if api.IsNotFound(err) {
		// Service was deleted outside terraform.
		state.ID = types.StringNull()

		return nil
	} else if err != nil {
		return err
	}

	// Overwrite items with refreshed state
	state.Name = types.StringValue(service.Name)
	state.CloudProvider = types.StringValue(service.Provider)
	state.Region = types.StringValue(service.Region)
	state.Tier = types.StringValue(service.Tier)
	state.IdleScaling = types.BoolValue(service.IdleScaling)
	if state.IdleScaling.ValueBool() {
		if service.IdleTimeoutMinutes != nil {
			state.IdleTimeoutMinutes = types.Int64Value(int64(*service.IdleTimeoutMinutes))
		}
	} else {
		state.IdleTimeoutMinutes = types.Int64Null()
	}

	if service.Tier == api.TierProduction {
		if service.MinTotalMemoryGb != nil {
			state.MinTotalMemoryGb = types.Int64Value(int64(*service.MinTotalMemoryGb))
		}
		if service.MaxTotalMemoryGb != nil {
			state.MaxTotalMemoryGb = types.Int64Value(int64(*service.MaxTotalMemoryGb))
		}
		if service.NumReplicas != nil {
			state.NumReplicas = types.Int64Value(int64(*service.NumReplicas))
		}
	}

	{
		var ipAccessList []attr.Value
		for _, ipAccess := range service.IpAccessList {
			ipAccessList = append(ipAccessList, models.IPAccessList{Source: types.StringValue(ipAccess.Source), Description: types.StringValue(ipAccess.Description)}.ObjectValue())
		}
		state.IpAccessList, _ = types.ListValue(models.IPAccessList{}.ObjectType(), ipAccessList)
	}

	{
		var endpoints []attr.Value
		for _, endpoint := range service.Endpoints {
			endpoints = append(endpoints, models.Endpoint{Protocol: types.StringValue(endpoint.Protocol), Host: types.StringValue(endpoint.Host), Port: types.Int64Value(int64(endpoint.Port))}.ObjectValue())
		}
		state.Endpoints, _ = types.ListValue(models.Endpoint{}.ObjectType(), endpoints)
	}

	state.IAMRole = types.StringValue(service.IAMRole)

	state.PrivateEndpointConfig = models.PrivateEndpointConfig{
		EndpointServiceID:  types.StringValue(service.PrivateEndpointConfig.EndpointServiceId),
		PrivateDNSHostname: types.StringValue(service.PrivateEndpointConfig.PrivateDnsHostname),
	}.ObjectValue()

	if service.EncryptionKey != "" {
		state.EncryptionKey = types.StringValue(service.EncryptionKey)
	} else {
		state.EncryptionKey = types.StringNull()
	}
	if service.EncryptionAssumedRoleIdentifier != "" {
		state.EncryptionAssumedRoleIdentifier = types.StringValue(service.EncryptionAssumedRoleIdentifier)
	} else {
		state.EncryptionAssumedRoleIdentifier = types.StringNull()
	}

	return nil
}

func servicePasswordUpdateFromPlainPassword(password string) api.ServicePasswordUpdate {
	hash := sha256.Sum256([]byte(password))

	singleSha1Hash := sha1.Sum([]byte(password))  // nolint:gosec
	doubleSha1Hash := sha1.Sum(singleSha1Hash[:]) // nolint:gosec

	return api.ServicePasswordUpdate{
		NewPasswordHash:   base64.StdEncoding.EncodeToString(hash[:]),
		NewDoubleSha1Hash: hex.EncodeToString(doubleSha1Hash[:]),
	}
}
