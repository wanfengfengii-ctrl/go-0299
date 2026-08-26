package service

import (
	"path/filepath"
	"reflect"
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func TestModel_TerminalVerdictStatusPersistence(t *testing.T) {
	tests := []struct {
		name       string
		kind       domain.VerdictKind
		wantStatus domain.TaskStatus
	}{
		{name: "clearance", kind: domain.VerdictClear, wantStatus: domain.TaskCleared},
		{name: "cancel", kind: domain.VerdictCancel, wantStatus: domain.TaskCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "task.db")
			st, err := store.OpenBoltStore(dbPath)
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			if err := st.Recover(); err != nil {
				t.Fatalf("recover store: %v", err)
			}

			svc := New(st)
			locked, err := svc.Lock(validSpec())
			if err != nil {
				t.Fatalf("lock task: %v", err)
			}
			for i, reviewer := range []string{"alice", "bob"} {
				review := ReviewRequest{
					OperationID: domain.OperationID("review-" + reviewer),
					Digest:      locked.Digest,
					Generation:  locked.Generation,
					At:          domain.LogicalTime(10 + i),
					Reviewer:    reviewer,
					Qualified:   true,
					Conclusion:  "pass",
				}
				if err := svc.SubmitReview(locked.TaskID, review); err != nil {
					t.Fatalf("submit review by %s: %v", reviewer, err)
				}
			}

			request := VerdictRequest{
				OperationID: "terminal",
				Digest:      locked.Digest,
				Generation:  locked.Generation,
				At:          20,
				Kind:        tt.kind,
			}
			first, err := svc.SubmitVerdict(locked.TaskID, request)
			if err != nil {
				t.Fatalf("submit %s verdict: %v", tt.kind, err)
			}
			if first.Kind != tt.kind {
				t.Errorf("verdict kind = %q, want %q", first.Kind, tt.kind)
			}
			if first.Credential == "" {
				t.Error("successful terminal verdict returned an empty credential")
			}

			snapshot, ok, err := svc.Snapshot(locked.TaskID)
			if err != nil || !ok {
				t.Fatalf("snapshot immediately after verdict: ok=%v err=%v", ok, err)
			}
			if snapshot.Status != tt.wantStatus {
				t.Errorf("snapshot status immediately after %s = %q, want %q", tt.kind, snapshot.Status, tt.wantStatus)
			}
			if snapshot.Verdict == nil || snapshot.Verdict.Kind != tt.kind {
				t.Errorf("snapshot verdict immediately after write = %#v, want kind %q", snapshot.Verdict, tt.kind)
			}

			if err := st.Close(); err != nil {
				t.Fatalf("close store: %v", err)
			}
			reopened, err := store.OpenBoltStore(dbPath)
			if err != nil {
				t.Fatalf("reopen store: %v", err)
			}
			defer reopened.Close()
			if err := reopened.Recover(); err != nil {
				t.Fatalf("recover reopened store: %v", err)
			}

			var recoveredStatus domain.TaskStatus
			ok, err = reopened.View(locked.TaskID, func(state *store.TaskState) {
				recoveredStatus = state.Header.Status
			})
			if err != nil || !ok {
				t.Fatalf("view recovered task: ok=%v err=%v", ok, err)
			}
			if recoveredStatus != tt.wantStatus {
				t.Errorf("recovered header status after %s = %q, want %q", tt.kind, recoveredStatus, tt.wantStatus)
			}

			recoveredService := New(reopened)
			replay, err := recoveredService.SubmitVerdict(locked.TaskID, request)
			if err != nil {
				t.Fatalf("replay verdict after restart: %v", err)
			}
			if !reflect.DeepEqual(replay, first) {
				t.Errorf("replayed verdict = %#v, want %#v", replay, first)
			}

			_, err = recoveredService.SubmitVerdict(locked.TaskID, VerdictRequest{
				OperationID: "conflicting-terminal",
				Digest:      locked.Digest,
				Generation:  locked.Generation,
				At:          30,
				Kind:        domain.VerdictIsolate,
			})
			if err == nil || !rules.Is(err, rules.CodeFinalConflict) {
				t.Errorf("conflicting verdict error = %v, want FINAL_CONFLICT", err)
			}
		})
	}
}
