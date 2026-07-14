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
