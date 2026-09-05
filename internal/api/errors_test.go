package api

import (
	"errors"
	"testing"
)

func TestIsForbidden(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "403", err: errors.New("status: 403, body: forbidden"), want: true},
		{name: "404 is not forbidden", err: errors.New("status: 404, body: not found"), want: false},
		{name: "non-status error", err: errors.New("connection refused"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsForbidden(tc.err); got != tc.want {
				t.Errorf("IsForbidden(%v) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsServiceIdle(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{
			name: "424 for idle service",
			err:  errors.New(`status: 424, body: {"requestId":"x","error":"FAILED_DEPENDENCY: ClickPipe creation is allowed only when the ClickHouse service is running. Current state: idle","status":424}`),
			want: true,
		},
		{
			name: "424 for stopped service must not trigger a wake",
			err:  errors.New(`status: 424, body: {"requestId":"x","error":"FAILED_DEPENDENCY: ClickPipe creation is allowed only when the ClickHouse service is running. Current state: stopped","status":424}`),
			want: false,
		},
		{
			name: "non-424 mentioning idle",
			err:  errors.New("status: 400, body: Current state: idle"),
			want: false,
		},
		{name: "non-status error", err: errors.New("connection refused"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsServiceIdle(tc.err); got != tc.want {
				t.Errorf("IsServiceIdle(%v) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestStatusFromMessage(t *testing.T) {
	cases := []struct {
		name    string
		message string
		want    int
	}{
		{name: "empty", message: "", want: 0},
		{
			name:    "raw client error",
			message: `status: 404, body: {"error":"not found"}`,
			want:    404,
		},
		{
			// The client records the status at the front, but callers wrap
			// errors with context before classifying them. The status must
			// still be found once it is no longer a prefix.
			name:    "wrapped with context",
			message: `check whether UDF "f" already exists: status: 404, body: not found`,
			want:    404,
		},
		{name: "server error", message: "status: 502, body: bad gateway", want: 502},
		{name: "no space after colon", message: "status:410, body: gone", want: 410},
		{name: "case insensitive", message: "STATUS: 422, body: unprocessable", want: 422},
		{name: "no status present", message: "dial tcp: connection refused", want: 0},
		{
			name:    "unrelated number is not a status",
			message: "gave up after 3 attempts",
			want:    0,
		},
		{
			// Guarded by the trailing word boundary, so a longer run of digits
			// is not silently truncated to a plausible status.
			name:    "four digits is not a status",
			message: "status: 4041, body: nope",
			want:    0,
		},
		{
			// Guarded by the leading word boundary.
			name:    "status must be its own word",
			message: "httpstatus: 404, body: not found",
			want:    0,
		},
		{
			name:    "first status wins",
			message: `status: 502, body: {"error":"upstream said status: 404"}`,
			want:    502,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StatusFromMessage(tc.message); got != tc.want {
				t.Errorf("StatusFromMessage(%q) = %v; want %v", tc.message, got, tc.want)
			}
		})
	}
}

func TestIsBadRequestWith(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		needle string
		want   bool
	}{
		{
			name:   "400 with matching needle",
			err:    errors.New("status: 400, body: cannot set upgrade window on a secondary service"),
			needle: "secondary service",
			want:   true,
		},
		{
			name:   "400 without matching needle",
			err:    errors.New("status: 400, body: malformed request"),
			needle: "secondary service",
			want:   false,
		},
		{
			name:   "403 even with matching body",
			err:    errors.New("status: 403, body: secondary service"),
			needle: "secondary service",
			want:   false,
		},
		{name: "nil", err: nil, needle: "anything", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBadRequestWith(tc.err, tc.needle); got != tc.want {
				t.Errorf("IsBadRequestWith(%v, %q) = %v; want %v", tc.err, tc.needle, got, tc.want)
			}
		})
	}
}
