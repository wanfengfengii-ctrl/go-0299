package service

import (
	"encoding/json"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// ReviewRequest submits one independent qualified-person review (interface 6).
type ReviewRequest struct {
	OperationID domain.OperationID `json:"operation_id"`
	Digest      domain.Digest      `json:"digest"`
	Generation  domain.Generation  `json:"generation"`
	At          domain.LogicalTime `json:"at"`
	Reviewer    string             `json:"reviewer"`
	Qualified   bool               `json:"qualified"`
	Conclusion  string             `json:"conclusion"`
}

// VerdictRequest competes for the single terminal verdict (interface 6).
type VerdictRequest struct {
	OperationID domain.OperationID `json:"operation_id"`
	Digest      domain.Digest      `json:"digest"`
	Generation  domain.Generation  `json:"generation"`
	At          domain.LogicalTime `json:"at"`
	Kind        domain.VerdictKind `json:"kind"`
}

// VerdictResult is the deterministic terminal verdict outcome.
type VerdictResult struct {
	VerdictID  string             `json:"verdict_id"`
	Kind       domain.VerdictKind `json:"kind"`
	Credential string             `json:"credential,omitempty"`
	WriteVer   int64              `json:"write_version"`
}

// SubmitReview records an independent review. A reviewer may not submit twice
// and must be qualified (test scenario 10).
func (s *Service) SubmitReview(id domain.TaskID, req ReviewRequest) error {
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
		if err := b.reviews.Add(domain.Review{
			ID:         "review-" + string(req.OperationID),
			Wall:       string(st.Header.ID),
			Reviewer:   req.Reviewer,
			Qualified:  req.Qualified,
			Conclusion: req.Conclusion,
			At:         req.At,
		}, req.OperationID); err != nil {
			return err
		}
		dehydrate(st, b)
		return nil
	})
	if err != nil {
		return err
	}
	return s.idemCommit(scope, req.OperationID, reqDigest, map[string]string{"status": "reviewed"}, 200)
}

// SubmitVerdict competes for the single-write terminal verdict. Only the first
// write wins; concurrent or conflicting writes read the existing verdict or
// receive FINAL_CONFLICT (failure boundary 7, acceptance 7).
func (s *Service) SubmitVerdict(id domain.TaskID, req VerdictRequest) (*VerdictResult, error) {
	scope := taskScope(id)
	reqDigest := digestOf(req)
	replay, hit, err := s.idemBegin(scope, req.OperationID, reqDigest)
	if err != nil {
		return nil, err
	}
	if hit {
		var out VerdictResult
		if err := json.Unmarshal(replay, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	result := &VerdictResult{}
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
		if !b.reviews.CanFinalize() {
			return rules.New(rules.CodeOutOfOrder, string(req.OperationID), "two qualified reviewers required before verdict")
		}

		verdict := domain.FinalVerdict{
			ID:         "verdict-" + string(req.OperationID),
			Wall:       string(st.Header.ID),
			Kind:       req.Kind,
			Credential: "cred-" + string(req.OperationID),
			At:         req.At,
		}
		written, err := b.verdict.Write(verdict, req.OperationID)
		if err != nil {
			return err
		}

		switch written.Kind {
		case domain.VerdictClear:
			st.Header.Status = domain.TaskCleared
		case domain.VerdictIsolate:
			b.task.Isolate()
			st.Header = b.task.Header()
		case domain.VerdictCancel:
			st.Header.Status = domain.TaskCancelled
		}

		result.VerdictID = written.ID
		result.Kind = written.Kind
		result.Credential = written.Credential
		result.WriteVer = written.WriteVer

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
