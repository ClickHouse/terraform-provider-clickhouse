package resource

import (
	"context"
	"testing"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/models"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/test"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestServiceResource_syncServiceState(t *testing.T) {
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
			name:  "Update Endpoints field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Endpoints = []api.Endpoint{
					{
						Protocol: "TCP",
						Host:     "a.b.c.d",
						Port:     1234,
					},
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				var endpoints []attr.Value
				obj, _ := types.ObjectValue(endpointObjectType.AttrTypes, map[string]attr.Value{
					"protocol": types.StringValue("TCP"),
					"host":     types.StringValue("a.b.c.d"),
					"port":     types.Int64Value(int64(1234)),
				})

				endpoints = append(endpoints, obj)
				src.Endpoints, _ = types.ListValue(endpointObjectType, endpoints)
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
			name:  "Update IpAccessList field",
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
				src.IpAccessList = []models.IPAccessModel{
					{
						Source:      types.StringValue("0.0.0.0/0"),
						Description: types.StringValue("whitelist"),
					},
				}
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Updates MinTotalMemoryGb field when in production tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierProduction
				minTotalMemory := 10
				src.MinTotalMemoryGb = &minTotalMemory
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierProduction)
				src.MinTotalMemoryGb = types.Int64Value(10)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Does not update MinTotalMemoryGb field when in development tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierDevelopment
				minTotalMemory := 10
				src.MinTotalMemoryGb = &minTotalMemory
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierDevelopment)
				src.MinTotalMemoryGb = types.Int64{}
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Updates MaxTotalMemoryGb field when in production tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierProduction
				maxTotalMemory := 10
				src.MaxTotalMemoryGb = &maxTotalMemory
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierProduction)
				src.MaxTotalMemoryGb = types.Int64Value(10)
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Does not update MaxTotalMemoryGb field when in development tier",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.Tier = api.TierDevelopment
				maxTotalMemory := 10
				src.MaxTotalMemoryGb = &maxTotalMemory
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.Tier = types.StringValue(api.TierDevelopment)
				src.MaxTotalMemoryGb = types.Int64{}
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
				src.PrivateEndpointConfig, _ = types.ObjectValue(privateEndpointConfigType.AttrTypes, map[string]attr.Value{
					"endpoint_service_id":  types.StringValue("newendpointserviceid"),
					"private_dns_hostname": types.StringValue("new.endpoint.service.hostname"),
				})
			}).Get(),
			updateTimestamp: false,
			wantErr:         false,
		},
		{
			name:  "Update PrivateEndpointIds field",
			state: state,
			response: test.NewUpdater(getBaseResponse(state.ID.ValueString())).Update(func(src *api.Service) {
				src.PrivateEndpointIds = []string{
					"newendpointid",
				}
			}).GetPtr(),
			responseErr: nil,
			desiredState: test.NewUpdater(state).Update(func(src *models.ServiceResourceModel) {
				src.PrivateEndpointIds, _ = types.ListValueFrom(context.Background(), types.StringType, []string{"newendpointid"})
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
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := minimock.NewController(t)

			apiClientMock := api.NewClientMock(mc).
				GetServiceMock.
				Expect(tt.state.ID.ValueString()).
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

	endpoints, _ := types.ListValue(endpointObjectType, []attr.Value{})
	privateEndpointConfig, _ := types.ObjectValue(privateEndpointConfigType.AttrTypes, map[string]attr.Value{
		"endpoint_service_id":  types.StringValue(""),
		"private_dns_hostname": types.StringValue(""),
	})
	privateEndpointIds, _ := types.ListValue(types.StringType, []attr.Value{})

	state := models.ServiceResourceModel{
		ID:                              types.StringValue(uuid),
		Name:                            types.StringValue(""),
		Password:                        types.String{},
		PasswordHash:                    types.String{},
		DoubleSha1PasswordHash:          types.String{},
		Endpoints:                       endpoints,
		CloudProvider:                   types.StringValue(""),
		Region:                          types.StringValue(""),
		Tier:                            types.StringValue(""),
		IdleScaling:                     types.BoolValue(false),
		IpAccessList:                    make([]models.IPAccessModel, 0),
		MinTotalMemoryGb:                types.Int64{},
		MaxTotalMemoryGb:                types.Int64{},
		NumReplicas:                     types.Int64{},
		IdleTimeoutMinutes:              types.Int64{},
		IAMRole:                         types.StringValue(""),
		LastUpdated:                     types.String{},
		PrivateEndpointConfig:           privateEndpointConfig,
		PrivateEndpointIds:              privateEndpointIds,
		EncryptionKey:                   types.StringNull(),
		EncryptionAssumedRoleIdentifier: types.StringNull(),
	}

	return state
}

func getBaseResponse(id string) api.Service {
	return api.Service{
		Id: id,
		// Name: "newname",
		// Provider:                        "",
		// Region:                          "",
		// Tier:                            "",
		IdleScaling: false,
		// IpAccessList:                    nil,
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
		// PrivateEndpointIds:              nil,
		// EncryptionKey:                   "",
		// EncryptionAssumedRoleIdentifier: "",
	}
}
