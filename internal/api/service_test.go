package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// A horizontal create must pass the mode, the replica band, and the fixed (min == max) per-replica memory
// through on POST /services, and must not fabricate any vertical-only fields (num_replicas or the deprecated
// totals — FixMemoryBounds should be a no-op when the band is present).
func TestCreateService_horizontalBand(t *testing.T) {
	var gotBody map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"result":{"service":{"id":"svc-1","name":"svc"},"password":"p"}}`))
	})

	_, _, err := client.CreateService(context.Background(), Service{
		Name:               "svc",
		Provider:           "aws",
		Region:             "us-east-1",
		Tier:               TierPPv2,
		AutoscalingMode:    "horizontal",
		MinReplicas:        intPtr(2),
		MaxReplicas:        intPtr(6),
		MinReplicaMemoryGb: intPtr(16),
		MaxReplicaMemoryGb: intPtr(16),
	})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	if gotBody["autoscalingMode"] != "horizontal" ||
		gotBody["minReplicas"] != float64(2) || gotBody["maxReplicas"] != float64(6) ||
		gotBody["minReplicaMemoryGb"] != float64(16) || gotBody["maxReplicaMemoryGb"] != float64(16) {
		t.Errorf("POST body missing the horizontal mode/band/memory: %v", gotBody)
	}
	// Horizontal must not carry a fixed replica count or the deprecated totals — FixMemoryBounds is a no-op here.
	for _, k := range []string{"numReplicas", "minTotalMemoryGb", "maxTotalMemoryGb"} {
		if _, present := gotBody[k]; present {
			t.Errorf("POST body unexpectedly carried vertical-only field %q for a horizontal create: %v", k, gotBody)
		}
	}
}
