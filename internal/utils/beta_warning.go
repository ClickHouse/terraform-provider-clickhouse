package utils

import (
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// SuppressBetaWarningsEnvVar lets a practitioner acknowledge beta status once
// and stop the notices. On a config with many beta resources they crowd out the
// warnings that need acting on: https://github.com/ClickHouse/terraform-provider-clickhouse/issues/696
const SuppressBetaWarningsEnvVar = "CLICKHOUSE_SUPPRESS_BETA_WARNINGS"

func BetaWarning(resourceName string, diags *diag.Diagnostics) {
	// A value that isn't a bool leaves the warnings on, so a typo can't silence
	// them without anyone noticing.
	if suppressed, _ := strconv.ParseBool(os.Getenv(SuppressBetaWarningsEnvVar)); suppressed {
		return
	}

	diags.AddWarning(
		"Beta Resource",
		"\""+resourceName+"\" is in beta and its behavior may change in future provider versions.",
	)
}
