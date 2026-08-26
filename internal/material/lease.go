package material

import (
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

// LeaseManager tracks exclusive, time-limited resource leases keyed by
// resource kind and ID (data model item 5). At most one active lease may exist
// per (kind, id).
type LeaseManager struct {
	leases map[string]*domain.ResourceLease
}

// NewLeaseManager creates an empty lease manager.
func NewLeaseManager() *LeaseManager {
	return &LeaseManager{leases: make(map[string]*domain.ResourceLease)}
}

// NewLeaseManagerFrom reconstructs a lease manager from persisted leases. The
// map is copied so later mutations do not alias the snapshot.
func NewLeaseManagerFrom(leases map[string]*domain.ResourceLease) *LeaseManager {
	m := NewLeaseManager()
	for k, v := range leases {
		cp := *v
		m.leases[k] = &cp
	}
	return m
}

// Leases returns a copy of the current leases for persistence.
func (m *LeaseManager) Leases() map[string]*domain.ResourceLease {
	out := make(map[string]*domain.ResourceLease, len(m.leases))
	for k, v := range m.leases {
		cp := *v
		out[k] = &cp
	}
	return out
}

func leaseKey(kind domain.ResourceKind, id string) string {
	return string(kind) + ":" + id
}

// Acquire atomically acquires an exclusive lease. If a non-expired lease is
// already held by another operation the request fails with LEASE_BUSY
// (failure boundary 3). Acquisition and material deduction share a transaction
// at the store layer.
func (m *LeaseManager) Acquire(l domain.ResourceLease, opID domain.OperationID) error {
	k := leaseKey(l.Kind, l.ResourceID)
	if held, ok := m.leases[k]; ok && held.Status == domain.LeaseActive && held.ExpiresAt >= l.AcquiredAt {
		return rules.New(rules.CodeLeaseBusy, string(opID), "resource already leased")
	}
	cp := l
	cp.Status = domain.LeaseActive
	m.leases[k] = &cp
	return nil
}

// Renew extends a lease if the token matches and the lease is still active.
func (m *LeaseManager) Renew(kind domain.ResourceKind, id string, token domain.LeaseToken, until domain.LogicalTime, opID domain.OperationID) error {
	k := leaseKey(kind, id)
	held, ok := m.leases[k]
	if !ok || held.Status != domain.LeaseActive {
		return rules.New(rules.CodeLeaseExpired, string(opID), "no active lease to renew")
	}
	if held.Token != token {
		return rules.New(rules.CodeLeaseBusy, string(opID), "lease token mismatch")
	}
	held.ExpiresAt = until
	return nil
}

// Release frees a lease when the token matches.
func (m *LeaseManager) Release(kind domain.ResourceKind, id string, token domain.LeaseToken, opID domain.OperationID) error {
	k := leaseKey(kind, id)
	held, ok := m.leases[k]
	if !ok || held.Token != token {
		return rules.New(rules.CodeLeaseBusy, string(opID), "lease token mismatch")
	}
	held.Status = domain.LeaseReleased
	return nil
}

// ReclaimExpired marks every lease whose expiry has passed as expired, driven
// by persisted logical time at startup (failure boundary 8).
func (m *LeaseManager) ReclaimExpired(now domain.LogicalTime) {
	for _, l := range m.leases {
		if l.Status == domain.LeaseActive && l.ExpiresAt < now {
			l.Status = domain.LeaseExpired
			l.ReclaimVer++
		}
	}
}
