package service

import (
	"path/filepath"
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// TestRestartRecovery verifies that state written to the bbolt backend survives
// a close/reopen cycle: the locked task, material balances and the reopened
// aggregate are all restored from disk (failure boundary 8).
func TestRestartRecovery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "task.db")

	st, err := store.OpenBoltStore(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	s := New(st)
	res, err := s.Lock(validSpec())
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if _, err := s.ApplyOperation(res.TaskID, OperationRequest{
		OperationID: "op-mix-1", Digest: res.Digest, Generation: 1, At: 10,
		Kind:   domain.ProcessMixing,
		Recipe: &domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20},
	}); err != nil {
		t.Fatalf("mix: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen and recover.
	st2, err := store.OpenBoltStore(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	if err := st2.Recover(); err != nil {
		t.Fatalf("recover after reopen: %v", err)
	}
	if !st2.Ready() {
		t.Fatal("store not ready after recovery")
	}
	s2 := New(st2)
	snap, ok, err := s2.Snapshot(res.TaskID)
	if err != nil || !ok {
		t.Fatalf("snapshot after reopen: ok=%v err=%v", ok, err)
	}
	if snap.Balances[domain.ComponentMix] != 1000 {
		t.Fatalf("mix balance=%d want 1000 after recovery", snap.Balances[domain.ComponentMix])
	}
	if snap.Clock != 10 {
		t.Fatalf("clock=%d want 10 after recovery", snap.Clock)
	}
}

// TestConcurrentMaterialOverclaim runs multiple goroutines contending for a
// limited mix supply and asserts exactly the expected number succeed with no
// negative balance and no over-claim (failure boundary 3). The store's per-task
// Update lock serialises the atomic read-modify-write; the ledger rejects any
// deduction that would drive a source account negative.
func TestConcurrentMaterialOverclaim(t *testing.T) {
	dir := t.TempDir()
	st, err := store.OpenBoltStore(filepath.Join(dir, "task.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	if err := st.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}
	s := New(st)
	res, err := s.Lock(validSpec())
	if err != nil {
		t.Fatalf("lock: %v", err)
	}

	// Only 100000g of raw earth; each mix consumes 900g -> 111 mixes possible.
	// 200 concurrent mixes must therefore leave a non-negative raw earth balance
	// with exactly 111 successes and 89 MATERIAL_OVERCLAIM failures.
	const workers = 200
	start := make(chan struct{})
	done := make(chan bool, workers)
	recipe := domain.Recipe{RawEarthG: 900, GravelG: 50, StabilizerG: 30, WaterG: 20}
	for i := 0; i < workers; i++ {
		go func(i int) {
			<-start
			err := s.store.Update(res.TaskID, func(st *store.TaskState) error {
				b := hydrate(st)
				total := recipe.RawEarthG + recipe.GravelG + recipe.StabilizerG + recipe.WaterG
				if err := b.ledger.PostTransaction([]domain.MassLedgerEntry{
					{Account: domain.ComponentMix, Side: domain.SideDebit, AmountG: total},
					{Account: domain.ComponentRawEarth, Side: domain.SideCredit, AmountG: recipe.RawEarthG},
					{Account: domain.ComponentGravel, Side: domain.SideCredit, AmountG: recipe.GravelG},
					{Account: domain.ComponentStabilizer, Side: domain.SideCredit, AmountG: recipe.StabilizerG},
					{Account: domain.ComponentWater, Side: domain.SideCredit, AmountG: recipe.WaterG},
				}, domain.OperationID("op")); err != nil {
					return err
				}
				dehydrate(st, b)
				return nil
			})
			done <- (err == nil)
		}(i)
	}
	close(start)
	successes := 0
	for i := 0; i < workers; i++ {
		if <-done {
			successes++
		}
	}
	snap, _, _ := s.Snapshot(res.TaskID)
	if snap.Balances[domain.ComponentRawEarth] < 0 {
		t.Fatalf("raw earth went negative: %d", snap.Balances[domain.ComponentRawEarth])
	}
	if successes != 111 {
		t.Fatalf("successes=%d want 111", successes)
	}
}
