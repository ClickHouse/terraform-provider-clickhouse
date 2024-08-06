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
		a    ServiceResource
		b    ServiceResource
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
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.ID = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Name changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.Name = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Password changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.Password = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "PasswordHash changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.PasswordHash = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "DoubleSha1PasswordHash changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.DoubleSha1PasswordHash = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Endpoints added",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				data := append(src.Endpoints.Elements(), Endpoint{Protocol: types.StringValue("changed"), Host: types.StringValue("changed"), Port: types.Int64Value(1236)}.ObjectValue())

				src.Endpoints, _ = types.ListValueFrom(ctx, src.Endpoints.ElementType(ctx).(types.ObjectType), data)
			}).Get(),
			want: false,
		},
		{
			name: "Endpoints deleted",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
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
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
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
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.CloudProvider = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Region changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.Region = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Tier changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.Tier = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "IdleScaling changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				old := src.IdleScaling.ValueBool()
				src.IdleScaling = types.BoolValue(!old)
			}).Get(),
			want: false,
		},
		{
			name: "IAMRole changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.IAMRole = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "LastUpdated changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.LastUpdated = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "PrivateEndpointConfig endpoint_service_id changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
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
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.PrivateEndpointConfig = PrivateEndpointConfig{
					EndpointServiceID:  types.StringValue(""),
					PrivateDNSHostname: types.StringValue("changed"),
				}.ObjectValue()
			}).Get(),
			want: false,
		},
		{
			name: "PrivateEndpointIds added",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
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
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				existing := src.PrivateEndpointIds.Elements()
				privateEndpointIds, _ := types.ListValue(types.StringType, []attr.Value{existing[0]})
				src.PrivateEndpointIds = privateEndpointIds
			}).Get(),
			want: false,
		},
		{
			name: "PrivateEndpointIds order changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				existing := src.PrivateEndpointIds.Elements()
				privateEndpointIds, _ := types.ListValue(types.StringType, []attr.Value{existing[1], existing[0]})
				src.PrivateEndpointIds = privateEndpointIds
			}).Get(),
			want: false,
		},
		{
			name: "EncryptionKey changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
				src.EncryptionKey = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "EncryptionAssumedRoleIdentifier changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResource) {
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

func getBaseModel() ServiceResource {
	uuid := "773bb8b4-34e8-4ecf-8e23-4f7e20aa14b3"

	var endpoints []attr.Value
	endpoints = append(endpoints, Endpoint{Protocol: types.StringValue("changed"), Host: types.StringValue("changed"), Port: types.Int64Value(1234)}.ObjectValue())
	endpoints = append(endpoints, Endpoint{Protocol: types.StringValue("changed"), Host: types.StringValue("changed"), Port: types.Int64Value(1235)}.ObjectValue())
	ep, _ := types.ListValue(Endpoint{}.ObjectType(), endpoints)

	privateEndpointIds, _ := types.ListValueFrom(context.Background(), types.StringType, []string{"id1", "id2"})

	state := ServiceResource{
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
		IpAccessList:                    tfutils.CreateEmptyList(IpAccessList{}.ObjectType()),
		MinTotalMemoryGb:                types.Int64{},
		MaxTotalMemoryGb:                types.Int64{},
		NumReplicas:                     types.Int64{},
		IdleTimeoutMinutes:              types.Int64{},
		IAMRole:                         types.StringValue(""),
		LastUpdated:                     types.String{},
		PrivateEndpointConfig:           PrivateEndpointConfig{}.ObjectValue(),
		PrivateEndpointIds:              privateEndpointIds,
		EncryptionKey:                   types.String{},
		EncryptionAssumedRoleIdentifier: types.String{},
	}

	return state
}
