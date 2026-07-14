package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The provider passes an explicit horizontal autoscaling_mode plus the replica band and fixed per-replica
// memory straight through to the API. This test pins that they reach the wire on the update (PATCH
// /replicaScaling) path. The create-path counterpart lives in service_test.go (source-aligned with
// CreateService).

func TestUpdateReplicaScaling_horizontalBand(t *testing.T) {
	var gotBody map[string]any
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		_, _ = w.Write([]byte(`{"result":{"id":"svc-1","name":"svc","autoscalingMode":"horizontal","minReplicas":2,"maxReplicas":6,"minReplicaMemoryGb":16,"maxReplicaMemoryGb":16}}`))
	})

	_, err := client.UpdateReplicaScaling(context.Background(), "svc-1", ReplicaScalingUpdate{
		AutoscalingMode:    strPtr("horizontal"),
		MinReplicas:        intPtr(2),
		MaxReplicas:        intPtr(6),
		MinReplicaMemoryGb: intPtr(16),
		MaxReplicaMemoryGb: intPtr(16),
	})
	if err != nil {
		t.Fatalf("UpdateReplicaScaling: %v", err)
	}

	want := map[string]any{
		"autoscalingMode":    "horizontal",
		"minReplicas":        float64(2),
		"maxReplicas":        float64(6),
		"minReplicaMemoryGb": float64(16),
		"maxReplicaMemoryGb": float64(16),
	}
	if diff := cmp.Diff(want, gotBody); diff != "" {
		t.Errorf("PATCH body mismatch (-want +got):\n%s", diff)
	}
}
