package material

import (
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

func TestLedgerPostOverclaim(t *testing.T) {
	l := NewLedger()
	// Deposit 100g of raw earth.
	if err := l.Post(domain.MassLedgerEntry{Account: domain.ComponentRawEarth, Side: domain.SideDebit, AmountG: 100}, "op1"); err != nil {
		t.Fatalf("deposit: %v", err)
	}
	// Try to over-claim 200g.
	if err := l.Post(domain.MassLedgerEntry{Account: domain.ComponentRawEarth, Side: domain.SideCredit, AmountG: 200}, "op2"); err == nil {
		t.Fatal("want MATERIAL_OVERCLAIM")
	} else if !rules.Is(err, rules.CodeMaterialOverclaim) {
		t.Fatalf("want MATERIAL_OVERCLAIM, got %v", err)
	}
	if got := l.Balance(domain.ComponentRawEarth); got != 100 {
		t.Fatalf("balance=%d want 100 (no partial state)", got)
	}
}

func TestLedgerBalancedTransaction(t *testing.T) {
	l := NewLedger()
	// Move 100g from raw earth into in-wall: balanced debit/credit.
	entries := []domain.MassLedgerEntry{
		{Account: domain.ComponentInWall, Side: domain.SideDebit, AmountG: 100},
		{Account: domain.ComponentRawEarth, Side: domain.SideCredit, AmountG: 100},
	}
	// Seed raw earth.
	if err := l.Post(domain.MassLedgerEntry{Account: domain.ComponentRawEarth, Side: domain.SideDebit, AmountG: 100}, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := l.PostTransaction(entries, "tx1"); err != nil {
		t.Fatalf("PostTransaction: %v", err)
	}
	if got := l.Balance(domain.ComponentRawEarth); got != 0 {
		t.Fatalf("raw earth=%d want 0", got)
	}
	if got := l.Balance(domain.ComponentInWall); got != 100 {
		t.Fatalf("in wall=%d want 100", got)
	}
}

func TestLedgerUnbalancedTransactionRollsBack(t *testing.T) {
	l := NewLedger()
	if err := l.Post(domain.MassLedgerEntry{Account: domain.ComponentRawEarth, Side: domain.SideDebit, AmountG: 100}, "seed"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Debits 100 but credits only 50 -> unbalanced.
	entries := []domain.MassLedgerEntry{
		{Account: domain.ComponentInWall, Side: domain.SideDebit, AmountG: 100},
		{Account: domain.ComponentRawEarth, Side: domain.SideCredit, AmountG: 50},
	}
	if err := l.PostTransaction(entries, "tx2"); err == nil || !rules.Is(err, rules.CodeInvalidSign) {
		t.Fatalf("want INVALID_SIGN, got %v", err)
	}
	if got := l.Balance(domain.ComponentRawEarth); got != 100 {
		t.Fatalf("raw earth=%d want 100 (rollback)", got)
	}
	if got := l.Balance(domain.ComponentInWall); got != 0 {
		t.Fatalf("in wall=%d want 0 (rollback)", got)
	}
}
