package service

import (
	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// Snapshot is a deterministic, read-only view of a task's full state returned
// by the snapshot endpoint (interface 6).
type Snapshot struct {
	TaskID      domain.TaskID                  `json:"task_id"`
	Area        string                         `json:"area"`
	Digest      domain.Digest                  `json:"digest"`
	Generation  domain.Generation              `json:"generation"`
	Direction   domain.Direction               `json:"direction"`
	Clock       domain.LogicalTime             `json:"clock"`
	Status      domain.TaskStatus              `json:"status"`
	Balances    map[domain.ComponentKind]int64 `json:"balances"`
	Pans        []domain.MixPan                `json:"pans"`
	Events      []domain.EvidenceEvent         `json:"events"`
	Calls       []domain.InstrumentCall        `json:"calls"`
	Findings    []domain.InspectionFinding     `json:"findings"`
	Rebuilds    []domain.RebuildPlan           `json:"rebuilds"`
	Reviews     []domain.Review                `json:"reviews"`
	Verdict     *domain.FinalVerdict           `json:"verdict,omitempty"`
	ClosedCells map[int]int                    `json:"closed_cells"`
}

// Snapshot returns the current task state. It returns ok=false when the task
// does not exist.
func (s *Service) Snapshot(id domain.TaskID) (*Snapshot, bool, error) {
	var out *Snapshot
	ok, err := s.store.View(id, func(st *store.TaskState) {
		snap := &Snapshot{
			TaskID:      st.Header.ID,
			Area:        st.Header.Area,
			Digest:      st.Header.Digest,
			Generation:  st.Header.Generation,
			Direction:   st.Header.Direction,
			Clock:       st.Header.Clock,
			Status:      st.Header.Status,
			Balances:    cloneBalances(st.Balances),
			Pans:        append([]domain.MixPan(nil), st.Pans...),
			Events:      append([]domain.EvidenceEvent(nil), st.Events...),
			Findings:    append([]domain.InspectionFinding(nil), st.Findings...),
			Rebuilds:    append([]domain.RebuildPlan(nil), st.Rebuilds...),
			Reviews:     append([]domain.Review(nil), st.Reviews...),
			ClosedCells: cloneClosed(st.Closed),
		}
		if st.Verdict != nil {
			v := *st.Verdict
			snap.Verdict = &v
		}
		calls := make([]domain.InstrumentCall, 0, len(st.Calls))
		for _, c := range st.Calls {
			calls = append(calls, *c)
		}
		snap.Calls = calls
		out = snap
	})
	return out, ok, err
}

func cloneBalances(m map[domain.ComponentKind]int64) map[domain.ComponentKind]int64 {
	out := make(map[domain.ComponentKind]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneClosed(m map[int]int) map[int]int {
	out := make(map[int]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
