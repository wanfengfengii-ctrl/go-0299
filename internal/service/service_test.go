package service

import (
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// validSpec returns a minimal but complete, valid task spec.
func validSpec() domain.TaskSpec {
	return domain.TaskSpec{
		Area:      "building-a",
		Direction: domain.DirectionRising,
		Geometry: domain.WallGeometry{
			Wall: domain.Rect{X: 0, Y: 0, W: 1000, H: 2000},
			Layers: []domain.Layer{
				{Number: 1, Rect: domain.Rect{X: 0, Y: 0, W: 1000, H: 1000}},
				{Number: 2, Rect: domain.Rect{X: 0, Y: 1000, W: 1000, H: 1000}},
			},
			Cells: []domain.Cell{
				{Layer: 1, Seq: 0, Rect: domain.Rect{X: 0, Y: 0, W: 1000, H: 1000}},
				{Layer: 2, Seq: 0, Rect: domain.Rect{X: 0, Y: 1000, W: 1000, H: 1000}},
			},
			PressureAdj: [][2]domain.CellRef{
				{{Layer: 1, Seq: 0}, {Layer: 2, Seq: 0}},
			},
		},
		Batches: []domain.MaterialBatch{
			{ID: "b1", Component: domain.ComponentRawEarth, Source: "pit-1", BalanceG: 100000},
			{ID: "b2", Component: domain.ComponentGravel, Source: "pit-1", BalanceG: 100000},
			{ID: "b3", Component: domain.ComponentStabilizer, Source: "pit-1", BalanceG: 100000},
			{ID: "b4", Component: domain.ComponentWater, Source: "pit-1", BalanceG: 100000},
		},
		Recipe:         domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20},
		TargetMoisture: 120,
		Program:        domain.CompactionProgram{LooseThickness: 100, PassesPerCell: 2, BlowsPerPass: 10, RammerWeightG: 10000, FallHeightMM: 500},
		Thresholds:     domain.Thresholds{MinDryDensity: 1800000, MaxDryDensity: 2000000, MinCompaction: 950, MinMoisture: 80, MaxMoisture: 150, MinShear: 1000, MaxErosion: 50, MaxDeviation: 5},
		Curing:         domain.CuringSchedule{HoursPerLayer: 24, MinHours: 72},
		MixPlan:        domain.MixPlan{PanCount: 10, PanSizeG: 1000, UsableUnits: 100},
	}
}

func lockTask(t *testing.T, s *Service) *LockResult {
	t.Helper()
	res, err := s.Lock(validSpec())
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	return res
}

func TestLockRejectsStaleGeometry(t *testing.T) {
	s := New(store.NewMemoryStore())
	spec := validSpec()
	spec.Geometry.Cells = append(spec.Geometry.Cells, domain.Cell{Layer: 1, Seq: 1, Rect: domain.Rect{X: 500, Y: 0, W: 600, H: 1000}})
	if _, err := s.Lock(spec); err == nil {
		t.Fatal("want overlap rejection")
	}
}

func TestMaterialConservation(t *testing.T) {
	s := New(store.NewMemoryStore())
	res := lockTask(t, s)

	// Mix pan 1 (consumes recipe from source into mix).
	mix := OperationRequest{
		OperationID: "op-mix-1", Digest: res.Digest, Generation: 1, At: 10,
		Kind:   domain.ProcessMixing,
		Recipe: &domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20},
	}
	if _, err := s.ApplyOperation(res.TaskID, mix); err != nil {
		t.Fatalf("mix: %v", err)
	}

	// Place 500g of pan 1 into wall.
	place := OperationRequest{
		OperationID: "op-place-1", Digest: res.Digest, Generation: 1, At: 20,
		Kind: domain.ProcessPlacing, Layer: 1, Seq: 0, PanSeq: 1, AmountG: 500,
	}
	if _, err := s.ApplyOperation(res.TaskID, place); err != nil {
		t.Fatalf("place: %v", err)
	}

	snap, ok, err := s.Snapshot(res.TaskID)
	if err != nil || !ok {
		t.Fatalf("snapshot: ok=%v err=%v", ok, err)
	}
	if snap.Balances[domain.ComponentRawEarth] != 99100 {
		t.Fatalf("raw_earth=%d want 99100", snap.Balances[domain.ComponentRawEarth])
	}
	if snap.Balances[domain.ComponentGravel] != 99950 {
		t.Fatalf("gravel=%d want 99950", snap.Balances[domain.ComponentGravel])
	}
	if snap.Balances[domain.ComponentMix] != 500 {
		t.Fatalf("mix=%d want 500", snap.Balances[domain.ComponentMix])
	}
	if snap.Balances[domain.ComponentInWall] != 500 {
		t.Fatalf("in_wall=%d want 500", snap.Balances[domain.ComponentInWall])
	}
}

func TestIdempotencyReplayAndConflict(t *testing.T) {
	s := New(store.NewMemoryStore())
	res := lockTask(t, s)

	mix := OperationRequest{
		OperationID: "op-mix-1", Digest: res.Digest, Generation: 1, At: 10,
		Kind:   domain.ProcessMixing,
		Recipe: &domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20},
	}
	first, err := s.ApplyOperation(res.TaskID, mix)
	if err != nil {
		t.Fatalf("first mix: %v", err)
	}
	// Replay with identical content returns the same result.
	second, err := s.ApplyOperation(res.TaskID, mix)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if first.PanSeq != second.PanSeq {
		t.Fatalf("replay pan seq %d != %d", first.PanSeq, second.PanSeq)
	}
	// Reuse the same operation id with different content -> conflict.
	conflict := mix
	conflict.Recipe = &domain.Recipe{RawEarthG: 800, GravelG: 100, StabilizerG: 50, WaterG: 50}
	if _, err := s.ApplyOperation(res.TaskID, conflict); err == nil || !rules.Is(err, rules.CodeIdempotencyConflict) {
		t.Fatalf("want IDEMPOTENCY_CONFLICT, got %v", err)
	}
}

func TestGenerationIsolationAfterRebuild(t *testing.T) {
	s := New(store.NewMemoryStore())
	res := lockTask(t, s)

	// Record a finding on cell (1,0).
	insp := InspectionRequest{
		OperationID: "op-insp-1", Digest: res.Digest, Generation: 1, At: 10,
		Metric: "dry_density", Value: 1000, Point: "p1", Layer: 1, Seq: 0,
	}
	insRes, err := s.Inspect(res.TaskID, insp)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insRes.Passed {
		t.Fatal("expected finding (value below min density)")
	}

	reb := RebuildRequest{
		OperationID: "op-rebuild-1", Digest: res.Digest, Generation: 1, At: 20,
		FindingID: insRes.FindingID, Reason: "low density",
	}
	rebRes, err := s.Rebuild(res.TaskID, reb)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if rebRes.NewGen != 2 {
		t.Fatalf("new gen=%d want 2", rebRes.NewGen)
	}

	// A late receipt carrying the old generation is rejected.
	stale := OperationRequest{
		OperationID: "op-stale", Digest: res.Digest, Generation: 1, At: 30,
		Kind: domain.ProcessCuring, Layer: 1,
	}
	if _, err := s.ApplyOperation(res.TaskID, stale); err == nil || !rules.Is(err, rules.CodeGenerationStale) {
		t.Fatalf("want GENERATION_STALE, got %v", err)
	}
}

func TestVerdictCompetition(t *testing.T) {
	s := New(store.NewMemoryStore())
	res := lockTask(t, s)

	if err := s.SubmitReview(res.TaskID, ReviewRequest{OperationID: "r1", Digest: res.Digest, Generation: 1, At: 10, Reviewer: "alice", Qualified: true, Conclusion: "pass"}); err != nil {
		t.Fatalf("review1: %v", err)
	}
	if err := s.SubmitReview(res.TaskID, ReviewRequest{OperationID: "r2", Digest: res.Digest, Generation: 1, At: 20, Reviewer: "bob", Qualified: true, Conclusion: "pass"}); err != nil {
		t.Fatalf("review2: %v", err)
	}

	v1, err := s.SubmitVerdict(res.TaskID, VerdictRequest{OperationID: "v1", Digest: res.Digest, Generation: 1, At: 30, Kind: domain.VerdictClear})
	if err != nil {
		t.Fatalf("verdict1: %v", err)
	}
	if v1.Kind != domain.VerdictClear {
		t.Fatalf("kind=%s want clearance", v1.Kind)
	}
	// A conflicting terminal verdict is rejected.
	if _, err := s.SubmitVerdict(res.TaskID, VerdictRequest{OperationID: "v2", Digest: res.Digest, Generation: 1, At: 40, Kind: domain.VerdictIsolate}); err == nil || !rules.Is(err, rules.CodeFinalConflict) {
		t.Fatalf("want FINAL_CONFLICT, got %v", err)
	}
}
