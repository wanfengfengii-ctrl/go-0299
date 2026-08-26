package arbiter

import (
	"sort"

	"rammed-earth-roof-beam-clearance/internal/domain"
)

// RebuildSet computes the unique rebuild set for a finding as a pure function
// of the locked vertical bearing adjacency and same-pan relationships
// (domain rule 7). The result is the transitive closure of:
//
//   - the origin cell (the finding's measured point),
//   - every cell sharing the origin's mix pan (same-pan relationship), and
//   - every cell reachable along a vertical bearing adjacency edge
//     (inter-layer contact faces).
//
// Identical inputs always produce an identical, deterministically sorted set
// (by layer, then cell sequence). New generations only accept their own
// receipts, so this set is re-derived rather than mutated in place.
func RebuildSet(origin domain.CellRef, adj [][2]domain.CellRef, panOf map[domain.CellRef]int) []domain.CellRef {
	pan := panOf[origin]

	inSet := map[domain.CellRef]bool{origin: true}
	work := []domain.CellRef{origin}

	// Same-pan extension: a pan's material is non-mergeable, so every cell
	// poured from the same pan is equally suspect.
	if pan > 0 {
		for c, p := range panOf {
			if p == pan {
				if !inSet[c] {
					inSet[c] = true
					work = append(work, c)
				}
			}
		}
	}

	// Adjacency closure along the locked pressure path.
	for len(work) > 0 {
		cur := work[len(work)-1]
		work = work[:len(work)-1]
		for _, e := range adj {
			var n domain.CellRef
			switch {
			case e[0] == cur:
				n = e[1]
			case e[1] == cur:
				n = e[0]
			default:
				continue
			}
			if !inSet[n] {
				inSet[n] = true
				work = append(work, n)
			}
		}
	}

	out := make([]domain.CellRef, 0, len(inSet))
	for c := range inSet {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Layer != out[j].Layer {
			return out[i].Layer < out[j].Layer
		}
		return out[i].Seq < out[j].Seq
	})
	return out
}
