package service

import (
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func TestModel_InstrumentCallTerminalOutcome(t *testing.T) {
	tests := []struct {
		name            string
		maxAttempts     int
		firstSucceeded  bool
		firstFault      string
		firstResponse   domain.Digest
		secondSucceeded bool
		secondFault     string
		secondResponse  domain.Digest
		wantSecondError bool
		wantStatus      domain.CallStatus
		wantAttempts    int
		wantFault       string
		wantRawResponse domain.Digest
	}{
		{
			name:        "exhausted refusal rejects late success",
			maxAttempts: 1, firstFault: "REFUSED",
			secondSucceeded: true, secondResponse: "late-success-response",
			wantSecondError: true, wantStatus: domain.CallFailed, wantAttempts: 1,
			wantFault: "REFUSED",
		},
		{
			name:        "exhausted refusal rejects another failure",
			maxAttempts: 1, firstFault: "REFUSED",
			secondFault:     "GATEWAY_TIMEOUT",
			wantSecondError: true, wantStatus: domain.CallFailed, wantAttempts: 1,
			wantFault: "REFUSED",
		},
		{
			name:        "pending call accepts success",
			maxAttempts: 2, firstFault: "TEMPORARY",
			secondSucceeded: true, secondResponse: "press-reading",
			wantStatus: domain.CallSucceeded, wantAttempts: 1,
			wantFault: "TEMPORARY", wantRawResponse: "press-reading",
		},
		{
			name:        "pending call can exhaust with failure",
			maxAttempts: 2, firstFault: "TEMPORARY",
			secondFault: "REFUSED",
			wantStatus:  domain.CallFailed, wantAttempts: 2, wantFault: "REFUSED",
		},
		{
			name:        "successful call rejects late failure",
			maxAttempts: 2, firstSucceeded: true, firstResponse: "original-reading",
			secondFault:     "REFUSED",
			wantSecondError: true, wantStatus: domain.CallSucceeded,
			wantRawResponse: "original-reading",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := New(store.NewMemoryStore())
			locked := lockTask(t, svc)
			const callID = "press-call"

			if err := svc.RegisterCall(locked.TaskID, InstrumentCallRequest{
				OperationID: "register-call", Digest: locked.Digest, Generation: 1, At: 1,
				CallID: callID, Instrument: domain.InstrumentPress,
				InputDigest: "press-input", MaxAttempts: tt.maxAttempts,
			}); err != nil {
				t.Fatalf("register call: %v", err)
			}

			first, err := svc.ResolveCall(locked.TaskID, InstrumentOutcomeRequest{
				OperationID: "first-receipt", Digest: locked.Digest, Generation: 1, At: 2,
				CallID: callID, Succeeded: tt.firstSucceeded, FaultCode: tt.firstFault,
				Response: tt.firstResponse, RetryDelay: 1,
			})
			if err != nil {
				t.Fatalf("first receipt: %v", err)
			}
			if tt.maxAttempts == 1 && !tt.firstSucceeded {
				if first.Status != domain.CallFailed || first.Attempts != 1 || first.RawResponse != "" {
					t.Fatalf("exhausted call before late receipt = %+v", *first)
				}
			}

			_, err = svc.ResolveCall(locked.TaskID, InstrumentOutcomeRequest{
				OperationID: "second-receipt", Digest: locked.Digest, Generation: 1, At: 3,
				CallID: callID, Succeeded: tt.secondSucceeded, FaultCode: tt.secondFault,
				Response: tt.secondResponse, RetryDelay: 1,
			})
			if tt.wantSecondError {
				if err == nil || !rules.Is(err, rules.CodeOutOfOrder) {
					t.Fatalf("second receipt error = %v, want %s", err, rules.CodeOutOfOrder)
				}
				if ruleErr, ok := err.(*rules.Error); !ok || ruleErr.OperationID != "second-receipt" {
					t.Fatalf("second receipt returned unstable error: %#v", err)
				}
			} else if err != nil {
				t.Fatalf("second receipt: %v", err)
			}

			snapshot, ok, err := svc.Snapshot(locked.TaskID)
			if err != nil || !ok {
				t.Fatalf("snapshot: ok=%v err=%v", ok, err)
			}
			if len(snapshot.Calls) != 1 {
				t.Fatalf("snapshot calls = %d, want 1", len(snapshot.Calls))
			}
			call := snapshot.Calls[0]
			if call.Status != tt.wantStatus || call.Attempts != tt.wantAttempts ||
				call.FaultCode != tt.wantFault || call.RawResponse != tt.wantRawResponse {
				t.Fatalf("final call = %+v, want status=%s attempts=%d fault=%q raw_response=%q",
					call, tt.wantStatus, tt.wantAttempts, tt.wantFault, tt.wantRawResponse)
			}
		})
	}
}
