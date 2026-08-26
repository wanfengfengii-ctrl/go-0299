package service

import (
	"reflect"
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/material"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func TestModel_MixingTransactionAtomicity(t *testing.T) {
	type observedState struct {
		balances map[domain.ComponentKind]int64
		pans     []domain.MixPan
		panSeq   int
		events   []domain.EvidenceEvent
		leases   map[string]domain.ResourceLease
	}

	observe := func(t *testing.T, s *Service, id domain.TaskID) observedState {
		t.Helper()
		var got observedState
		ok, err := s.Store().View(id, func(st *store.TaskState) {
			got.balances = make(map[domain.ComponentKind]int64, len(st.Balances))
			for kind, amount := range st.Balances {
				got.balances[kind] = amount
			}
			got.pans = append([]domain.MixPan(nil), st.Pans...)
			got.panSeq = st.PanSeq
			got.events = append([]domain.EvidenceEvent(nil), st.Events...)
			got.leases = make(map[string]domain.ResourceLease, len(st.Leases))
			for key, lease := range st.Leases {
				got.leases[key] = *lease
			}
		})
		if err != nil || !ok {
			t.Fatalf("observe task: ok=%v err=%v", ok, err)
		}
		return got
	}

	recipe := domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20}
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "mixer conflict leaves the complete transaction state unchanged",
			run: func(t *testing.T) {
				s := New(store.NewMemoryStore())
				locked, err := s.Lock(validSpec())
				if err != nil {
					t.Fatalf("lock: %v", err)
				}
				if _, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "hold-mixer", Digest: locked.Digest, Generation: locked.Generation,
					At: 5, Kind: domain.ResourceMixer, ResourceID: "mixer", ExpiresAt: 50,
				}); err != nil {
					t.Fatalf("acquire conflicting mixer lease: %v", err)
				}

				before := observe(t, s, locked.TaskID)
				_, err = s.ApplyOperation(locked.TaskID, OperationRequest{
					OperationID: "mix-while-mixer-busy", Digest: locked.Digest, Generation: locked.Generation,
					At: 10, Kind: domain.ProcessMixing, Recipe: &recipe, Actor: "operator",
				})
				if err == nil || !rules.Is(err, rules.CodeLeaseBusy) {
					t.Fatalf("mix error=%v, want LEASE_BUSY", err)
				}
				after := observe(t, s, locked.TaskID)
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("failed mix changed task state\nbefore: %#v\nafter:  %#v", before, after)
				}
			},
		},
		{
			name: "moisture meter conflict leaves the complete transaction state unchanged",
			run: func(t *testing.T) {
				s := New(store.NewMemoryStore())
				locked, err := s.Lock(validSpec())
				if err != nil {
					t.Fatalf("lock: %v", err)
				}
				if _, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "hold-meter", Digest: locked.Digest, Generation: locked.Generation,
					At: 5, Kind: domain.ResourceMoistureMeter, ResourceID: "moisture_meter", ExpiresAt: 50,
				}); err != nil {
					t.Fatalf("acquire conflicting moisture-meter lease: %v", err)
				}

				before := observe(t, s, locked.TaskID)
				_, err = s.ApplyOperation(locked.TaskID, OperationRequest{
					OperationID: "mix-while-meter-busy", Digest: locked.Digest, Generation: locked.Generation,
					At: 10, Kind: domain.ProcessMixing, Recipe: &recipe, Actor: "operator",
				})
				if err == nil || !rules.Is(err, rules.CodeLeaseBusy) {
					t.Fatalf("mix error=%v, want LEASE_BUSY", err)
				}
				after := observe(t, s, locked.TaskID)
				if !reflect.DeepEqual(after, before) {
					t.Fatalf("failed mix changed task state\nbefore: %#v\nafter:  %#v", before, after)
				}
			},
		},
		{
			name: "available resources commit material pan evidence and leases together",
			run: func(t *testing.T) {
				s := New(store.NewMemoryStore())
				locked, err := s.Lock(validSpec())
				if err != nil {
					t.Fatalf("lock: %v", err)
				}
				result, err := s.ApplyOperation(locked.TaskID, OperationRequest{
					OperationID: "mix-with-free-resources", Digest: locked.Digest, Generation: locked.Generation,
					At: 10, Kind: domain.ProcessMixing, Recipe: &recipe, Actor: "operator",
				})
				if err != nil {
					t.Fatalf("mix: %v", err)
				}
				if !result.Applied || result.PanSeq != 1 {
					t.Fatalf("result=%+v, want applied pan 1", result)
				}

				got := observe(t, s, locked.TaskID)
				wantBalances := map[domain.ComponentKind]int64{
					domain.ComponentRawEarth: 99100, domain.ComponentGravel: 99950,
					domain.ComponentStabilizer: 99970, domain.ComponentWater: 99980,
					domain.ComponentMix: 1000,
				}
				if !reflect.DeepEqual(got.balances, wantBalances) {
					t.Fatalf("balances=%v, want %v", got.balances, wantBalances)
				}
				if got.panSeq != 1 || len(got.pans) != 1 || got.pans[0].PanSeq != 1 || got.pans[0].Recipe != recipe {
					t.Fatalf("pan state: seq=%d pans=%+v", got.panSeq, got.pans)
				}
				if len(got.events) != 1 || got.events[0].Process != domain.ProcessMixing || got.events[0].PanSeq != 1 {
					t.Fatalf("mix evidence=%+v", got.events)
				}
				if len(got.leases) != 2 {
					t.Fatalf("leases=%+v, want mixer and moisture meter", got.leases)
				}
				kinds := map[domain.ResourceKind]bool{}
				for _, lease := range got.leases {
					if lease.Status != domain.LeaseActive || lease.HolderOp != "mix-with-free-resources" {
						t.Fatalf("unexpected committed lease: %+v", lease)
					}
					kinds[lease.Kind] = true
				}
				if !kinds[domain.ResourceMixer] || !kinds[domain.ResourceMoistureMeter] {
					t.Fatalf("lease kinds=%v, want mixer and moisture meter", kinds)
				}
			},
		},
		{
			name: "restored ledger does not alias its persisted input",
			run: func(t *testing.T) {
				persisted := map[domain.ComponentKind]int64{domain.ComponentRawEarth: 1000}
				ledger := material.NewLedgerFrom(persisted)
				persisted[domain.ComponentRawEarth] = 2000
				if got := ledger.Balance(domain.ComponentRawEarth); got != 1000 {
					t.Fatalf("caller mutation changed restored ledger balance to %d, want 1000", got)
				}
				if err := ledger.PostTransaction([]domain.MassLedgerEntry{
					{Account: domain.ComponentMix, Side: domain.SideDebit, AmountG: 100},
					{Account: domain.ComponentRawEarth, Side: domain.SideCredit, AmountG: 100},
				}, "restored-ledger-post"); err != nil {
					t.Fatalf("post restored transaction: %v", err)
				}
				wantPersisted := map[domain.ComponentKind]int64{domain.ComponentRawEarth: 2000}
				if !reflect.DeepEqual(persisted, wantPersisted) {
					t.Fatalf("ledger mutation leaked into persisted input: got %v want %v", persisted, wantPersisted)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}
