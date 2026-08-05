package models

import "github.com/hashicorp/terraform-plugin-framework/types"

type UDFArgumentModel struct {
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}

type UDFResourceModel struct {
	FunctionName            types.String `tfsdk:"function_name"`
	Runtime                 types.String `tfsdk:"runtime"`
	Arguments               types.List   `tfsdk:"arguments"`
	ReturnType              types.String `tfsdk:"return_type"`
	Type                    types.String `tfsdk:"type"`
	PoolSize                types.Int64  `tfsdk:"pool_size"`
	SourceArchivePath       types.String `tfsdk:"source_archive_path"`
	SourceArchiveHash       types.String `tfsdk:"source_archive_hash"`
	ReturnName              types.String `tfsdk:"return_name"`
	CommandReadTimeout      types.Int64  `tfsdk:"command_read_timeout"`
	CommandWriteTimeout     types.Int64  `tfsdk:"command_write_timeout"`
	MaxCommandExecutionTime types.Int64  `tfsdk:"max_command_execution_time"`
	SendChunkHeader         types.Bool   `tfsdk:"send_chunk_header"`
	Format                  types.String `tfsdk:"format"`
	SandboxType             types.String `tfsdk:"sandbox_type"`
	SandboxVersion          types.String `tfsdk:"sandbox_version"`
	FailOnBuildError        types.Bool   `tfsdk:"fail_on_build_error"`
	Version                 types.Int64  `tfsdk:"version"`
	Status                  types.String `tfsdk:"status"`
	Error                   types.String `tfsdk:"error"`
	CreatedAt               types.String `tfsdk:"created_at"`
	UpdatedAt               types.String `tfsdk:"updated_at"`
}

type UDFAttachmentResourceModel struct {
	FunctionName types.String `tfsdk:"function_name"`
	ServiceID    types.String `tfsdk:"service_id"`
	Version      types.Int64  `tfsdk:"version"`
	Status       types.String `tfsdk:"status"`
}
