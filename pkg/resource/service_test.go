package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/test"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestServiceResource_syncServiceState(t *testing.T) {
	ctx := context.Background()
	state := getInitialState()

	tests := []struct {
		name            string
		state           models.ServiceResourceModel
		response        *api.Service
		responseErr     error
		desiredState    models.ServiceResourceModel
		updateTimestamp bool
		wantErr         bool
	}{
		{
			name:  "Updates name field in state",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Name = "newname"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Name = types.StringValue("newname")
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update Endpoints field with mysql disabled",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Endpoints = []api.Endpoint{
					{
						Protocol: "nativesecure",
						Host:     "a.b.c.d",
						Port:     1234,
					},
					{
						Protocol: "https",
						Host:     "e.f.g.h",
						Port:     5678,
					},
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Endpoints = models.Endpoints{
					NativeSecure: models.Endpoint{
						Host: types.StringValue("a.b.c.d"),
						Port: types.Int32Value(1234),
					}.ObjectValue(),
					HTTPS: models.Endpoint{
						Host: types.StringValue("e.f.g.h"),
						Port: types.Int32Value(5678),
					}.ObjectValue(),
					MySQL: models.OptionalEndpoint{
						Enabled: types.BoolValue(false),
						Host:    types.StringNull(),
						Port:    types.Int32Null(),
					}.ObjectValue(),
				}.ObjectValue()
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update Endpoints field with mysql enabled",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Endpoints = []api.Endpoint{
					{
						Protocol: "nativesecure",
						Host:     "a.b.c.d",
						Port:     1234,
					},
					{
						Protocol: "https",
						Host:     "e.f.g.h",
						Port:     5678,
					},
					{
						Protocol: "mysql",
						Host:     "i.j.k.l",
						Port:     9012,
					},
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Endpoints = models.Endpoints{
					NativeSecure: models.Endpoint{
						Host: types.StringValue("a.b.c.d"),
						Port: types.Int32Value(1234),
					}.ObjectValue(),
					HTTPS: models.Endpoint{
						Host: types.StringValue("e.f.g.h"),
						Port: types.Int32Value(5678),
					}.ObjectValue(),
					MySQL: models.OptionalEndpoint{
						Enabled: types.BoolValue(true),
						Host:    types.StringValue("i.j.k.l"),
						Port:    types.Int32Value(9012),
					}.ObjectValue(),
				}.ObjectValue()
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Updates provider field in state",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Provider = "newprovider"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.CloudProvider = types.StringValue("newprovider")
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Updates region field in state",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Region = "newregion"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Region = types.StringValue("newregion")
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Updates tier field in state",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = "newtier"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue("newtier")
				src.AutoscalingMode = types.StringNull()
				src.BackupConfiguration = types.ObjectNull(models.BackupConfiguration{}.ObjectType().AttrTypes)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Set IdleScaling field to true",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.IdleScaling = true
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.IdleScaling = types.BoolValue(true)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Set IdleScaling field to false",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.IdleScaling = false
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.IdleScaling = types.BoolValue(false)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update IPAccessList field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.IpAccessList = []api.IpAccess{
					{
						Source:      "0.0.0.0/0",
						Description: "whitelist",
					},
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				ipAccessList := []attr.Value{
					models.IPAccessList{Source: types.StringValue("0.0.0.0/0"), Description: types.StringValue("whitelist")}.ObjectValue(),
				}

				src.IpAccessList, _ = types.ListValue(models.IPAccessList{}.ObjectType(), ipAccessList)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Updates MinReplicaMemoryGb field when in production tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierProduction
				minReplicaMemory := 10
				src.MinReplicaMemoryGb = &minReplicaMemory
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierProduction)
				src.AutoscalingMode = types.StringValue(api.AutoscalingModeVertical)
				src.MinReplicaMemoryGb = types.Int64Value(10)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Does not update MinTotalMemoryGb field when in development tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierDevelopment
				minReplicaMemory := 10
				src.MinReplicaMemoryGb = &minReplicaMemory
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierDevelopment)
				src.AutoscalingMode = types.StringNull()
				src.MinReplicaMemoryGb = types.Int64{}
				src.BackupConfiguration = types.ObjectNull(models.BackupConfiguration{}.ObjectType().AttrTypes)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Updates MaxReplicaMemoryGb field when in production tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierProduction
				maxReplicaMemory := 10
				src.MaxReplicaMemoryGb = &maxReplicaMemory
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierProduction)
				src.AutoscalingMode = types.StringValue(api.AutoscalingModeVertical)
				src.MaxReplicaMemoryGb = types.Int64Value(10)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Does not update MaxReplicaMemoryGb field when in development tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierDevelopment
				maxReplicaMemory := 10
				src.MaxReplicaMemoryGb = &maxReplicaMemory
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierDevelopment)
				src.AutoscalingMode = types.StringNull()
				src.MaxTotalMemoryGb = types.Int64{}
				src.BackupConfiguration = types.ObjectNull(models.BackupConfiguration{}.ObjectType().AttrTypes)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Updates NumReplicas field when in production tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierProduction
				replicas := 3
				src.NumReplicas = &replicas
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierProduction)
				src.AutoscalingMode = types.StringValue(api.AutoscalingModeVertical)
				src.NumReplicas = types.Int64Value(3)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Reflects the horizontal autoscaling mode the API returns in production tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierProduction
				minReplicas := 2
				maxReplicas := 6
				replicaMemoryGb := 16
				src.AutoscalingMode = api.AutoscalingModeHorizontal
				src.MinReplicas = &minReplicas
				src.MaxReplicas = &maxReplicas
				src.MinReplicaMemoryGb = &replicaMemoryGb
				src.MaxReplicaMemoryGb = &replicaMemoryGb
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierProduction)
				src.AutoscalingMode = types.StringValue(api.AutoscalingModeHorizontal)
				src.MinReplicas = types.Int64Value(2)
				src.MaxReplicas = types.Int64Value(6)
				src.MinReplicaMemoryGb = types.Int64Value(16)
				src.MaxReplicaMemoryGb = types.Int64Value(16)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Horizontal read that echoes a concrete num_replicas nulls it (matches the plan's off-mode nulling)",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierProduction
				minReplicas := 2
				maxReplicas := 6
				numReplicas := 4
				replicaMemoryGb := 16
				src.AutoscalingMode = api.AutoscalingModeHorizontal
				src.MinReplicas = &minReplicas
				src.MaxReplicas = &maxReplicas
				// A horizontal service whose read echoes the live count in num_replicas: ModifyPlan force-nulls
				// num_replicas for horizontal, so the read must null it too or the apply is inconsistent.
				src.NumReplicas = &numReplicas
				src.MinReplicaMemoryGb = &replicaMemoryGb
				src.MaxReplicaMemoryGb = &replicaMemoryGb
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierProduction)
				src.AutoscalingMode = types.StringValue(api.AutoscalingModeHorizontal)
				src.MinReplicas = types.Int64Value(2)
				src.MaxReplicas = types.Int64Value(6)
				src.NumReplicas = types.Int64Null()
				src.MinReplicaMemoryGb = types.Int64Value(16)
				src.MaxReplicaMemoryGb = types.Int64Value(16)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Derives horizontal from a distinct replica band when the API omits the mode",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierProduction
				minReplicas := 2
				maxReplicas := 6
				replicaMemoryGb := 16
				// autoscalingMode intentionally left empty — a response from an API predating the explicit
				// field falls back to deriving horizontal from a distinct (min != max) band.
				src.MinReplicas = &minReplicas
				src.MaxReplicas = &maxReplicas
				src.MinReplicaMemoryGb = &replicaMemoryGb
				src.MaxReplicaMemoryGb = &replicaMemoryGb
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierProduction)
				src.AutoscalingMode = types.StringValue(api.AutoscalingModeHorizontal)
				src.MinReplicas = types.Int64Value(2)
				src.MaxReplicas = types.Int64Value(6)
				src.MinReplicaMemoryGb = types.Int64Value(16)
				src.MaxReplicaMemoryGb = types.Int64Value(16)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Falls back to vertical when the API response has a partial band (min only, no mode)",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierProduction
				minReplicas := 3
				replicaMemoryGb := 16
				// Only MinReplicas set (no MaxReplicas, no mode): the min != max horizontal fallback needs BOTH
				// bounds, so a partial band resolves vertical. The off-mode nulling then clears the whole band
				// (a vertical service's count is num_replicas), so a lopsided min-only band never reaches state.
				src.MinReplicas = &minReplicas
				src.MinReplicaMemoryGb = &replicaMemoryGb
				src.MaxReplicaMemoryGb = &replicaMemoryGb
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierProduction)
				src.AutoscalingMode = types.StringValue(api.AutoscalingModeVertical)
				src.MinReplicas = types.Int64Null()
				src.MaxReplicas = types.Int64Null()
				src.MinReplicaMemoryGb = types.Int64Value(16)
				src.MaxReplicaMemoryGb = types.Int64Value(16)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Does not update NumReplicas field when in development tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierDevelopment
				replicas := 3
				src.NumReplicas = &replicas
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierDevelopment)
				src.AutoscalingMode = types.StringNull()
				src.NumReplicas = types.Int64{}
				src.BackupConfiguration = types.ObjectNull(models.BackupConfiguration{}.ObjectType().AttrTypes)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update IdleTimeoutMinutes field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.IdleScaling = true
				timeout := 25
				src.IdleTimeoutMinutes = &timeout
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.IdleScaling = types.BoolValue(true)
				src.IdleTimeoutMinutes = types.Int64Value(25)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Nullify IdleTimeoutMinutes when idle scaling is false",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.IdleScaling = false
				timeout := 25
				src.IdleTimeoutMinutes = &timeout
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.IdleScaling = types.BoolValue(false)
				src.IdleTimeoutMinutes = types.Int64Null()
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update IAMRole field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.IAMRole = "newiamrole"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.IAMRole = types.StringValue("newiamrole")
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update PrivateEndpointConfig field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.PrivateEndpointConfig = &api.ServicePrivateEndpointConfig{
					EndpointServiceId:  "newendpointserviceid",
					PrivateDnsHostname: "new.endpoint.service.hostname",
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.PrivateEndpointConfig = models.PrivateEndpointConfig{
					EndpointServiceID:  types.StringValue("newendpointserviceid"),
					PrivateDNSHostname: types.StringValue("new.endpoint.service.hostname"),
				}.ObjectValue()
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update EncryptionKey field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.EncryptionKey = "newencryptionkey"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.EncryptionKey = types.StringValue("newencryptionkey")
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update EncryptionAssumedRoleIdentifier field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.EncryptionAssumedRoleIdentifier = "newroleidentifier"
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.EncryptionAssumedRoleIdentifier = types.StringValue("newroleidentifier")
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update EncryptionKey field to null",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.EncryptionKey = ""
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.EncryptionKey = types.StringNull()
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update EncryptionAssumedRoleIdentifier field to null",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.EncryptionAssumedRoleIdentifier = ""
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.EncryptionAssumedRoleIdentifier = types.StringNull()
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update BackupConfiguration.BackupPeriodInHours field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				ten := int32(10)
				src.BackupConfiguration = &api.BackupConfiguration{
					BackupPeriodInHours: &ten,
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.BackupConfiguration = models.BackupConfiguration{
					BackupPeriodInHours: types.Int32Value(10),
				}.ObjectValue()
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update BackupConfiguration.BackupRetentionPeriodInHours field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				ten := int32(10)
				src.BackupConfiguration = &api.BackupConfiguration{
					BackupRetentionPeriodInHours: &ten,
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.BackupConfiguration = models.BackupConfiguration{
					BackupRetentionPeriodInHours: types.Int32Value(10),
				}.ObjectValue()
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update BackupConfiguration.BackupStartTime field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				chg := "changed"
				src.BackupConfiguration = &api.BackupConfiguration{
					BackupStartTime: &chg,
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.BackupConfiguration = models.BackupConfiguration{
					BackupStartTime: types.StringValue("changed"),
				}.ObjectValue()
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update Tags field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tags = []api.Tag{
					{
						Key:   "cost-center",
						Value: "business-a",
					},
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				tagsMap := make(map[string]attr.Value)
				tagsMap["cost-center"] = types.StringValue("business-a")
				src.Tags, _ = types.MapValue(types.StringType, tagsMap)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Tags field empty array returns null map",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tags = []api.Tag{}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tags = types.MapNull(types.StringType)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)

			apiClientMock := api.NewClientMock(mc).
				GetServiceMock.
				Expect(context.Background(), tt.state.ID.ValueString()).
				Return(tt.response, tt.responseErr)

			r := &ServiceResource{
				client: apiClientMock,
			}

			err := r.syncServiceState(ctx, &tt.state, tt.updateTimestamp)

			if (err != nil) != tt.wantErr {
				t.Errorf("%s error does not match:\ngot  = %v\nwant = %v", tt.name, err, tt.wantErr)
			}

			if !tt.state.Equals(tt.desiredState) {
				t.Errorf("%s state does not match:\ngot  = %v\nwant = %v\n", tt.name, tt.state, tt.desiredState)
			}
		})
	}
}

func getInitialState() models.ServiceResourceModel {
	uuid := "773bb8b4-34e8-4ecf-8e23-4f7e20aa14b3"

	endpoints := models.Endpoints{
		NativeSecure: models.Endpoint{
			Host: types.StringNull(),
			Port: types.Int32Null(),
		}.ObjectValue(),
		HTTPS: models.Endpoint{
			Host: types.StringNull(),
			Port: types.Int32Null(),
		}.ObjectValue(),
		MySQL: models.OptionalEndpoint{
			Enabled: types.BoolValue(false),
			Host:    types.StringNull(),
			Port:    types.Int32Null(),
		}.ObjectValue(),
	}.ObjectValue()
	ipAccessList, _ := types.ListValue(models.IPAccessList{}.ObjectType(), []attr.Value{})
	privateEndpointConfig := models.PrivateEndpointConfig{
		EndpointServiceID:  types.StringValue(""),
		PrivateDNSHostname: types.StringValue(""),
	}.ObjectValue()
	backupConfiguration := models.BackupConfiguration{
		BackupPeriodInHours:          types.Int32{},
		BackupRetentionPeriodInHours: types.Int32{},
		BackupStartTime:              types.String{},
	}.ObjectValue()
	tags := types.MapNull(types.StringType)

	state := models.ServiceResourceModel{
		ID:                              types.StringValue(uuid),
		BYOCID:                          types.StringNull(),
		DataWarehouseID:                 types.StringNull(),
		IsPrimary:                       types.BoolValue(true),
		ReadOnly:                        types.BoolValue(false),
		Name:                            types.StringValue(""),
		Password:                        types.String{},
		PasswordHash:                    types.String{},
		DoubleSha1PasswordHash:          types.String{},
		PasswordWO:                      types.StringNull(),
		PasswordWOVersion:               types.Int64Null(),
		Endpoints:                       endpoints,
		CloudProvider:                   types.StringValue(""),
		Region:                          types.StringValue(""),
		Tier:                            types.StringValue("production"),
		ReleaseChannel:                  types.StringValue("default"),
		IdleScaling:                     types.BoolValue(false),
		IpAccessList:                    ipAccessList,
		MinTotalMemoryGb:                types.Int64{},
		MaxTotalMemoryGb:                types.Int64{},
		NumReplicas:                     types.Int64{},
		AutoscalingMode:                 types.StringValue(api.AutoscalingModeVertical),
		MinReplicas:                     types.Int64Null(),
		MaxReplicas:                     types.Int64Null(),
		IdleTimeoutMinutes:              types.Int64{},
		IAMRole:                         types.StringValue(""),
		PrivateEndpointConfig:           privateEndpointConfig,
		EncryptionKey:                   types.StringNull(),
		EncryptionAssumedRoleIdentifier: types.StringNull(),
		BackupConfiguration:             backupConfiguration,
		TransparentEncryptionData: models.TransparentEncryptionData{
			Enabled: types.BoolValue(false),
			RoleID:  types.StringNull(),
		}.ObjectValue(),
		ComplianceType:  types.StringNull(),
		Tags:            tags,
		EnableCoreDumps: types.BoolNull(),
	}

	return state
}

func getBaseResponse(id string) api.Service {
	trueVal := true
	return api.Service{
		Id:        id,
		IsPrimary: &trueVal,
		// Name: "newname",
		// Provider:                        "",
		// Region:                          "",

		Tier:        "production",
		IdleScaling: false,
		// IPAccessList:                    nil,
		// MinTotalMemoryGb:                nil,
		// MaxTotalMemoryGb:                nil,
		// NumReplicas:                     nil,
		// IdleTimeoutMinutes:              nil,
		// State:                           "",
		// Endpoints:                       nil,
		// IAMRole:                         "",
		PrivateEndpointConfig: &api.ServicePrivateEndpointConfig{
			EndpointServiceId:  "",
			PrivateDnsHostname: "",
		},
		// EncryptionKey:                   "",
		// EncryptionAssumedRoleIdentifier: "",
		BackupConfiguration: &api.BackupConfiguration{
			BackupPeriodInHours:          nil,
			BackupRetentionPeriodInHours: nil,
			BackupStartTime:              nil,
		},
		HasTransparentDataEncryption:   false,
		TransparentEncryptionDataKeyID: "",
		ReleaseChannel:                 "default",
		EnableCoreDumps:                nil,
	}
}

func TestComputeTagChanges(t *testing.T) {
	tests := []struct {
		name        string
		currentTags map[string]string
		desiredTags map[string]string
		wantAdd     []api.Tag
		wantRemove  []api.Tag
	}{
		{
			name:        "Returns empty slices when both maps are empty",
			currentTags: map[string]string{},
			desiredTags: map[string]string{},
			wantAdd:     []api.Tag{},
			wantRemove:  []api.Tag{},
		},
		{
			name:        "Returns empty slices when tags are identical",
			currentTags: map[string]string{"env": "prod", "team": "backend"},
			desiredTags: map[string]string{"team": "backend", "env": "prod"},
			wantAdd:     []api.Tag{},
			wantRemove:  []api.Tag{},
		},
		{
			name:        "Adds new tags when current is empty",
			currentTags: map[string]string{},
			desiredTags: map[string]string{"env": "prod", "team": "backend"},
			wantAdd: []api.Tag{
				{Key: "env", Value: "prod"},
				{Key: "team", Value: "backend"},
			},
			wantRemove: []api.Tag{},
		},
		{
			name:        "Removes all tags when desired is empty",
			currentTags: map[string]string{"env": "prod", "team": "backend"},
			desiredTags: map[string]string{},
			wantAdd:     []api.Tag{},
			wantRemove: []api.Tag{
				{Key: "env", Value: "prod"},
				{Key: "team", Value: "backend"},
			},
		},
		{
			name:        "Updates tag value by removing old and adding new",
			currentTags: map[string]string{"env": "staging"},
			desiredTags: map[string]string{"env": "production"},
			wantAdd: []api.Tag{
				{Key: "env", Value: "production"},
			},
			wantRemove: []api.Tag{
				{Key: "env", Value: "staging"},
			},
		},
		{
			name:        "Handles mixed operations with add, remove, update and keep",
			currentTags: map[string]string{"env": "staging", "team": "backend", "region": "us-west"},
			desiredTags: map[string]string{"env": "production", "region": "us-west", "owner": "alice"},
			wantAdd: []api.Tag{
				{Key: "env", Value: "production"},
				{Key: "owner", Value: "alice"},
			},
			wantRemove: []api.Tag{
				{Key: "env", Value: "staging"},
				{Key: "team", Value: "backend"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			add, remove := computeTagChanges(tt.currentTags, tt.desiredTags)

			if !tagsEqual(add, tt.wantAdd) {
				t.Errorf("'%s' add tags do not match:\ngot  = %v\nwant = %v", tt.name, add, tt.wantAdd)
			}

			if !tagsEqual(remove, tt.wantRemove) {
				t.Errorf("'%s' remove tags do not match:\ngot  = %v\nwant = %v", tt.name, remove, tt.wantRemove)
			}
		})
	}
}

// tagsEqual compares two slices of tags as sets (order-independent)
func tagsEqual(a, b []api.Tag) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]string, len(a))
	for _, tag := range a {
		aMap[tag.Key] = tag.Value
	}

	for _, tag := range b {
		value, exists := aMap[tag.Key]
		if !exists || value != tag.Value {
			return false
		}
	}

	return true
}

// buildServiceSchema compiles the resource schema for plan-layer tests.
func buildServiceSchema(t *testing.T, ctx context.Context, r *ServiceResource) schema.Schema {
	t.Helper()
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("building resource schema failed: %v", schemaResp.Diagnostics.Errors())
	}
	return schemaResp.Schema
}

// encodableInitialState returns a model that round-trips through the resource schema. getInitialState
// leaves query_api_endpoints as a bare object (fine for Equals-based tests, but tfsdk.Plan.Set needs a
// properly-typed null), so set it here.
func encodableInitialState() models.ServiceResourceModel {
	return test.NewUpdater(getInitialState()).Update(func(s *models.ServiceResourceModel) {
		s.QueryAPIEndpoints = types.ObjectNull(models.QueryAPIEndpoints{}.ObjectType().AttrTypes)
	}).Get()
}

func detailContains(diags diag.Diagnostics, substr string) bool {
	for _, d := range diags.Errors() {
		if strings.Contains(d.Detail(), substr) {
			return true
		}
	}
	return false
}

func TestServiceResource_ValidateConfig(t *testing.T) {
	ctx := context.Background()
	r := &ServiceResource{}
	sch := buildServiceSchema(t, ctx, r)

	run := func(t *testing.T, cfg models.ServiceResourceModel) diag.Diagnostics {
		t.Helper()
		planVal := tfsdk.Plan{Schema: sch}
		if d := planVal.Set(ctx, &cfg); d.HasError() {
			t.Fatalf("encoding config failed: %v", d.Errors())
		}
		resp := &resource.ValidateConfigResponse{}
		r.ValidateConfig(ctx, resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: sch, Raw: planVal.Raw}}, resp)
		return resp.Diagnostics
	}

	// A horizontal config: the replica band plus a fixed (min == max) per-replica memory.
	horizontal := func(mut func(*models.ServiceResourceModel)) models.ServiceResourceModel {
		return test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringValue(api.AutoscalingModeHorizontal)
			s.MinReplicas = types.Int64Value(3)
			s.MaxReplicas = types.Int64Value(10)
			s.MinReplicaMemoryGb = types.Int64Value(16)
			s.MaxReplicaMemoryGb = types.Int64Value(16)
			s.NumReplicas = types.Int64Null()
			s.MinTotalMemoryGb = types.Int64Null()
			s.MaxTotalMemoryGb = types.Int64Null()
			if mut != nil {
				mut(s)
			}
		}).Get()
	}

	t.Run("valid horizontal config passes", func(t *testing.T) {
		if diags := run(t, horizontal(nil)); diags.HasError() {
			t.Errorf("valid horizontal config should pass, got: %v", diags.Errors())
		}
	})

	t.Run("valid vertical config passes", func(t *testing.T) {
		cfg := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringValue(api.AutoscalingModeVertical)
			s.NumReplicas = types.Int64Value(3)
			s.MinReplicaMemoryGb = types.Int64Value(8)
			s.MaxReplicaMemoryGb = types.Int64Value(32)
			s.MinReplicas = types.Int64Null()
			s.MaxReplicas = types.Int64Null()
		}).Get()
		if diags := run(t, cfg); diags.HasError() {
			t.Errorf("valid vertical config should pass, got: %v", diags.Errors())
		}
	})

	// The mode ↔ field contradiction rules (band-without-mode, num_replicas-in-horizontal, fixed-vs-ranged
	// memory) are owned by the API, not re-checked here — so a band without an explicit mode, or num_replicas
	// alongside horizontal, must NOT be rejected at plan time. ValidateConfig keeps only ordering checks.
	t.Run("band without an explicit mode is not rejected at plan time", func(t *testing.T) {
		cfg := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringNull()
			s.MinReplicas = types.Int64Value(3)
			s.MaxReplicas = types.Int64Value(10)
			s.MinReplicaMemoryGb = types.Int64Value(16)
			s.MaxReplicaMemoryGb = types.Int64Value(16)
			s.NumReplicas = types.Int64Null()
		}).Get()
		if diags := run(t, cfg); diags.HasError() {
			t.Errorf("a band without an explicit mode must defer to the server, not fail at plan time; got: %v", diags.Errors())
		}
	})

	t.Run("inverted replica band (min > max) is rejected", func(t *testing.T) {
		cfg := horizontal(func(s *models.ServiceResourceModel) {
			s.MinReplicas = types.Int64Value(10)
			s.MaxReplicas = types.Int64Value(3)
		})
		if !detailContains(run(t, cfg), "min_replicas must be less than or equal to max_replicas") {
			t.Errorf("an inverted replica band must be rejected")
		}
	})

	t.Run("inverted per-replica memory range (min > max) is rejected", func(t *testing.T) {
		cfg := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringValue(api.AutoscalingModeVertical)
			s.MinReplicaMemoryGb = types.Int64Value(32)
			s.MaxReplicaMemoryGb = types.Int64Value(8)
		}).Get()
		if !detailContains(run(t, cfg), "min_replica_memory_gb must be less than or equal to max_replica_memory_gb") {
			t.Errorf("an inverted per-replica memory range must be rejected")
		}
	})
}

func TestResolveIsHorizontal(t *testing.T) {
	horizontalState := models.ServiceResourceModel{AutoscalingMode: types.StringValue(api.AutoscalingModeHorizontal)}
	verticalState := models.ServiceResourceModel{AutoscalingMode: types.StringValue(api.AutoscalingModeVertical)}

	cases := []struct {
		name     string
		config   models.ServiceResourceModel
		state    models.ServiceResourceModel
		hasState bool
		want     bool
	}{
		{"explicit horizontal token wins", models.ServiceResourceModel{AutoscalingMode: types.StringValue(api.AutoscalingModeHorizontal)}, verticalState, true, true},
		{"explicit vertical token wins", models.ServiceResourceModel{AutoscalingMode: types.StringValue(api.AutoscalingModeVertical)}, horizontalState, true, false},
		{"distinct band, no mode, is horizontal", models.ServiceResourceModel{MinReplicas: types.Int64Value(3), MaxReplicas: types.Int64Value(10)}, models.ServiceResourceModel{}, false, true},
		{"equal band, no mode, is vertical", models.ServiceResourceModel{MinReplicas: types.Int64Value(2), MaxReplicas: types.Int64Value(2)}, models.ServiceResourceModel{}, false, false},
		{"num_replicas, no mode, is vertical", models.ServiceResourceModel{NumReplicas: types.Int64Value(3)}, models.ServiceResourceModel{}, false, false},
		{"no scaling signal keeps horizontal state", models.ServiceResourceModel{}, horizontalState, true, true},
		{"no scaling signal keeps vertical state", models.ServiceResourceModel{}, verticalState, true, false},
		{"no scaling signal, no state, defaults vertical", models.ServiceResourceModel{}, models.ServiceResourceModel{}, false, false},
		{"band resize on a horizontal service stays horizontal", models.ServiceResourceModel{MinReplicas: types.Int64Value(3), MaxReplicas: types.Int64Value(15)}, horizontalState, true, true},
		{"equal band on a horizontal state keeps horizontal (no silent flip)", models.ServiceResourceModel{MinReplicas: types.Int64Value(5), MaxReplicas: types.Int64Value(5)}, horizontalState, true, true},
		{"equal band on a vertical state stays vertical", models.ServiceResourceModel{MinReplicas: types.Int64Value(5), MaxReplicas: types.Int64Value(5)}, verticalState, true, false},
		{"explicit vertical token beats a distinct band", models.ServiceResourceModel{AutoscalingMode: types.StringValue(api.AutoscalingModeVertical), MinReplicas: types.Int64Value(3), MaxReplicas: types.Int64Value(10)}, verticalState, true, false},
		{"band with one bound unknown is horizontal", models.ServiceResourceModel{MinReplicas: types.Int64Value(3), MaxReplicas: types.Int64Unknown()}, verticalState, true, true},
		{"band with both bounds unknown is horizontal", models.ServiceResourceModel{MinReplicas: types.Int64Unknown(), MaxReplicas: types.Int64Unknown()}, verticalState, true, true},
		{"half-band (min set, max omitted) is not a horizontal signal", models.ServiceResourceModel{MinReplicas: types.Int64Value(3)}, models.ServiceResourceModel{}, false, false},
		{"concrete num_replicas flips an imported horizontal service to vertical (config-presence)", models.ServiceResourceModel{NumReplicas: types.Int64Value(3)}, horizontalState, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveIsHorizontal(tc.config, tc.state, tc.hasState); got != tc.want {
				t.Errorf("resolveIsHorizontal = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestServiceResource_ModifyPlan_horizontal(t *testing.T) {
	ctx := context.Background()
	r := &ServiceResource{}
	sch := buildServiceSchema(t, ctx, r)

	run := func(t *testing.T, plan models.ServiceResourceModel) diag.Diagnostics {
		t.Helper()
		planVal := tfsdk.Plan{Schema: sch}
		if d := planVal.Set(ctx, &plan); d.HasError() {
			t.Fatalf("encoding plan failed: %v", d.Errors())
		}
		req := resource.ModifyPlanRequest{
			State:  tfsdk.State{Schema: sch}, // null Raw => create
			Plan:   planVal,
			Config: tfsdk.Config{Schema: sch, Raw: planVal.Raw},
		}
		resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: sch, Raw: planVal.Raw}}
		r.ModifyPlan(ctx, req, resp)
		return resp.Diagnostics
	}

	// Horizontal config on PPv2 — the band plus a fixed (min == max) per-replica memory.
	horizontalPPv2 := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
		s.Tier = types.StringValue(api.TierPPv2)
		s.AutoscalingMode = types.StringValue(api.AutoscalingModeHorizontal)
		s.MinReplicas = types.Int64Value(3)
		s.MaxReplicas = types.Int64Value(10)
		s.MinReplicaMemoryGb = types.Int64Value(16)
		s.MaxReplicaMemoryGb = types.Int64Value(16)
		s.NumReplicas = types.Int64Null()
		s.MinTotalMemoryGb = types.Int64Null()
		s.MaxTotalMemoryGb = types.Int64Null()
	}).Get()

	t.Run("horizontal production/PPv2 plan with fixed per-replica memory plans cleanly", func(t *testing.T) {
		diags := run(t, horizontalPPv2)
		if diags.HasError() {
			t.Errorf("a horizontal plan carrying the fixed per-replica memory must plan cleanly; got: %v", diags.Errors())
		}
	})

	t.Run("horizontal production/PPv2 plan without per-replica memory is rejected", func(t *testing.T) {
		// Both modes require the per-replica memory fields now (horizontal fixes them at min == max).
		noMemory := test.NewUpdater(horizontalPPv2).Update(func(s *models.ServiceResourceModel) {
			s.MinReplicaMemoryGb = types.Int64Null()
			s.MaxReplicaMemoryGb = types.Int64Null()
		}).Get()
		if !detailContains(run(t, noMemory), "must be defined if the service tier is production or PPv2") {
			t.Errorf("a horizontal plan missing the per-replica memory must be rejected")
		}
	})

	t.Run("horizontal on development tier is rejected", func(t *testing.T) {
		devHorizontal := test.NewUpdater(horizontalPPv2).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierDevelopment)
		}).Get()
		if !detailContains(run(t, devHorizontal), "cannot be defined if the service tier is development") {
			t.Errorf("horizontal autoscaling on development tier must be rejected")
		}
	})

	t.Run("development tier rejects an explicit autoscaling_mode = vertical", func(t *testing.T) {
		devVertical := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierDevelopment)
			s.AutoscalingMode = types.StringValue(api.AutoscalingModeVertical)
			s.MinReplicaMemoryGb = types.Int64Null()
			s.MaxReplicaMemoryGb = types.Int64Null()
		}).Get()
		if !detailContains(run(t, devVertical), "cannot be defined if the service tier is development") {
			t.Errorf("an explicit autoscaling_mode on development tier must be rejected")
		}
	})

	// A create that omits autoscaling_mode leaves the plan value Unknown (Computed, no static default), so the
	// vertical-memory guard must key off the config (null ⇒ vertical), not the plan's unknown mode — else every
	// vertical create silently skips the guard. Config-on-create carries a null (not unknown) mode.
	t.Run("production create omitting the memory fields is rejected", func(t *testing.T) {
		// The guard reads the config: a field the user omitted is Null there, so a memory-less production
		// create is rejected at plan time rather than POSTing a 0 GiB bound. (This harness sets config == plan.)
		omittedMode := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierProduction)
			s.AutoscalingMode = types.StringNull()
			s.MinReplicas = types.Int64Null()
			s.MaxReplicas = types.Int64Null()
			s.MinReplicaMemoryGb = types.Int64Null()
			s.MaxReplicaMemoryGb = types.Int64Null()
			s.MinTotalMemoryGb = types.Int64Null()
			s.MaxTotalMemoryGb = types.Int64Null()
		}).Get()
		if !detailContains(run(t, omittedMode), "must be defined if the service tier is production or PPv2") {
			t.Errorf("an omitted-mode production create with no memory fields must still require them")
		}
	})

	t.Run("production create with interpolated (unknown) per-replica memory is allowed", func(t *testing.T) {
		// An interpolated `min_replica_memory_gb = local.mem` is Unknown in the config (present, not omitted);
		// the required-field guard must defer to apply, not reject it as missing.
		interpolatedMemory := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierProduction)
			s.AutoscalingMode = types.StringNull()
			s.MinReplicas = types.Int64Null()
			s.MaxReplicas = types.Int64Null()
			s.MinReplicaMemoryGb = types.Int64Unknown()
			s.MaxReplicaMemoryGb = types.Int64Unknown()
			s.MinTotalMemoryGb = types.Int64Null()
			s.MaxTotalMemoryGb = types.Int64Null()
		}).Get()
		if detailContains(run(t, interpolatedMemory), "must be defined if the service tier is production or PPv2") {
			t.Errorf("an interpolated (unknown) per-replica memory create must not be rejected as missing")
		}
	})

	t.Run("production create omitting autoscaling_mode with the memory fields plans cleanly", func(t *testing.T) {
		omittedModeWithMemory := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierProduction)
			s.AutoscalingMode = types.StringNull()
			s.MinReplicas = types.Int64Null()
			s.MaxReplicas = types.Int64Null()
			s.MinReplicaMemoryGb = types.Int64Value(8)
			s.MaxReplicaMemoryGb = types.Int64Value(32)
			s.MinTotalMemoryGb = types.Int64Null()
			s.MaxTotalMemoryGb = types.Int64Null()
		}).Get()
		if detailContains(run(t, omittedModeWithMemory), "must be defined if the service tier is production or PPv2") {
			t.Errorf("an omitted-mode production create with the memory fields set must plan cleanly")
		}
	})

	t.Run("deprecated total-memory only (per-replica null) does not trip the mutual-exclusion conflict", func(t *testing.T) {
		// A config using only the deprecated min/max_total_memory_gb (per-replica fields null, as a module
		// passing null produces) must not be rejected as "can't be specified at the same time".
		totalsOnly := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierProduction)
			s.AutoscalingMode = types.StringNull()
			s.MinReplicaMemoryGb = types.Int64Null()
			s.MaxReplicaMemoryGb = types.Int64Null()
			s.MinTotalMemoryGb = types.Int64Value(24)
			s.MaxTotalMemoryGb = types.Int64Value(96)
		}).Get()
		if detailContains(run(t, totalsOnly), "can't be specified at the same time") {
			t.Errorf("a deprecated-total-memory-only config must not trip the per-replica mutual-exclusion error")
		}
	})

	t.Run("deprecated total-memory only with unknown per-replica fields (a real create) does not conflict", func(t *testing.T) {
		// On a real create the omitted Optional+Computed per-replica fields are Unknown (not Null); the
		// mutual-exclusion guard must treat Unknown as "not set" or a totals-only create is wrongly rejected.
		totalsOnlyUnknown := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierProduction)
			s.AutoscalingMode = types.StringNull()
			s.MinReplicaMemoryGb = types.Int64Unknown()
			s.MaxReplicaMemoryGb = types.Int64Unknown()
			s.MinTotalMemoryGb = types.Int64Value(24)
			s.MaxTotalMemoryGb = types.Int64Value(96)
		}).Get()
		if detailContains(run(t, totalsOnlyUnknown), "can't be specified at the same time") {
			t.Errorf("a totals-only create with unknown per-replica fields must not trip the mutual-exclusion error")
		}
	})

	t.Run("horizontal with num_replicas is rejected at plan time", func(t *testing.T) {
		// The num_replicas<->band ConflictsWith validator doesn't cover mode+num_replicas, so ModifyPlan must
		// reject horizontal + num_replicas (no band) to make the schema's "Forbidden when horizontal" true.
		horizontalWithCount := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierProduction)
			s.AutoscalingMode = types.StringValue(api.AutoscalingModeHorizontal)
			s.NumReplicas = types.Int64Value(3)
			s.MinReplicas = types.Int64Null()
			s.MaxReplicas = types.Int64Null()
			s.MinReplicaMemoryGb = types.Int64Value(16)
			s.MaxReplicaMemoryGb = types.Int64Value(16)
			s.MinTotalMemoryGb = types.Int64Null()
			s.MaxTotalMemoryGb = types.Int64Null()
		}).Get()
		if !detailContains(run(t, horizontalWithCount), "num_replicas can't be set when autoscaling_mode is \"horizontal\"") {
			t.Errorf("horizontal + num_replicas must be rejected at plan time")
		}
	})
}

// On an in-place mode switch the Computed scaling fields are pinned by UseStateForUnknown, so the plan pins
// them to the prior state value while the post-apply read reflects the new mode's shape — "inconsistent
// result after apply" unless ModifyPlan nulls the fields the resolved mode doesn't use. Both modes carry the
// band and per-replica memory now; the only field a mode forbids is num_replicas (horizontal) and, when a
// vertical config sets num_replicas, the stale band pinned from a prior horizontal plan. This drives
// ModifyPlan with a non-null prior state and asserts the resolved plan matches what syncServiceState writes.
func TestServiceResource_ModifyPlan_modeSwitch(t *testing.T) {
	ctx := context.Background()
	r := &ServiceResource{}
	sch := buildServiceSchema(t, ctx, r)

	// state = prior state, config = the user's HCL (dropped fields omitted on a switch),
	// plan = config with the dropped fields pinned to prior state by UseStateForUnknown. Mode derivation
	// keys off config, so config and plan must differ exactly as they do for a real in-place switch.
	run := func(t *testing.T, state, config, plan models.ServiceResourceModel) models.ServiceResourceModel {
		t.Helper()
		stateVal := tfsdk.State{Schema: sch}
		if d := stateVal.Set(ctx, &state); d.HasError() {
			t.Fatalf("encoding state failed: %v", d.Errors())
		}
		configPlan := tfsdk.Plan{Schema: sch}
		if d := configPlan.Set(ctx, &config); d.HasError() {
			t.Fatalf("encoding config failed: %v", d.Errors())
		}
		planVal := tfsdk.Plan{Schema: sch}
		if d := planVal.Set(ctx, &plan); d.HasError() {
			t.Fatalf("encoding plan failed: %v", d.Errors())
		}
		req := resource.ModifyPlanRequest{
			State:  stateVal, // non-null Raw => update
			Plan:   planVal,
			Config: tfsdk.Config{Schema: sch, Raw: configPlan.Raw},
		}
		resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: sch, Raw: planVal.Raw}}
		r.ModifyPlan(ctx, req, resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("ModifyPlan returned errors: %v", resp.Diagnostics.Errors())
		}
		var out models.ServiceResourceModel
		if d := resp.Plan.Get(ctx, &out); d.HasError() {
			t.Fatalf("decoding plan failed: %v", d.Errors())
		}
		return out
	}

	// runErr mirrors run but returns the diagnostics instead of failing on them, for plan-time rejection cases.
	runErr := func(t *testing.T, state, config, plan models.ServiceResourceModel) diag.Diagnostics {
		t.Helper()
		stateVal := tfsdk.State{Schema: sch}
		if d := stateVal.Set(ctx, &state); d.HasError() {
			t.Fatalf("encoding state failed: %v", d.Errors())
		}
		configPlan := tfsdk.Plan{Schema: sch}
		if d := configPlan.Set(ctx, &config); d.HasError() {
			t.Fatalf("encoding config failed: %v", d.Errors())
		}
		planVal := tfsdk.Plan{Schema: sch}
		if d := planVal.Set(ctx, &plan); d.HasError() {
			t.Fatalf("encoding plan failed: %v", d.Errors())
		}
		req := resource.ModifyPlanRequest{
			State:  stateVal,
			Plan:   planVal,
			Config: tfsdk.Config{Schema: sch, Raw: configPlan.Raw},
		}
		resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: sch, Raw: planVal.Raw}}
		r.ModifyPlan(ctx, req, resp)
		return resp.Diagnostics
	}

	vertical := func(mut func(*models.ServiceResourceModel)) models.ServiceResourceModel {
		return test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierProduction)
			s.AutoscalingMode = types.StringValue(api.AutoscalingModeVertical)
			s.NumReplicas = types.Int64Value(3)
			s.MinReplicaMemoryGb = types.Int64Value(8)
			s.MaxReplicaMemoryGb = types.Int64Value(32)
			s.MinReplicas = types.Int64Null()
			s.MaxReplicas = types.Int64Null()
			s.MinTotalMemoryGb = types.Int64Null()
			s.MaxTotalMemoryGb = types.Int64Null()
			if mut != nil {
				mut(s)
			}
		}).Get()
	}

	horizontal := func(mut func(*models.ServiceResourceModel)) models.ServiceResourceModel {
		return test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
			s.Tier = types.StringValue(api.TierProduction)
			s.AutoscalingMode = types.StringValue(api.AutoscalingModeHorizontal)
			s.MinReplicas = types.Int64Value(3)
			s.MaxReplicas = types.Int64Value(10)
			s.MinReplicaMemoryGb = types.Int64Value(16)
			s.MaxReplicaMemoryGb = types.Int64Value(16)
			s.NumReplicas = types.Int64Null()
			s.MinTotalMemoryGb = types.Int64Null()
			s.MaxTotalMemoryGb = types.Int64Null()
			if mut != nil {
				mut(s)
			}
		}).Get()
	}

	t.Run("vertical->horizontal nulls num_replicas but keeps the per-replica memory", func(t *testing.T) {
		// Config is horizontal (num_replicas omitted); the plan pins num_replicas to the prior state value.
		// The per-replica memory is used by both modes, so it must be preserved, not nulled.
		plan := horizontal(func(s *models.ServiceResourceModel) {
			s.NumReplicas = types.Int64Value(3)
		})
		out := run(t, vertical(nil), horizontal(nil), plan)
		if !out.NumReplicas.IsNull() {
			t.Errorf("num_replicas must be nulled on a switch to horizontal; got %v", out.NumReplicas)
		}
		if out.MinReplicaMemoryGb.IsNull() || out.MaxReplicaMemoryGb.IsNull() {
			t.Errorf("the per-replica memory must be preserved on a switch to horizontal; got minMem=%v maxMem=%v",
				out.MinReplicaMemoryGb, out.MaxReplicaMemoryGb)
		}
		if out.AutoscalingMode.ValueString() != api.AutoscalingModeHorizontal {
			t.Errorf("planned autoscaling_mode must resolve to horizontal; got %v", out.AutoscalingMode)
		}
	})

	t.Run("horizontal->vertical with num_replicas nulls the stale band", func(t *testing.T) {
		// Config is vertical with num_replicas (band omitted); the plan pins the band to the prior state.
		// Vertical + num_replicas forbids a band, so the stale band must be nulled.
		plan := vertical(func(s *models.ServiceResourceModel) {
			s.MinReplicas = types.Int64Value(3)
			s.MaxReplicas = types.Int64Value(10)
		})
		out := run(t, horizontal(nil), vertical(nil), plan)
		if !out.MinReplicas.IsNull() || !out.MaxReplicas.IsNull() {
			t.Errorf("the band must be nulled on a switch to vertical with num_replicas; got min=%v max=%v",
				out.MinReplicas, out.MaxReplicas)
		}
		if out.AutoscalingMode.ValueString() != api.AutoscalingModeVertical {
			t.Errorf("planned autoscaling_mode must resolve to vertical; got %v", out.AutoscalingMode)
		}
	})

	t.Run("horizontal->vertical with omitted autoscaling_mode resolves the planned mode to vertical", func(t *testing.T) {
		// The user removes the band + adds num_replicas but omits autoscaling_mode; UseStateForUnknown pins the
		// planned mode to the prior "horizontal". ModifyPlan must reset it to vertical so the plan matches the
		// post-apply read (else "inconsistent result after apply").
		omittedModeConfig := vertical(func(s *models.ServiceResourceModel) { s.AutoscalingMode = types.StringNull() })
		plan := vertical(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringValue(api.AutoscalingModeHorizontal)
			s.MinReplicas = types.Int64Value(3)
			s.MaxReplicas = types.Int64Value(10)
		})
		out := run(t, horizontal(nil), omittedModeConfig, plan)
		if out.AutoscalingMode.ValueString() != api.AutoscalingModeVertical {
			t.Errorf("planned autoscaling_mode must resolve to vertical on an omitted-mode switch; got %v", out.AutoscalingMode)
		}
		if !out.MinReplicas.IsNull() || !out.MaxReplicas.IsNull() {
			t.Errorf("band fields must be nulled; got min=%v max=%v", out.MinReplicas, out.MaxReplicas)
		}
	})

	t.Run("imported horizontal service with no scaling fields in config stays horizontal", func(t *testing.T) {
		// terraform import brings horizontal state but the user's HCL omits every scaling field. Omitting an
		// Optional+Computed field means "keep current", so the service must stay horizontal — not be misread
		// as vertical (which would rewrite the plan back to vertical).
		noScalingConfig := test.NewUpdater(horizontal(nil)).Update(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringNull()
			s.MinReplicas = types.Int64Null()
			s.MaxReplicas = types.Int64Null()
			s.MinReplicaMemoryGb = types.Int64Null()
			s.MaxReplicaMemoryGb = types.Int64Null()
		}).Get()
		// Plan: UseStateForUnknown pins the prior horizontal mode + band.
		out := run(t, horizontal(nil), noScalingConfig, horizontal(nil))
		if out.AutoscalingMode.ValueString() != api.AutoscalingModeHorizontal {
			t.Errorf("an imported horizontal service with no scaling config must stay horizontal; got %v", out.AutoscalingMode)
		}
		if out.MinReplicas.IsNull() || out.MaxReplicas.IsNull() {
			t.Errorf("the band must be preserved, not nulled; got min=%v max=%v", out.MinReplicas, out.MaxReplicas)
		}
	})

	t.Run("vertical with an equal min==max replicas band is rejected (must use num_replicas)", func(t *testing.T) {
		// The API reports every vertical service's replica count as num_replicas — an equal min==max band is
		// collapsed on read (UC-1173) — so a vertical service spelled with an equal band can't round-trip (the
		// read returns num_replicas with the band cleared → inconsistent result after apply). The provider
		// rejects it at plan time and steers to num_replicas rather than letting it fail at apply.
		bandConfig := vertical(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringNull()
			s.NumReplicas = types.Int64Null()
			s.MinReplicas = types.Int64Value(3)
			s.MaxReplicas = types.Int64Value(3)
		})
		diags := runErr(t, vertical(nil), bandConfig, bandConfig)
		if !diags.HasError() {
			t.Fatalf("a vertical service with an equal min==max band must be rejected at plan time; got no error")
		}
		foundBandGuard := false
		for _, e := range diags.Errors() {
			if strings.Contains(e.Detail(), "min_replicas/max_replicas define a horizontal replica band") {
				foundBandGuard = true
			}
		}
		if !foundBandGuard {
			t.Errorf("expected the vertical-band guard message; got %v", diags.Errors())
		}
	})

	t.Run("interpolated (Unknown) mode with an equal band defers, not rejected", func(t *testing.T) {
		// resolveIsHorizontal can't resolve an Unknown mode, so the vertical-band guard must defer (like the
		// mode-switch nulling block and the scheduled validator) rather than false-rejecting a config that may
		// resolve to a valid horizontal service at apply.
		cfg := vertical(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringUnknown()
			s.NumReplicas = types.Int64Null()
			s.MinReplicas = types.Int64Value(3)
			s.MaxReplicas = types.Int64Value(3)
		})
		diags := runErr(t, vertical(nil), cfg, cfg)
		if diags.HasError() {
			t.Errorf("an interpolated mode + equal band must defer, not reject; got %v", diags.Errors())
		}
	})

	t.Run("horizontal with deprecated total-memory is rejected (requires per-replica)", func(t *testing.T) {
		// A horizontal config that omits per-replica memory and sets the deprecated totals would hit the
		// legacy /3 conversion and silently mis-size, since horizontal has no fixed total across the band.
		cfg := horizontal(func(s *models.ServiceResourceModel) {
			s.MinReplicaMemoryGb = types.Int64Null()
			s.MaxReplicaMemoryGb = types.Int64Null()
			s.MinTotalMemoryGb = types.Int64Value(48)
			s.MaxTotalMemoryGb = types.Int64Value(48)
		})
		diags := runErr(t, horizontal(nil), cfg, cfg)
		if !diags.HasError() {
			t.Fatalf("horizontal + deprecated total-memory must be rejected; got no error")
		}
	})

	t.Run("backwards compatibility: legacy num_replicas vertical service upgrades cleanly", func(t *testing.T) {
		// A service created by an older provider version (num_replicas, no autoscaling_mode, no replica band)
		// must be unaffected by the upgrade: it resolves to vertical, keeps num_replicas, and the new guard
		// does NOT fire (it only rejects a min/max band on a vertical service, which legacy configs never set).
		legacy := vertical(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringNull()
			s.NumReplicas = types.Int64Value(3)
			s.MinReplicas = types.Int64Null()
			s.MaxReplicas = types.Int64Null()
		})
		out := run(t, legacy, legacy, legacy)
		if out.NumReplicas.ValueInt64() != 3 {
			t.Errorf("legacy num_replicas must be preserved on upgrade; got %v", out.NumReplicas)
		}
		if !out.MinReplicas.IsNull() || !out.MaxReplicas.IsNull() {
			t.Errorf("upgrade must not introduce a replica band; got min=%v max=%v", out.MinReplicas, out.MaxReplicas)
		}
	})

	t.Run("resize a horizontal service with omitted autoscaling_mode stays horizontal", func(t *testing.T) {
		// The user edits only max_replicas on an existing horizontal service and omits autoscaling_mode. The
		// band is still in the config, so the mode must resolve horizontal — not be silently flipped to
		// vertical (which would PATCH autoscaling_mode=vertical on a routine resize).
		resizedConfig := test.NewUpdater(horizontal(nil)).Update(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringNull()
			s.MaxReplicas = types.Int64Value(15)
		}).Get()
		// Plan: UseStateForUnknown pins the prior horizontal mode; max_replicas takes the new value.
		plan := test.NewUpdater(horizontal(nil)).Update(func(s *models.ServiceResourceModel) {
			s.MaxReplicas = types.Int64Value(15)
		}).Get()
		out := run(t, horizontal(nil), resizedConfig, plan)
		if out.AutoscalingMode.ValueString() != api.AutoscalingModeHorizontal {
			t.Errorf("a resize with omitted autoscaling_mode must keep the service horizontal; got %v", out.AutoscalingMode)
		}
		if out.MinReplicas.IsNull() || out.MaxReplicas.ValueInt64() != 15 {
			t.Errorf("the resized band must be preserved; got min=%v max=%v", out.MinReplicas, out.MaxReplicas)
		}
	})

	t.Run("imported vertical service with no scaling fields in config stays vertical", func(t *testing.T) {
		// The mirror of the horizontal case: the fallback must not spuriously flip a vertical service when the
		// imported HCL omits every scaling field.
		noScalingConfig := test.NewUpdater(vertical(nil)).Update(func(s *models.ServiceResourceModel) {
			s.AutoscalingMode = types.StringNull()
			s.NumReplicas = types.Int64Null()
			s.MinReplicaMemoryGb = types.Int64Null()
			s.MaxReplicaMemoryGb = types.Int64Null()
		}).Get()
		out := run(t, vertical(nil), noScalingConfig, vertical(nil))
		if out.AutoscalingMode.ValueString() != api.AutoscalingModeVertical {
			t.Errorf("an imported vertical service with no scaling config must stay vertical; got %v", out.AutoscalingMode)
		}
	})
}

// Update diffs the scaling fields into a ReplicaScalingUpdate. On a mode switch ModifyPlan nulls the fields
// the target mode doesn't use, so the `!plan.X.IsNull()` inner guards keep them OUT of the PATCH body. This
// drives the resource Update with a mock client and asserts both the captured PATCH carries the mode plus the
// target mode's fields AND the post-apply synced state reflects the new mode. The plan is hand-built in the
// post-ModifyPlan shape — this test deliberately bypasses ModifyPlan to isolate the Update diff + sync contract.
func TestServiceResource_Update_horizontal(t *testing.T) {
	ctx := context.Background()
	r := &ServiceResource{}
	sch := buildServiceSchema(t, ctx, r)

	horizontalResp := test.NewUpdater(getBaseResponse(encodableInitialState().ID.ValueString())).Update(func(src *api.Service) {
		mn, mx, rm := 3, 10, 16
		src.Tier = api.TierProduction
		src.AutoscalingMode = api.AutoscalingModeHorizontal
		src.MinReplicas = &mn
		src.MaxReplicas = &mx
		src.MinReplicaMemoryGb = &rm
		src.MaxReplicaMemoryGb = &rm
	}).GetPtr()
	verticalResp := test.NewUpdater(getBaseResponse(encodableInitialState().ID.ValueString())).Update(func(src *api.Service) {
		num, mn, mx := 3, 8, 32
		src.Tier = api.TierProduction
		src.AutoscalingMode = api.AutoscalingModeVertical
		src.NumReplicas = &num
		src.MinReplicaMemoryGb = &mn
		src.MaxReplicaMemoryGb = &mx
	}).GetPtr()

	verticalState := test.NewUpdater(encodableInitialState()).Update(func(s *models.ServiceResourceModel) {
		s.Tier = types.StringValue(api.TierProduction)
		s.AutoscalingMode = types.StringValue(api.AutoscalingModeVertical)
		s.NumReplicas = types.Int64Value(3)
		s.MinReplicaMemoryGb = types.Int64Value(8)
		s.MaxReplicaMemoryGb = types.Int64Value(32)
		s.MinReplicas = types.Int64Null()
		s.MaxReplicas = types.Int64Null()
		s.MinTotalMemoryGb = types.Int64Null()
		s.MaxTotalMemoryGb = types.Int64Null()
	}).Get()
	horizontalPlan := test.NewUpdater(verticalState).Update(func(s *models.ServiceResourceModel) {
		s.AutoscalingMode = types.StringValue(api.AutoscalingModeHorizontal)
		s.NumReplicas = types.Int64Null()
		s.MinReplicaMemoryGb = types.Int64Value(16)
		s.MaxReplicaMemoryGb = types.Int64Value(16)
		s.MinReplicas = types.Int64Value(3)
		s.MaxReplicas = types.Int64Value(10)
	}).Get()

	tests := []struct {
		name     string
		state    models.ServiceResourceModel
		plan     models.ServiceResourceModel
		response *api.Service // what GetService returns for the post-apply sync
		wantMode string       // expected autoscaling_mode in the synced post-apply state
		check    func(t *testing.T, u api.ReplicaScalingUpdate)
	}{
		{
			name:     "vertical->horizontal sends the mode, band, and fixed memory but not num_replicas",
			state:    verticalState,
			plan:     horizontalPlan,
			response: horizontalResp,
			wantMode: api.AutoscalingModeHorizontal,
			check: func(t *testing.T, u api.ReplicaScalingUpdate) {
				if u.AutoscalingMode == nil || *u.AutoscalingMode != api.AutoscalingModeHorizontal {
					t.Errorf("autoscaling_mode not sent: %v", u.AutoscalingMode)
				}
				if u.MinReplicas == nil || *u.MinReplicas != 3 || u.MaxReplicas == nil || *u.MaxReplicas != 10 {
					t.Errorf("band not sent: min=%v max=%v", u.MinReplicas, u.MaxReplicas)
				}
				if u.MinReplicaMemoryGb == nil || *u.MinReplicaMemoryGb != 16 || u.MaxReplicaMemoryGb == nil || *u.MaxReplicaMemoryGb != 16 {
					t.Errorf("fixed per-replica memory not sent: minMem=%v maxMem=%v", u.MinReplicaMemoryGb, u.MaxReplicaMemoryGb)
				}
				if u.NumReplicas != nil {
					t.Errorf("num_replicas must not be sent on a switch to horizontal: %v", u.NumReplicas)
				}
			},
		},
		{
			name:     "horizontal->vertical sends num_replicas and the memory range but not the band",
			state:    horizontalPlan,
			plan:     verticalState,
			response: verticalResp,
			wantMode: api.AutoscalingModeVertical,
			check: func(t *testing.T, u api.ReplicaScalingUpdate) {
				if u.AutoscalingMode == nil || *u.AutoscalingMode != api.AutoscalingModeVertical {
					t.Errorf("autoscaling_mode not sent: %v", u.AutoscalingMode)
				}
				if u.NumReplicas == nil || *u.NumReplicas != 3 || u.MinReplicaMemoryGb == nil || *u.MinReplicaMemoryGb != 8 || u.MaxReplicaMemoryGb == nil || *u.MaxReplicaMemoryGb != 32 {
					t.Errorf("vertical fields not sent: num=%v minMem=%v maxMem=%v", u.NumReplicas, u.MinReplicaMemoryGb, u.MaxReplicaMemoryGb)
				}
				if u.MinReplicas != nil || u.MaxReplicas != nil {
					t.Errorf("band fields must not be sent on a switch to vertical: min=%v max=%v", u.MinReplicas, u.MaxReplicas)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)
			var captured api.ReplicaScalingUpdate
			apiClientMock := api.NewClientMock(mc).
				UpdateReplicaScalingMock.Set(func(_ context.Context, _ string, u api.ReplicaScalingUpdate) (*api.Service, error) {
				captured = u
				return tt.response, nil
			}).
				GetServiceMock.Return(tt.response, nil)

			r.client = apiClientMock

			stateVal := tfsdk.State{Schema: sch}
			if d := stateVal.Set(ctx, &tt.state); d.HasError() {
				t.Fatalf("encoding state: %v", d.Errors())
			}
			planVal := tfsdk.Plan{Schema: sch}
			if d := planVal.Set(ctx, &tt.plan); d.HasError() {
				t.Fatalf("encoding plan: %v", d.Errors())
			}
			req := resource.UpdateRequest{
				State:  stateVal,
				Plan:   planVal,
				Config: tfsdk.Config{Schema: sch, Raw: planVal.Raw},
			}
			resp := &resource.UpdateResponse{State: tfsdk.State{Schema: sch}}
			r.Update(ctx, req, resp)
			if resp.Diagnostics.HasError() {
				t.Fatalf("Update returned errors: %v", resp.Diagnostics.Errors())
			}
			tt.check(t, captured)

			// The post-apply state (synced from the band-carrying GetService response) must reflect the new
			// mode — a regression in syncServiceState's horizontal derivation would otherwise pass silently.
			var out models.ServiceResourceModel
			if d := resp.State.Get(ctx, &out); d.HasError() {
				t.Fatalf("decoding post-apply state: %v", d.Errors())
			}
			if out.AutoscalingMode.ValueString() != tt.wantMode {
				t.Errorf("post-apply autoscaling_mode = %v, want %q", out.AutoscalingMode, tt.wantMode)
			}
		})
	}
}
