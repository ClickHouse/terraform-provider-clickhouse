package resource

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickhouse/resource/models"
)

func backupBucketConfig(t *testing.T, vals map[string]string) resource.ValidateConfigRequest {
	t.Helper()
	ctx := context.Background()
	r := NewServiceBackupBucketResource().(*ServiceBackupBucketResource)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

	attrs := map[string]tftypes.Value{}
	for _, f := range append([]string{"id", "service_id", "bucket_provider"}, backupBucketAllProviderFields...) {
		if v, ok := vals[f]; ok {
			attrs[f] = tftypes.NewValue(tftypes.String, v)
		} else {
			attrs[f] = tftypes.NewValue(tftypes.String, nil)
		}
	}
	return resource.ValidateConfigRequest{Config: tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), attrs),
	}}
}

func TestServiceBackupBucket_ValidateConfig(t *testing.T) {
	r := &ServiceBackupBucketResource{}
	for name, tc := range map[string]struct {
		vals    map[string]string
		wantErr bool
	}{
		"aws ok":              {map[string]string{"bucket_provider": "AWS", "bucket_path": "p", "iam_role_arn": "a"}, false},
		"aws with session":    {map[string]string{"bucket_provider": "AWS", "bucket_path": "p", "iam_role_arn": "a", "iam_role_session_name": "s"}, false},
		"aws missing arn":     {map[string]string{"bucket_provider": "AWS", "bucket_path": "p"}, true},
		"aws with gcp secret": {map[string]string{"bucket_provider": "AWS", "bucket_path": "p", "iam_role_arn": "a", "secret_access_key": "x"}, true},
		"gcp ok":              {map[string]string{"bucket_provider": "GCP", "bucket_path": "p", "access_key_id": "k", "secret_access_key": "s"}, false},
		"gcp missing secret":  {map[string]string{"bucket_provider": "GCP", "bucket_path": "p", "access_key_id": "k"}, true},
		"azure ok":            {map[string]string{"bucket_provider": "AZURE", "container_name": "c", "connection_string": "s"}, false},
		"azure with path":     {map[string]string{"bucket_provider": "AZURE", "container_name": "c", "connection_string": "s", "bucket_path": "p"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			resp := &resource.ValidateConfigResponse{}
			r.ValidateConfig(context.Background(), backupBucketConfig(t, tc.vals), resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Errorf("HasError = %v; want %v (%v)", resp.Diagnostics.HasError(), tc.wantErr, resp.Diagnostics)
			}
		})
	}
}

func TestBackupBucketRequestFromPlan_OmitsUnsetFields(t *testing.T) {
	plan := models.ServiceBackupBucketResourceModel{
		BucketProvider:     types.StringValue("AWS"),
		BucketPath:         types.StringValue("p"),
		IamRoleArn:         types.StringValue("a"),
		IamRoleSessionName: types.StringUnknown(),
		AccessKeyID:        types.StringNull(),
	}

	got := backupBucketRequestFromPlan(plan)
	if got.IamRoleSessionName != nil || got.AccessKeyId != nil || got.SecretAccessKey != nil {
		t.Errorf("unset fields must be nil: %+v", got)
	}
	if *got.BucketPath != "p" || *got.IamRoleArn != "a" || got.BucketProvider != "AWS" {
		t.Errorf("unexpected request: %+v", got)
	}
}

func TestApplyBackupBucketToState_KeepsSecrets(t *testing.T) {
	state := &models.ServiceBackupBucketResourceModel{
		SecretAccessKey:  types.StringValue("keep-me"),
		ConnectionString: types.StringValue("keep-me-too"),
	}

	applyBackupBucketToState(&api.BackupBucket{Id: "b-1", BucketProvider: "GCP", BucketPath: "p", AccessKeyId: "k"}, state)

	if state.ID.ValueString() != "b-1" || state.AccessKeyID.ValueString() != "k" || !state.ContainerName.IsNull() {
		t.Errorf("unexpected state: %+v", state)
	}
	if state.SecretAccessKey.ValueString() != "keep-me" || state.ConnectionString.ValueString() != "keep-me-too" {
		t.Errorf("secrets must be preserved: %+v", state)
	}
}
