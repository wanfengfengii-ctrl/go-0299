package arbiter

import (
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
)

func ref(layer, seq int) domain.CellRef { return domain.CellRef{Layer: layer, Seq: seq} }

func TestRebuildSetSamePanAndAdjacency(t *testing.T) {
	// Cells 0 and 1 in layer 1 share a pan; layer 2 cell 0 bears on layer 1
	// cell 0. A finding on (1,0) must pull in (1,1) via the pan and (2,0) via
	// the vertical adjacency edge.
	adj := [][2]domain.CellRef{
		{ref(1, 0), ref(2, 0)},
	}
	panOf := map[domain.CellRef]int{
		ref(1, 0): 1,
		ref(1, 1): 1,
		ref(2, 0): 2,
	}
	set := RebuildSet(ref(1, 0), adj, panOf)
	want := []domain.CellRef{ref(1, 0), ref(1, 1), ref(2, 0)}
	if len(set) != len(want) {
		t.Fatalf("set=%v want %v", set, want)
	}
	for i := range want {
		if set[i] != want[i] {
			t.Fatalf("set[%d]=%v want %v (set=%v)", i, set[i], want[i], set)
		}
	}
}

func TestRebuildSetDeterministicOrder(t *testing.T) {
	adj := [][2]domain.CellRef{
		{ref(2, 1), ref(1, 0)},
		{ref(1, 0), ref(2, 0)},
	}
	panOf := map[domain.CellRef]int{}
	// Origin has no pan, so only adjacency reachable.
	set := RebuildSet(ref(2, 0), adj, panOf)
	// (2,0) is adjacent to (1,0); (1,0) is adjacent to (2,1). Sorted by layer,seq.
	want := []domain.CellRef{ref(1, 0), ref(2, 0), ref(2, 1)}
	if len(set) != len(want) {
		t.Fatalf("set=%v want %v", set, want)
	}
	for i := range want {
		if set[i] != want[i] {
			t.Fatalf("set[%d]=%v want %v", i, set[i], want[i])
		}
	}
}

func TestJudgeThresholds(t *testing.T) {
	th := domain.Thresholds{
		MinDryDensity: 1800000,
		MaxDryDensity: 2000000,
		MinMoisture:   80,
		MaxMoisture:   150,
		MinShear:      1000,
		MaxErosion:    50,
	}
	if k, ok := Judge(MetricDryDensity, 1700000, th); !ok || k != domain.FindingLowDensity {
		t.Fatalf("low density: kind=%s ok=%v", k, ok)
	}
	if _, ok := Judge(MetricDryDensity, 1900000, th); ok {
		t.Fatal("in-range density should pass")
	}
	if k, ok := Judge(MetricMoisture, 200, th); !ok || k != domain.FindingMoistureOut {
		t.Fatalf("moisture out: kind=%s ok=%v", k, ok)
	}
	if k, ok := Judge(MetricErosion, 60, th); !ok || k != domain.FindingErosion {
		t.Fatalf("erosion over: kind=%s ok=%v", k, ok)
	}
	if k, ok := Judge(MetricShear, 500, th); !ok || k != domain.FindingLowShear {
		t.Fatalf("low shear: kind=%s ok=%v", k, ok)
	}
}
