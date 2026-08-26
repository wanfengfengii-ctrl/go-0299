package store

import "encoding/json"

// mustJSONMarshal serialises a TaskState to JSON for persistence. A TaskState
// contains only plain data (exported maps, slices and domain structs), so the
// encoding is deterministic and round-trips cleanly.
func mustJSONMarshal(st *TaskState) []byte {
	data, err := json.Marshal(st)
	if err != nil {
		panic("store: marshal task state: " + err.Error())
	}
	return data
}

// jsonUnmarshal decodes a persisted TaskState. It returns an error (rather
// than panicking) so corrupt snapshots can be reported as a recovery integrity
// failure instead of crashing the process.
func jsonUnmarshal(data []byte, st *TaskState) error {
	return json.Unmarshal(data, st)
}
