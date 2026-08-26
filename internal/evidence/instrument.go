package evidence

import (
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

// InstrumentLog manages the persistent, deterministic instrument-call retry
// queue. Failures never produce readings or advance state; they only enqueue a
// bounded retry with a fixed fault code (failure boundary 5).
type InstrumentLog struct {
	calls map[string]*domain.InstrumentCall
}

// NewInstrumentLog creates an empty instrument-call log.
func NewInstrumentLog() *InstrumentLog {
	return &InstrumentLog{calls: make(map[string]*domain.InstrumentCall)}
}

// NewInstrumentLogFrom reconstructs an instrument log from persisted calls.
func NewInstrumentLogFrom(calls map[string]*domain.InstrumentCall) *InstrumentLog {
	l := NewInstrumentLog()
	for k, v := range calls {
		cp := *v
		l.calls[k] = &cp
	}
	return l
}

// Enqueue registers a pending scripted instrument call.
func (l *InstrumentLog) Enqueue(call domain.InstrumentCall) {
	cp := call
	cp.Status = domain.CallPending
	cp.Attempts = 0
	l.calls[call.ID] = &cp
}

// Fail records a non-successful outcome: it increments the attempt counter,
// stores a stable fault code and computes the next allowed retry time. It does
// not advance any aggregate state or produce a reading.
func (l *InstrumentLog) Fail(id string, faultCode string, now domain.LogicalTime, retryDelay domain.LogicalTime, opID domain.OperationID) error {
	c, ok := l.calls[id]
	if !ok {
		return rules.New(rules.CodeOutOfOrder, string(opID), "unknown instrument call")
	}
	if c.Status == domain.CallSucceeded {
		return rules.New(rules.CodeOutOfOrder, string(opID), "call already succeeded")
	}
	c.Attempts++
	c.FaultCode = faultCode
	if c.Attempts >= c.MaxAttempts {
		c.Status = domain.CallFailed
		return nil
	}
	c.Status = domain.CallPending
	c.NextRetryAt = now + retryDelay
	return nil
}

// Succeed records a structured successful receipt, terminating retries.
func (l *InstrumentLog) Succeed(id string, response domain.Digest, opID domain.OperationID) error {
	c, ok := l.calls[id]
	if !ok {
		return rules.New(rules.CodeOutOfOrder, string(opID), "unknown instrument call")
	}
	c.Status = domain.CallSucceeded
	c.RawResponse = response
	return nil
}

// Pending returns the pending retry queue in deterministic insertion order.
func (l *InstrumentLog) Pending(now domain.LogicalTime) []domain.InstrumentCall {
	var out []domain.InstrumentCall
	for _, c := range l.calls {
		if c.Status == domain.CallPending && c.NextRetryAt <= now {
			out = append(out, *c)
		}
	}
	return out
}

// Calls returns the full call log keyed by call ID.
func (l *InstrumentLog) Calls() map[string]*domain.InstrumentCall { return l.calls }
