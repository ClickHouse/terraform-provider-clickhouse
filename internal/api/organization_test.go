package api

import (
	"encoding/json"
	"testing"
)

func TestOrgResultUnmarshalsCapabilities(t *testing.T) {
	var withCap OrgResult
	if err := json.Unmarshal([]byte(`{"id":"o1","capabilities":{"snapshots":true}}`), &withCap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if withCap.Capabilities == nil || withCap.Capabilities.Snapshots == nil || !*withCap.Capabilities.Snapshots {
		t.Errorf("capabilities.snapshots = %+v, want a pointer to true", withCap.Capabilities)
	}

	// An API version predating the field omits capabilities entirely — it must stay nil so the plan-time gate
	// treats it as "unknown" rather than "ineligible".
	var without OrgResult
	if err := json.Unmarshal([]byte(`{"id":"o1"}`), &without); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if without.Capabilities != nil {
		t.Errorf("capabilities = %+v, want nil when absent from the response", without.Capabilities)
	}
}
