package service

import (
	"path/filepath"
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func TestModel_InstrumentCallIdentitySurvivesReregistrationAndRestart(t *testing.T) {
	tests := []struct {
		name    string
		restart bool
	}{
		{name: "memory_store", restart: false},
		{name: "bolt_store_restart", restart: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var st store.Store
			var dbPath string
			if tt.restart {
				dbPath = filepath.Join(t.TempDir(), "task.db")
				bolt, err := store.OpenBoltStore(dbPath)
				if err != nil {
					t.Fatalf("open bolt store: %v", err)
				}
				if err := bolt.Recover(); err != nil {
					t.Fatalf("recover bolt store: %v", err)
				}
				st = bolt
			} else {
				st = store.NewMemoryStore()
			}

			svc := New(st)
			locked := lockTask(t, svc)
			original := InstrumentCallRequest{
				OperationID: "register-press-1",
				Digest:      locked.Digest,
				Generation:  1,
				At:          10,
				CallID:      "press-call-1",
				Instrument:  domain.InstrumentPress,
				InputDigest: "pressure-specimen-1",
				MaxAttempts: 3,
			}
			if err := svc.RegisterCall(locked.TaskID, original); err != nil {
				t.Fatalf("register original call: %v", err)
			}
			if err := svc.RegisterCall(locked.TaskID, original); err != nil {
				t.Fatalf("replay original registration: %v", err)
			}

			failed, err := svc.ResolveCall(locked.TaskID, InstrumentOutcomeRequest{
				OperationID: "timeout-press-1",
				Digest:      locked.Digest,
				Generation:  1,
				At:          20,
				CallID:      original.CallID,
				Succeeded:   false,
				FaultCode:   "TIMEOUT",
				RetryDelay:  30,
			})
			if err != nil {
				t.Fatalf("record timeout: %v", err)
			}
			if failed.Attempts != 1 || failed.Status != domain.CallPending || failed.FaultCode != "TIMEOUT" || failed.NextRetryAt != 50 {
				t.Fatalf("timeout state = %+v, want one pending TIMEOUT attempt due at 50", *failed)
			}
			if pending, err := svc.PendingCalls(locked.TaskID); err != nil {
				t.Fatalf("pending before retry time: %v", err)
			} else if len(pending) != 0 {
				t.Fatalf("pending before retry time = %+v, want none", pending)
			}

			duplicate := original
			duplicate.OperationID = "register-press-duplicate"
			duplicate.At = 21
			duplicate.Instrument = domain.InstrumentScale
			duplicate.InputDigest = "unrelated-input"
			duplicate.MaxAttempts = 9
			conflict := svc.RegisterCall(locked.TaskID, duplicate)
			if conflict == nil || (!rules.Is(conflict, rules.CodeIdempotencyConflict) && !rules.Is(conflict, rules.CodeOutOfOrder)) {
				t.Fatalf("reuse call_id with a different operation = %v, want stable conflict or out-of-order error", conflict)
			}
			conflictAgain := svc.RegisterCall(locked.TaskID, duplicate)
			firstRule, firstOK := conflict.(*rules.Error)
			secondRule, secondOK := conflictAgain.(*rules.Error)
			if !firstOK || !secondOK || firstRule.Code != secondRule.Code {
				t.Fatalf("repeated call_id conflict changed: first=%v second=%v", conflict, conflictAgain)
			}
			if err := svc.RegisterCall(locked.TaskID, original); err != nil {
				t.Fatalf("original operation replay after conflict: %v", err)
			}

			if tt.restart {
				if err := st.Close(); err != nil {
					t.Fatalf("close before restart: %v", err)
				}
				bolt, err := store.OpenBoltStore(dbPath)
				if err != nil {
					t.Fatalf("reopen bolt store: %v", err)
				}
				t.Cleanup(func() { _ = bolt.Close() })
				if err := bolt.Recover(); err != nil {
					t.Fatalf("recover after restart: %v", err)
				}
				st = bolt
				svc = New(st)
			}

			snap, ok, err := svc.Snapshot(locked.TaskID)
			if err != nil || !ok {
				t.Fatalf("snapshot after rejected registration: ok=%v err=%v", ok, err)
			}
			if len(snap.Calls) != 1 {
				t.Fatalf("snapshot calls = %+v, want exactly the original call", snap.Calls)
			}
			got := snap.Calls[0]
			if got.ID != original.CallID || got.Instrument != original.Instrument || got.InputDigest != original.InputDigest || got.At != original.At || got.MaxAttempts != original.MaxAttempts || got.Attempts != 1 || got.Status != domain.CallPending || got.FaultCode != "TIMEOUT" || got.NextRetryAt != 50 {
				t.Fatalf("preserved call = %+v, want original registration with one pending TIMEOUT attempt due at 50", got)
			}

			unique := InstrumentCallRequest{
				OperationID: "register-scale-unique",
				Digest:      locked.Digest,
				Generation:  1,
				At:          49,
				CallID:      "scale-call-unique",
				Instrument:  domain.InstrumentScale,
				InputDigest: "unique-input",
				MaxAttempts: 2,
			}
			if err := svc.RegisterCall(locked.TaskID, unique); err != nil {
				t.Fatalf("register unique call: %v", err)
			}
			pending, err := svc.PendingCalls(locked.TaskID)
			if err != nil {
				t.Fatalf("pending at 49: %v", err)
			}
			if len(pending) != 1 || pending[0].ID != unique.CallID {
				t.Fatalf("pending at 49 = %+v, want only the new unique call", pending)
			}
			if _, err := svc.ResolveCall(locked.TaskID, InstrumentOutcomeRequest{
				OperationID: "resolve-scale-unique",
				Digest:      locked.Digest,
				Generation:  1,
				At:          50,
				CallID:      unique.CallID,
				Succeeded:   true,
				Response:    "scale-response",
			}); err != nil {
				t.Fatalf("resolve unique call: %v", err)
			}
			pending, err = svc.PendingCalls(locked.TaskID)
			if err != nil {
				t.Fatalf("pending at retry time: %v", err)
			}
			if len(pending) != 1 || pending[0].ID != original.CallID || pending[0].Attempts != 1 || pending[0].FaultCode != "TIMEOUT" || pending[0].NextRetryAt != 50 {
				t.Fatalf("pending at retry time = %+v, want preserved original retry", pending)
			}
		})
	}
}
