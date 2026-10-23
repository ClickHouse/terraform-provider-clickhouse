package models

import "github.com/hashicorp/terraform-plugin-framework/types"

// ServiceBackupBucketResourceModel is the Terraform state model for the
// clickhouse_service_backup_bucket resource.
type ServiceBackupBucketResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	ServiceID          types.String `tfsdk:"service_id"`
	BucketProvider     types.String `tfsdk:"bucket_provider"`
	BucketPath         types.String `tfsdk:"bucket_path"`
	IamRoleArn         types.String `tfsdk:"iam_role_arn"`
	IamRoleSessionName types.String `tfsdk:"iam_role_session_name"`
	AccessKeyID        types.String `tfsdk:"access_key_id"`
	SecretAccessKey    types.String `tfsdk:"secret_access_key"`
	ContainerName      types.String `tfsdk:"container_name"`
	ConnectionString   types.String `tfsdk:"connection_string"`
}
