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

	entries, planModels, d := planEntriesToAPI(ctx, plan.Entries)
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
	resp.Diagnostics.Append(applyScheduleToStateWithPlan(schedule, planModels, &plan)...)
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
	resp.Diagnostics.Append(applyScheduleToState(ctx, schedule, &state)...)
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

	entries, planModels, d := planEntriesToAPI(ctx, plan.Entries)
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
	resp.Diagnostics.Append(applyScheduleToStateWithPlan(schedule, planModels, &plan)...)
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

// entryObjectTypeV1 is the entry object type frozen as of schema v1. The v0->v1
// upgrader targets this rather than models.ScheduledScalingEntryModel{}.ObjectType()
// so a future v2 that changes the entry shape cannot silently break this
// upgrader: the 0: upgrader must keep emitting the v1 type while re-wrapping
// v0-typed elements, and a live-model type would then mismatch.
func entryObjectTypeV1() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"name":                  types.StringType,
			"weekdays":              types.SetType{ElemType: types.Int64Type},
			"start_hour_utc":        types.Int64Type,
			"end_hour_utc":          types.Int64Type,
			"min_replica_memory_gb": types.Int64Type,
			"max_replica_memory_gb": types.Int64Type,
			"min_replicas":          types.Int64Type,
			"max_replicas":          types.Int64Type,
			"idle_scaling":          types.BoolType,
			"idle_timeout_minutes":  types.Int64Type,
		},
	}
}

// UpgradeState migrates v0 state (entries as a set) to v1 (entries as a list).
// The v0 and v1 entry object types are identical, so the collection is simply
// re-wrapped into the frozen v1 type.
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

				entryType := entryObjectTypeV1()
				entries := types.ListNull(entryType)
				if !prior.Entries.IsNull() && !prior.Entries.IsUnknown() {
					// The set's elements are already values of the frozen v1
					// element type; re-wrap them directly. This relies on the
					// v0 and v1 entry object types being identical — if a
					// future schema version changes the entry type, this
					// upgrader must decode and convert instead.
					list, d := types.ListValue(entryType, prior.Entries.Elements())
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

// planEntriesToAPI converts the Terraform plan list of entries into API
// entries. It also returns the decoded plan models so Create/Update can
// reconcile the server response against the plan without decoding twice.
func planEntriesToAPI(ctx context.Context, entriesList types.List) ([]api.AutoScalingScheduleEntry, []models.ScheduledScalingEntryModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	if entriesList.IsNull() || entriesList.IsUnknown() {
		return []api.AutoScalingScheduleEntry{}, nil, diags
	}

	var entryModels []models.ScheduledScalingEntryModel
	diags.Append(entriesList.ElementsAs(ctx, &entryModels, false)...)
	if diags.HasError() {
		return nil, nil, diags
	}

	result := make([]api.AutoScalingScheduleEntry, len(entryModels))
	for i, em := range entryModels {
		var weekdays []int64
		diags.Append(em.Weekdays.ElementsAs(ctx, &weekdays, false)...)
		if diags.HasError() {
			return nil, nil, diags
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

	return result, entryModels, diags
}

// applyScheduleToState maps an API AutoScalingSchedule response into the
// Terraform state model, taking entry contents from the server response. Used
// on Read, where no plan is available to reconcile against.
//
// Because entries is a list, element identity is positional: writing the
// response order verbatim would surface a spurious, non-converging reorder
// diff after any refresh where the server returns entries in a different
// order than the config. Entries are therefore reordered to match the prior
// state (correlated by their identity key); entries not in prior state are
// appended in server order. On import, prior state is empty and server order
// is kept.
func applyScheduleToState(ctx context.Context, schedule *api.AutoScalingSchedule, state *models.ServiceScheduledScalingResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	serverModels := make([]models.ScheduledScalingEntryModel, len(schedule.Entries))
	for i, e := range schedule.Entries {
		entryModel, d := apiEntryToModel(e)
		diags.Append(d...)
		if diags.HasError() {
			return diags
		}
		serverModels[i] = entryModel
	}

	var priorModels []models.ScheduledScalingEntryModel
	if !state.Entries.IsNull() && !state.Entries.IsUnknown() {
		diags.Append(state.Entries.ElementsAs(ctx, &priorModels, false)...)
		if diags.HasError() {
			return diags
		}
	}

	ordered := make([]models.ScheduledScalingEntryModel, 0, len(serverModels))
	used := make([]bool, len(serverModels))
	for _, pm := range priorModels {
		key := modelEntryKey(pm)
		for j, sm := range serverModels {
			if !used[j] && modelEntryKey(sm) == key {
				ordered = append(ordered, sm)
				used[j] = true
				break
			}
		}
	}
	for j, sm := range serverModels {
		if !used[j] {
			ordered = append(ordered, sm)
		}
	}

	entryValues := make([]attr.Value, len(ordered))
	for i, em := range ordered {
		entryValues[i] = em.ObjectValue()
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

// apiEntryKey / modelEntryKey compute a schedule entry's identity from its
// user-declared fields: name plus the window itself (sorted weekdays and
// hours). Names alone are not guaranteed unique, but two entries cannot cover
// the same window (the server rejects overlaps), so the combined key is. Both
// delegate to entryKey so the two representations can never drift in format —
// a divergence would silently break correlation with no compile error.
func entryKey(name string, weekdays []int, startHourUtc, endHourUtc int) string {
	wd := append([]int(nil), weekdays...)
	sort.Ints(wd)
	return fmt.Sprintf("%s|%v|%d|%d", name, wd, startHourUtc, endHourUtc)
}

func apiEntryKey(e api.AutoScalingScheduleEntry) string {
	return entryKey(e.Name, e.Weekdays, e.StartHourUtc, e.EndHourUtc)
}

func modelEntryKey(m models.ScheduledScalingEntryModel) string {
	wd := make([]int, 0, len(m.Weekdays.Elements()))
	for _, v := range m.Weekdays.Elements() {
		if iv, ok := v.(types.Int64); ok && !iv.IsNull() && !iv.IsUnknown() {
			wd = append(wd, int(iv.ValueInt64()))
		}
	}
	return entryKey(m.Name.ValueString(), wd, int(m.StartHourUtc.ValueInt64()), int(m.EndHourUtc.ValueInt64()))
}

// applyScheduleToStateWithPlan is the Create/Update counterpart of
// applyScheduleToState. It reconciles the server response against the plan so
// that values the user set explicitly survive server-side normalization: any
// field the server echoes differently from how it was sent (or not at all)
// would otherwise land in state as null and trip Terraform's "produced
// inconsistent result after apply" check. The known instance of this was
// UC-1252 (issue #611): the server normalized a vertical entry's equal
// minReplicas/maxReplicas band to numReplicas and omitted the band from the
// response. That is fixed server-side (control-plane#35956), but reconciling
// against the plan keeps Create/Update correct regardless of which fields any
// server version echoes.
//
// Each planned value the user set (known in the plan) is kept; only fields the
// user left unset (unknown) resolve from the server's echoed value. Entries
// correlate to the server response by index — the POST replaces the full
// schedule and the server preserves entry order — but the entry's identity key
// (name + window, see apiEntryKey) is verified before merging so a reordered
// response can never fill unset fields from the wrong entry (they resolve to
// null instead, which is a valid outcome for an unknown).
func applyScheduleToStateWithPlan(schedule *api.AutoScalingSchedule, planModels []models.ScheduledScalingEntryModel, state *models.ServiceScheduledScalingResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	values := make([]attr.Value, len(planModels))
	for i, pm := range planModels {
		// The zero model is all-null: the fallback when the server returned no
		// matching entry at this index.
		var server models.ScheduledScalingEntryModel
		if i < len(schedule.Entries) && apiEntryKey(schedule.Entries[i]) == modelEntryKey(pm) {
			sm, d := apiEntryToModel(schedule.Entries[i])
			diags.Append(d...)
			if diags.HasError() {
				return diags
			}
			server = sm
		}

		if pm.MinReplicaMemoryGb.IsUnknown() {
			pm.MinReplicaMemoryGb = server.MinReplicaMemoryGb
		}
		if pm.MaxReplicaMemoryGb.IsUnknown() {
			pm.MaxReplicaMemoryGb = server.MaxReplicaMemoryGb
		}
		if pm.MinReplicas.IsUnknown() {
			pm.MinReplicas = server.MinReplicas
		}
		if pm.MaxReplicas.IsUnknown() {
			pm.MaxReplicas = server.MaxReplicas
		}
		if pm.IdleScaling.IsUnknown() {
			pm.IdleScaling = server.IdleScaling
		}
		if pm.IdleTimeoutMinutes.IsUnknown() {
			pm.IdleTimeoutMinutes = server.IdleTimeoutMinutes
		}
		values[i] = pm.ObjectValue()
	}

	entriesList, d := types.ListValue(models.ScheduledScalingEntryModel{}.ObjectType(), values)
	diags.Append(d...)
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
		// The server echoes idle_scaling in practice, but treat a missing value
		// as its effective default, false, so a refresh of a non-idle entry can
		// never show perpetual drift if a server version omits it.
		IdleScaling:        types.BoolValue(e.IdleScaling != nil && *e.IdleScaling),
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
