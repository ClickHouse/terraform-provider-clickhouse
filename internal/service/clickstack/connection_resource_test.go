package clickstack

import (
	"context"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/clickstack/client"
)

func TestConnectionResource_Schema(t *testing.T) {
	t.Parallel()

	r := NewConnectionResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "team", "name", "host", "username", "password", "hyperdx_setting_prefix", "is_prometheus_endpoint"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected resource schema to contain attribute %q", attr)
		}
	}

	password, ok := resp.Schema.Attributes["password"]
	if !ok {
		t.Fatal("password attribute missing")
	}
	if !password.IsSensitive() {
		t.Error("expected password attribute to be sensitive")
	}
}

// The API omits isPrometheusEndpoint for a connection that never set it. It
// must read as false, the attribute's default, so the connection does not plan
// a change.
func TestConnectionResource_ApplyIsPrometheusEndpoint(t *testing.T) {
	t.Parallel()

	yes, no := true, false
	for name, tc := range map[string]struct {
		in   *bool
		want bool
	}{
		"absent": {nil, false},
		"false":  {&no, false},
		"true":   {&yes, true},
	} {
		var m connectionResourceModel
		m.applyConnection(&client.Connection{IsPrometheusEndpoint: tc.in})
		if !m.IsPrometheusEndpoint.Equal(types.BoolValue(tc.want)) {
			t.Errorf("%s: is_prometheus_endpoint=%s, want %v", name, m.IsPrometheusEndpoint, tc.want)
		}
	}
}
