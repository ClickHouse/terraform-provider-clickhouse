package api

import (
	"strings"
)

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

// is4xxPermanent reports whether err is a 4xx client error that will never
// resolve by retrying (bad request, unauthorized, forbidden, not found,
// etc.). Callers polling for a resource to reach some state should treat
// this as terminal, not as "not there yet".
//
// 429 (rate limited) and 408 (request timeout) are excluded: both are
// transient by definition. doRequest already retries a 429 internally
// (honoring X-RateLimit-Reset) within its own bounded window
// (backoff.WithMaxElapsedTime(61*time.Second) in doRequestWithStatus) — if
// that budget is exhausted and a plain "status: 429" error reaches here,
// it should still be retried by the caller's own backoff, not treated as
// permanent.
func is4xxPermanent(err error) bool {
	if err == nil || !strings.HasPrefix(err.Error(), "status: 4") {
		return false
	}

	msg := err.Error()
	return !strings.HasPrefix(msg, "status: 429") && !strings.HasPrefix(msg, "status: 408")
}
