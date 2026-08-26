package httpapi

import (
	"net/http"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/service"
)

// handleLock locks a complete task design (interface 1).
func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	var spec domain.TaskSpec
	if !decode(w, r, &spec) {
		return
	}
	res, err := s.svc.Lock(spec)
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// handleOperation applies one unified process operation (interface 2).
func (s *Server) handleOperation(w http.ResponseWriter, r *http.Request) {
	var req service.OperationRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.svc.ApplyOperation(taskID(r), req)
	respond(w, http.StatusOK, res, err)
}

// handleLeaseAcquire acquires an exclusive lease (interface 3).
func (s *Server) handleLeaseAcquire(w http.ResponseWriter, r *http.Request) {
	var req service.LeaseRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.svc.AcquireLease(taskID(r), req)
	respond(w, http.StatusOK, res, err)
}

// handleLeaseRenew renews a lease (interface 3).
func (s *Server) handleLeaseRenew(w http.ResponseWriter, r *http.Request) {
	var req service.LeaseRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.svc.RenewLease(taskID(r), req)
	respond(w, http.StatusOK, res, err)
}

// handleLeaseRelease releases a lease (interface 3).
func (s *Server) handleLeaseRelease(w http.ResponseWriter, r *http.Request) {
	var req service.LeaseRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.svc.ReleaseLease(taskID(r), req)
	respond(w, http.StatusOK, res, err)
}

// handleInstrumentCall registers a scripted instrument request (interface 4).
func (s *Server) handleInstrumentCall(w http.ResponseWriter, r *http.Request) {
	var req service.InstrumentCallRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.svc.RegisterCall(taskID(r), req); err != nil {
		respond(w, 0, nil, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "registered"})
}

// handleInstrumentRetry records a deterministic instrument outcome (interface 4).
func (s *Server) handleInstrumentRetry(w http.ResponseWriter, r *http.Request) {
	var req service.InstrumentOutcomeRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.svc.ResolveCall(taskID(r), req)
	respond(w, http.StatusOK, res, err)
}

// handleInstrumentPending lists the pending retry queue (interface 4).
func (s *Server) handleInstrumentPending(w http.ResponseWriter, r *http.Request) {
	calls, err := s.svc.PendingCalls(taskID(r))
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pending": calls})
}

// handleInspection records inspection evidence and judges it (interface 5).
func (s *Server) handleInspection(w http.ResponseWriter, r *http.Request) {
	var req service.InspectionRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.svc.Inspect(taskID(r), req)
	respond(w, http.StatusOK, res, err)
}

// handleRebuild triggers a rebuild plan (interface 5).
func (s *Server) handleRebuild(w http.ResponseWriter, r *http.Request) {
	var req service.RebuildRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.svc.Rebuild(taskID(r), req)
	respond(w, http.StatusOK, res, err)
}

// handleReview submits an independent review (interface 6).
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var req service.ReviewRequest
	if !decode(w, r, &req) {
		return
	}
	if err := s.svc.SubmitReview(taskID(r), req); err != nil {
		respond(w, 0, nil, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reviewed"})
}

// handleVerdict competes for the terminal verdict (interface 6).
func (s *Server) handleVerdict(w http.ResponseWriter, r *http.Request) {
	var req service.VerdictRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := s.svc.SubmitVerdict(taskID(r), req)
	respond(w, http.StatusOK, res, err)
}

// handleSnapshot returns the current task state (interface 6).
func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	snap, ok, err := s.svc.Snapshot(taskID(r))
	if err != nil {
		respond(w, 0, nil, err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, errorBody{Code: "TASK_NOT_FOUND", Reasons: []string{"no such task"}})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}
