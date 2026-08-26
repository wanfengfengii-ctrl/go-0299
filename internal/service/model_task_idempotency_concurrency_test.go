package service_test

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/service"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// idempotencyGateStore makes both callers observe the same pre-operation
// idempotency state. This models a gateway delivering duplicate requests at
// the same instant without relying on scheduler timing.
type idempotencyGateStore struct {
	store.Store
	mu       sync.Mutex
	arrivals int
	released bool
	release  chan struct{}
}

func newIdempotencyGateStore(st store.Store) *idempotencyGateStore {
	return &idempotencyGateStore{Store: st, release: make(chan struct{})}
}

func (s *idempotencyGateStore) Idempotency(scope string, op domain.OperationID) (domain.IdempotencyRecord, bool) {
	rec, ok := s.Store.Idempotency(scope, op)
	s.mu.Lock()
	if s.arrivals < 2 {
		s.arrivals++
		if s.arrivals == 1 {
			time.AfterFunc(250*time.Millisecond, func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				if !s.released {
					s.released = true
					close(s.release)
				}
			})
		}
		if s.arrivals == 2 && !s.released {
			s.released = true
			close(s.release)
		}
	}
	release := s.release
	s.mu.Unlock()
	<-release
	return rec, ok
}

func modelTaskSpec(area string) domain.TaskSpec {
	return domain.TaskSpec{
		Area: area, Direction: domain.DirectionRising,
		Geometry: domain.WallGeometry{
			Wall:   domain.Rect{X: 0, Y: 0, W: 1000, H: 1000},
			Layers: []domain.Layer{{Number: 1, Rect: domain.Rect{X: 0, Y: 0, W: 1000, H: 1000}}},
			Cells:  []domain.Cell{{Layer: 1, Seq: 0, Rect: domain.Rect{X: 0, Y: 0, W: 1000, H: 1000}}},
		},
		Batches: []domain.MaterialBatch{
			{ID: "raw", Component: domain.ComponentRawEarth, Source: "pit", BalanceG: 100000},
			{ID: "gravel", Component: domain.ComponentGravel, Source: "pit", BalanceG: 100000},
			{ID: "stabilizer", Component: domain.ComponentStabilizer, Source: "pit", BalanceG: 100000},
			{ID: "water", Component: domain.ComponentWater, Source: "pit", BalanceG: 100000},
		},
		Recipe:         domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20},
		TargetMoisture: 120,
		Program:        domain.CompactionProgram{LooseThickness: 100, PassesPerCell: 2, BlowsPerPass: 10, RammerWeightG: 10000, FallHeightMM: 500},
		Thresholds:     domain.Thresholds{MinDryDensity: 1800000, MaxDryDensity: 2000000, MinCompaction: 950, MinMoisture: 80, MaxMoisture: 150, MinShear: 1000, MaxErosion: 50, MaxDeviation: 5},
		Curing:         domain.CuringSchedule{HoursPerLayer: 24, MinHours: 72},
		MixPlan:        domain.MixPlan{PanCount: 10, PanSizeG: 1000, UsableUnits: 100},
	}
}

func TestModel_TaskScopedConcurrentIdempotency(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "concurrent identical sieving replays the canonical result once",
			run: func(t *testing.T) {
				gated := newIdempotencyGateStore(store.NewMemoryStore())
				svc := service.New(gated)
				locked, err := svc.Lock(modelTaskSpec("same-content"))
				if err != nil {
					t.Fatalf("lock task: %v", err)
				}
				req := service.OperationRequest{OperationID: "sieve-duplicate", Digest: locked.Digest, Generation: locked.Generation, At: 10, Kind: domain.ProcessSieving, AmountG: 100}

				results := make([]*service.OperationResult, 2)
				errs := make([]error, 2)
				var wg sync.WaitGroup
				for i := range results {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						results[i], errs[i] = svc.ApplyOperation(locked.TaskID, req)
					}(i)
				}
				wg.Wait()

				for i, err := range errs {
					if err != nil {
						t.Fatalf("caller %d: identical replay returned %v", i, err)
					}
				}
				if !reflect.DeepEqual(results[0], results[1]) {
					t.Fatalf("replay results differ: %#v and %#v", results[0], results[1])
				}
				snap, ok, err := svc.Snapshot(locked.TaskID)
				if err != nil || !ok {
					t.Fatalf("snapshot: ok=%v err=%v", ok, err)
				}
				if got := len(snap.Events); got != 1 {
					t.Fatalf("sieving evidence count=%d, want 1", got)
				}
				if got := snap.Balances[domain.ComponentRawEarth]; got != 99900 {
					t.Fatalf("raw earth balance=%d, want 99900", got)
				}
			},
		},
		{
			name: "concurrent changed quantity conflicts and mutates once",
			run: func(t *testing.T) {
				gated := newIdempotencyGateStore(store.NewMemoryStore())
				svc := service.New(gated)
				locked, err := svc.Lock(modelTaskSpec("changed-content"))
				if err != nil {
					t.Fatalf("lock task: %v", err)
				}
				requests := []service.OperationRequest{
					{OperationID: "sieve-correction", Digest: locked.Digest, Generation: locked.Generation, At: 10, Kind: domain.ProcessSieving, AmountG: 100},
					{OperationID: "sieve-correction", Digest: locked.Digest, Generation: locked.Generation, At: 10, Kind: domain.ProcessSieving, AmountG: 250},
				}
				errs := make([]error, len(requests))
				var wg sync.WaitGroup
				for i := range requests {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						_, errs[i] = svc.ApplyOperation(locked.TaskID, requests[i])
					}(i)
				}
				wg.Wait()

				successes, conflicts := 0, 0
				for _, err := range errs {
					switch {
					case err == nil:
						successes++
					case rules.Is(err, rules.CodeIdempotencyConflict):
						conflicts++
					default:
						t.Fatalf("changed replay returned non-idempotency error: %v", err)
					}
				}
				if successes != 1 || conflicts != 1 {
					t.Fatalf("successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
				}
				snap, ok, err := svc.Snapshot(locked.TaskID)
				if err != nil || !ok {
					t.Fatalf("snapshot: ok=%v err=%v", ok, err)
				}
				if got := len(snap.Events); got != 1 {
					t.Fatalf("sieving evidence count=%d, want 1", got)
				}
				amount := snap.Events[0].ValueFixed
				if amount != 100 && amount != 250 {
					t.Fatalf("committed sieving amount=%d, want 100 or 250", amount)
				}
				if got, want := snap.Balances[domain.ComponentRawEarth], int64(100000)-amount; got != want {
					t.Fatalf("raw earth balance=%d, want %d", got, want)
				}
			},
		},
		{
			name: "serial replay and distinct operation ids retain normal behavior",
			run: func(t *testing.T) {
				svc := service.New(store.NewMemoryStore())
				locked, err := svc.Lock(modelTaskSpec("serial"))
				if err != nil {
					t.Fatalf("lock task: %v", err)
				}
				firstReq := service.OperationRequest{OperationID: "sieve-one", Digest: locked.Digest, Generation: locked.Generation, At: 10, Kind: domain.ProcessSieving, AmountG: 100}
				first, err := svc.ApplyOperation(locked.TaskID, firstReq)
				if err != nil {
					t.Fatalf("first operation: %v", err)
				}
				replay, err := svc.ApplyOperation(locked.TaskID, firstReq)
				if err != nil {
					t.Fatalf("serial replay: %v", err)
				}
				if !reflect.DeepEqual(first, replay) {
					t.Fatalf("serial replay differs: %#v and %#v", first, replay)
				}
				secondReq := firstReq
				secondReq.OperationID = "sieve-two"
				secondReq.At = 20
				secondReq.AmountG = 200
				if _, err := svc.ApplyOperation(locked.TaskID, secondReq); err != nil {
					t.Fatalf("distinct operation: %v", err)
				}
				snap, ok, err := svc.Snapshot(locked.TaskID)
				if err != nil || !ok {
					t.Fatalf("snapshot: ok=%v err=%v", ok, err)
				}
				if got := len(snap.Events); got != 2 {
					t.Fatalf("sieving evidence count=%d, want 2", got)
				}
				if got := snap.Balances[domain.ComponentRawEarth]; got != 99700 {
					t.Fatalf("raw earth balance=%d, want 99700", got)
				}
			},
		},
		{
			name: "the same operation id is independent across task scopes",
			run: func(t *testing.T) {
				svc := service.New(store.NewMemoryStore())
				for _, area := range []string{"scope-a", "scope-b"} {
					locked, err := svc.Lock(modelTaskSpec(area))
					if err != nil {
						t.Fatalf("lock %s: %v", area, err)
					}
					req := service.OperationRequest{OperationID: "shared-operation", Digest: locked.Digest, Generation: locked.Generation, At: 10, Kind: domain.ProcessSieving, AmountG: 100}
					if _, err := svc.ApplyOperation(locked.TaskID, req); err != nil {
						t.Fatalf("apply in %s: %v", area, err)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}
