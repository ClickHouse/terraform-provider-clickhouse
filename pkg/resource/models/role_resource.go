package models

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ClickHouse/terraform-provider-clickhouse/pkg/internal/api"
)

type RolePolicyTagsModel struct {
	RoleV2 types.String `tfsdk:"role"`
}

func (r RolePolicyTagsModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"role": types.StringType,
		},
	}
}

func (r RolePolicyTagsModel) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(r.ObjectType().AttrTypes, map[string]attr.Value{
		"role": r.RoleV2,
	})
}

type RolePolicyModel struct {
	ID          types.String `tfsdk:"id"`
	RoleID      types.String `tfsdk:"role_id"`
	TenantID    types.String `tfsdk:"tenant_id"`
	Effect      types.String `tfsdk:"effect"`
	Permissions types.Set    `tfsdk:"permissions"`
	Resources   types.Set    `tfsdk:"resources"`
	Tags        types.Object `tfsdk:"tags"`
}

func (r RolePolicyModel) ObjectType() types.ObjectType {
	return types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":          types.StringType,
			"role_id":     types.StringType,
			"tenant_id":   types.StringType,
			"effect":      types.StringType,
			"permissions": types.SetType{ElemType: types.StringType},
			"resources":   types.SetType{ElemType: types.StringType},
			"tags":        RolePolicyTagsModel{}.ObjectType(),
		},
	}
}

func (r RolePolicyModel) ObjectValue() basetypes.ObjectValue {
	return types.ObjectValueMust(r.ObjectType().AttrTypes, map[string]attr.Value{
		"id":          r.ID,
		"role_id":     r.RoleID,
		"tenant_id":   r.TenantID,
		"effect":      r.Effect,
		"permissions": r.Permissions,
		"resources":   r.Resources,
		"tags":        r.Tags,
	})
}

type RoleResourceModel struct {
	ID        types.String `tfsdk:"id"`
	TenantID  types.String `tfsdk:"tenant_id"`
	OwnerID   types.String `tfsdk:"owner_id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Policies  types.List   `tfsdk:"policies"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

// APIToRolePolicyModel converts an API RBACPolicy into the Terraform model.
func APIToRolePolicyModel(p api.RBACPolicy) (RolePolicyModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	var permsSet types.Set
	if len(p.Permissions) == 0 {
		permsSet = types.SetNull(types.StringType)
	} else {
		permValues := make([]attr.Value, len(p.Permissions))
		for i, perm := range p.Permissions {
			permValues[i] = types.StringValue(perm)
		}
		var d diag.Diagnostics
		permsSet, d = types.SetValue(types.StringType, permValues)
		diags.Append(d...)
		if diags.HasError() {
			return RolePolicyModel{}, diags
		}
	}

	var resSet types.Set
	if len(p.Resources) == 0 {
		resSet = types.SetNull(types.StringType)
	} else {
		resValues := make([]attr.Value, len(p.Resources))
		for i, res := range p.Resources {
			resValues[i] = types.StringValue(res)
		}
		var d diag.Diagnostics
		resSet, d = types.SetValue(types.StringType, resValues)
		diags.Append(d...)
		if diags.HasError() {
			return RolePolicyModel{}, diags
		}
	}

	var tagsObj types.Object
	if p.Tags != nil && p.Tags.RoleV2 != "" {
		tagsObj = RolePolicyTagsModel{RoleV2: types.StringValue(p.Tags.RoleV2)}.ObjectValue()
	} else {
		tagsObj = types.ObjectNull(RolePolicyTagsModel{}.ObjectType().AttrTypes)
	}

	return RolePolicyModel{
		ID:          types.StringValue(p.ID),
		RoleID:      types.StringValue(p.RoleID),
		TenantID:    types.StringValue(p.TenantID),
		Effect:      types.StringValue(string(p.AllowDeny)),
		Permissions: permsSet,
		Resources:   resSet,
		Tags:        tagsObj,
	}, diags
}
