package store_test

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/store"
)

func TestModel_BoltViewKeepsTaskSnapshotTransactional(t *testing.T) {
	tests := []struct {
		name                      string
		updateViewedTask          bool
		wantUpdateEnterDuringView bool
	}{
		{name: "same task update waits for complete view", updateViewedTask: true, wantUpdateEnterDuringView: false},
		{name: "different task update remains independent", updateViewedTask: false, wantUpdateEnterDuringView: true},
	}

	type visibleState struct {
		clock       domain.LogicalTime
		findings    []domain.InspectionFinding
		events      []domain.EvidenceEvent
		balances    map[domain.ComponentKind]int64
		pans        []domain.MixPan
		calls       map[string]domain.InstrumentCall
		closedCells map[int]int
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := store.OpenBoltStore(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatalf("open bbolt store: %v", err)
			}
			defer db.Close()
			if err := db.Recover(); err != nil {
				t.Fatalf("recover bbolt store: %v", err)
			}

			viewedID := domain.TaskID("viewed")
			otherID := domain.TaskID("other")
			old := store.NewTaskState(domain.LockedTask{ID: viewedID, Clock: 10, Status: domain.TaskActive}, domain.WallGeometry{}, domain.TaskSpec{})
			old.Findings = []domain.InspectionFinding{{ID: "finding-10", At: 10}}
			old.Events = []domain.EvidenceEvent{{ID: "event-10", At: 10}}
			old.Balances = map[domain.ComponentKind]int64{domain.ComponentMix: 10}
			old.Pans = []domain.MixPan{{ID: "pan-10", MadeAt: 10}}
			old.Calls = map[string]*domain.InstrumentCall{"call": {ID: "call-10", At: 10}}
			old.Closed = map[int]int{1: 10}
			if err := db.Create(viewedID, old); err != nil {
				t.Fatalf("create viewed task: %v", err)
			}
			if err := db.Create(otherID, store.NewTaskState(domain.LockedTask{ID: otherID, Clock: 10, Status: domain.TaskActive}, domain.WallGeometry{}, domain.TaskSpec{})); err != nil {
				t.Fatalf("create other task: %v", err)
			}

			viewEntered := make(chan struct{})
			updateStarted := make(chan struct{})
			updateEntered := make(chan struct{})
			enteredDuringView := make(chan bool, 1)
			viewResult := make(chan visibleState, 1)
			viewErr := make(chan error, 1)
			go func() {
				var got visibleState
				_, err := db.View(viewedID, func(st *store.TaskState) {
					got.clock = st.Header.Clock
					close(viewEntered)
					<-updateStarted
					select {
					case <-updateEntered:
						enteredDuringView <- true
					case <-time.After(500 * time.Millisecond):
						enteredDuringView <- false
					}
					got.findings = append([]domain.InspectionFinding(nil), st.Findings...)
					got.events = append([]domain.EvidenceEvent(nil), st.Events...)
					got.balances = make(map[domain.ComponentKind]int64, len(st.Balances))
					for k, v := range st.Balances {
						got.balances[k] = v
					}
					got.pans = append([]domain.MixPan(nil), st.Pans...)
					got.calls = make(map[string]domain.InstrumentCall, len(st.Calls))
					for k, v := range st.Calls {
						got.calls[k] = *v
					}
					got.closedCells = make(map[int]int, len(st.Closed))
					for k, v := range st.Closed {
						got.closedCells[k] = v
					}
				})
				viewResult <- got
				viewErr <- err
			}()

			<-viewEntered
			targetID := otherID
			if tc.updateViewedTask {
				targetID = viewedID
			}
			updateErr := make(chan error, 1)
			go func() {
				close(updateStarted)
				updateErr <- db.Update(targetID, func(st *store.TaskState) error {
					st.Header.Clock = 20
					st.Findings = []domain.InspectionFinding{{ID: "finding-20", At: 20}}
					st.Events = []domain.EvidenceEvent{{ID: "event-20", At: 20}}
					st.Balances = map[domain.ComponentKind]int64{domain.ComponentMix: 20}
					st.Pans = []domain.MixPan{{ID: "pan-20", MadeAt: 20}}
					st.Calls = map[string]*domain.InstrumentCall{"call": {ID: "call-20", At: 20}}
					st.Closed = map[int]int{1: 20}
					close(updateEntered)
					return nil
				})
			}()

			got := <-viewResult
			if err := <-viewErr; err != nil {
				t.Fatalf("view: %v", err)
			}
			if err := <-updateErr; err != nil {
				t.Fatalf("update: %v", err)
			}

			if entered := <-enteredDuringView; entered != tc.wantUpdateEnterDuringView {
				t.Fatalf("update entered during View = %v, want %v", entered, tc.wantUpdateEnterDuringView)
			}
			want := visibleState{
				clock:       10,
				findings:    []domain.InspectionFinding{{ID: "finding-10", At: 10}},
				events:      []domain.EvidenceEvent{{ID: "event-10", At: 10}},
				balances:    map[domain.ComponentKind]int64{domain.ComponentMix: 10},
				pans:        []domain.MixPan{{ID: "pan-10", MadeAt: 10}},
				calls:       map[string]domain.InstrumentCall{"call": {ID: "call-10", At: 10}},
				closedCells: map[int]int{1: 10},
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("View combined fields from different committed task states:\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}
