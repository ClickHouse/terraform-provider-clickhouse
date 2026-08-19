package utils

import (
	"github.com/hashicorp/terraform-plugin-framework/diag"
)

func BetaWarning(resourceName string, diags *diag.Diagnostics) {
	diags.AddWarning(
		"Beta Resource",
		"\""+resourceName+"\" is in beta and its behavior may change in future provider versions.",
	)
}
