package material

import (
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

func TestLeaseExclusive(t *testing.T) {
	m := NewLeaseManager()
	l1 := domain.ResourceLease{Kind: domain.ResourceMixer, ResourceID: "m1", HolderOp: "op1", Token: "tok1", AcquiredAt: 0, ExpiresAt: 100}
	if err := m.Acquire(l1, "op1"); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	l2 := domain.ResourceLease{Kind: domain.ResourceMixer, ResourceID: "m1", HolderOp: "op2", Token: "tok2", AcquiredAt: 50, ExpiresAt: 150}
	if err := m.Acquire(l2, "op2"); err == nil || !rules.Is(err, rules.CodeLeaseBusy) {
		t.Fatalf("want LEASE_BUSY, got %v", err)
	}
}

func TestLeaseExpiryAllowsReacquire(t *testing.T) {
	m := NewLeaseManager()
	l1 := domain.ResourceLease{Kind: domain.ResourceMixer, ResourceID: "m1", HolderOp: "op1", Token: "tok1", AcquiredAt: 0, ExpiresAt: 100}
	if err := m.Acquire(l1, "op1"); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	// Expire the lease, then reacquire at a later time.
	m.ReclaimExpired(101)
	l2 := domain.ResourceLease{Kind: domain.ResourceMixer, ResourceID: "m1", HolderOp: "op2", Token: "tok2", AcquiredAt: 101, ExpiresAt: 200}
	if err := m.Acquire(l2, "op2"); err != nil {
		t.Fatalf("reacquire after expiry: %v", err)
	}
}

func TestLeaseRenewTokenMismatch(t *testing.T) {
	m := NewLeaseManager()
	l1 := domain.ResourceLease{Kind: domain.ResourceRammer, ResourceID: "r1", HolderOp: "op1", Token: "tok1", AcquiredAt: 0, ExpiresAt: 100}
	if err := m.Acquire(l1, "op1"); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := m.Renew(domain.ResourceRammer, "r1", "wrong", 200, "op1"); err == nil || !rules.Is(err, rules.CodeLeaseBusy) {
		t.Fatalf("want LEASE_BUSY, got %v", err)
	}
	if err := m.Renew(domain.ResourceRammer, "r1", "tok1", 200, "op1"); err != nil {
		t.Fatalf("renew: %v", err)
	}
}
