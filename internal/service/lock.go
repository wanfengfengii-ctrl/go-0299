package service

import (
	"crypto/rand"
	"encoding/hex"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// RuleVersion is the immutable rules-catalog version stamped onto every locked
// task at creation.
const RuleVersion = 1

// LockResult is returned by a successful lock: the task id, the immutable
// design digest and the initial generation (acceptance 1).
type LockResult struct {
	TaskID     domain.TaskID     `json:"task_id"`
	Area       string            `json:"area"`
	Digest     domain.Digest     `json:"digest"`
	Generation domain.Generation `json:"generation"`
	Status     domain.TaskStatus `json:"status"`
}

// Lock validates and locks a complete task design. It rejects invalid geometry
// (gaps, overlap, degenerate, negative, overflow), forbidden-zone conflicts,
// invalid recipe/program/thresholds and coverage gaps before generating the
// immutable digest and initial generation.
func (s *Service) Lock(spec domain.TaskSpec) (*LockResult, error) {
	if err := rules.ValidateGeometry(spec.Geometry); err != nil {
		return nil, err
	}
	if err := rules.ValidateCoverage(spec.Geometry); err != nil {
		return nil, err
	}
	if err := rules.ValidateSpec(spec); err != nil {
		return nil, err
	}
	if spec.Direction != domain.DirectionRising && spec.Direction != domain.DirectionFalling {
		return nil, rules.New(rules.CodeInvalidSign, "", "direction must be rising or falling")
	}

	id, err := newTaskID()
	if err != nil {
		return nil, err
	}
	digest := domain.Digest(rules.Hash(rules.CanonicalSpec(spec)))

	header := domain.LockedTask{
		ID:          id,
		Area:        spec.Area,
		Digest:      digest,
		Generation:  1,
		Direction:   spec.Direction,
		Clock:       0,
		Status:      domain.TaskActive,
		RuleVersion: RuleVersion,
	}

	st := store.NewTaskState(header, spec.Geometry, spec)
	// Seed the source component balances from the locked batches (the opening
	// balance injection; source components may only decrease afterwards).
	for _, b := range spec.Batches {
		st.Balances[b.Component] += b.BalanceG
	}

	if err := s.store.Create(id, st); err != nil {
		return nil, err
	}

	return &LockResult{
		TaskID:     id,
		Area:       spec.Area,
		Digest:     digest,
		Generation: header.Generation,
		Status:     header.Status,
	}, nil
}

// newTaskID generates a random opaque task identifier.
func newTaskID() (domain.TaskID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return domain.TaskID(hex.EncodeToString(b[:])), nil
}
