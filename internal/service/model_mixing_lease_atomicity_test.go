package service

import (
	"reflect"
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func TestModel_MixingLeaseAtomicity(t *testing.T) {
	tests := []struct {
		name    string
		blocker domain.ResourceKind
		wantErr bool
	}{
		{name: "mixer_busy_rolls_back_entire_mix", blocker: domain.ResourceMixer, wantErr: true},
		{name: "moisture_meter_busy_rolls_back_entire_mix", blocker: domain.ResourceMoistureMeter, wantErr: true},
		{name: "both_leases_available_commits_one_conserved_pan"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(store.NewMemoryStore())
			locked := lockTask(t, s)

			if tt.blocker != "" {
				_, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "occupy-" + domain.OperationID(tt.blocker),
					Digest:      locked.Digest,
					Generation:  1,
					At:          10,
					Kind:        tt.blocker,
					ResourceID:  string(tt.blocker),
					ExpiresAt:   100,
				})
				if err != nil {
					t.Fatalf("occupy %s: %v", tt.blocker, err)
				}
			}

			before, ok, err := s.Snapshot(locked.TaskID)
			if err != nil || !ok {
				t.Fatalf("snapshot before mix: ok=%v err=%v", ok, err)
			}

			result, err := s.ApplyOperation(locked.TaskID, OperationRequest{
				OperationID: "mix-under-test",
				Digest:      locked.Digest,
				Generation:  1,
				At:          20,
				Kind:        domain.ProcessMixing,
				Recipe:      &domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20},
			})

			after, ok, snapErr := s.Snapshot(locked.TaskID)
			if snapErr != nil || !ok {
				t.Fatalf("snapshot after mix: ok=%v err=%v", ok, snapErr)
			}

			if tt.wantErr {
				if !rules.Is(err, rules.CodeLeaseBusy) {
					t.Fatalf("mix error=%v, want stable %s", err, rules.CodeLeaseBusy)
				}
				if result != nil {
					t.Fatalf("rejected mix returned result: %+v", result)
				}
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("lease conflict changed snapshot\nbefore=%+v\nafter=%+v", before, after)
				}
				return
			}

			if err != nil {
				t.Fatalf("mix with available leases: %v", err)
			}
			if result == nil || !result.Applied || result.PanSeq != 1 {
				t.Fatalf("mix result=%+v, want one newly applied pan", result)
			}
			if len(after.Pans) != 1 || after.Pans[0].ID != "pan-1" || after.Pans[0].PanSeq != 1 {
				t.Fatalf("pans=%+v, want one distinct non-mergeable pan", after.Pans)
			}
			if len(after.Events) != len(before.Events)+1 || after.Events[len(after.Events)-1].Process != domain.ProcessMixing {
				t.Fatalf("events=%+v, want exactly one mixing evidence event", after.Events)
			}
			wantBalances := map[domain.ComponentKind]int64{
				domain.ComponentRawEarth:   99100,
				domain.ComponentGravel:     99950,
				domain.ComponentStabilizer: 99970,
				domain.ComponentWater:      99980,
				domain.ComponentMix:        1000,
			}
			for component, want := range wantBalances {
				if got := after.Balances[component]; got != want {
					t.Errorf("balance[%s]=%d, want %d", component, got, want)
				}
			}
			if got := after.Balances[domain.ComponentRawEarth] + after.Balances[domain.ComponentGravel] + after.Balances[domain.ComponentStabilizer] + after.Balances[domain.ComponentWater] + after.Balances[domain.ComponentMix]; got != 400000 {
				t.Errorf("total conserved material=%d, want 400000", got)
			}

			for _, kind := range []domain.ResourceKind{domain.ResourceMixer, domain.ResourceMoistureMeter} {
				_, leaseErr := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "probe-" + domain.OperationID(kind),
					Digest:      locked.Digest,
					Generation:  1,
					At:          21,
					Kind:        kind,
					ResourceID:  string(kind),
					ExpiresAt:   50,
				})
				if !rules.Is(leaseErr, rules.CodeLeaseBusy) {
					t.Errorf("%s lease after mix error=%v, want %s", kind, leaseErr, rules.CodeLeaseBusy)
				}
			}
		})
	}
}
