package store

import (
	"sort"
	"sync"

	"rammed-earth-roof-beam-clearance/internal/domain"
)

// MemoryStore is a deterministic in-memory implementation of Store. It pins
// the Store contract for tests and the runnable entry point, exercising the
// same per-task locking and atomic read-modify-write semantics as the bbolt
// backend.
type MemoryStore struct {
	mu     sync.RWMutex
	tasks  map[domain.TaskID]*taskEntry
	idem   map[string]domain.IdempotencyRecord
	ready  bool
	closed bool
}

type taskEntry struct {
	mu    sync.RWMutex
	state *TaskState
}

// NewMemoryStore creates an empty memory store that is immediately ready.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tasks: make(map[domain.TaskID]*taskEntry),
		idem:  make(map[string]domain.IdempotencyRecord),
		ready: true,
	}
}

func idemKey(scope string, op domain.OperationID) string {
	return scope + "\x00" + string(op)
}

// Create implements Store.
func (s *MemoryStore) Create(id domain.TaskID, st *TaskState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if _, ok := s.tasks[id]; ok {
		return ErrDuplicate
	}
	cp := cloneTaskState(st)
	s.tasks[id] = &taskEntry{state: cp}
	return nil
}

// Update implements Store.
func (s *MemoryStore) Update(id domain.TaskID, fn func(st *TaskState) error) error {
	entry := s.entry(id)
	if entry == nil {
		return ErrNotFound
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := fn(entry.state); err != nil {
		return err
	}
	return nil
}

// View implements Store.
func (s *MemoryStore) View(id domain.TaskID, fn func(st *TaskState)) (bool, error) {
	entry := s.entry(id)
	if entry == nil {
		return false, nil
	}
	entry.mu.RLock()
	defer entry.mu.RUnlock()
	fn(entry.state)
	return true, nil
}

func (s *MemoryStore) entry(id domain.TaskID) *taskEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil
	}
	return s.tasks[id]
}

// TaskIDs implements Store.
func (s *MemoryStore) TaskIDs() []domain.TaskID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.TaskID, 0, len(s.tasks))
	for id := range s.tasks {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// PutIdempotency implements Store.
func (s *MemoryStore) PutIdempotency(rec domain.IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	s.idem[idemKey(rec.Scope, rec.OperationID)] = rec
	return nil
}

// Idempotency implements Store.
func (s *MemoryStore) Idempotency(scope string, op domain.OperationID) (domain.IdempotencyRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.idem[idemKey(scope, op)]
	return rec, ok
}

// Recover implements Store. The in-memory store has nothing to reload.
func (s *MemoryStore) Recover() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
	return nil
}

// Ready implements Store.
func (s *MemoryStore) Ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready && !s.closed
}

// Close implements Store.
func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func cloneTaskState(st *TaskState) *TaskState {
	if st == nil {
		return nil
	}
	data := mustJSONMarshal(st)
	out := &TaskState{}
	_ = jsonUnmarshal(data, out)
	return out
}
