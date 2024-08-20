package models

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/test"
	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/tfutils"
)

func TestServiceResource_Equals(t *testing.T) {
	base := getBaseModel()
	ctx := context.Background()

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
				data := append(src.Endpoints.Elements(), Endpoint{Protocol: types.StringValue("changed"), Host: types.StringValue("changed"), Port: types.Int64Value(1236)}.ObjectValue())

				src.Endpoints, _ = types.ListValueFrom(ctx, src.Endpoints.ElementType(ctx).(types.ObjectType), data)
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
				src.Endpoints, _ = types.ListValue(src.Endpoints.ElementType(ctx).(types.ObjectType), endpoints)
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
				src.Endpoints, _ = types.ListValue(src.Endpoints.ElementType(ctx).(types.ObjectType), endpoints)
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
			name: "PrivateEndpointConfig endpoint_service_id changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.PrivateEndpointConfig = PrivateEndpointConfig{
					EndpointServiceID:  types.StringValue("changed"),
					PrivateDNSHostname: types.StringValue(""),
				}.ObjectValue()
			}).Get(),
			want: false,
		},
		{
			name: "PrivateEndpointConfig private_dns_hostname changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.PrivateEndpointConfig = PrivateEndpointConfig{
					EndpointServiceID:  types.StringValue(""),
					PrivateDNSHostname: types.StringValue("changed"),
				}.ObjectValue()
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
				t.Errorf("%s wanted Equals() to return %v, got %v", tt.name, tt.want, got)
			}
		})
	}
}

func getBaseModel() ServiceResourceModel {
	uuid := "773bb8b4-34e8-4ecf-8e23-4f7e20aa14b3"

	var endpoints []attr.Value
	endpoints = append(endpoints, Endpoint{Protocol: types.StringValue("changed"), Host: types.StringValue("changed"), Port: types.Int64Value(1234)}.ObjectValue())
	endpoints = append(endpoints, Endpoint{Protocol: types.StringValue("changed"), Host: types.StringValue("changed"), Port: types.Int64Value(1235)}.ObjectValue())
	ep, _ := types.ListValue(Endpoint{}.ObjectType(), endpoints)

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
		IpAccessList:                    tfutils.CreateEmptyList(IPAccessList{}.ObjectType()),
		MinTotalMemoryGb:                types.Int64{},
		MaxTotalMemoryGb:                types.Int64{},
		NumReplicas:                     types.Int64{},
		IdleTimeoutMinutes:              types.Int64{},
		IAMRole:                         types.StringValue(""),
		PrivateEndpointConfig:           PrivateEndpointConfig{}.ObjectValue(),
		EncryptionKey:                   types.String{},
		EncryptionAssumedRoleIdentifier: types.String{},
	}

	return state
}
