// Package httpapi implements the versioned JSON HTTP API: transaction
// boundaries, idempotent operation handling, stable error codes with
// deterministic reason ordering, health/readiness probes and state queries.
package httpapi

import (
	"encoding/json"
	"net/http"

	"rammed-earth-roof-beam-clearance/internal/rules"
)

// errorBody is the canonical JSON error shape {code, reasons, operation_id}
// required by the spec.
type errorBody struct {
	Code        string   `json:"code"`
	Reasons     []string `json:"reasons"`
	OperationID string   `json:"operation_id"`
}

// writeJSON writes a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a stable structured error response.
func writeError(w http.ResponseWriter, status int, err *rules.Error) {
	writeJSON(w, status, errorBody{
		Code:        string(err.Code),
		Reasons:     err.Reasons,
		OperationID: err.OperationID,
	})
}

// errorStatus maps a stable error code to an HTTP status code.
func errorStatus(code rules.Code) int {
	switch code {
	case rules.CodeDesignDigestStale, rules.CodeGenerationStale, rules.CodeIdempotencyConflict,
		rules.CodeMaterialOverclaim, rules.CodeLeaseBusy, rules.CodeFinalConflict,
		rules.CodeLeaseExpired, rules.CodeMixExpired, rules.CodeClockRegression:
		return http.StatusConflict
	case rules.CodeOutOfOrder, rules.CodeWrongPan, rules.CodeInvalidSign,
		rules.CodeDivideByZero, rules.CodeFixedOverflow:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusBadRequest
	}
}
