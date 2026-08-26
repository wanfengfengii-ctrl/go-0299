#!/usr/bin/env bash
# Smoke test for the rammed-earth roof-beam clearance backend. It builds the
# binary, starts the service against a temporary embedded database, probes its
# health and readiness endpoints, locks a task over the public JSON API and
# reads the resulting snapshot. It cleans up every process and temporary file
# and finishes deterministically without external network access.
set -euo pipefail

BIN="$(mktemp /tmp/rammed-earth-server-XXXXXX)"
DB_PATH="$(mktemp -u /tmp/rammed-earth-smoke-XXXXXX.db)"
LOG_FILE="$(mktemp /tmp/rammed-earth-smoke-XXXXXX.log)"
PORT="${SMOKE_PORT:-18080}"
BASE="http://127.0.0.1:${PORT}"
SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -f "$BIN" "$DB_PATH" "$LOG_FILE"
}
trap cleanup EXIT

echo "building server"
go build -o "$BIN" ./cmd/server

echo "starting server on :${PORT}"
DB_PATH="$DB_PATH" ADDR=":${PORT}" "$BIN" >"$LOG_FILE" 2>&1 &
SERVER_PID=$!

# Wait for the health endpoint to come up.
ready=0
for _ in $(seq 1 100); do
  if curl -sf "$BASE/healthz" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.05
done
if [[ "$ready" != "1" ]]; then
  echo "server did not become healthy" >&2
  cat "$LOG_FILE" >&2 || true
  exit 1
fi

# Assert readiness (recovery completed, store usable).
READY_RESP="$(curl -sf "$BASE/readyz")"
if ! grep -q '"status":"ready"' <<<"$READY_RESP"; then
  echo "readyz did not report ready: $READY_RESP" >&2
  exit 1
fi

# Lock a complete task over the public API.
LOCK_BODY='{
  "area":"smoke-building","direction":"rising",
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
}'

LOCK_RESP="$(curl -sf -X POST -H 'Content-Type: application/json' -d "$LOCK_BODY" "$BASE/v1/tasks/lock")"
if ! grep -q '"generation":1' <<<"$LOCK_RESP"; then
  echo "lock failed: $LOCK_RESP" >&2
  exit 1
fi

TASK_ID="$(sed -n 's/.*"task_id":"\([^"]*\)".*/\1/p' <<<"$LOCK_RESP")"
if [[ -z "$TASK_ID" ]]; then
  echo "lock response missing task_id: $LOCK_RESP" >&2
  exit 1
fi

# Read the task snapshot and assert the material opening balance is present.
SNAP_RESP="$(curl -sf "$BASE/v1/tasks/${TASK_ID}/snapshot")"
if ! grep -q '"raw_earth":100000' <<<"$SNAP_RESP"; then
  echo "snapshot missing opening balance: $SNAP_RESP" >&2
  exit 1
fi
if ! grep -q '"status":"active"' <<<"$SNAP_RESP"; then
  echo "snapshot missing active status: $SNAP_RESP" >&2
  exit 1
fi

echo "smoke ok: locked ${TASK_ID} and verified snapshot"
