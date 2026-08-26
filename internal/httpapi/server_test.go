package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rammed-earth-roof-beam-clearance/internal/service"
	"rammed-earth-roof-beam-clearance/internal/store"
)

// validLockBody returns a minimal but complete, valid lock payload.
func validLockBody() string {
	return `{
		"area":"building-a","direction":"rising",
		"geometry":{
			"wall":{"x":0,"y":0,"w":1000,"h":1000},
			"layers":[{"number":1,"rect":{"x":0,"y":0,"w":1000,"h":1000}}],
			"cells":[{"layer":1,"seq":0,"rect":{"x":0,"y":0,"w":1000,"h":1000}}]
		},
		"batches":[{"id":"b1","component":"raw_earth","source":"pit-1","balance_g":100000}],
		"recipe":{"raw_earth_g":900,"gravel_g":0,"stabilizer_g":0,"water_g":100},
		"target_moisture":120,
		"program":{"loose_thickness":100,"passes_per_cell":2,"blows_per_pass":10,"rammer_weight_g":10000,"fall_height_mm":500},
		"thresholds":{"min_dry_density":1800000,"max_dry_density":2000000,"min_compaction":950,"min_moisture":80,"max_moisture":150,"min_shear":1000,"max_erosion":50,"max_deviation":5},
		"curing":{"hours_per_layer":24,"min_hours":72},
		"mix_plan":{"pan_count":1,"pan_size_g":1000,"usable_units":100}
	}`
}

func TestHealthz(t *testing.T) {
	srv := New(store.NewMemoryStore())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	srv := New(store.NewMemoryStore())
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
}

func TestLockTask(t *testing.T) {
	srv := New(store.NewMemoryStore())
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/lock", strings.NewReader(validLockBody()))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d want 201 body=%s", rec.Code, rec.Body.String())
	}
	var resp service.LockResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TaskID == "" || resp.Digest == "" || resp.Generation != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestLockInvalidDirection(t *testing.T) {
	srv := New(store.NewMemoryStore())
	body := strings.Replace(validLockBody(), `"rising"`, `"sideways"`, 1)
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/lock", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d want 422", rec.Code)
	}
	var e errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if e.Code == "" {
		t.Fatalf("want stable error code, got %+v", e)
	}
}

func TestLockInvalidGeometry(t *testing.T) {
	srv := New(store.NewMemoryStore())
	body := strings.Replace(validLockBody(), `"w":1000,"h":1000}`, `"w":-10,"h":1000}`, 1)
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/lock", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusCreated {
		t.Fatalf("status=%d want rejection body=%s", rec.Code, rec.Body.String())
	}
}

func TestSnapshotNotFound(t *testing.T) {
	srv := New(store.NewMemoryStore())
	req := httptest.NewRequest(http.MethodGet, "/v1/tasks/missing/snapshot", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	srv := New(store.NewMemoryStore())
	req := httptest.NewRequest(http.MethodPost, "/v1/tasks/lock", strings.NewReader(validLockBody()))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	var created service.LockResult
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/tasks/"+string(created.TaskID)+"/snapshot", nil)
	req2.SetPathValue("id", string(created.TaskID))
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec2.Code)
	}
	var snap service.Snapshot
	if err := json.Unmarshal(rec2.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Digest != created.Digest {
		t.Fatalf("digest mismatch: %s != %s", snap.Digest, created.Digest)
	}
	if snap.Balances["raw_earth"] != 100000 {
		t.Fatalf("raw_earth balance=%d want 100000", snap.Balances["raw_earth"])
	}
}
