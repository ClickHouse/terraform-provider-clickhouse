package utils

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func AlphaWarning(resourceName string, diags *diag.Diagnostics) {
	diags.AddWarning(
		"Alpha Resource",
		"\""+resourceName+"\" is in alpha and its behavior may change in future provider versions.",
	)
}
