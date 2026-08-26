package service

import "rammed-earth-roof-beam-clearance/internal/domain"

// countCellsInLayer returns the number of locked cells in a layer.
func countCellsInLayer(cells []domain.Cell, layer int) int {
	n := 0
	for _, c := range cells {
		if c.Layer == layer {
			n++
		}
	}
	return n
}

// cellsOfLayer returns the locked cells of a layer in sequence order.
func cellsOfLayer(cells []domain.Cell, layer int) []domain.Cell {
	var out []domain.Cell
	for _, c := range cells {
		if c.Layer == layer {
			out = append(out, c)
		}
	}
	return out
}

// highestPass returns the highest compaction pass recorded for a cell, or -1
// if none has been recorded yet. Passes must form a continuous prefix starting
// at zero.
func highestPass(events []domain.EvidenceEvent, layer, seq int) int {
	highest := -1
	for _, e := range events {
		if e.Process == domain.ProcessCompaction && e.Layer == layer && e.Seq == seq {
			if e.Pass > highest {
				highest = e.Pass
			}
		}
	}
	return highest
}

// hasProcess reports whether a layer has any evidence event of the given
// process kind.
func hasProcess(events []domain.EvidenceEvent, layer int, p domain.ProcessKind) bool {
	for _, e := range events {
		if e.Layer == layer && e.Process == p {
			return true
		}
	}
	return false
}
