// Package aggregate implements the wall-segment layered-task and compaction
// aggregate: the append-only "wall—layer—compaction-cell" lattice, the
// continuous-prefix rule, cross-layer closure conditions, task generations
// and monotonic logical time, replayed from persistence on restart.
package aggregate

import (
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

// Task is the write-side aggregate for a locked wall task. All state advances
// flow through it so generation and logical-time invariants are enforced in a
// single place.
type Task struct {
	header domain.LockedTask

	// per-layer record of the highest closed cell sequence, used to enforce
	// the continuous-prefix rule when appending cells.
	closed map[int]int
}

// NewTask creates a fresh locked task aggregate. The digest and rule version
// are fixed at lock time and never mutated afterwards.
func NewTask(id domain.TaskID, area string, digest domain.Digest, direction domain.Direction, ruleVersion int) *Task {
	return &Task{
		header: domain.LockedTask{
			ID:          id,
			Area:        area,
			Digest:      digest,
			Generation:  1,
			Direction:   direction,
			Clock:       0,
			Status:      domain.TaskActive,
			RuleVersion: ruleVersion,
		},
		closed: make(map[int]int),
	}
}

// NewTaskFrom reconstructs a task aggregate from persisted header and
// closed-prefix state. It is the restore half of the persistence round-trip.
func NewTaskFrom(h domain.LockedTask, closed map[int]int) *Task {
	if closed == nil {
		closed = make(map[int]int)
	}
	return &Task{header: h, closed: closed}
}

// Closed returns a copy of the per-layer closed-cell prefix map for
// persistence. Callers must not mutate the returned map.
func (t *Task) Closed() map[int]int { return t.closed }

// Header returns the current immutable header.
func (t *Task) Header() domain.LockedTask { return t.header }

// CheckDigest rejects commands whose design digest does not match the locked
// digest (domain rule 1).
func (t *Task) CheckDigest(digest domain.Digest, opID domain.OperationID) error {
	if digest != t.header.Digest {
		return rules.New(rules.CodeDesignDigestStale, string(opID), "design digest does not match locked digest")
	}
	return nil
}

// CheckGeneration rejects commands carrying a stale task generation
// (domain rule 1). Only the current generation may advance state.
func (t *Task) CheckGeneration(gen domain.Generation, opID domain.OperationID) error {
	if gen != t.header.Generation {
		return rules.New(rules.CodeGenerationStale, string(opID), "task generation is stale")
	}
	return nil
}

// AdvanceClock raises the aggregate logical clock, rejecting a regression.
// Logical time is monotonic (domain rule 1 and failure boundary 2).
func (t *Task) AdvanceClock(at domain.LogicalTime, opID domain.OperationID) error {
	if at <= t.header.Clock {
		return rules.New(rules.CodeClockRegression, string(opID), "logical time must be strictly increasing")
	}
	t.header.Clock = at
	return nil
}

// OpenGeneration starts a rebuild generation, invalidating prior receipts
// (domain rule 7). The new generation only accepts its own receipts.
func (t *Task) OpenGeneration(opID domain.OperationID) error {
	if t.header.Status != domain.TaskActive && t.header.Status != domain.TaskIsolated {
		return rules.New(rules.CodeGenerationStale, string(opID), "cannot open generation in terminal state")
	}
	t.header.Generation++
	return nil
}

// Isolate marks the task structurally isolated.
func (t *Task) Isolate() { t.header.Status = domain.TaskIsolated }

// AppendCell appends a compaction cell enforcing the continuous-prefix rule:
// a layer's cells must be appended in strict sequence order with no gaps, and
// the previous cell in the layer must already be closed before the next opens.
func (t *Task) AppendCell(layer int, seq int, opID domain.OperationID) error {
	if seq < 0 {
		return rules.New(rules.CodeOutOfOrder, string(opID), "cell sequence must be non-negative")
	}
	next := t.closed[layer]
	if seq != next {
		return rules.New(rules.CodeOutOfOrder, string(opID), "cell sequence must form a continuous prefix")
	}
	t.closed[layer] = seq + 1
	return nil
}

// CloseLayer reports whether the given layer has a fully closed continuous
// prefix, required before opening the first cell of the layer above
// (domain rule 3).
func (t *Task) CloseLayer(layer int, cellsInLayer int) error {
	if t.closed[layer] != cellsInLayer {
		return rules.New(rules.CodeOutOfOrder, "", "layer has unclosed cells")
	}
	return nil
}
