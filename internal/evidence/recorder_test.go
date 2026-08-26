package evidence

import (
	"testing"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

func TestOpenPanDeadline(t *testing.T) {
	r := NewRecorder()
	if err := r.OpenPan(1, 10, 20, "op"); err != nil {
		t.Fatalf("open: %v", err)
	}
	// Deadline must follow make time.
	if err := r.OpenPan(2, 10, 10, "op"); err == nil || !rules.Is(err, rules.CodeMixExpired) {
		t.Fatalf("want MIX_EXPIRED, got %v", err)
	}
	// Pan sequence must increase.
	if err := r.OpenPan(1, 10, 20, "op"); err == nil || !rules.Is(err, rules.CodeWrongPan) {
		t.Fatalf("want WRONG_PAN, got %v", err)
	}
}

func TestRecordMixExpired(t *testing.T) {
	r := NewRecorder()
	if err := r.OpenPan(1, 10, 20, "op"); err != nil {
		t.Fatalf("open: %v", err)
	}
	ev := domain.EvidenceEvent{PanSeq: 1, At: 21, Process: domain.ProcessCompaction}
	if err := r.Record(ev, "op"); err == nil || !rules.Is(err, rules.CodeMixExpired) {
		t.Fatalf("want MIX_EXPIRED, got %v", err)
	}
	// Within deadline succeeds.
	ev.At = 20
	if err := r.Record(ev, "op"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if got := len(r.Events()); got != 1 {
		t.Fatalf("events=%d want 1", got)
	}
}

func TestInstrumentFailRetry(t *testing.T) {
	l := NewInstrumentLog()
	l.Enqueue(domain.InstrumentCall{ID: "c1", Instrument: domain.InstrumentScale, MaxAttempts: 3})
	if err := l.Fail("c1", "TIMEOUT", 10, 5, "op"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	got := l.Calls()["c1"]
	if got.Attempts != 1 || got.Status != domain.CallPending || got.FaultCode != "TIMEOUT" || got.NextRetryAt != 15 {
		t.Fatalf("unexpected call state: %+v", got)
	}
	// Pending queue should be empty until the retry time arrives.
	if n := len(l.Pending(10)); n != 0 {
		t.Fatalf("pending=%d want 0", n)
	}
	if n := len(l.Pending(15)); n != 1 {
		t.Fatalf("pending=%d want 1", n)
	}
}

func TestInstrumentExhaustRetries(t *testing.T) {
	l := NewInstrumentLog()
	l.Enqueue(domain.InstrumentCall{ID: "c1", Instrument: domain.InstrumentPress, MaxAttempts: 2})
	if err := l.Fail("c1", "REFUSED", 0, 1, "op"); err != nil {
		t.Fatalf("fail1: %v", err)
	}
	if err := l.Fail("c1", "REFUSED", 2, 1, "op"); err != nil {
		t.Fatalf("fail2: %v", err)
	}
	if got := l.Calls()["c1"].Status; got != domain.CallFailed {
		t.Fatalf("status=%s want failed", got)
	}
}
