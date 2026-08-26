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
