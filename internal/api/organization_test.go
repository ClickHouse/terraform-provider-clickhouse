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

// The organization payload carries both accountId and its deprecated alias
// accountName; decoding must read accountId.
func TestOrgResultUnmarshalsByocConfigAccountId(t *testing.T) {
	payload := `{
		"id": "o1",
		"byocConfig": [{
			"id": "byoc-1",
			"state": "ready",
			"accountId": "123456789012",
			"accountName": "123456789012",
			"regionId": "us-east-1",
			"cloudProvider": "aws",
			"displayName": "prod"
		}]
	}`
	var org OrgResult
	if err := json.Unmarshal([]byte(payload), &org); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(org.ByocConfig) != 1 {
		t.Fatalf("byocConfig length = %d, want 1", len(org.ByocConfig))
	}
	if got := org.ByocConfig[0].AccountId; got != "123456789012" {
		t.Errorf("accountId = %q, want 123456789012", got)
	}
}
