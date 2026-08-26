package service

import (
	"encoding/json"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// LeaseRequest is the payload for the lease acquire/renew/release endpoints
// (interface 3). Material deduction and lease acquisition share a transaction
// when submitted via the operations endpoint (mixing); standalone leases use
// these methods.
type LeaseRequest struct {
	OperationID domain.OperationID  `json:"operation_id"`
	Digest      domain.Digest       `json:"digest"`
	Generation  domain.Generation   `json:"generation"`
	At          domain.LogicalTime  `json:"at"`
	Kind        domain.ResourceKind `json:"kind"`
	ResourceID  string              `json:"resource_id"`
	Token       domain.LeaseToken   `json:"token,omitempty"`
	ExpiresAt   domain.LogicalTime  `json:"expires_at,omitempty"`
}

// LeaseResult is the deterministic result of a lease operation.
type LeaseResult struct {
	Kind       domain.ResourceKind `json:"kind"`
	ResourceID string              `json:"resource_id"`
	Token      domain.LeaseToken   `json:"token,omitempty"`
	Status     domain.LeaseStatus  `json:"status"`
}

// AcquireLease atomically acquires an exclusive lease (failure boundary 3).
func (s *Service) AcquireLease(id domain.TaskID, req LeaseRequest) (*LeaseResult, error) {
	return s.leaseOp(id, req, domain.LeaseActive, false)
}

// RenewLease extends an existing lease, requiring a matching token.
func (s *Service) RenewLease(id domain.TaskID, req LeaseRequest) (*LeaseResult, error) {
	return s.leaseOp(id, req, domain.LeaseActive, true)
}

// ReleaseLease frees an existing lease, requiring a matching token.
func (s *Service) ReleaseLease(id domain.TaskID, req LeaseRequest) (*LeaseResult, error) {
	return s.leaseOp(id, req, domain.LeaseReleased, true)
}

func (s *Service) leaseOp(id domain.TaskID, req LeaseRequest, target domain.LeaseStatus, requireToken bool) (*LeaseResult, error) {
	scope := taskScope(id)
	reqDigest := digestOf(req)

	replay, hit, err := s.idemBegin(scope, req.OperationID, reqDigest)
	if err != nil {
		return nil, err
	}
	if hit {
		var out LeaseResult
		if err := json.Unmarshal(replay, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	result := &LeaseResult{Kind: req.Kind, ResourceID: req.ResourceID}
	err = s.store.Update(id, func(st *store.TaskState) error {
		b := hydrate(st)
		if err := b.task.CheckDigest(req.Digest, req.OperationID); err != nil {
			return err
		}
		if err := b.task.CheckGeneration(req.Generation, req.OperationID); err != nil {
			return err
		}
		if err := b.task.AdvanceClock(req.At, req.OperationID); err != nil {
			return err
		}

		switch target {
		case domain.LeaseActive:
			if requireToken {
				if err := b.leases.Renew(req.Kind, req.ResourceID, req.Token, req.ExpiresAt, req.OperationID); err != nil {
					return err
				}
				result.Status = domain.LeaseActive
				result.Token = req.Token
			} else {
				if req.ExpiresAt <= req.At {
					return rules.New(rules.CodeLeaseExpired, string(req.OperationID), "lease expiry must follow acquisition time")
				}
				token := domain.LeaseToken(digestOf(req.OperationID))
				if err := b.leases.Acquire(domain.ResourceLease{
					Kind: req.Kind, ResourceID: req.ResourceID, HolderOp: req.OperationID,
					Token: token, AcquiredAt: req.At, ExpiresAt: req.ExpiresAt,
				}, req.OperationID); err != nil {
					return err
				}
				result.Status = domain.LeaseActive
				result.Token = token
			}
		case domain.LeaseReleased:
			if err := b.leases.Release(req.Kind, req.ResourceID, req.Token, req.OperationID); err != nil {
				return err
			}
			result.Status = domain.LeaseReleased
		}

		dehydrate(st, b)
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.idemCommit(scope, req.OperationID, reqDigest, result, 200); err != nil {
		return nil, err
	}
	return result, nil
}
