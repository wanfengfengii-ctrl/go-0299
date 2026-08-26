package service

import (
	"encoding/json"
	"strconv"

	"rammed-earth-roof-beam-clearance/internal/arbiter"
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// InspectionRequest submits a measurement (or a directly-observed defect) for a
// measured point or contact face (interface 5). A numeric measurement is judged
// against the locked thresholds; a defect is a finding in its own right.
type InspectionRequest struct {
	OperationID domain.OperationID `json:"operation_id"`
	Digest      domain.Digest      `json:"digest"`
	Generation  domain.Generation  `json:"generation"`
	At          domain.LogicalTime `json:"at"`
	Point       string             `json:"point"`
	Layer       int                `json:"layer,omitempty"`
	Seq         int                `json:"seq,omitempty"`
	Metric      string             `json:"metric,omitempty"`
	Value       int64              `json:"value,omitempty"`
	Defect      domain.FindingKind `json:"defect,omitempty"`
	Actor       string             `json:"actor,omitempty"`
}

// InspectionResult reports whether the measurement produced a finding.
type InspectionResult struct {
	FindingID string             `json:"finding_id,omitempty"`
	Kind      domain.FindingKind `json:"kind,omitempty"`
	Passed    bool               `json:"passed"`
}

// Inspect records inspection evidence and judges it against thresholds,
// producing a finding when the measurement fails (acceptance 5).
func (s *Service) Inspect(id domain.TaskID, req InspectionRequest) (*InspectionResult, error) {
	scope := taskScope(id)
	reqDigest := digestOf(req)
	replay, hit, err := s.idemBegin(scope, req.OperationID, reqDigest)
	if err != nil {
		return nil, err
	}
	if hit {
		var out InspectionResult
		if err := json.Unmarshal(replay, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	result := &InspectionResult{Passed: true}
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

		if req.Defect != "" && !arbiter.IsDefect(req.Defect) {
			return rules.New(rules.CodeInvalidSign, string(req.OperationID), "unknown defect kind")
		}

		kind, isFinding, thFixed := s.judge(st, req)
		if isFinding {
			finding := domain.InspectionFinding{
				ID:             "finding-" + strconv.Itoa(len(st.Findings)+1),
				Wall:           string(st.Header.ID),
				Kind:           kind,
				Point:          req.Point,
				Layer:          req.Layer,
				Seq:            req.Seq,
				ThresholdFixed: thFixed,
				MeasuredFixed:  req.Value,
				At:             req.At,
			}
			st.Findings = append(st.Findings, finding)
			result.FindingID = finding.ID
			result.Kind = kind
			result.Passed = false
		}

		// Record the inspection evidence regardless of pass/fail.
		recordEvent(b, domain.EvidenceEvent{
			Wall: string(st.Header.ID), Layer: req.Layer, Seq: req.Seq,
			Process: domain.ProcessCuring, At: req.At, Actor: req.Actor,
			ValueFixed: req.Value, Digest: st.Header.Digest, Valid: !isFinding,
		}, req.OperationID)

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

// judge decides whether an inspection produces a finding. It returns the
// finding kind, whether it is a finding, and the violated threshold (for
// numeric findings).
func (s *Service) judge(st *store.TaskState, req InspectionRequest) (domain.FindingKind, bool, int64) {
	if req.Defect != "" {
		return req.Defect, true, 0
	}
	kind, ok := arbiter.Judge(req.Metric, req.Value, st.Spec.Thresholds)
	if !ok {
		return "", false, 0
	}
	return kind, true, thresholdFor(req.Metric, st.Spec.Thresholds)
}

// thresholdFor returns the threshold value that a failed metric violated.
func thresholdFor(metric string, th domain.Thresholds) int64 {
	switch metric {
	case arbiter.MetricDryDensity:
		return th.MinDryDensity
	case arbiter.MetricMoisture:
		return th.MaxMoisture
	case arbiter.MetricShear:
		return th.MinShear
	case arbiter.MetricErosion:
		return th.MaxErosion
	}
	return 0
}
