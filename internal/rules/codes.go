// Package rules implements the rammed-earth construction and material rules
// catalog: the immutable design digest, integer-millimetre geometry
// validation, overflow-checked fixed-point arithmetic, and the stable error
// taxonomy shared by every rejection boundary in the system.
package rules

import "sort"

// Code is a stable, machine-readable error code. Rejections are compared and
// reported by these codes so tests and clients can assert on them exactly.
type Code string

const (
	CodeDesignDigestStale       Code = "DESIGN_DIGEST_STALE"
	CodeGenerationStale         Code = "GENERATION_STALE"
	CodeIdempotencyConflict     Code = "IDEMPOTENCY_CONFLICT"
	CodeMaterialOverclaim       Code = "MATERIAL_OVERCLAIM"
	CodeLeaseBusy               Code = "LEASE_BUSY"
	CodeFinalConflict           Code = "FINAL_CONFLICT"
	CodeRecoveryIntegrityFailed Code = "RECOVERY_INTEGRITY_FAILED"

	CodeGeometryGap        Code = "GEOMETRY_GAP"
	CodeGeometryOverlap    Code = "GEOMETRY_OVERLAP"
	CodeGeometryDegenerate Code = "GEOMETRY_DEGENERATE"
	CodeGeometryNegative   Code = "GEOMETRY_NEGATIVE"
	CodeGeometryOverflow   Code = "GEOMETRY_OVERFLOW"
	CodeLayerOutOfRange    Code = "LAYER_OUT_OF_RANGE"
	CodeForbiddenZone      Code = "FORBIDDEN_ZONE"

	CodeOutOfOrder      Code = "OUT_OF_ORDER"
	CodeWrongPan        Code = "WRONG_PAN"
	CodeMixExpired      Code = "MIX_EXPIRED"
	CodeLeaseExpired    Code = "LEASE_EXPIRED"
	CodeClockRegression Code = "CLOCK_REGRESSION"

	CodeDivideByZero  Code = "DIVIDE_BY_ZERO"
	CodeFixedOverflow Code = "FIXED_OVERFLOW"
	CodeInvalidSign   Code = "INVALID_SIGN"
)

// Error is the canonical structured rejection. All HTTP error responses use
// the JSON shape {code, reasons, operation_id} described by the spec.
type Error struct {
	Code        Code
	Reasons     []string
	OperationID string
}

func (e *Error) Error() string {
	if len(e.Reasons) == 0 {
		return string(e.Code)
	}
	return string(e.Code) + ": " + e.Reasons[0]
}

// New returns a rules error with a deterministically sorted reason set.
// Sorting is stable so identical inputs always yield byte-identical reasons.
func New(code Code, operationID string, reasons ...string) *Error {
	sort.Strings(reasons)
	return &Error{Code: code, Reasons: reasons, OperationID: operationID}
}

// Is reports whether err is a rules error carrying the given code.
func Is(err error, code Code) bool {
	re, ok := err.(*Error)
	if !ok {
		return false
	}
	return re.Code == code
}
