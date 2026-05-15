package resource

import (
	"context"
	_ "embed"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

var (
	_ resource.Resource                = &ServiceScheduledScalingResource{}
	_ resource.ResourceWithConfigure   = &ServiceScheduledScalingResource{}
	_ resource.ResourceWithImportState = &ServiceScheduledScalingResource{}
)

//go:embed descriptions/service_scheduled_scaling.md
var serviceScheduledScalingResourceDescription string

func NewServiceScheduledScalingResource() resource.Resource {
	return &ServiceScheduledScalingResource{}
}

type ServiceScheduledScalingResource struct {
	client api.Client
}

func (r *ServiceScheduledScalingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_scheduled_scaling"
}

func (r *ServiceScheduledScalingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: serviceScheduledScalingResourceDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Resource identifier. Equal to service_id (one schedule per service).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"service_id": schema.StringAttribute{
				Description: "ClickHouse Cloud service ID this schedule applies to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"entries": schema.ListNestedAttribute{
				Description: "Ordered list of recurring scaling windows. The first entry whose weekday and hour range covers \"now\" is applied; otherwise base_config applies.",
				Required:    true,
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
					listvalidator.SizeAtMost(api.MaxAutoScalingScheduleEntries),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Human-readable name for the entry (e.g. \"Business hours\").",
							Required:    true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"weekdays": schema.SetAttribute{
							Description: "Weekdays this entry covers. 0 = Sunday … 6 = Saturday.",
							Required:    true,
							ElementType: types.Int64Type,
							Validators: []validator.Set{
								setvalidator.SizeAtLeast(1),
								setvalidator.ValueInt64sAre(int64validator.Between(0, 6)),
							},
						},
						"start_hour_utc": schema.Int64Attribute{
							Description: "Start hour in UTC (0-23). If end_hour_utc < start_hour_utc the window wraps overnight. Set start_hour_utc=0 and end_hour_utc=24 for a 24-hour window.",
							Required:    true,
							Validators: []validator.Int64{
								int64validator.Between(0, 23),
							},
						},
						"end_hour_utc": schema.Int64Attribute{
							Description: "End hour in UTC (1-24). Must differ from start_hour_utc. Note the asymmetric range: end_hour_utc=0 is invalid; use end_hour_utc=24 to mean midnight at end of day.",
							Required:    true,
							Validators: []validator.Int64{
								int64validator.Between(1, 24),
							},
						},
						"min_replica_memory_gb": schema.Int64Attribute{
							Description: "Minimum memory per replica in GiB. Must be set together with max_replica_memory_gb.",
							Optional:    true,
							Computed:    true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
								int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("max_replica_memory_gb")),
							},
						},
						"max_replica_memory_gb": schema.Int64Attribute{
							Description: "Maximum memory per replica in GiB. Must be set together with min_replica_memory_gb.",
							Optional:    true,
							Computed:    true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
								int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("min_replica_memory_gb")),
							},
						},
						"min_replicas": schema.Int64Attribute{
							Description: "Minimum replica count while the window is active. Currently the server requires min_replicas == max_replicas.",
							Optional:    true,
							Computed:    true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
							Validators: []validator.Int64{
								int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("max_replicas")),
							},
						},
						"max_replicas": schema.Int64Attribute{
							Description: "Maximum replica count while the window is active. Currently the server requires min_replicas == max_replicas.",
							Optional:    true,
							Computed:    true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
							Validators: []validator.Int64{
								int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("min_replicas")),
							},
						},
						"idle_scaling": schema.BoolAttribute{
							Description: "Whether idle scaling is enabled while the window is active.",
							Optional:    true,
							Computed:    true,
							PlanModifiers: []planmodifier.Bool{
								boolplanmodifier.UseStateForUnknown(),
							},
						},
						"idle_timeout_minutes": schema.Int64Attribute{
							Description: "Minutes of inactivity before the service scales to zero. Must be at least 5. Only meaningful when idle_scaling is true.",
							Optional:    true,
							Computed:    true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
							Validators: []validator.Int64{
								int64validator.AtLeast(5),
							},
						},
					},
				},
			},
			"base_config": schema.SingleNestedAttribute{
				Description: "Fallback configuration applied when no entry is currently active. Server-managed.",
				Computed:    true,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
				Attributes: map[string]schema.Attribute{
					"min_replica_memory_gb": schema.Int64Attribute{Computed: true},
					"max_replica_memory_gb": schema.Int64Attribute{Computed: true},
					"min_replicas":          schema.Int64Attribute{Computed: true},
					"max_replicas":          schema.Int64Attribute{Computed: true},
					"idle_scaling":          schema.BoolAttribute{Computed: true},
					"idle_timeout_minutes":  schema.Int64Attribute{Computed: true},
				},
			},
		},
	}
}

func (r *ServiceScheduledScalingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(api.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected api.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

func (r *ServiceScheduledScalingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.ServiceScheduledScalingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()

	// Refuse to clobber an existing schedule. The user should import it.
	existing, err := r.client.GetScheduledScaling(ctx, serviceID)
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error checking for existing scheduled scaling", err.Error())
		return
	}
	if existing != nil && len(existing.Entries) > 0 {
		resp.Diagnostics.AddError(
			"Scheduled scaling already exists for this service",
			fmt.Sprintf("Service %s already has a scaling schedule with %d entries. Import it into Terraform with: terraform import clickhouse_service_scheduled_scaling.<name> %s", serviceID, len(existing.Entries), serviceID),
		)
		return
	}

	entries, d := planEntriesToAPI(ctx, plan.Entries)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	schedule, err := r.client.UpdateScheduledScaling(ctx, serviceID, api.AutoScalingScheduleUpdate{Entries: entries})
	if err != nil {
		if api.IsNotFound(err) {
			resp.Diagnostics.AddError(
				"Service not found",
				fmt.Sprintf("Service %s does not exist or has been deleted. Confirm clickhouse_service.<name>.id is correct.", serviceID),
			)
			return
		}
		resp.Diagnostics.AddError("Error creating scheduled scaling", err.Error())
		return
	}

	plan.ID = plan.ServiceID
	resp.Diagnostics.Append(applyScheduleToState(schedule, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceScheduledScalingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ServiceScheduledScalingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := state.ServiceID.ValueString()

	schedule, err := r.client.GetScheduledScaling(ctx, serviceID)
	if err != nil {
		if api.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading scheduled scaling", err.Error())
		return
	}

	state.ID = state.ServiceID
	resp.Diagnostics.Append(applyScheduleToState(schedule, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ServiceScheduledScalingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.ServiceScheduledScalingResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	entries, d := planEntriesToAPI(ctx, plan.Entries)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	schedule, err := r.client.UpdateScheduledScaling(ctx, plan.ServiceID.ValueString(), api.AutoScalingScheduleUpdate{Entries: entries})
	if err != nil {
		if api.IsNotFound(err) {
			resp.Diagnostics.AddError(
				"Service not found",
				fmt.Sprintf("Service %s no longer exists. Remove the resource from configuration or recreate the service.", plan.ServiceID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError("Error updating scheduled scaling", err.Error())
		return
	}

	plan.ID = plan.ServiceID
	resp.Diagnostics.Append(applyScheduleToState(schedule, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceScheduledScalingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ServiceScheduledScalingResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteScheduledScaling(ctx, state.ServiceID.ValueString())
	if err != nil && !api.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting scheduled scaling", err.Error())
	}
}

func (r *ServiceScheduledScalingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// `id` and `service_id` are equal — write both so Read finds the service.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("service_id"), req.ID)...)
}

func (r *ServiceScheduledScalingResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config models.ServiceScheduledScalingResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Entries.IsNull() || config.Entries.IsUnknown() {
		return
	}

	var entries []models.ScheduledScalingEntryModel
	resp.Diagnostics.Append(config.Entries.ElementsAs(ctx, &entries, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(validateScheduledScalingEntries(entries)...)
}

// validateScheduledScalingEntries enforces per-entry cross-field rules that
// can't be expressed as individual attribute validators (non-zero window,
// min <= max for memory, min == max for replicas). The "set together"
// constraints on the memory and replica pairs are handled at schema level
// via int64validator.AlsoRequires.
func validateScheduledScalingEntries(entries []models.ScheduledScalingEntryModel) diag.Diagnostics {
	var diags diag.Diagnostics
	entriesPath := path.Root("entries")
	for i, e := range entries {
		entryPath := entriesPath.AtListIndex(i)

		if !e.StartHourUtc.IsNull() && !e.StartHourUtc.IsUnknown() && !e.EndHourUtc.IsNull() && !e.EndHourUtc.IsUnknown() {
			if e.StartHourUtc.ValueInt64() == e.EndHourUtc.ValueInt64() {
				diags.AddAttributeError(
					entryPath,
					"start_hour_utc and end_hour_utc must differ",
					"Zero-duration windows are not allowed.",
				)
			}
		}

		minMemSet := !e.MinReplicaMemoryGb.IsNull() && !e.MinReplicaMemoryGb.IsUnknown()
		maxMemSet := !e.MaxReplicaMemoryGb.IsNull() && !e.MaxReplicaMemoryGb.IsUnknown()
		if minMemSet && maxMemSet && e.MinReplicaMemoryGb.ValueInt64() > e.MaxReplicaMemoryGb.ValueInt64() {
			diags.AddAttributeError(
				entryPath,
				"min_replica_memory_gb must be <= max_replica_memory_gb",
				fmt.Sprintf("Got min=%d, max=%d.", e.MinReplicaMemoryGb.ValueInt64(), e.MaxReplicaMemoryGb.ValueInt64()),
			)
		}

		minRepSet := !e.MinReplicas.IsNull() && !e.MinReplicas.IsUnknown()
		maxRepSet := !e.MaxReplicas.IsNull() && !e.MaxReplicas.IsUnknown()
		if minRepSet && maxRepSet && e.MinReplicas.ValueInt64() != e.MaxReplicas.ValueInt64() {
			diags.AddAttributeError(
				entryPath,
				"min_replicas must equal max_replicas",
				"The scheduled scaling API currently requires a fixed replica count per entry.",
			)
		}
	}
	return diags
}

// planEntriesToAPI converts the Terraform plan list of entries into API entries.
func planEntriesToAPI(ctx context.Context, entriesList types.List) ([]api.AutoScalingScheduleEntry, diag.Diagnostics) {
	var diags diag.Diagnostics

	if entriesList.IsNull() || entriesList.IsUnknown() {
		return []api.AutoScalingScheduleEntry{}, diags
	}

	var entryModels []models.ScheduledScalingEntryModel
	diags.Append(entriesList.ElementsAs(ctx, &entryModels, false)...)
	if diags.HasError() {
		return nil, diags
	}

	result := make([]api.AutoScalingScheduleEntry, len(entryModels))
	for i, em := range entryModels {
		var weekdays []int64
		diags.Append(em.Weekdays.ElementsAs(ctx, &weekdays, false)...)
		if diags.HasError() {
			return nil, diags
		}
		intWeekdays := make([]int, len(weekdays))
		for j, w := range weekdays {
			intWeekdays[j] = int(w)
		}
		// Sort for deterministic wire output across applies — set iteration is
		// non-deterministic and would otherwise produce gratuitously different
		// request bodies for the same config.
		sort.Ints(intWeekdays)

		entry := api.AutoScalingScheduleEntry{
			Name:         em.Name.ValueString(),
			Weekdays:     intWeekdays,
			StartHourUtc: int(em.StartHourUtc.ValueInt64()),
			EndHourUtc:   int(em.EndHourUtc.ValueInt64()),
		}
		if !em.MinReplicaMemoryGb.IsNull() && !em.MinReplicaMemoryGb.IsUnknown() {
			v := int(em.MinReplicaMemoryGb.ValueInt64())
			entry.MinReplicaMemoryGb = &v
		}
		if !em.MaxReplicaMemoryGb.IsNull() && !em.MaxReplicaMemoryGb.IsUnknown() {
			v := int(em.MaxReplicaMemoryGb.ValueInt64())
			entry.MaxReplicaMemoryGb = &v
		}
		if !em.MinReplicas.IsNull() && !em.MinReplicas.IsUnknown() {
			v := int(em.MinReplicas.ValueInt64())
			entry.MinReplicas = &v
		}
		if !em.MaxReplicas.IsNull() && !em.MaxReplicas.IsUnknown() {
			v := int(em.MaxReplicas.ValueInt64())
			entry.MaxReplicas = &v
		}
		if !em.IdleScaling.IsNull() && !em.IdleScaling.IsUnknown() {
			v := em.IdleScaling.ValueBool()
			entry.IdleScaling = &v
		}
		if !em.IdleTimeoutMinutes.IsNull() && !em.IdleTimeoutMinutes.IsUnknown() {
			v := int(em.IdleTimeoutMinutes.ValueInt64())
			entry.IdleTimeoutMinutes = &v
		}

		result[i] = entry
	}

	return result, diags
}

// applyScheduleToState maps an API AutoScalingSchedule response into the
// Terraform state model.
func applyScheduleToState(schedule *api.AutoScalingSchedule, state *models.ServiceScheduledScalingResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	entryValues := make([]attr.Value, len(schedule.Entries))
	for i, e := range schedule.Entries {
		entryModel, d := apiEntryToModel(e)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		entryValues[i] = entryModel.ObjectValue()
	}
	entriesList, d := types.ListValue(models.ScheduledScalingEntryModel{}.ObjectType(), entryValues)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}
	state.Entries = entriesList

	if schedule.BaseConfig != nil {
		state.BaseConfig = apiBaseConfigToModel(*schedule.BaseConfig).ObjectValue()
	} else {
		state.BaseConfig = types.ObjectNull(models.ScheduledScalingBaseConfigModel{}.ObjectType().AttrTypes)
	}

	return diags
}

func apiEntryToModel(e api.AutoScalingScheduleEntry) (models.ScheduledScalingEntryModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	weekdayValues := make([]attr.Value, len(e.Weekdays))
	for i, w := range e.Weekdays {
		weekdayValues[i] = types.Int64Value(int64(w))
	}
	weekdaySet, d := types.SetValue(types.Int64Type, weekdayValues)
	diags.Append(d...)
	if diags.HasError() {
		return models.ScheduledScalingEntryModel{}, diags
	}

	model := models.ScheduledScalingEntryModel{
		Name:               types.StringValue(e.Name),
		Weekdays:           weekdaySet,
		StartHourUtc:       types.Int64Value(int64(e.StartHourUtc)),
		EndHourUtc:         types.Int64Value(int64(e.EndHourUtc)),
		MinReplicaMemoryGb: int64PtrToValue(e.MinReplicaMemoryGb),
		MaxReplicaMemoryGb: int64PtrToValue(e.MaxReplicaMemoryGb),
		MinReplicas:        int64PtrToValue(e.MinReplicas),
		MaxReplicas:        int64PtrToValue(e.MaxReplicas),
		IdleScaling:        boolPtrToValue(e.IdleScaling),
		IdleTimeoutMinutes: int64PtrToValue(e.IdleTimeoutMinutes),
	}

	return model, diags
}

func apiBaseConfigToModel(b api.AutoScalingScheduleBaseConfig) models.ScheduledScalingBaseConfigModel {
	return models.ScheduledScalingBaseConfigModel{
		MinReplicaMemoryGb: int64PtrToValue(b.MinReplicaMemoryGb),
		MaxReplicaMemoryGb: int64PtrToValue(b.MaxReplicaMemoryGb),
		MinReplicas:        int64PtrToValue(b.MinReplicas),
		MaxReplicas:        int64PtrToValue(b.MaxReplicas),
		IdleScaling:        boolPtrToValue(b.IdleScaling),
		IdleTimeoutMinutes: int64PtrToValue(b.IdleTimeoutMinutes),
	}
}

func int64PtrToValue(v *int) types.Int64 {
	if v == nil {
		return types.Int64Null()
	}
	return types.Int64Value(int64(*v))
}

func boolPtrToValue(v *bool) types.Bool {
	if v == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*v)
}
