package tfutils

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func CreateEmptyList(listType attr.Type) types.List {
	emptyList, _ := types.ListValue(listType, []attr.Value{})
	return emptyList
}
