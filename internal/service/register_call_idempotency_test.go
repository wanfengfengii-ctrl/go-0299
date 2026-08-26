package service

import (
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func TestModel_RegisterCallIdempotencyAfterValidation(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*InstrumentCallRequest)
		wantCode rules.Code
		valid    bool
	}{
		{
			name: "invalid retry budget is rejected on every replay",
			mutate: func(req *InstrumentCallRequest) {
				req.MaxAttempts = 0
			},
			wantCode: rules.CodeInvalidSign,
		},
		{
			name: "stale digest is rejected on every replay",
			mutate: func(req *InstrumentCallRequest) {
				req.Digest = "stale-digest"
			},
			wantCode: rules.CodeDesignDigestStale,
		},
		{
			name: "stale generation is rejected on every replay",
			mutate: func(req *InstrumentCallRequest) {
				req.Generation++
			},
			wantCode: rules.CodeGenerationStale,
		},
		{
			name: "non-increasing logical time is rejected on every replay",
			mutate: func(req *InstrumentCallRequest) {
				req.At = 0
			},
			wantCode: rules.CodeClockRegression,
		},
		{
			name:   "successful registration replays once and conflicts on different content",
			mutate: func(*InstrumentCallRequest) {},
			valid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(store.NewMemoryStore())
			locked := lockTask(t, s)
			req := InstrumentCallRequest{
				OperationID: "register-moisture-1",
				Digest:      locked.Digest,
				Generation:  locked.Generation,
				At:          10,
				CallID:      "moisture-call-1",
				Instrument:  domain.InstrumentMoisture,
				InputDigest: "sample-digest-1",
				MaxAttempts: 1,
			}
			tt.mutate(&req)

			if !tt.valid {
				var firstError string
				for attempt := 1; attempt <= 2; attempt++ {
					err := s.RegisterCall(locked.TaskID, req)
					if err == nil || !rules.Is(err, tt.wantCode) {
						t.Fatalf("attempt %d: want %s, got %v", attempt, tt.wantCode, err)
					}
					if attempt == 1 {
						firstError = err.Error()
					} else if err.Error() != firstError {
						t.Fatalf("replay error = %q, want stable error %q", err.Error(), firstError)
					}

					pending, err := s.PendingCalls(locked.TaskID)
					if err != nil {
						t.Fatalf("pending calls after attempt %d: %v", attempt, err)
					}
					if len(pending) != 0 {
						t.Fatalf("pending calls after rejected attempt %d = %d, want 0", attempt, len(pending))
					}

					snapshot, ok, err := s.Snapshot(locked.TaskID)
					if err != nil || !ok {
						t.Fatalf("snapshot after attempt %d: ok=%v err=%v", attempt, ok, err)
					}
					if len(snapshot.Calls) != 0 {
						t.Fatalf("snapshot calls after rejected attempt %d = %d, want 0", attempt, len(snapshot.Calls))
					}
				}
				return
			}

			if err := s.RegisterCall(locked.TaskID, req); err != nil {
				t.Fatalf("first registration: %v", err)
			}
			if err := s.RegisterCall(locked.TaskID, req); err != nil {
				t.Fatalf("identical replay: %v", err)
			}

			pending, err := s.PendingCalls(locked.TaskID)
			if err != nil {
				t.Fatalf("pending calls: %v", err)
			}
			if len(pending) != 1 || pending[0].ID != req.CallID || pending[0].Status != domain.CallPending {
				t.Fatalf("pending calls = %#v, want one pending call %q", pending, req.CallID)
			}

			snapshot, ok, err := s.Snapshot(locked.TaskID)
			if err != nil || !ok {
				t.Fatalf("snapshot: ok=%v err=%v", ok, err)
			}
			if len(snapshot.Calls) != 1 || snapshot.Calls[0].ID != req.CallID {
				t.Fatalf("snapshot calls = %#v, want exactly one call %q", snapshot.Calls, req.CallID)
			}

			conflict := req
			conflict.InputDigest = "different-sample-digest"
			if err := s.RegisterCall(locked.TaskID, conflict); err == nil || !rules.Is(err, rules.CodeIdempotencyConflict) {
				t.Fatalf("different content with reused operation id: want %s, got %v", rules.CodeIdempotencyConflict, err)
			}
		})
	}
}
