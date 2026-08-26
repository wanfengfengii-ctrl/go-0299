package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	bolt "go.etcd.io/bbolt"

	"rammed-earth-roof-beam-clearance/internal/domain"
)

var (
	bucketTasks       = []byte("tasks")
	bucketIdempotency = []byte("idempotency")
)

// BoltStore is the production embedded transactional database backend. It
// persists task state and idempotency records to a bbolt file, providing real
// restart recovery: on Recover it reloads every incomplete aggregate, verifies
// ledger integrity, marks expired leases by persisted logical time and restores
// the pending retry queue (failure boundary 8).
type BoltStore struct {
	db     *bolt.DB
	mu     sync.RWMutex
	tasks  map[domain.TaskID]*taskEntry
	idem   map[string]domain.IdempotencyRecord
	ready  bool
	closed bool
}

// OpenBoltStore opens (or creates) the bbolt database at path and returns a
// store whose recovery has not yet run. Call Recover before use.
func OpenBoltStore(path string) (*BoltStore, error) {
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("open bbolt: %w", err)
	}
	return &BoltStore{
		db:    db,
		tasks: make(map[domain.TaskID]*taskEntry),
		idem:  make(map[string]domain.IdempotencyRecord),
	}, nil
}

// Recover implements Store. It loads all tasks and idempotency records from
// disk, verifies ledger integrity and marks expired leases. Any corruption
// leaves the store non-ready and returns an error so the readiness endpoint
// reports RECOVERY_INTEGRITY_FAILED.
func (s *BoltStore) Recover() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		tb := tx.Bucket(bucketTasks)
		if tb != nil {
			if err := tb.ForEach(func(k, v []byte) error {
				st := &TaskState{}
				if err := jsonUnmarshal(v, st); err != nil {
					return fmt.Errorf("corrupt task %q: %w", string(k), err)
				}
				st.ensureMaps()
				if err := verifyIntegrity(st); err != nil {
					return fmt.Errorf("integrity failed for %q: %w", string(k), err)
				}
				s.tasks[domain.TaskID(k)] = &taskEntry{state: st}
				return nil
			}); err != nil {
				return err
			}
		}
		ib := tx.Bucket(bucketIdempotency)
		if ib != nil {
			if err := ib.ForEach(func(k, v []byte) error {
				var rec domain.IdempotencyRecord
				if err := json.Unmarshal(v, &rec); err != nil {
					return fmt.Errorf("corrupt idempotency record: %w", err)
				}
				s.idem[string(k)] = rec
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Mark expired leases by persisted logical time.
	for _, e := range s.tasks {
		reclaimExpired(e.state)
	}
	s.ready = true
	return nil
}

// verifyIntegrity rejects a ledger with a negative balance (overclaim would
// have been prevented at write time, so a negative balance indicates damage).
func verifyIntegrity(st *TaskState) error {
	for acct, v := range st.Balances {
		if v < 0 {
			return fmt.Errorf("negative balance for account %s: %d", acct, v)
		}
	}
	return nil
}

// reclaimExpired marks every active lease whose expiry has passed as expired,
// using the persisted logical clock as "now".
func reclaimExpired(st *TaskState) {
	now := st.Header.Clock
	for k, l := range st.Leases {
		if l.Status == domain.LeaseActive && l.ExpiresAt < now {
			l.Status = domain.LeaseExpired
			l.ReclaimVer++
			st.Leases[k] = l
		}
	}
}

// Ready implements Store.
func (s *BoltStore) Ready() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready && !s.closed
}

// Close implements Store.
func (s *BoltStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}

// Create implements Store.
func (s *BoltStore) Create(id domain.TaskID, st *TaskState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if _, ok := s.tasks[id]; ok {
		return ErrDuplicate
	}
	st.ensureMaps()
	cp := cloneTaskState(st)
	if err := s.persistTask(id, cp); err != nil {
		return err
	}
	s.tasks[id] = &taskEntry{state: cp}
	return nil
}

// Update implements Store.
func (s *BoltStore) Update(id domain.TaskID, fn func(st *TaskState) error) error {
	entry := s.entry(id)
	if entry == nil {
		return ErrNotFound
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := fn(entry.state); err != nil {
		return err
	}
	return s.persistTask(id, cloneTaskState(entry.state))
}

// View implements Store.
func (s *BoltStore) View(id domain.TaskID, fn func(st *TaskState)) (bool, error) {
	entry := s.entry(id)
	if entry == nil {
		return false, nil
	}
	entry.mu.RLock()
	entry.mu.RUnlock()
	fn(entry.state)
	return true, nil
}

func (s *BoltStore) entry(id domain.TaskID) *taskEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil
	}
	return s.tasks[id]
}

// persistTask writes the serialised task state to the bbolt file in a single
// transaction. It assumes s.mu or the entry lock is held by the caller.
func (s *BoltStore) persistTask(id domain.TaskID, st *TaskState) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketTasks)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), mustJSONMarshal(st))
	})
}

// TaskIDs implements Store.
func (s *BoltStore) TaskIDs() []domain.TaskID {
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
func (s *BoltStore) PutIdempotency(rec domain.IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	key := idemKey(rec.Scope, rec.OperationID)
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketIdempotency)
		if err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(key), data)
	}); err != nil {
		return err
	}
	s.idem[key] = rec
	return nil
}

// Idempotency implements Store.
func (s *BoltStore) Idempotency(scope string, op domain.OperationID) (domain.IdempotencyRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.idem[idemKey(scope, op)]
	return rec, ok
}
