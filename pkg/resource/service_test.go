package resource

import (
	"context"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/test"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/resource/models"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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
				src.NumReplicas = types.Int64Value(3)
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
		ComplianceType: types.StringNull(),
		Tags:           tags,
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
	}
}
