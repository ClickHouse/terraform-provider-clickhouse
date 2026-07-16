package clickstack

import (
	"context"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestConnectionResource_Schema(t *testing.T) {
	t.Parallel()

	r := NewConnectionResource()
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %s", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "team", "name", "host", "username", "password", "hyperdx_setting_prefix", "prometheus_endpoint"} {
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
