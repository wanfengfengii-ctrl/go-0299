package store

import "rammed-earth-roof-beam-clearance/internal/domain"

// TaskState is the persisted, per-task aggregate state. It is pure data — no
// behaviour — so it round-trips through the embedded database as JSON. The
// service layer reconstructs the behaviour objects (ledger, leases, recorder,
// reviews, verdict barrier) from this state inside a single store.Update
// transaction and writes the results back before commit.
type TaskState struct {
	Header   domain.LockedTask   `json:"header"`
	Geometry domain.WallGeometry `json:"geometry"`
	// Spec is the immutable design submitted at lock time. It is fixed by the
	// digest but persisted so the service can validate runtime dependencies
	// (pass counts, usable windows, thresholds) without client resubmission.
	Spec domain.TaskSpec `json:"spec"`

	Balances map[domain.ComponentKind]int64 `json:"balances"`

	Leases map[string]*domain.ResourceLease `json:"leases"`

	Pans []domain.MixPan `json:"pans"`

	Events []domain.EvidenceEvent `json:"events"`

	Calls map[string]*domain.InstrumentCall `json:"calls"`

	Closed map[int]int `json:"closed"`

	PanSeq     int                `json:"pan_seq"`
	PanExpires domain.LogicalTime `json:"pan_expires"`

	Findings []domain.InspectionFinding `json:"findings"`
	Rebuilds []domain.RebuildPlan       `json:"rebuilds"`
	Reviews  []domain.Review            `json:"reviews"`
	Verdict  *domain.FinalVerdict       `json:"verdict"`
}

// NewTaskState constructs an empty task state seeded with the locked header,
// geometry and immutable spec. All collections are initialised so the service
// can mutate them directly without nil-map panics.
func NewTaskState(h domain.LockedTask, g domain.WallGeometry, spec domain.TaskSpec) *TaskState {
	return &TaskState{
		Header:   h,
		Geometry: g,
		Spec:     spec,
		Balances: make(map[domain.ComponentKind]int64),
		Leases:   make(map[string]*domain.ResourceLease),
		Calls:    make(map[string]*domain.InstrumentCall),
		Closed:   make(map[int]int),
	}
}

// ensureMaps guards against nil maps after a JSON round-trip so the service
// never mutates a nil map.
func (st *TaskState) ensureMaps() {
	if st.Balances == nil {
		st.Balances = make(map[domain.ComponentKind]int64)
	}
	if st.Leases == nil {
		st.Leases = make(map[string]*domain.ResourceLease)
	}
	if st.Calls == nil {
		st.Calls = make(map[string]*domain.InstrumentCall)
	}
	if st.Closed == nil {
		st.Closed = make(map[int]int)
	}
}
