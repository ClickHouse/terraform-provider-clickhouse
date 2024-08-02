package clickhouse

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-clickhouse/internal/test"
)

func TestServiceResourceModel_Equals(t *testing.T) {
	base := getBaseModel()

	tests := []struct {
		name string
		a    ServiceResourceModel
		b    ServiceResourceModel
		want bool
	}{
		{
			name: "Unchanged",
			a:    base,
			b:    base,
			want: true,
		},
		{
			name: "ID changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.ID = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Name changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.Name = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Password changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.Password = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "PasswordHash changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.PasswordHash = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "DoubleSha1PasswordHash changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.DoubleSha1PasswordHash = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Endpoints added",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				current := src.Endpoints.Elements()
				obj, _ := types.ObjectValue(endpointObjectType.AttrTypes, map[string]attr.Value{
					"protocol": types.StringValue("changed"),
					"host":     types.StringValue("changed"),
					"port":     types.Int64Value(int64(1236)),
				})
				current = append(current, obj)
				src.Endpoints, _ = types.ListValue(endpointObjectType, current)
			}).Get(),
			want: false,
		},
		{
			name: "Endpoints deleted",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				current := src.Endpoints.Elements()
				var endpoints []attr.Value
				endpoints = append(endpoints, current[0])
				src.Endpoints, _ = types.ListValue(endpointObjectType, endpoints)
			}).Get(),
			want: false,
		},
		{
			name: "Endpoints order changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				current := src.Endpoints.Elements()
				var endpoints []attr.Value
				endpoints = append(endpoints, current[1])
				endpoints = append(endpoints, current[0])
				src.Endpoints, _ = types.ListValue(endpointObjectType, endpoints)
			}).Get(),
			want: false,
		},
		{
			name: "CloudProvider changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.CloudProvider = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Region changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.Region = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Tier changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.Tier = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "IdleScaling changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				old := src.IdleScaling.ValueBool()
				src.IdleScaling = types.BoolValue(!old)
			}).Get(),
			want: false,
		},
		{
			name: "IAMRole changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.IAMRole = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "LastUpdated changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.LastUpdated = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "PrivateEndpointConfig endpoint_service_id changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				privateEndpointConfig, _ := types.ObjectValue(privateEndpointConfigType.AttrTypes, map[string]attr.Value{
					"endpoint_service_id":  types.StringValue("changed"),
					"private_dns_hostname": types.StringValue(""),
				})
				src.PrivateEndpointConfig = privateEndpointConfig
			}).Get(),
			want: false,
		},
		{
			name: "PrivateEndpointConfig private_dns_hostname changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				privateEndpointConfig, _ := types.ObjectValue(privateEndpointConfigType.AttrTypes, map[string]attr.Value{
					"endpoint_service_id":  types.StringValue(""),
					"private_dns_hostname": types.StringValue("changed"),
				})
				src.PrivateEndpointConfig = privateEndpointConfig
			}).Get(),
			want: false,
		},
		{
			name: "PrivateEndpointIds added",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				existing := src.PrivateEndpointIds.Elements()
				existing = append(existing, types.StringValue("added"))
				privateEndpointIds, _ := types.ListValue(types.StringType, existing)
				src.PrivateEndpointIds = privateEndpointIds
			}).Get(),
			want: false,
		},
		{
			name: "PrivateEndpointIds removed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				existing := src.PrivateEndpointIds.Elements()
				privateEndpointIds, _ := types.ListValue(types.StringType, []attr.Value{existing[0]})
				src.PrivateEndpointIds = privateEndpointIds
			}).Get(),
			want: false,
		},
		{
			name: "PrivateEndpointIds order changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				existing := src.PrivateEndpointIds.Elements()
				privateEndpointIds, _ := types.ListValue(types.StringType, []attr.Value{existing[1], existing[0]})
				src.PrivateEndpointIds = privateEndpointIds
			}).Get(),
			want: false,
		},
		{
			name: "EncryptionKey changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.EncryptionKey = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "EncryptionAssumedRoleIdentifier changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.EncryptionAssumedRoleIdentifier = types.StringValue("changed")
			}).Get(),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.a.Equals(tt.b); got != tt.want {
				t.Errorf("Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func getBaseModel() ServiceResourceModel {
	uuid := "773bb8b4-34e8-4ecf-8e23-4f7e20aa14b3"

	var endpoints []attr.Value
	obj, _ := types.ObjectValue(endpointObjectType.AttrTypes, map[string]attr.Value{
		"protocol": types.StringValue("changed"),
		"host":     types.StringValue("changed"),
		"port":     types.Int64Value(int64(1234)),
	})
	endpoints = append(endpoints, obj)
	obj, _ = types.ObjectValue(endpointObjectType.AttrTypes, map[string]attr.Value{
		"protocol": types.StringValue("changed"),
		"host":     types.StringValue("changed"),
		"port":     types.Int64Value(int64(1235)),
	})
	endpoints = append(endpoints, obj)
	ep, _ := types.ListValue(endpointObjectType, endpoints)

	privateEndpointConfig, _ := types.ObjectValue(privateEndpointConfigType.AttrTypes, map[string]attr.Value{
		"endpoint_service_id":  types.StringValue(""),
		"private_dns_hostname": types.StringValue(""),
	})
	privateEndpointIds, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"id1", "id2"})

	state := ServiceResourceModel{
		ID:                              types.StringValue(uuid),
		Name:                            types.StringValue(""),
		Password:                        types.String{},
		PasswordHash:                    types.String{},
		DoubleSha1PasswordHash:          types.String{},
		Endpoints:                       ep,
		CloudProvider:                   types.StringValue(""),
		Region:                          types.StringValue(""),
		Tier:                            types.StringValue(""),
		IdleScaling:                     types.Bool{},
		IpAccessList:                    make([]IpAccessModel, 0),
		MinTotalMemoryGb:                types.Int64{},
		MaxTotalMemoryGb:                types.Int64{},
		NumReplicas:                     types.Int64{},
		IdleTimeoutMinutes:              types.Int64{},
		IAMRole:                         types.StringValue(""),
		LastUpdated:                     types.String{},
		PrivateEndpointConfig:           privateEndpointConfig,
		PrivateEndpointIds:              privateEndpointIds,
		EncryptionKey:                   types.String{},
		EncryptionAssumedRoleIdentifier: types.String{},
	}

	return state
}
