// Package store defines the persistence boundary for the workflow: the
// transactional repository over per-task aggregate state and idempotency
// records, plus the recovery contract for startup. Two implementations satisfy
// the boundary — a deterministic in-memory store for tests and a bbolt-backed
// embedded transactional database for production — so the atomic failure
// boundary (failure boundary 1) is identical regardless of backend.
package store

import (
	"errors"

	"rammed-earth-roof-beam-clearance/internal/domain"
)

// Store is the persistence boundary. All material ledgers, leases, evidence,
// instrument calls, generations and verdicts are written through it so the
// failure boundary of a single atomic transaction is preserved.
type Store interface {
	// Create inserts a new task state. It fails if the ID already exists.
	Create(id domain.TaskID, st *TaskState) error
	// Update runs fn under an exclusive per-task lock. If fn returns nil the
	// (mutated) state is persisted atomically; otherwise the in-memory changes
	// are discarded. This is the atomic boundary for material deduction, lease
	// acquisition, lattice appends, evidence and terminal verdicts.
	Update(id domain.TaskID, fn func(st *TaskState) error) error
	// View runs fn under a shared lock for read access. ok is false when the
	// task does not exist.
	View(id domain.TaskID, fn func(st *TaskState)) (bool, error)
	// TaskIDs returns all persisted task IDs in deterministic order.
	TaskIDs() []domain.TaskID

	// PutIdempotency records the canonical result of an operation for replay
	// across restarts (domain rule 8).
	PutIdempotency(rec domain.IdempotencyRecord) error
	// Idempotency returns a previously recorded idempotency result.
	Idempotency(scope string, op domain.OperationID) (domain.IdempotencyRecord, bool)

	// Recover performs startup recovery: reloads incomplete aggregates, marks
	// expired leases and restores the pending retry queue (failure boundary 8).
	// It returns an error when integrity checks fail.
	Recover() error
	// Ready reports whether recovery completed and the store is usable.
	Ready() bool

	// Close releases any underlying resources.
	Close() error
}

// ErrClosed is returned by operations on a closed store.
var ErrClosed = errors.New("store is closed")

// ErrNotFound is returned when a requested task does not exist.
var ErrNotFound = errors.New("task not found")

// ErrDuplicate is returned when creating a task that already exists.
var ErrDuplicate = errors.New("task already exists")
