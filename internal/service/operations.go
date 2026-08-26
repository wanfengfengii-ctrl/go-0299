package service

import (
	"encoding/json"
	"strconv"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// OperationRequest is the unified operation payload accepted by the operations
// endpoint (interface 2). The Kind discriminator selects the process step;
// only the fields relevant to that step are read.
type OperationRequest struct {
	OperationID domain.OperationID `json:"operation_id"`
	Digest      domain.Digest      `json:"digest"`
	Generation  domain.Generation  `json:"generation"`
	At          domain.LogicalTime `json:"at"`
	Kind        domain.ProcessKind `json:"kind"`
	Layer       int                `json:"layer,omitempty"`
	Seq         int                `json:"seq,omitempty"`
	Pass        int                `json:"pass,omitempty"`
	PanSeq      int                `json:"pan_seq,omitempty"`
	Actor       string             `json:"actor,omitempty"`
	AmountG     int64              `json:"amount_g,omitempty"`
	ValueFixed  int64              `json:"value_fixed,omitempty"`
	Recipe      *domain.Recipe     `json:"recipe,omitempty"`
}

// OperationResult is the deterministic result of an applied operation.
type OperationResult struct {
	Generation domain.Generation  `json:"generation"`
	Clock      domain.LogicalTime `json:"clock"`
	PanSeq     int                `json:"pan_seq,omitempty"`
	Applied    bool               `json:"applied"`
}

// ApplyOperation applies one process step inside a single atomic transaction.
// It enforces digest/generation/logical-time checks, then the step-specific
// dependency, material and lease rules (domain rules 1, 3, 4, 5).
func (s *Service) ApplyOperation(id domain.TaskID, req OperationRequest) (*OperationResult, error) {
	scope := taskScope(id)
	reqDigest := digestOf(req)

	replay, hit, err := s.idemBegin(scope, req.OperationID, reqDigest)
	if err != nil {
		return nil, err
	}
	if hit {
		var out OperationResult
		if err := json.Unmarshal(replay, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	result := &OperationResult{Applied: true}
	err = s.store.Update(id, func(st *store.TaskState) error {
		b := hydrate(st)
		opID := req.OperationID
		if err := b.task.CheckDigest(req.Digest, opID); err != nil {
			return err
		}
		if err := b.task.CheckGeneration(req.Generation, opID); err != nil {
			return err
		}
		if err := b.task.AdvanceClock(req.At, opID); err != nil {
			return err
		}

		switch req.Kind {
		case domain.ProcessSieving:
			err = s.applySieving(st, b, req, opID)
		case domain.ProcessConditioning:
			err = s.applyConditioning(st, b, req, opID)
		case domain.ProcessMixing:
			err = s.applyMixing(st, b, req, opID)
		case domain.ProcessFormAccept:
			err = s.applyFormAccept(st, b, req, opID)
		case domain.ProcessPlacing:
			err = s.applyPlacing(st, b, req, opID)
		case domain.ProcessCompaction:
			err = s.applyCompaction(st, b, req, opID)
		case domain.ProcessLevelling:
			err = s.applyLevelling(st, b, req, opID)
		case domain.ProcessTieEmbed:
			err = s.applyTieEmbed(st, b, req, opID)
		case domain.ProcessCuring:
			err = s.applyCuring(st, b, req, opID)
		default:
			err = rules.New(rules.CodeInvalidSign, string(opID), "unknown process kind")
		}
		if err != nil {
			return err
		}

		dehydrate(st, b)
		result.Generation = st.Header.Generation
		result.Clock = st.Header.Clock
		result.PanSeq = st.PanSeq
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.idemCommit(scope, req.OperationID, reqDigest, result, 200); err != nil {
		return nil, err
	}
	return result, nil
}

// recordEvent appends an immutable evidence event to the recorder.
func recordEvent(b *behaviours, ev domain.EvidenceEvent, opID domain.OperationID) error {
	return b.recorder.Record(ev, opID)
}

// applySieving removes sieving rejects from raw earth into waste (balanced:
// debit waste, credit raw earth).
func (s *Service) applySieving(st *store.TaskState, b *behaviours, req OperationRequest, opID domain.OperationID) error {
	if req.AmountG <= 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "sieving amount must be positive")
	}
	if err := b.ledger.PostTransaction([]domain.MassLedgerEntry{
		{Account: domain.ComponentWaste, Side: domain.SideDebit, AmountG: req.AmountG, Reason: "sieving rejects", At: req.At},
		{Account: domain.ComponentRawEarth, Side: domain.SideCredit, AmountG: req.AmountG, Reason: "sieving rejects", At: req.At},
	}, opID); err != nil {
		return err
	}
	return recordEvent(b, domain.EvidenceEvent{Wall: string(st.Header.ID), Process: domain.ProcessSieving, At: req.At, Actor: req.Actor, ValueFixed: req.AmountG, Digest: st.Header.Digest, Valid: true}, opID)
}

// applyConditioning adds water to an open mix pan to adjust moisture (balanced:
// debit mix, credit water).
func (s *Service) applyConditioning(st *store.TaskState, b *behaviours, req OperationRequest, opID domain.OperationID) error {
	if req.AmountG <= 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "conditioning water amount must be positive")
	}
	if req.PanSeq <= 0 || req.PanSeq != b.recorder.PanSequence() {
		return rules.New(rules.CodeWrongPan, string(opID), "conditioning requires the current open mix pan")
	}
	if err := b.ledger.PostTransaction([]domain.MassLedgerEntry{
		{Account: domain.ComponentMix, Side: domain.SideDebit, AmountG: req.AmountG, PanSeq: req.PanSeq, Reason: "moisture conditioning", At: req.At},
		{Account: domain.ComponentWater, Side: domain.SideCredit, AmountG: req.AmountG, PanSeq: req.PanSeq, Reason: "moisture conditioning", At: req.At},
	}, opID); err != nil {
		return err
	}
	return recordEvent(b, domain.EvidenceEvent{Wall: string(st.Header.ID), Process: domain.ProcessConditioning, PanSeq: req.PanSeq, At: req.At, Actor: req.Actor, ValueFixed: req.AmountG, Digest: st.Header.Digest, Valid: true}, opID)
}

// applyMixing consumes the recipe into a new non-mergeable mix pan and acquires
// the mixer and moisture-meter leases atomically (domain rule 4 and 5).
func (s *Service) applyMixing(st *store.TaskState, b *behaviours, req OperationRequest, opID domain.OperationID) error {
	if req.Recipe == nil {
		return rules.New(rules.CodeInvalidSign, string(opID), "mixing requires a recipe")
	}
	total := req.Recipe.RawEarthG + req.Recipe.GravelG + req.Recipe.StabilizerG + req.Recipe.WaterG
	if total <= 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "recipe total must be positive")
	}
	panSeq := st.PanSeq + 1
	expiresAt := req.At + domain.LogicalTime(st.Spec.MixPlan.UsableUnits)
	if err := b.recorder.OpenPan(panSeq, req.At, expiresAt, opID); err != nil {
		return err
	}
	entries := []domain.MassLedgerEntry{
		{Account: domain.ComponentMix, Side: domain.SideDebit, AmountG: total, PanSeq: panSeq, Gen: req.Generation, Reason: "mix pan", At: req.At},
		{Account: domain.ComponentRawEarth, Side: domain.SideCredit, AmountG: req.Recipe.RawEarthG, PanSeq: panSeq, Gen: req.Generation, Reason: "mix pan", At: req.At},
		{Account: domain.ComponentGravel, Side: domain.SideCredit, AmountG: req.Recipe.GravelG, PanSeq: panSeq, Gen: req.Generation, Reason: "mix pan", At: req.At},
		{Account: domain.ComponentStabilizer, Side: domain.SideCredit, AmountG: req.Recipe.StabilizerG, PanSeq: panSeq, Gen: req.Generation, Reason: "mix pan", At: req.At},
		{Account: domain.ComponentWater, Side: domain.SideCredit, AmountG: req.Recipe.WaterG, PanSeq: panSeq, Gen: req.Generation, Reason: "mix pan", At: req.At},
	}
	if err := b.ledger.PostTransaction(entries, opID); err != nil {
		return err
	}
	// Acquire mixer and moisture-meter leases in the same transaction.
	token := domain.LeaseToken(digestOf(req.OperationID))
	if err := b.leases.Acquire(domain.ResourceLease{
		Kind: domain.ResourceMixer, ResourceID: "mixer", HolderOp: opID, Token: token,
		AcquiredAt: req.At, ExpiresAt: expiresAt,
	}, opID); err != nil {
		return err
	}
	_ = b.leases.Acquire(domain.ResourceLease{
		Kind: domain.ResourceMoistureMeter, ResourceID: "moisture_meter", HolderOp: opID, Token: token,
		AcquiredAt: req.At, ExpiresAt: expiresAt,
	}, opID)
	st.Pans = append(st.Pans, domain.MixPan{
		ID:             "pan-" + strconv.Itoa(panSeq),
		PanSeq:         panSeq,
		MaterialGen:    req.Generation,
		MadeAt:         req.At,
		ExpiresAt:      expiresAt,
		Recipe:         *req.Recipe,
		TargetMoisture: st.Spec.TargetMoisture,
	})
	return recordEvent(b, domain.EvidenceEvent{Wall: string(st.Header.ID), Process: domain.ProcessMixing, PanSeq: panSeq, At: req.At, Actor: req.Actor, Digest: st.Header.Digest, Valid: true}, opID)
}

// applyFormAccept records formwork acceptance for a layer (evidence only).
func (s *Service) applyFormAccept(st *store.TaskState, b *behaviours, req OperationRequest, opID domain.OperationID) error {
	if req.Layer <= 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "form acceptance requires a layer")
	}
	return recordEvent(b, domain.EvidenceEvent{Wall: string(st.Header.ID), Layer: req.Layer, Process: domain.ProcessFormAccept, At: req.At, Actor: req.Actor, Digest: st.Header.Digest, Valid: true}, opID)
}

// applyPlacing opens a compaction cell (continuous prefix) and moves mix
// material into the wall (balanced: debit in_wall, credit mix).
func (s *Service) applyPlacing(st *store.TaskState, b *behaviours, req OperationRequest, opID domain.OperationID) error {
	if req.Layer <= 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "placing requires a layer")
	}
	if req.Seq < 0 {
		return rules.New(rules.CodeOutOfOrder, string(opID), "cell sequence must be non-negative")
	}
	// Cross-layer closure: before opening the first cell of a higher layer,
	// the previous layer must be fully closed.
	if req.Seq == 0 && req.Layer > 1 {
		if err := s.checkPreviousLayerClosed(st, b, req.Layer-1, opID); err != nil {
			return err
		}
	}
	if req.PanSeq <= 0 || req.PanSeq != b.recorder.PanSequence() {
		return rules.New(rules.CodeWrongPan, string(opID), "placing requires the current open mix pan")
	}
	if req.AmountG <= 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "placing amount must be positive")
	}
	if err := b.task.AppendCell(req.Layer, req.Seq, opID); err != nil {
		return err
	}
	if err := b.ledger.PostTransaction([]domain.MassLedgerEntry{
		{Account: domain.ComponentInWall, Side: domain.SideDebit, AmountG: req.AmountG, PanSeq: req.PanSeq, Gen: req.Generation, Reason: "placing", At: req.At},
		{Account: domain.ComponentMix, Side: domain.SideCredit, AmountG: req.AmountG, PanSeq: req.PanSeq, Gen: req.Generation, Reason: "placing", At: req.At},
	}, opID); err != nil {
		return err
	}
	return recordEvent(b, domain.EvidenceEvent{Wall: string(st.Header.ID), Layer: req.Layer, Seq: req.Seq, Process: domain.ProcessPlacing, PanSeq: req.PanSeq, At: req.At, Actor: req.Actor, Digest: st.Header.Digest, Valid: true}, opID)
}

// applyCompaction records a compaction pass. Passes within a cell must form a
// continuous prefix (domain rule 3).
func (s *Service) applyCompaction(st *store.TaskState, b *behaviours, req OperationRequest, opID domain.OperationID) error {
	if req.Layer <= 0 || req.Seq < 0 {
		return rules.New(rules.CodeOutOfOrder, string(opID), "compaction requires a valid cell")
	}
	highest := highestPass(b.recorder.Events(), req.Layer, req.Seq)
	if req.Pass != highest+1 {
		return rules.New(rules.CodeOutOfOrder, string(opID), "compaction pass must form a continuous prefix")
	}
	return recordEvent(b, domain.EvidenceEvent{Wall: string(st.Header.ID), Layer: req.Layer, Seq: req.Seq, Pass: req.Pass, Process: domain.ProcessCompaction, PanSeq: req.PanSeq, At: req.At, Actor: req.Actor, Digest: st.Header.Digest, Valid: true}, opID)
}

// applyLevelling records the level re-measurement for a layer.
func (s *Service) applyLevelling(st *store.TaskState, b *behaviours, req OperationRequest, opID domain.OperationID) error {
	if req.Layer <= 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "levelling requires a layer")
	}
	return recordEvent(b, domain.EvidenceEvent{Wall: string(st.Header.ID), Layer: req.Layer, Process: domain.ProcessLevelling, At: req.At, Actor: req.Actor, ValueFixed: req.ValueFixed, Digest: st.Header.Digest, Valid: true}, opID)
}

// applyTieEmbed records tie-bar embedding evidence.
func (s *Service) applyTieEmbed(st *store.TaskState, b *behaviours, req OperationRequest, opID domain.OperationID) error {
	if req.Layer <= 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "tie embed requires a layer")
	}
	return recordEvent(b, domain.EvidenceEvent{Wall: string(st.Header.ID), Layer: req.Layer, Process: domain.ProcessTieEmbed, At: req.At, Actor: req.Actor, Digest: st.Header.Digest, Valid: true}, opID)
}

// applyCuring records a humidity-equilibration curing event.
func (s *Service) applyCuring(st *store.TaskState, b *behaviours, req OperationRequest, opID domain.OperationID) error {
	if req.Layer <= 0 {
		return rules.New(rules.CodeInvalidSign, string(opID), "curing requires a layer")
	}
	return recordEvent(b, domain.EvidenceEvent{Wall: string(st.Header.ID), Layer: req.Layer, Process: domain.ProcessCuring, At: req.At, Actor: req.Actor, Digest: st.Header.Digest, Valid: true}, opID)
}

// checkPreviousLayerClosed verifies that the layer below is fully closed before
// the layer above may open its first cell (domain rule 3).
func (s *Service) checkPreviousLayerClosed(st *store.TaskState, b *behaviours, layer int, opID domain.OperationID) error {
	cellsIn := countCellsInLayer(st.Geometry.Cells, layer)
	if b.task.Closed()[layer] != cellsIn {
		return rules.New(rules.CodeOutOfOrder, string(opID), "previous layer has unclosed cells")
	}
	for _, c := range cellsOfLayer(st.Geometry.Cells, layer) {
		if highestPass(b.recorder.Events(), layer, c.Seq)+1 < st.Spec.Program.PassesPerCell {
			return rules.New(rules.CodeOutOfOrder, string(opID), "previous layer has incomplete compaction passes")
		}
	}
	if !hasProcess(b.recorder.Events(), layer, domain.ProcessLevelling) {
		return rules.New(rules.CodeOutOfOrder, string(opID), "previous layer missing levelling evidence")
	}
	return nil
}
