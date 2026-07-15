package api

import "testing"

// FixMemoryBounds translates the deprecated total-memory fields into per-replica bounds only when the
// per-replica band is absent; for a service that already carries the replica band (min/max replicas) it must
// leave the per-replica memory untouched and fabricate no total-memory fields.
func TestFixMemoryBounds_horizontalNoOp(t *testing.T) {
	s := Service{
		MinReplicas:        intPtr(2),
		MaxReplicas:        intPtr(6),
		MinReplicaMemoryGb: intPtr(16),
		MaxReplicaMemoryGb: intPtr(16),
	}
	s.FixMemoryBounds()

	if s.MinReplicaMemoryGb == nil || *s.MinReplicaMemoryGb != 16 ||
		s.MaxReplicaMemoryGb == nil || *s.MaxReplicaMemoryGb != 16 {
		t.Errorf("FixMemoryBounds altered the per-replica memory of a banded service: min=%v max=%v", s.MinReplicaMemoryGb, s.MaxReplicaMemoryGb)
	}
	if s.MinTotalMemoryGb != nil || s.MaxTotalMemoryGb != nil {
		t.Errorf("FixMemoryBounds fabricated total-memory fields: min=%v max=%v", s.MinTotalMemoryGb, s.MaxTotalMemoryGb)
	}
}
