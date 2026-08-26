// Package material implements the material-conservation and resource-lease
// manager: an integer-gram double-entry ledger over the conserved components
// and the exclusive, time-limited leases over shared equipment and areas.
package material

import (
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

// Ledger is an in-memory integer-gram double-entry mass ledger keyed by
// component account. Every posted transaction must balance (total debits equal
// total credits) and must never drive an account negative.
type Ledger struct {
	balances map[domain.ComponentKind]int64
}

// NewLedger creates an empty ledger.
func NewLedger() *Ledger {
	return &Ledger{balances: make(map[domain.ComponentKind]int64)}
}

// NewLedgerFrom reconstructs a ledger from persisted balances.
func NewLedgerFrom(balances map[domain.ComponentKind]int64) *Ledger {
	return &Ledger{balances: balances}
}

// Balances returns a copy of the current balances for persistence.
func (l *Ledger) Balances() map[domain.ComponentKind]int64 {
	out := make(map[domain.ComponentKind]int64, len(l.balances))
	for k, v := range l.balances {
		out[k] = v
	}
	return out
}

// Balance returns the current integer-gram balance of an account.
func (l *Ledger) Balance(acct domain.ComponentKind) int64 {
	return l.balances[acct]
}

// Post applies one side of a balanced transaction. A negative amount on a
// debit side is rejected; overclaim (debiting more than available) yields
// MATERIAL_OVERCLAIM (failure boundary 3).
func (l *Ledger) Post(entry domain.MassLedgerEntry, opID domain.OperationID) error {
	if entry.AmountG < 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "negative mass amount")
	}
	next := l.balances[entry.Account]
	switch entry.Side {
	case domain.SideDebit:
		next += entry.AmountG
	case domain.SideCredit:
		if entry.AmountG > l.balances[entry.Account] {
			return rules.New(rules.CodeMaterialOverclaim, string(opID), "insufficient balance for "+string(entry.Account))
		}
		next -= entry.AmountG
	default:
		return rules.New(rules.CodeInvalidSign, string(opID), "unknown ledger side")
	}
	l.balances[entry.Account] = next
	return nil
}

// PostTransaction atomically applies a balanced transaction. It validates that
// total debits equal total credits before mutating any balance, so a failed
// transaction leaves no partial state (failure boundary 1).
func (l *Ledger) PostTransaction(entries []domain.MassLedgerEntry, opID domain.OperationID) error {
	var debits, credits int64
	for _, e := range entries {
		switch e.Side {
		case domain.SideDebit:
			d, err := rules.CheckedAdd(debits, e.AmountG)
			if err != nil {
				return rules.New(rules.CodeFixedOverflow, string(opID), "debit total overflow")
			}
			debits = d
		case domain.SideCredit:
			c, err := rules.CheckedAdd(credits, e.AmountG)
			if err != nil {
				return rules.New(rules.CodeFixedOverflow, string(opID), "credit total overflow")
			}
			credits = c
		default:
			return rules.New(rules.CodeInvalidSign, string(opID), "unknown ledger side")
		}
	}
	if debits != credits {
		return rules.New(rules.CodeInvalidSign, string(opID), "transaction must balance: debits != credits")
	}

	// Validate all debits are affordable before applying anything.
	for _, e := range entries {
		if e.Side == domain.SideCredit && e.AmountG > l.balances[e.Account] {
			return rules.New(rules.CodeMaterialOverclaim, string(opID), "insufficient balance for "+string(e.Account))
		}
	}
	for _, e := range entries {
		if e.Side == domain.SideDebit {
			l.balances[e.Account] += e.AmountG
		} else {
			l.balances[e.Account] -= e.AmountG
		}
	}
	return nil
}
