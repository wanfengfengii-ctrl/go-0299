package service

import (
	"encoding/json"
	"sort"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// InstrumentCallRequest registers a scripted instrument request (interface 4).
type InstrumentCallRequest struct {
	OperationID domain.OperationID    `json:"operation_id"`
	Digest      domain.Digest         `json:"digest"`
	Generation  domain.Generation     `json:"generation"`
	At          domain.LogicalTime    `json:"at"`
	CallID      string                `json:"call_id"`
	Instrument  domain.InstrumentKind `json:"instrument"`
	InputDigest domain.Digest         `json:"input_digest"`
	MaxAttempts int                   `json:"max_attempts"`
}

// InstrumentOutcomeRequest resolves a scripted instrument call (interface 4).
// A structured success stores the reading; a failure only records the fault and
// retry state, never producing a reading or advancing aggregate state.
type InstrumentOutcomeRequest struct {
	OperationID domain.OperationID `json:"operation_id"`
	Digest      domain.Digest      `json:"digest"`
	Generation  domain.Generation  `json:"generation"`
	At          domain.LogicalTime `json:"at"`
	CallID      string             `json:"call_id"`
	Succeeded   bool               `json:"succeeded"`
	FaultCode   string             `json:"fault_code,omitempty"`
	Response    domain.Digest      `json:"response,omitempty"`
	RetryDelay  domain.LogicalTime `json:"retry_delay,omitempty"`
}

// RegisterCall enqueues a pending instrument call with its deterministic retry
// budget.
func (s *Service) RegisterCall(id domain.TaskID, req InstrumentCallRequest) error {
	scope := taskScope(id)
	reqDigest := digestOf(req)
	replay, hit, err := s.idemBegin(scope, req.OperationID, reqDigest)
	if err != nil {
		return err
	}
	if hit && len(replay) > 0 {
		return nil
	}

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
		if req.MaxAttempts <= 0 {
			return rules.New(rules.CodeInvalidSign, string(req.OperationID), "max attempts must be positive")
		}
		b.instruments.Enqueue(domain.InstrumentCall{
			ID: req.CallID, Instrument: req.Instrument, InputDigest: req.InputDigest,
			At: req.At, MaxAttempts: req.MaxAttempts,
		})
		dehydrate(st, b)
		return nil
	})
	if err != nil {
		return err
	}
	return s.idemCommit(scope, req.OperationID, reqDigest, map[string]string{"status": "registered"}, 201)
}

// ResolveCall records the deterministic outcome of a scripted call.
func (s *Service) ResolveCall(id domain.TaskID, req InstrumentOutcomeRequest) (*domain.InstrumentCall, error) {
	scope := taskScope(id)
	reqDigest := digestOf(req)
	replay, hit, err := s.idemBegin(scope, req.OperationID, reqDigest)
	if err != nil {
		return nil, err
	}
	if hit {
		var out domain.InstrumentCall
		if err := json.Unmarshal(replay, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	var out domain.InstrumentCall
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
		if req.Succeeded {
			if err := b.instruments.Succeed(req.CallID, req.Response, req.OperationID); err != nil {
				return err
			}
		} else {
			if err := b.instruments.Fail(req.CallID, req.FaultCode, req.At, req.RetryDelay, req.OperationID); err != nil {
				return err
			}
		}
		out = *b.instruments.Calls()[req.CallID]
		dehydrate(st, b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.idemCommit(scope, req.OperationID, reqDigest, &out, 200); err != nil {
		return nil, err
	}
	return &out, nil
}

// PendingCalls returns the pending retry queue for a task in deterministic
// order (interface 4). It is read-only.
func (s *Service) PendingCalls(id domain.TaskID) ([]domain.InstrumentCall, error) {
	var out []domain.InstrumentCall
	_, err := s.store.View(id, func(st *store.TaskState) {
		b := hydrate(st)
		out = b.instruments.Pending(st.Header.Clock)
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	})
	return out, err
}
