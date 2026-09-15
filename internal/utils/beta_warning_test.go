package utils

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// Pins the user-visible wording. Every pre-GA resource routes its notice
// through BetaWarning, so this is the one place a revert to "alpha" shows up.
func TestBetaWarning(t *testing.T) {
	var diags diag.Diagnostics
	BetaWarning("clickhouse_clickstack_dashboard", &diags)

	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %s", len(diags), diags)
	}
	d := diags[0]
	if d.Severity() != diag.SeverityWarning {
		t.Errorf("severity = %v, want warning", d.Severity())
	}
	if got, want := d.Summary(), "Beta Resource"; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
	if got, want := d.Detail(), `"clickhouse_clickstack_dashboard" is in beta and its behavior may change in future provider versions.`; got != want {
		t.Errorf("detail = %q, want %q", got, want)
	}
}

func TestBetaWarningSuppressed(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(SuppressBetaWarningsEnvVar, value)

			var diags diag.Diagnostics
			BetaWarning("clickhouse_clickstack_dashboard", &diags)

			if len(diags) != 0 {
				t.Errorf("got %d diagnostics, want 0: %s", len(diags), diags)
			}
		})
	}
}

// A value that isn't a bool must not silence the notice: a typo in the env var
// should leave the warnings on rather than hide them.
func TestBetaWarningIgnoresNonBoolEnv(t *testing.T) {
	t.Setenv(SuppressBetaWarningsEnvVar, "yes-please")

	var diags diag.Diagnostics
	BetaWarning("clickhouse_clickstack_dashboard", &diags)

	if len(diags) != 1 {
		t.Errorf("got %d diagnostics, want 1: %s", len(diags), diags)
	}
}
