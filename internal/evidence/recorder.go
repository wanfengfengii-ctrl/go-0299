// Package evidence implements the mixing, compaction and curing evidence
// recorder: the immutable process-event log, mixed-material generation and
// usable-deadline management, and the persistent instrument-call retry state.
package evidence

import (
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
)

// Recorder holds the immutable evidence event log for a task and enforces the
// mixed-material usable-deadline and pan-sequencing rules.
type Recorder struct {
	events []domain.EvidenceEvent
	// panSeq tracks the highest mix-pan sequence seen, so each pan is used in
	// order and never merged or reused.
	panSeq int
	// expiresAt records the latest usable deadline among open pans.
	expiresAt domain.LogicalTime
}

// NewRecorder creates an empty evidence recorder.
func NewRecorder() *Recorder { return &Recorder{} }

// NewRecorderFrom reconstructs a recorder from persisted events and pan state.
func NewRecorderFrom(events []domain.EvidenceEvent, panSeq int, expiresAt domain.LogicalTime) *Recorder {
	return &Recorder{events: events, panSeq: panSeq, expiresAt: expiresAt}
}

// Events returns the recorded evidence events in insertion order.
func (r *Recorder) Events() []domain.EvidenceEvent { return r.events }

// Snapshot returns the recorder's persisted state: events, open pan sequence
// and the usable deadline.
func (r *Recorder) Snapshot() ([]domain.EvidenceEvent, int, domain.LogicalTime) {
	return r.events, r.panSeq, r.expiresAt
}

// Record appends an immutable evidence event after validating that a mix pan,
// when referenced, is not stale and not reused out of order (domain rule 4).
func (r *Recorder) Record(ev domain.EvidenceEvent, opID domain.OperationID) error {
	if ev.PanSeq > 0 {
		if ev.PanSeq != r.panSeq {
			return rules.New(rules.CodeWrongPan, string(opID), "mix pan sequence out of order")
		}
		if r.expiresAt > 0 && ev.At > r.expiresAt {
			return rules.New(rules.CodeMixExpired, string(opID), "mixed material exceeded usable deadline")
		}
	}
	r.events = append(r.events, ev)
	return nil
}

// OpenPan registers a new mix pan with its usable deadline and material
// generation. Pans cannot be merged: each new pan advances the sequence.
func (r *Recorder) OpenPan(panSeq int, madeAt, expiresAt domain.LogicalTime, opID domain.OperationID) error {
	if panSeq <= r.panSeq {
		return rules.New(rules.CodeWrongPan, string(opID), "mix pan sequence must increase")
	}
	if expiresAt <= madeAt {
		return rules.New(rules.CodeMixExpired, string(opID), "usable deadline must follow make time")
	}
	r.panSeq = panSeq
	r.expiresAt = expiresAt
	return nil
}

// PanSequence returns the current open pan sequence.
func (r *Recorder) PanSequence() int { return r.panSeq }
