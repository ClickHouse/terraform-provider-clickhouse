package tfutils

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func CreateEmptyList(listType attr.Type) types.List {
	emptyList, _ := types.ListValue(listType, []attr.Value{})
	return emptyList
}

func Equal[T comparable](a []T, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
