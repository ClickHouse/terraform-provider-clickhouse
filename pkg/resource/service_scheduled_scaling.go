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
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/utils"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
)

var (
	_ resource.Resource                 = &ServiceScheduledScalingResource{}
	_ resource.ResourceWithConfigure    = &ServiceScheduledScalingResource{}
	_ resource.ResourceWithImportState  = &ServiceScheduledScalingResource{}
	_ resource.ResourceWithUpgradeState = &ServiceScheduledScalingResource{}
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
		// v1 changed `entries` from a set to a list; see UpgradeState.
		Version:             1,
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
				Description: "Recurring scaling windows. The server rejects any pair of entries that overlap in time, so at most one window is active at any moment; otherwise base_config applies. Ordering is not significant to the server, but as a list the order is tracked in state.",
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
	resp.Diagnostics.Append(applyScheduleToStateWithPlan(ctx, schedule, plan.Entries, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ServiceScheduledScalingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	utils.AlphaWarning("clickhouse_service_scheduled_scaling", &resp.Diagnostics)
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
	resp.Diagnostics.Append(applyScheduleToStateWithPlan(ctx, schedule, plan.Entries, &plan)...)
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

// scheduledScalingModelV0 is the v0 state layout, where `entries` was a set.
type scheduledScalingModelV0 struct {
	ID         types.String `tfsdk:"id"`
	ServiceID  types.String `tfsdk:"service_id"`
	Entries    types.Set    `tfsdk:"entries"`
	BaseConfig types.Object `tfsdk:"base_config"`
}

// scheduledScalingSchemaV0 mirrors the v0 schema's types so prior state can be
// decoded. Only attribute types matter here (validators, descriptions, and plan
// modifiers do not affect decoding), so this is intentionally minimal.
func scheduledScalingSchemaV0() schema.Schema {
	entryAttrs := map[string]schema.Attribute{
		"name":                  schema.StringAttribute{Required: true},
		"weekdays":              schema.SetAttribute{Required: true, ElementType: types.Int64Type},
		"start_hour_utc":        schema.Int64Attribute{Required: true},
		"end_hour_utc":          schema.Int64Attribute{Required: true},
		"min_replica_memory_gb": schema.Int64Attribute{Optional: true, Computed: true},
		"max_replica_memory_gb": schema.Int64Attribute{Optional: true, Computed: true},
		"min_replicas":          schema.Int64Attribute{Optional: true, Computed: true},
		"max_replicas":          schema.Int64Attribute{Optional: true, Computed: true},
		"idle_scaling":          schema.BoolAttribute{Optional: true, Computed: true},
		"idle_timeout_minutes":  schema.Int64Attribute{Optional: true, Computed: true},
	}
	return schema.Schema{
		Version: 0,
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true},
			"service_id": schema.StringAttribute{Required: true},
			"entries": schema.SetNestedAttribute{
				Required:     true,
				NestedObject: schema.NestedAttributeObject{Attributes: entryAttrs},
			},
			"base_config": schema.SingleNestedAttribute{
				Computed: true,
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

// UpgradeState migrates v0 state (entries as a set) to v1 (entries as a list).
// The entry object type is unchanged, so the collection is simply re-wrapped.
func (r *ServiceScheduledScalingResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	priorSchema := scheduledScalingSchemaV0()
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &priorSchema,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior scheduledScalingModelV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				entryType := models.ScheduledScalingEntryModel{}.ObjectType()
				entries := types.ListNull(entryType)
				if !prior.Entries.IsNull() && !prior.Entries.IsUnknown() {
					var entryModels []models.ScheduledScalingEntryModel
					resp.Diagnostics.Append(prior.Entries.ElementsAs(ctx, &entryModels, false)...)
					if resp.Diagnostics.HasError() {
						return
					}
					values := make([]attr.Value, len(entryModels))
					for i, em := range entryModels {
						values[i] = em.ObjectValue()
					}
					list, d := types.ListValue(entryType, values)
					resp.Diagnostics.Append(d...)
					if resp.Diagnostics.HasError() {
						return
					}
					entries = list
				}

				resp.Diagnostics.Append(resp.State.Set(ctx, &models.ServiceScheduledScalingResourceModel{
					ID:         prior.ID,
					ServiceID:  prior.ServiceID,
					Entries:    entries,
					BaseConfig: prior.BaseConfig,
				})...)
			},
		},
	}
}

func (r *ServiceScheduledScalingResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	utils.AlphaWarning("clickhouse_service_scheduled_scaling", &resp.Diagnostics)
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
	for _, e := range entries {
		entryRef := fmt.Sprintf("Entry %q", e.Name.ValueString())

		if !e.StartHourUtc.IsNull() && !e.StartHourUtc.IsUnknown() && !e.EndHourUtc.IsNull() && !e.EndHourUtc.IsUnknown() {
			if e.StartHourUtc.ValueInt64() == e.EndHourUtc.ValueInt64() {
				diags.AddAttributeError(
					entriesPath,
					"start_hour_utc and end_hour_utc must differ",
					fmt.Sprintf("%s has a zero-duration window.", entryRef),
				)
			}
		}

		minMemSet := !e.MinReplicaMemoryGb.IsNull() && !e.MinReplicaMemoryGb.IsUnknown()
		maxMemSet := !e.MaxReplicaMemoryGb.IsNull() && !e.MaxReplicaMemoryGb.IsUnknown()
		if minMemSet && maxMemSet && e.MinReplicaMemoryGb.ValueInt64() > e.MaxReplicaMemoryGb.ValueInt64() {
			diags.AddAttributeError(
				entriesPath,
				"min_replica_memory_gb must be <= max_replica_memory_gb",
				fmt.Sprintf("%s has min=%d, max=%d.", entryRef, e.MinReplicaMemoryGb.ValueInt64(), e.MaxReplicaMemoryGb.ValueInt64()),
			)
		}

		minRepSet := !e.MinReplicas.IsNull() && !e.MinReplicas.IsUnknown()
		maxRepSet := !e.MaxReplicas.IsNull() && !e.MaxReplicas.IsUnknown()
		if minRepSet && maxRepSet && e.MinReplicas.ValueInt64() != e.MaxReplicas.ValueInt64() {
			diags.AddAttributeError(
				entriesPath,
				"min_replicas must equal max_replicas",
				fmt.Sprintf("%s has min=%d, max=%d. The scheduled scaling API currently requires a fixed replica count per entry.", entryRef, e.MinReplicas.ValueInt64(), e.MaxReplicas.ValueInt64()),
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
// Terraform state model, taking entry contents from the server response. Used
// on Read, where no plan is available to reconcile against.
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

	applyBaseConfigToState(schedule, state)
	return diags
}

// applyScheduleToStateWithPlan is the Create/Update counterpart of
// applyScheduleToState. It reconciles the server response against the plan so
// that values the user set explicitly survive server-side normalization. The
// server drops the idle fields for a non-idle entry (idle_scaling=false), so a
// server-only mapping would turn a planned idle_scaling=false into null and
// trip Terraform's "produced inconsistent result after apply" check.
func applyScheduleToStateWithPlan(ctx context.Context, schedule *api.AutoScalingSchedule, planEntries types.List, state *models.ServiceScheduledScalingResourceModel) diag.Diagnostics {
	entriesList, diags := reconcileEntriesWithPlan(ctx, schedule, planEntries)
	if diags.HasError() {
		return diags
	}
	state.Entries = entriesList

	applyBaseConfigToState(schedule, state)
	return diags
}

func applyBaseConfigToState(schedule *api.AutoScalingSchedule, state *models.ServiceScheduledScalingResourceModel) {
	if schedule.BaseConfig != nil {
		state.BaseConfig = apiBaseConfigToModel(*schedule.BaseConfig).ObjectValue()
	} else {
		state.BaseConfig = types.ObjectNull(models.ScheduledScalingBaseConfigModel{}.ObjectType().AttrTypes)
	}
}

// reconcileEntriesWithPlan builds the entries list for state by keeping each
// planned value the user set (known in the plan) and only falling back to the
// server's echoed value where the plan left a field unset (unknown). Entries
// correlate to the server response by index: the POST replaces the full
// schedule and the server preserves entry order.
func reconcileEntriesWithPlan(ctx context.Context, schedule *api.AutoScalingSchedule, planEntries types.List) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	entryType := models.ScheduledScalingEntryModel{}.ObjectType()

	var planModels []models.ScheduledScalingEntryModel
	diags.Append(planEntries.ElementsAs(ctx, &planModels, false)...)
	if diags.HasError() {
		return types.ListNull(entryType), diags
	}

	values := make([]attr.Value, len(planModels))
	for i, pm := range planModels {
		// When the server returns a matching entry, use its values to resolve
		// fields the user left unset; otherwise resolve them to null.
		server := models.ScheduledScalingEntryModel{
			MinReplicaMemoryGb: types.Int64Null(),
			MaxReplicaMemoryGb: types.Int64Null(),
			MinReplicas:        types.Int64Null(),
			MaxReplicas:        types.Int64Null(),
			IdleScaling:        types.BoolNull(),
			IdleTimeoutMinutes: types.Int64Null(),
		}
		if i < len(schedule.Entries) {
			sm, d := apiEntryToModel(schedule.Entries[i])
			diags.Append(d...)
			if diags.HasError() {
				return types.ListNull(entryType), diags
			}
			server = sm
		}

		merged := models.ScheduledScalingEntryModel{
			// Required fields are always known in the plan.
			Name:         pm.Name,
			Weekdays:     pm.Weekdays,
			StartHourUtc: pm.StartHourUtc,
			EndHourUtc:   pm.EndHourUtc,
			// Optional+Computed: keep the planned value when the user set one.
			MinReplicaMemoryGb: pickInt64(pm.MinReplicaMemoryGb, server.MinReplicaMemoryGb),
			MaxReplicaMemoryGb: pickInt64(pm.MaxReplicaMemoryGb, server.MaxReplicaMemoryGb),
			MinReplicas:        pickInt64(pm.MinReplicas, server.MinReplicas),
			MaxReplicas:        pickInt64(pm.MaxReplicas, server.MaxReplicas),
			IdleScaling:        pickBool(pm.IdleScaling, server.IdleScaling),
			IdleTimeoutMinutes: pickInt64(pm.IdleTimeoutMinutes, server.IdleTimeoutMinutes),
		}
		values[i] = merged.ObjectValue()
	}

	entriesList, d := types.ListValue(entryType, values)
	diags.Append(d...)
	if diags.HasError() {
		return types.ListNull(entryType), diags
	}
	return entriesList, diags
}

// pickInt64 returns the planned value unless the user left it unset (unknown),
// in which case the server-resolved value is used.
func pickInt64(plan, server types.Int64) types.Int64 {
	if plan.IsUnknown() {
		return server
	}
	return plan
}

func pickBool(plan, server types.Bool) types.Bool {
	if plan.IsUnknown() {
		return server
	}
	return plan
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
		// The server omits the idle fields for a non-idle entry; treat a missing
		// idle_scaling as its effective value, false, so a refresh of an entry
		// written with idle_scaling=false does not show perpetual drift.
		IdleScaling:        boolPtrToValueDefault(e.IdleScaling, false),
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

func boolPtrToValueDefault(v *bool, def bool) types.Bool {
	if v == nil {
		return types.BoolValue(def)
	}
	return types.BoolValue(*v)
}
