package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestConfigSelected guards the fix that a mode counts as chosen only when its
// attribute is non-null AND non-empty: an attribute wired to an empty-defaulting
// variable is non-null with "" and must not select a mode (which would drop
// legitimate env creds or resurrect the false-conflict abort).
func TestConfigSelected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		v    types.String
		want bool
	}{
		{"null", types.StringNull(), false},
		{"unknown", types.StringUnknown(), false},
		{"empty string", types.StringValue(""), false},
		{"non-empty", types.StringValue("svc"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := configSelected(tc.v); got != tc.want {
				t.Errorf("configSelected(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestResolveClickStackCreds covers the "written config wins" precedence: an
// explicit mode in the provider block must drop stray environment values for
// the other mode, while leaving genuine conflicts (both modes, or both from the
// environment) for the caller's validation to reject.
func TestResolveClickStackCreds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                                    string
		endpoint, apiKey, serviceID             string
		cloudInConfig, selfHostInConfig         bool
		wantEndpoint, wantAPIKey, wantServiceID string
	}{
		{
			name: "cloud config ignores stray self-hosted env",
			// service_id from config, endpoint+api_key leaked from env.
			endpoint: "http://stray:8000", apiKey: "stray-key", serviceID: "svc",
			cloudInConfig: true, selfHostInConfig: false,
			wantEndpoint: "", wantAPIKey: "", wantServiceID: "svc",
		},
		{
			name:     "self-hosted config ignores stray cloud env",
			endpoint: "http://ch:8000", apiKey: "key", serviceID: "stray-svc",
			cloudInConfig: false, selfHostInConfig: true,
			wantEndpoint: "http://ch:8000", wantAPIKey: "key", wantServiceID: "",
		},
		{
			name: "self-hosted config keeps same-mode env gap-fill",
			// endpoint from config, api_key supplied by env for the same mode.
			endpoint: "http://ch:8000", apiKey: "env-key", serviceID: "",
			cloudInConfig: false, selfHostInConfig: true,
			wantEndpoint: "http://ch:8000", wantAPIKey: "env-key", wantServiceID: "",
		},
		{
			name:     "both modes in config passes through (conflict left to validation)",
			endpoint: "", apiKey: "key", serviceID: "svc",
			cloudInConfig: true, selfHostInConfig: true,
			wantEndpoint: "", wantAPIKey: "key", wantServiceID: "svc",
		},
		{
			name:     "neither in config passes env through unchanged",
			endpoint: "", apiKey: "env-key", serviceID: "env-svc",
			cloudInConfig: false, selfHostInConfig: false,
			wantEndpoint: "", wantAPIKey: "env-key", wantServiceID: "env-svc",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, k, s := resolveClickStackCreds(tc.endpoint, tc.apiKey, tc.serviceID, tc.cloudInConfig, tc.selfHostInConfig)
			if e != tc.wantEndpoint || k != tc.wantAPIKey || s != tc.wantServiceID {
				t.Errorf("resolveClickStackCreds = (%q,%q,%q), want (%q,%q,%q)",
					e, k, s, tc.wantEndpoint, tc.wantAPIKey, tc.wantServiceID)
			}
		})
	}
}
