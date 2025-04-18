package models

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

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
			name: "BYOC ID changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.BYOCID = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Data Warehouse ID changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.DataWarehouseID = types.StringValue("changed")
			}).Get(),
			want: false,
		},
		{
			name: "Readonly changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.ReadOnly = types.BoolValue(true)
			}).Get(),
			want: false,
		},
		{
			name: "Is primary changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.IsPrimary = types.BoolValue(true)
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
			name: "Nativesecure host changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				endpoints := Endpoints{}
				diag := src.Endpoints.As(ctx, &endpoints, basetypes.ObjectAsOptions{
					UnhandledNullAsEmpty:    false,
					UnhandledUnknownAsEmpty: false,
				})
				if diag.HasError() {
					t.Fatal(diag.Errors())
				}

				ep := Endpoint{}
				diag = endpoints.NativeSecure.As(ctx, &ep, basetypes.ObjectAsOptions{
					UnhandledNullAsEmpty:    false,
					UnhandledUnknownAsEmpty: false,
				})
				if diag.HasError() {
					t.Fatal(diag.Errors())
				}

				ep.Host = types.StringValue("changed")
				endpoints.NativeSecure = ep.ObjectValue()

				src.Endpoints = endpoints.ObjectValue()
			}).Get(),
			want: false,
		},
		{
			name: "Mysql Endpoint disabled",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				endpoints := Endpoints{}
				diag := src.Endpoints.As(ctx, &endpoints, basetypes.ObjectAsOptions{
					UnhandledNullAsEmpty:    false,
					UnhandledUnknownAsEmpty: false,
				})
				if diag.HasError() {
					t.Fatal(diag.Errors())
				}

				ep := OptionalEndpoint{}
				diag = endpoints.MySQL.As(ctx, &ep, basetypes.ObjectAsOptions{
					UnhandledNullAsEmpty:    false,
					UnhandledUnknownAsEmpty: false,
				})
				if diag.HasError() {
					t.Fatal(diag.Errors())
				}

				ep.Enabled = types.BoolValue(false)
				endpoints.MySQL = ep.ObjectValue()

				src.Endpoints = endpoints.ObjectValue()
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
			name: "Release channel changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.ReleaseChannel = types.StringValue("changed")
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
		{
			name: "BackupConfiguration.BackupPeriodInHours changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.BackupConfiguration = BackupConfiguration{
					BackupPeriodInHours: types.Int32Value(10),
				}.ObjectValue()
			}).Get(),
			want: false,
		},
		{
			name: "BackupConfiguration.BackupStartTime changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.BackupConfiguration = BackupConfiguration{
					BackupStartTime: types.StringValue("changed"),
				}.ObjectValue()
			}).Get(),
			want: false,
		},
		{
			name: "BackupConfiguration.BackupRetentionPeriodInHours changed",
			a:    base,
			b: test.NewUpdater(base).Update(func(src *ServiceResourceModel) {
				src.BackupConfiguration = BackupConfiguration{
					BackupRetentionPeriodInHours: types.Int32Value(10),
				}.ObjectValue()
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

	endpoints := Endpoints{
		NativeSecure: Endpoint{
			Host: types.StringValue("hostname"),
			Port: types.Int32Value(80),
		}.ObjectValue(),
		HTTPS: Endpoint{
			Host: types.StringValue("hostname2"),
			Port: types.Int32Value(8080),
		}.ObjectValue(),
		MySQL: OptionalEndpoint{
			Enabled: types.BoolValue(true),
			Host:    types.StringValue("hostname3"),
			Port:    types.Int32Value(8081),
		}.ObjectValue(),
	}

	state := ServiceResourceModel{
		ID:                              types.StringValue(uuid),
		BYOCID:                          types.StringNull(),
		DataWarehouseID:                 types.StringNull(),
		ReadOnly:                        types.BoolValue(false),
		IsPrimary:                       types.BoolValue(false),
		Name:                            types.StringValue(""),
		Password:                        types.String{},
		PasswordHash:                    types.String{},
		DoubleSha1PasswordHash:          types.String{},
		Endpoints:                       endpoints.ObjectValue(),
		CloudProvider:                   types.StringValue(""),
		Region:                          types.StringValue(""),
		Tier:                            types.StringValue(""),
		ReleaseChannel:                  types.StringValue(""),
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
		BackupConfiguration:             BackupConfiguration{}.ObjectValue(),
	}

	return state
}
