// Package service orchestrates the rammed-earth roof-beam clearance workflow.
// It owns the transaction boundary: every business flow runs inside a single
// store.Update transaction, reconstructs the behaviour objects from persisted
// state, validates digest/generation/logical-time, mutates the aggregate and
// writes the result back atomically. No partial material deduction, lease,
// lattice append, evidence or verdict can survive a failed flow.
package service

import (
	"encoding/json"

	"rammed-earth-roof-beam-clearance/internal/aggregate"
	"rammed-earth-roof-beam-clearance/internal/arbiter"
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/evidence"
	"rammed-earth-roof-beam-clearance/internal/material"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// Service is the application service coordinating all business flows over the
// store persistence boundary.
type Service struct {
	store store.Store
}

// New constructs a service over the given store.
func New(st store.Store) *Service { return &Service{store: st} }

// Store exposes the underlying store for readiness/health reporting.
func (s *Service) Store() store.Store { return s.store }

// taskScope returns the idempotency scope for a task's operations.
func taskScope(id domain.TaskID) string { return "tasks/" + string(id) }

// behaviours reconstructs the in-memory behaviour objects from persisted task
// state. It is the hydrate half of the persistence round-trip.
type behaviours struct {
	task        *aggregate.Task
	ledger      *material.Ledger
	leases      *material.LeaseManager
	recorder    *evidence.Recorder
	instruments *evidence.InstrumentLog
	reviews     *arbiter.Reviews
	verdict     *arbiter.VerdictBarrier
}

func hydrate(st *store.TaskState) *behaviours {
	return &behaviours{
		task:        aggregate.NewTaskFrom(st.Header, st.Closed),
		ledger:      material.NewLedgerFrom(st.Balances),
		leases:      material.NewLeaseManagerFrom(st.Leases),
		recorder:    evidence.NewRecorderFrom(st.Events, st.PanSeq, st.PanExpires),
		instruments: evidence.NewInstrumentLogFrom(st.Calls),
		reviews:     arbiter.NewReviewsFrom(st.Reviews),
		verdict:     arbiter.NewVerdictBarrierFrom(st.Verdict),
	}
}

// dehydrate writes the behaviour objects back into persisted task state. It is
// the snapshot half of the round-trip.
func dehydrate(st *store.TaskState, b *behaviours) {
	st.Header = b.task.Header()
	st.Closed = b.task.Closed()
	st.Balances = b.ledger.Balances()
	st.Leases = b.leases.Leases()
	st.Events, st.PanSeq, st.PanExpires = b.recorder.Snapshot()
	st.Calls = b.instruments.Calls()
	st.Reviews = b.reviews.Reviews()
	if v := b.verdict.Current(); v != nil {
		cp := *v
		st.Verdict = &cp
	}
}

// digestOf hashes a canonical request encoding into a digest for idempotency.
func digestOf(v any) domain.Digest {
	data, err := json.Marshal(v)
	if err != nil {
		return domain.Digest("")
	}
	return domain.Digest(rules.Hash(data))
}

// idemBegin checks idempotency before applying an operation. It returns the
// stored response (as raw JSON) when this is a replay, or reports a conflict
// when the same operation id is reused with different content (domain rule 8).
func (s *Service) idemBegin(scope string, op domain.OperationID, reqDigest domain.Digest) (replay []byte, hit bool, err error) {
	rec, ok := s.store.Idempotency(scope, op)
	if !ok {
		return nil, false, nil
	}
	if rec.RequestDigest != reqDigest {
		return nil, true, rules.New(rules.CodeIdempotencyConflict, string(op), "operation id reused with different content")
	}
	return []byte(rec.ResponseDigest), true, nil
}

// idemCommit persists the canonical result of a successful operation.
func (s *Service) idemCommit(scope string, op domain.OperationID, reqDigest domain.Digest, result any, status int) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.store.PutIdempotency(domain.IdempotencyRecord{
		Scope:          scope,
		OperationID:    op,
		RequestDigest:  reqDigest,
		ResultDigest:   digestOf(result),
		HTTPStatus:     status,
		ResponseDigest: domain.Digest(data),
	})
}
