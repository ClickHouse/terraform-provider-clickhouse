//go:build alpha

package resource

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"
	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource              = &ClickPipeCdcInfrastructureResource{}
	_ resource.ResourceWithConfigure = &ClickPipeCdcInfrastructureResource{}
)

func NewClickPipeCdcInfrastructureResource() resource.Resource {
	return &ClickPipeCdcInfrastructureResource{}
}

type ClickPipeCdcInfrastructureResource struct {
	client api.Client
}

func (r *ClickPipeCdcInfrastructureResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clickpipe_cdc_infrastructure"
}

func (r *ClickPipeCdcInfrastructureResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "ClickPipe CDC Infrastructure resource. Manages scaling settings for CDC ClickPipes infrastructure shared across all DB ClickPipes in a service.\n\n" +
			"**Important**: Only one CDC infrastructure resource per service is supported. Creating multiple instances for the same service will cause conflicts.\n\n" +
			"This endpoint becomes available once at least one DB ClickPipe has been provisioned. The resource will poll for up to 10 minutes waiting for the endpoint to become available.\n\n" +
			"For billing purposes, 2 CPU cores and 8 GB of RAM correspond to one compute unit.",
		Attributes: map[string]schema.Attribute{
			"service_id": schema.StringAttribute{
				MarkdownDescription: "ClickHouse Cloud service ID where the CDC infrastructure is located.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"replica_cpu_millicores": schema.Int64Attribute{
				MarkdownDescription: "CPU in millicores for DB ClickPipes. Must be a multiple of 1000, between 1000 and 24000.",
				Required:            true,
				Validators: []validator.Int64{
					int64validator.Between(1000, 24000),
				},
			},
			"replica_memory_gb": schema.Float64Attribute{
				MarkdownDescription: "Memory in GiB for DB ClickPipes. Must be a multiple of 4, between 4 and 96. Must be 4× the CPU core count.",
				Required:            true,
				Validators: []validator.Float64{
					float64validator.Between(4, 96),
				},
			},
		},
	}
}

func (r *ClickPipeCdcInfrastructureResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
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
}

func (r *ClickPipeCdcInfrastructureResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan models.ClickPipeCdcInfrastructureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()

	// Poll for CDC scaling endpoint to become available (up to 10 minutes)
	maxWait := 10 * time.Minute
	checkAvailable := func() error {
		_, err := r.client.GetClickPipeCdcScaling(ctx, serviceID)
		return err
	}

	exponentialBackoff := backoff.NewExponentialBackOff(
		backoff.WithMaxElapsedTime(maxWait),
		backoff.WithMaxInterval(30*time.Second),
	)

	if err := backoff.Retry(checkAvailable, exponentialBackoff); err != nil {
		resp.Diagnostics.AddError(
			"CDC Infrastructure Not Available",
			"The CDC infrastructure endpoint is not yet available after waiting 10 minutes. "+
				"This endpoint becomes available once at least one DB ClickPipe has been provisioned. "+
				"Please create a Postgres CDC ClickPipe first, then try creating this resource again.\n\n"+
				fmt.Sprintf("Error: %s", err.Error()),
		)
		return
	}

	// Update the scaling settings
	scalingReq := api.ClickPipeCdcScalingRequest{
		ReplicaCpuMillicores: plan.ReplicaCpuMillicores.ValueInt64(),
		ReplicaMemoryGb:      plan.ReplicaMemoryGb.ValueFloat64(),
	}

	_, err := r.client.UpdateClickPipeCdcScaling(ctx, serviceID, scalingReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating CDC Infrastructure",
			fmt.Sprintf("Could not create CDC infrastructure: %s", err.Error()),
		)
		return
	}

	// Wait for the scaling to be applied (typically takes 2-5 minutes)
	const maxWaitTime = 10 * time.Minute
	scaling, err := r.client.WaitForClickPipeCdcScaling(ctx, serviceID, plan.ReplicaCpuMillicores.ValueInt64(), plan.ReplicaMemoryGb.ValueFloat64(), maxWaitTime)
	if err != nil {
		resp.Diagnostics.AddWarning(
			"CDC scaling change accepted but not yet applied",
			fmt.Sprintf("CDC scaling change was accepted by the API but hasn't been fully applied after %v. "+
				"The infrastructure will eventually reach the desired state. Error: %s", maxWaitTime, err),
		)
		// Still set the state to the desired values since API accepted them
		plan.ReplicaCpuMillicores = types.Int64Value(plan.ReplicaCpuMillicores.ValueInt64())
		plan.ReplicaMemoryGb = types.Float64Value(plan.ReplicaMemoryGb.ValueFloat64())
	} else {
		// Update plan with actual applied values from API
		plan.ReplicaCpuMillicores = types.Int64Value(scaling.ReplicaCpuMillicores)
		plan.ReplicaMemoryGb = types.Float64Value(scaling.ReplicaMemoryGb)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ClickPipeCdcInfrastructureResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state models.ClickPipeCdcInfrastructureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := state.ServiceID.ValueString()

	scaling, err := r.client.GetClickPipeCdcScaling(ctx, serviceID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading CDC Infrastructure",
			fmt.Sprintf("Could not read CDC infrastructure: %s", err.Error()),
		)
		return
	}

	// Update state with API values
	state.ReplicaCpuMillicores = types.Int64Value(scaling.ReplicaCpuMillicores)
	state.ReplicaMemoryGb = types.Float64Value(scaling.ReplicaMemoryGb)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *ClickPipeCdcInfrastructureResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan models.ClickPipeCdcInfrastructureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	serviceID := plan.ServiceID.ValueString()

	// Update the scaling settings
	scalingReq := api.ClickPipeCdcScalingRequest{
		ReplicaCpuMillicores: plan.ReplicaCpuMillicores.ValueInt64(),
		ReplicaMemoryGb:      plan.ReplicaMemoryGb.ValueFloat64(),
	}

	_, err := r.client.UpdateClickPipeCdcScaling(ctx, serviceID, scalingReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating CDC Infrastructure",
			fmt.Sprintf("Could not update CDC infrastructure: %s", err.Error()),
		)
		return
	}

	// Wait for the scaling to be applied (typically takes 2-5 minutes)
	const maxWaitTime = 10 * time.Minute
	scaling, err := r.client.WaitForClickPipeCdcScaling(ctx, serviceID, plan.ReplicaCpuMillicores.ValueInt64(), plan.ReplicaMemoryGb.ValueFloat64(), maxWaitTime)
	if err != nil {
		resp.Diagnostics.AddWarning(
			"CDC scaling change accepted but not yet applied",
			fmt.Sprintf("CDC scaling change was accepted by the API but hasn't been fully applied after %v. "+
				"The infrastructure will eventually reach the desired state. Error: %s", maxWaitTime, err),
		)
		// Still set the state to the desired values since API accepted them
		plan.ReplicaCpuMillicores = types.Int64Value(plan.ReplicaCpuMillicores.ValueInt64())
		plan.ReplicaMemoryGb = types.Float64Value(plan.ReplicaMemoryGb.ValueFloat64())
	} else {
		// Update plan with actual applied values from API
		plan.ReplicaCpuMillicores = types.Int64Value(scaling.ReplicaCpuMillicores)
		plan.ReplicaMemoryGb = types.Float64Value(scaling.ReplicaMemoryGb)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *ClickPipeCdcInfrastructureResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state models.ClickPipeCdcInfrastructureModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// CDC infrastructure is shared and cannot be explicitly deleted
	// Just remove from state - the infrastructure will remain until all CDC pipes are deleted
	resp.Diagnostics.AddWarning(
		"CDC Infrastructure Deletion",
		"CDC infrastructure is shared across all DB ClickPipes in the service and cannot be explicitly deleted. "+
			"The infrastructure will automatically be removed when all DB ClickPipes are deleted. "+
			"This resource has been removed from Terraform state only.",
	)
}

func (r *ClickPipeCdcInfrastructureResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// If we're destroying, no validation needed
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan models.ClickPipeCdcInfrastructureModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Validate CPU is a multiple of 1000
	cpuMillicores := plan.ReplicaCpuMillicores.ValueInt64()
	if cpuMillicores%1000 != 0 {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			fmt.Sprintf("replica_cpu_millicores must be a multiple of 1000, got %d", cpuMillicores),
		)
		return
	}

	// Validate memory is a multiple of 4
	memoryGb := plan.ReplicaMemoryGb.ValueFloat64()
	if int(memoryGb)%4 != 0 {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			fmt.Sprintf("replica_memory_gb must be a multiple of 4, got %.1f", memoryGb),
		)
		return
	}

	// Validate memory is 4x CPU cores
	cpuCores := float64(cpuMillicores) / 1000.0
	expectedMemory := cpuCores * 4
	if memoryGb != expectedMemory {
		resp.Diagnostics.AddError(
			"Invalid Configuration",
			fmt.Sprintf("replica_memory_gb must be 4× the CPU core count. With %d millicores (%.1f cores), memory must be %.1f GB, but got %.1f GB",
				cpuMillicores, cpuCores, expectedMemory, memoryGb),
		)
		return
	}
}
