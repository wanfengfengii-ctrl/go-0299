package service

import (
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func TestModel_LeaseOperationsPersistAtomicState(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, s *Service, locked *LockResult)
	}{
		{
			name: "acquire persists exclusivity",
			run: func(t *testing.T, s *Service, locked *LockResult) {
				first, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-mixer-1", Digest: locked.Digest, Generation: 1, At: 10,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 30,
				})
				if err != nil {
					t.Fatalf("first acquire: %v", err)
				}
				if first.Status != domain.LeaseActive || first.Token == "" {
					t.Fatalf("first acquire = %+v, want active lease with token", first)
				}
				_, err = s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-mixer-2", Digest: locked.Digest, Generation: 1, At: 20,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 40,
				})
				if err == nil || !rules.Is(err, rules.CodeLeaseBusy) {
					t.Fatalf("contending acquire error = %v, want LEASE_BUSY", err)
				}
			},
		},
		{
			name: "renew persists new expiration",
			run: func(t *testing.T, s *Service, locked *LockResult) {
				lease, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-for-renew", Digest: locked.Digest, Generation: 1, At: 10,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 20,
				})
				if err != nil {
					t.Fatalf("acquire: %v", err)
				}
				renewed, err := s.RenewLease(locked.TaskID, LeaseRequest{
					OperationID: "renew-mixer", Digest: locked.Digest, Generation: 1, At: 15,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", Token: lease.Token, ExpiresAt: 40,
				})
				if err != nil {
					t.Fatalf("renew: %v", err)
				}
				if renewed.Status != domain.LeaseActive || renewed.Token != lease.Token {
					t.Fatalf("renew result = %+v, want active with original token", renewed)
				}
				_, err = s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-during-extension", Digest: locked.Digest, Generation: 1, At: 30,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 50,
				})
				if err == nil || !rules.Is(err, rules.CodeLeaseBusy) {
					t.Fatalf("acquire during renewed interval error = %v, want LEASE_BUSY", err)
				}
			},
		},
		{
			name: "release persists released status",
			run: func(t *testing.T, s *Service, locked *LockResult) {
				lease, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-for-release", Digest: locked.Digest, Generation: 1, At: 10,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 40,
				})
				if err != nil {
					t.Fatalf("acquire: %v", err)
				}
				released, err := s.ReleaseLease(locked.TaskID, LeaseRequest{
					OperationID: "release-mixer", Digest: locked.Digest, Generation: 1, At: 15,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", Token: lease.Token,
				})
				if err != nil {
					t.Fatalf("release: %v", err)
				}
				if released.Status != domain.LeaseReleased {
					t.Fatalf("release status = %q, want %q", released.Status, domain.LeaseReleased)
				}
				if _, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-after-release", Digest: locked.Digest, Generation: 1, At: 16,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 50,
				}); err != nil {
					t.Fatalf("acquire after release: %v", err)
				}
			},
		},
		{
			name: "failed operation is atomic and not idempotently committed",
			run: func(t *testing.T, s *Service, locked *LockResult) {
				lease, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-before-failure", Digest: locked.Digest, Generation: 1, At: 10,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 40,
				})
				if err != nil {
					t.Fatalf("acquire: %v", err)
				}
				failed := LeaseRequest{
					OperationID: "retryable-renew", Digest: locked.Digest, Generation: 1, At: 20,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", Token: "wrong-token", ExpiresAt: 50,
				}
				if _, err := s.RenewLease(locked.TaskID, failed); err == nil || !rules.Is(err, rules.CodeLeaseBusy) {
					t.Fatalf("wrong-token renew error = %v, want LEASE_BUSY", err)
				}
				if _, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-before-failed-time", Digest: locked.Digest, Generation: 1, At: 15,
					Kind: domain.ResourceMixer, ResourceID: "mixer-2", ExpiresAt: 30,
				}); err != nil {
					t.Fatalf("failed renew advanced task clock: %v", err)
				}
				failed.Token = lease.Token
				failed.At = 25
				if _, err := s.RenewLease(locked.TaskID, failed); err != nil {
					t.Fatalf("corrected retry after failed operation: %v", err)
				}
			},
		},
		{
			name: "different resource remains available",
			run: func(t *testing.T, s *Service, locked *LockResult) {
				for i, req := range []LeaseRequest{
					{OperationID: "acquire-distinct-1", Digest: locked.Digest, Generation: 1, At: 10, Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 30},
					{OperationID: "acquire-distinct-2", Digest: locked.Digest, Generation: 1, At: 11, Kind: domain.ResourceMixer, ResourceID: "mixer-2", ExpiresAt: 30},
				} {
					if _, err := s.AcquireLease(locked.TaskID, req); err != nil {
						t.Fatalf("acquire distinct resource %d: %v", i+1, err)
					}
				}
			},
		},
		{
			name: "expired lease can be reacquired",
			run: func(t *testing.T, s *Service, locked *LockResult) {
				if _, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-expiring", Digest: locked.Digest, Generation: 1, At: 10,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 20,
				}); err != nil {
					t.Fatalf("initial acquire: %v", err)
				}
				if _, err := s.AcquireLease(locked.TaskID, LeaseRequest{
					OperationID: "acquire-after-expiry", Digest: locked.Digest, Generation: 1, At: 21,
					Kind: domain.ResourceMixer, ResourceID: "mixer-1", ExpiresAt: 40,
				}); err != nil {
					t.Fatalf("acquire after expiry: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(store.NewMemoryStore())
			locked := lockTask(t, s)
			tt.run(t, s, locked)
		})
	}
}
