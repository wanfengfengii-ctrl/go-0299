// Package httpapi implements the versioned JSON HTTP API: transaction
// boundaries, idempotent operation handling, stable error codes with
// deterministic reason ordering, health/readiness probes and state queries. It
// is a thin adapter over the service layer: handlers decode JSON, call the
// service and map structured errors to the stable {code,reasons,operation_id}
// shape.
package httpapi

import (
	"encoding/json"
	"net/http"

	"rammed-earth-roof-beam-clearance/internal/domain"
	"rammed-earth-roof-beam-clearance/internal/rules"
	"rammed-earth-roof-beam-clearance/internal/service"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// Server exposes the versioned JSON API over the service layer.
type Server struct {
	svc *service.Service
	mux *http.ServeMux
}

// New creates an API server over the given store, wiring the service layer
// internally, and registers its routes.
func New(st store.Store) *Server {
	return NewWithService(service.New(st))
}

// NewWithService creates an API server over an already-constructed service.
func NewWithService(svc *service.Service) *Server {
	srv := &Server{svc: svc, mux: http.NewServeMux()}
	srv.routes()
	return srv
}

// Handler returns the underlying HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /readyz", s.handleReady)
	s.mux.HandleFunc("POST /v1/tasks/lock", s.handleLock)
	s.mux.HandleFunc("POST /v1/tasks/{id}/operations", s.handleOperation)
	s.mux.HandleFunc("POST /v1/tasks/{id}/leases/acquire", s.handleLeaseAcquire)
	s.mux.HandleFunc("POST /v1/tasks/{id}/leases/renew", s.handleLeaseRenew)
	s.mux.HandleFunc("POST /v1/tasks/{id}/leases/release", s.handleLeaseRelease)
	s.mux.HandleFunc("POST /v1/tasks/{id}/instruments/calls", s.handleInstrumentCall)
	s.mux.HandleFunc("POST /v1/tasks/{id}/instruments/retry", s.handleInstrumentRetry)
	s.mux.HandleFunc("GET /v1/tasks/{id}/instruments/pending", s.handleInstrumentPending)
	s.mux.HandleFunc("POST /v1/tasks/{id}/inspections", s.handleInspection)
	s.mux.HandleFunc("POST /v1/tasks/{id}/rebuilds", s.handleRebuild)
	s.mux.HandleFunc("POST /v1/tasks/{id}/reviews", s.handleReview)
	s.mux.HandleFunc("POST /v1/tasks/{id}/verdicts", s.handleVerdict)
	s.mux.HandleFunc("GET /v1/tasks/{id}/snapshot", s.handleSnapshot)
}

// handleHealth reports process liveness.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady reports whether recovery completed and the store is usable.
func (s *Server) handleReady(w http.ResponseWriter, _ *http.Request) {
	if !s.svc.Store().Ready() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"code":   string(rules.CodeRecoveryIntegrityFailed),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// decode parses a JSON request body, returning a 400 on malformed input.
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Code: "INVALID_JSON", Reasons: []string{err.Error()}})
		return false
	}
	return true
}

// respond writes a structured error or success payload.
func respond(w http.ResponseWriter, status int, v any, err error) {
	if err != nil {
		if re, ok := err.(*rules.Error); ok {
			writeError(w, errorStatus(re.Code), re)
			return
		}
		if err == store.ErrNotFound {
			writeJSON(w, http.StatusNotFound, errorBody{Code: "TASK_NOT_FOUND", Reasons: []string{"no such task"}})
			return
		}
		if err == store.ErrDuplicate {
			writeJSON(w, http.StatusConflict, errorBody{Code: "TASK_EXISTS", Reasons: []string{"task already exists"}})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorBody{Code: "INTERNAL", Reasons: []string{err.Error()}})
		return
	}
	writeJSON(w, status, v)
}

// taskID extracts the {id} path value as a domain task ID.
func taskID(r *http.Request) domain.TaskID {
	return domain.TaskID(r.PathValue("id"))
}
