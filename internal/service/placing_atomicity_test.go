package service

import (
	"reflect"
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func TestModel_PlacingRejectionsAreAtomic(t *testing.T) {
	tests := []struct {
		name     string
		rejectAt domain.LogicalTime
		amountG  int64
		wantCode rules.Code
		validAt  domain.LogicalTime
	}{
		{name: "expired mix evidence", rejectAt: 111, amountG: 500, wantCode: rules.CodeMixExpired, validAt: 20},
		{name: "material overclaim", rejectAt: 20, amountG: 1001, wantCode: rules.CodeMaterialOverclaim, validAt: 21},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := New(store.NewMemoryStore())
			locked, err := svc.Lock(domain.TaskSpec{
				Area:      "ecology-study-building",
				Direction: domain.DirectionRising,
				Geometry: domain.WallGeometry{
					Wall: domain.Rect{W: 1000, H: 2000},
					Layers: []domain.Layer{
						{Number: 1, Rect: domain.Rect{W: 1000, H: 1000}},
						{Number: 2, Rect: domain.Rect{Y: 1000, W: 1000, H: 1000}},
					},
					Cells: []domain.Cell{
						{Layer: 1, Seq: 0, Rect: domain.Rect{W: 1000, H: 1000}},
						{Layer: 2, Seq: 0, Rect: domain.Rect{Y: 1000, W: 1000, H: 1000}},
					},
					PressureAdj: [][2]domain.CellRef{{{Layer: 1, Seq: 0}, {Layer: 2, Seq: 0}}},
				},
				Batches: []domain.MaterialBatch{
					{ID: "earth", Component: domain.ComponentRawEarth, BalanceG: 900},
					{ID: "gravel", Component: domain.ComponentGravel, BalanceG: 50},
					{ID: "stabilizer", Component: domain.ComponentStabilizer, BalanceG: 30},
					{ID: "water", Component: domain.ComponentWater, BalanceG: 20},
				},
				Recipe:         domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20},
				TargetMoisture: 120,
				Program:        domain.CompactionProgram{LooseThickness: 100, PassesPerCell: 2, BlowsPerPass: 10, RammerWeightG: 10000, FallHeightMM: 500},
				Thresholds:     domain.Thresholds{MinDryDensity: 1800000, MaxDryDensity: 2000000, MinCompaction: 950, MinMoisture: 80, MaxMoisture: 150, MinShear: 1000, MaxErosion: 50, MaxDeviation: 5},
				Curing:         domain.CuringSchedule{HoursPerLayer: 24, MinHours: 72},
				MixPlan:        domain.MixPlan{PanCount: 1, PanSizeG: 1000, UsableUnits: 100},
			})
			if err != nil {
				t.Fatalf("lock task: %v", err)
			}

			mix := OperationRequest{
				OperationID: "mix-1", Digest: locked.Digest, Generation: 1, At: 10,
				Kind:   domain.ProcessMixing,
				Recipe: &domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20},
			}
			if _, err := svc.ApplyOperation(locked.TaskID, mix); err != nil {
				t.Fatalf("mix: %v", err)
			}
			before, ok, err := svc.Snapshot(locked.TaskID)
			if err != nil || !ok {
				t.Fatalf("snapshot before rejected placing: ok=%v err=%v", ok, err)
			}

			rejected := OperationRequest{
				OperationID: "place-rejected", Digest: locked.Digest, Generation: 1, At: tt.rejectAt,
				Kind: domain.ProcessPlacing, Layer: 1, Seq: 0, PanSeq: 1, AmountG: tt.amountG,
			}
			if _, err := svc.ApplyOperation(locked.TaskID, rejected); err == nil || !rules.Is(err, tt.wantCode) {
				t.Fatalf("rejected placing error = %v, want %s", err, tt.wantCode)
			}
			after, ok, err := svc.Snapshot(locked.TaskID)
			if err != nil || !ok {
				t.Fatalf("snapshot after rejected placing: ok=%v err=%v", ok, err)
			}
			if !reflect.DeepEqual(after.ClosedCells, before.ClosedCells) {
				t.Fatalf("rejected placing changed closed cells: before=%v after=%v", before.ClosedCells, after.ClosedCells)
			}
			if !reflect.DeepEqual(after.Balances, before.Balances) {
				t.Fatalf("rejected placing changed material balances: before=%v after=%v", before.Balances, after.Balances)
			}
			if !reflect.DeepEqual(after.Events, before.Events) {
				t.Fatalf("rejected placing changed evidence log or in-wall mapping: before=%v after=%v", before.Events, after.Events)
			}

			valid := OperationRequest{
				OperationID: "place-valid", Digest: locked.Digest, Generation: 1, At: tt.validAt,
				Kind: domain.ProcessPlacing, Layer: 1, Seq: 0, PanSeq: 1, AmountG: 500,
			}
			first, err := svc.ApplyOperation(locked.TaskID, valid)
			if err != nil {
				t.Fatalf("valid placing after rejection: %v", err)
			}
			placed, _, _ := svc.Snapshot(locked.TaskID)
			if placed.ClosedCells[1] != 1 || placed.Balances[domain.ComponentMix] != 500 || placed.Balances[domain.ComponentInWall] != 500 {
				t.Fatalf("valid placing state: closed=%v balances=%v", placed.ClosedCells, placed.Balances)
			}
			if len(placed.Events) != len(before.Events)+1 || placed.Events[len(placed.Events)-1].Process != domain.ProcessPlacing || placed.Events[len(placed.Events)-1].Layer != 1 || placed.Events[len(placed.Events)-1].Seq != 0 || placed.Events[len(placed.Events)-1].PanSeq != 1 {
				t.Fatalf("valid placing evidence not recorded exactly once: %v", placed.Events)
			}

			replay, err := svc.ApplyOperation(locked.TaskID, valid)
			if err != nil {
				t.Fatalf("idempotent replay: %v", err)
			}
			replayed, _, _ := svc.Snapshot(locked.TaskID)
			if !reflect.DeepEqual(replay, first) || !reflect.DeepEqual(replayed, placed) {
				t.Fatalf("idempotent replay changed result or state: first=%v replay=%v", first, replay)
			}
		})
	}
}
