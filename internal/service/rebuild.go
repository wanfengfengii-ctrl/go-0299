package service

import (
	"encoding/json"
	"sort"
	"strconv"

	"rammed-earth-roof-beam-clearance/internal/arbiter"
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// RebuildRequest triggers a rebuild from a recorded finding: it computes the
// unique rebuild set from the locked adjacency and same-pan relationships,
// isolates the affected set, opens a new generation and records the plan
// (interface 5, acceptance 6).
type RebuildRequest struct {
	OperationID domain.OperationID `json:"operation_id"`
	Digest      domain.Digest      `json:"digest"`
	Generation  domain.Generation  `json:"generation"`
	At          domain.LogicalTime `json:"at"`
	FindingID   string             `json:"finding_id"`
	Reason      string             `json:"reason"`
}

// RebuildResult is the deterministic rebuild plan outcome.
type RebuildResult struct {
	PlanID string            `json:"plan_id"`
	Set    []domain.CellRef  `json:"set"`
	OldGen domain.Generation `json:"old_generation"`
	NewGen domain.Generation `json:"new_generation"`
}

// Rebuild computes and records the unique rebuild plan and opens a new
// generation, isolating the old generation's receipts.
func (s *Service) Rebuild(id domain.TaskID, req RebuildRequest) (*RebuildResult, error) {
	scope := taskScope(id)
	reqDigest := digestOf(req)
	replay, hit, err := s.idemBegin(scope, req.OperationID, reqDigest)
	if err != nil {
		return nil, err
	}
	if hit {
		var out RebuildResult
		if err := json.Unmarshal(replay, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	result := &RebuildResult{}
	err = s.store.Update(id, func(st *store.TaskState) error {
		b := hydrate(st)
		if err := b.task.CheckDigest(req.Digest, req.OperationID); err != nil {
			return err
		}
		if err := b.task.CheckGeneration(req.Generation, req.OperationID); err != nil {
			return err
		}
		if err := b.task.AdvanceClock(req.At, req.OperationID); err != nil {
			return err
		}

		finding, ok := findFinding(st, req.FindingID)
		if !ok {
			return rules.New(rules.CodeOutOfOrder, string(req.OperationID), "unknown finding")
		}
		origin := domain.CellRef{Layer: finding.Layer, Seq: finding.Seq}
		panOf := panMap(st)
		set := arbiter.RebuildSet(origin, st.Spec.Geometry.PressureAdj, panOf)

		oldGen := st.Header.Generation
		if err := b.task.OpenGeneration(req.OperationID); err != nil {
			return err
		}
		newGen := b.task.Header().Generation

		plan := domain.RebuildPlan{
			ID:           "plan-" + strconv.Itoa(len(st.Rebuilds)+1),
			Wall:         string(st.Header.ID),
			SetDigest:    setDigest(set),
			OldGen:       oldGen,
			NewGen:       newGen,
			MaterialPath: req.Reason,
			Isolation:    "isolated",
			Complete:     false,
		}
		st.Rebuilds = append(st.Rebuilds, plan)

		result.PlanID = plan.ID
		result.Set = set
		result.OldGen = oldGen
		result.NewGen = newGen

		dehydrate(st, b)
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

// findFinding locates a finding by ID.
func findFinding(st *store.TaskState, id string) (domain.InspectionFinding, bool) {
	for _, f := range st.Findings {
		if f.ID == id {
			return f, true
		}
	}
	return domain.InspectionFinding{}, false
}

// panMap builds the cell -> pan sequence mapping from placing evidence.
func panMap(st *store.TaskState) map[domain.CellRef]int {
	out := make(map[domain.CellRef]int)
	for _, e := range st.Events {
		if e.Process == domain.ProcessPlacing {
			out[domain.CellRef{Layer: e.Layer, Seq: e.Seq}] = e.PanSeq
		}
	}
	return out
}

// setDigest hashes the deterministically-sorted rebuild set.
func setDigest(set []domain.CellRef) domain.Digest {
	refs := append([]domain.CellRef(nil), set...)
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Layer != refs[j].Layer {
			return refs[i].Layer < refs[j].Layer
		}
		return refs[i].Seq < refs[j].Seq
	})
	data, _ := json.Marshal(refs)
	return domain.Digest(rules.Hash(data))
}
