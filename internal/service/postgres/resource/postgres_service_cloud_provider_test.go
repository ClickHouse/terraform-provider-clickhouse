package resource

import (
	"context"
	"testing"

	"github.com/gojuno/minimock/v3"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
)

func TestPostgresSchema_cloudProviderAndSize(t *testing.T) {
	ctx := context.Background()
	r := &PostgresServiceResource{}
	var resp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &resp)

	for _, tc := range []struct {
		attribute string
		value     types.String
		wantError bool
	}{
		{"cloud_provider", types.StringValue("aws"), false},
		{"cloud_provider", types.StringValue("gcp"), false},
		{"cloud_provider", types.StringValue("azure"), true},
		{"cloud_provider", types.StringValue("GCP"), true},
		{"cloud_provider", types.StringValue(""), true},
		{"cloud_provider", types.StringNull(), false},
		{"cloud_provider", types.StringUnknown(), false},
		{"size", types.StringValue("m6gd.large"), false},
		// {"size", types.StringValue("c4a-highmem-4"), false}, // Requires access to the GCP ARM preview.
		{"size", types.StringValue("c4-standard-4"), false},
		{"size", types.StringValue("c4d-highmem-8"), false},
		{"size", types.StringValue("z3-highlssd-8"), false},
		{"size", types.StringValue("future-server-supported-size"), false},
		{"size", types.StringValue(""), true},
	} {
		t.Run(tc.attribute+"/"+tc.value.String(), func(t *testing.T) {
			attribute := resp.Schema.Attributes[tc.attribute].(schema.StringAttribute)
			var validation validator.StringResponse
			for _, v := range attribute.Validators {
				v.ValidateString(ctx, validator.StringRequest{
					Path: path.Root(tc.attribute), ConfigValue: tc.value,
				}, &validation)
			}
			require.Equal(t, tc.wantError, validation.Diagnostics.HasError(), "%v", validation.Diagnostics)
		})
	}
}

func TestPostgresValidateConfig_privatePreview(t *testing.T) {
	ctx := context.Background()
	r := &PostgresServiceResource{}
	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	for _, tc := range []struct {
		name      string
		provider  types.String
		wantCount int
	}{
		{"aws", types.StringValue("aws"), 1},
		{"gcp", types.StringValue("gcp"), 2},
		{"inherited", types.StringNull(), 1},
		{"unknown", types.StringUnknown(), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := gateModel(true)
			model.CloudProvider = tc.provider
			state := tfsdk.State{Schema: schemaResp.Schema}
			require.False(t, state.Set(ctx, model).HasError())
			var resp resource.ValidateConfigResponse
			r.ValidateConfig(ctx, resource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw},
			}, &resp)
			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			require.Equal(t, tc.wantCount, resp.Diagnostics.WarningsCount(), "%v", resp.Diagnostics)
			require.Equal(t, "Beta Resource", resp.Diagnostics.Warnings()[0].Summary())
			if tc.name == "gcp" {
				warning := resp.Diagnostics.Warnings()[1]
				require.Contains(t, warning.Summary(), "private preview")
				require.Contains(t, warning.Detail(), "Contact ClickHouse support")
				require.Equal(t, path.Root("cloud_provider"), warning.(diag.DiagnosticWithPath).Path())
			}
		})
	}
}

func TestPostgresCreate_cloudProviders(t *testing.T) {
	for _, tc := range []struct {
		provider string
		region   string
		size     string
	}{
		{"aws", "us-east-1", "m6gd.large"},
		{"gcp", "us-west1", "c4-highmem-4"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			ctx := context.Background()
			client := api.NewClientMock(minimock.NewController(t))
			r := &PostgresServiceResource{client: client}
			var schemaResp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			model := gateModel(true)
			model.CloudProvider = types.StringValue(tc.provider)
			model.Region = types.StringValue(tc.region)
			model.Size = types.StringValue(tc.size)
			model.Password = types.StringValue("ConfiguredPassword123")
			model.PgConfig = types.MapNull(types.StringType)
			model.PgBouncerConfig = types.MapNull(types.StringType)
			encoded := tfsdk.State{Schema: schemaResp.Schema}
			require.False(t, encoded.Set(ctx, model).HasError())

			pg := &api.Postgres{
				Id: "pg-new", Name: "n", Provider: tc.provider, Region: tc.region,
				Size: tc.size, PostgresVersion: "18", HaType: "none",
				State: api.PostgresStateRunning, IsPrimary: true,
				Hostname: "pg.example.com", Username: "postgres",
			}
			client.CreatePostgresMock.Expect(ctx, api.PostgresCreate{
				Name: "n", Provider: tc.provider, Region: tc.region, Size: tc.size,
				PostgresVersion: "18", HaType: "none", Tags: []api.Tag{},
			}).Return(pg, "initial-server-password", nil)
			client.WaitForPostgresStateMock.ExpectPostgresIdParam2(pg.Id).
				ExpectMaxWaitSecondsParam4(postgresDefaultCreateTimeoutSeconds).Return(nil)
			client.SetPostgresPasswordMock.Expect(ctx, pg.Id, api.PostgresPassword{
				Password: "ConfiguredPassword123",
			}).Return(nil, nil)
			client.GetPostgresMock.Expect(ctx, pg.Id).Return(pg, nil)
			client.GetPostgresConfigMock.Expect(ctx, pg.Id).Return(&api.PostgresConfig{}, nil)

			resp := resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			r.Create(ctx, resource.CreateRequest{
				Plan:   tfsdk.Plan{Schema: schemaResp.Schema, Raw: encoded.Raw},
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: encoded.Raw},
			}, &resp)
			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			var got models.PostgresServiceResourceModel
			require.False(t, resp.State.Get(ctx, &got).HasError())
			require.Equal(t, pg.Id, got.ID.ValueString())
			require.Equal(t, tc.provider, got.CloudProvider.ValueString())
			require.Equal(t, tc.region, got.Region.ValueString())
			require.Equal(t, tc.size, got.Size.ValueString())
			require.Equal(t, "ConfiguredPassword123", got.Password.ValueString())
		})
	}
}

func TestPostgresPlanInheritedAttributes_GCP(t *testing.T) {
	for _, origin := range []string{"replica", "restore"} {
		t.Run(origin, func(t *testing.T) {
			ctx := context.Background()
			client := api.NewClientMock(minimock.NewController(t))
			client.GetPostgresMock.Expect(ctx, "pg-source").Return(&api.Postgres{
				Provider: "gcp", Region: "us-west1", Size: "c4-highmem-4", PostgresVersion: "18",
			}, nil)
			r := &PostgresServiceResource{client: client}
			var schemaResp resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			model := gateModel(false)
			model.CloudProvider = types.StringNull()
			model.Region = types.StringNull()
			model.PostgresVersion = types.StringNull()
			model.Size = types.StringNull()
			model.HaType = types.StringNull()
			if origin == "replica" {
				model.ReadReplicaOf = types.StringValue("pg-source")
			} else {
				var diags diag.Diagnostics
				model.RestoreToPointInTime, diags = types.ObjectValueFrom(ctx, model.RestoreToPointInTime.AttributeTypes(ctx), models.PostgresRestoreModel{
					SourceID: types.StringValue("pg-source"), RestoreTarget: types.StringValue("2026-09-01T12:00:00Z"),
				})
				require.False(t, diags.HasError(), "%v", diags)
			}
			plan := tfsdk.Plan{Schema: schemaResp.Schema}
			require.False(t, plan.Set(ctx, model).HasError())
			resp := resource.ModifyPlanResponse{Plan: plan}
			r.planInheritedAttributes(ctx, model, &resp)
			require.False(t, resp.Diagnostics.HasError(), "%v", resp.Diagnostics)
			var got models.PostgresServiceResourceModel
			require.False(t, resp.Plan.Get(ctx, &got).HasError())
			require.Equal(t, "gcp", got.CloudProvider.ValueString())
			require.Equal(t, "us-west1", got.Region.ValueString())
			require.Equal(t, "18", got.PostgresVersion.ValueString())
			if origin == "replica" {
				require.Equal(t, "c4-highmem-4", got.Size.ValueString())
			} else {
				require.True(t, got.Size.IsUnknown())
			}
		})
	}
}
