package api

import (
	"regexp"
	"strconv"
	"strings"
)

// statusPattern matches the HTTP status this package records when a request
// fails, formatted as `status: %d, body: %s` (see common.go). The word
// boundaries keep it from matching a longer run of digits or a status-like
// suffix of another word.
var statusPattern = regexp.MustCompile(`(?i)\bstatus:\s*(\d{3})\b`)

// StatusFromMessage returns the HTTP status recorded in an error message or
// response body, or 0 if it holds none.
//
// This package owns the error format, so it also owns recovering the status
// from it: code outside this package must not parse error text itself. Unlike
// the prefix-matching predicates below, this finds the status anywhere in the
// message, so it still works once a caller has wrapped the error with context.
func StatusFromMessage(message string) int {
	match := statusPattern.FindStringSubmatch(message)
	if len(match) != 2 {
		return 0
	}

	status, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}

	return status
}

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}

	return strings.HasPrefix(err.Error(), "status: 404")
}

func IsConflict(err error) bool {
	if err == nil {
		return false
	}

	return strings.HasPrefix(err.Error(), "status: 409")
}

func IsForbidden(err error) bool {
	if err == nil {
		return false
	}

	return strings.HasPrefix(err.Error(), "status: 403")
}

// IsBadRequestWith returns true when err is a 400 whose body contains needle.
// Use to specialize diagnostics for documented client errors (e.g. the OpenAPI
// "cannot set upgrade window on a secondary service" 400).
func IsBadRequestWith(err error, needle string) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.HasPrefix(msg, "status: 400") && strings.Contains(msg, needle)
}

// IsServiceIdle returns true when err is the 424 FAILED_DEPENDENCY the
// ClickPipes API returns because the target ClickHouse service is idle.
// A 424 for a stopped service does NOT match: stopping is an explicit user
// action the provider must not override by waking the service.
func IsServiceIdle(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	return strings.HasPrefix(msg, "status: 424") && strings.Contains(msg, "Current state: idle")
}

func is5xx(err error) bool {
	if err == nil {
		return false
	}

	return strings.HasPrefix(err.Error(), "status: 5")
}
