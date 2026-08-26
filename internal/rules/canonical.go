package rules

import (
	"sort"

	"rammed-earth-roof-beam-clearance/internal/domain"
)

// CanonicalSpec serializes a TaskSpec into a deterministic byte encoding. All
// collections are sorted by a stable key so identical designs always produce
// identical digests regardless of request field order (domain rule 1).
func CanonicalSpec(spec domain.TaskSpec) []byte {
	var b []byte
	b = appendStr(b, spec.Area)
	b = appendStr(b, string(spec.Direction))

	b = appendRect(b, spec.Geometry.Wall)

	openings := append([]domain.Opening(nil), spec.Geometry.Openings...)
	sort.Slice(openings, func(i, j int) bool { return openings[i].Name < openings[j].Name })
	b = appendInt(b, len(openings))
	for _, o := range openings {
		b = appendStr(b, o.Name)
		b = appendRect(b, o.Rect)
	}

	ties := append([]domain.Tie(nil), spec.Geometry.Ties...)
	sort.Slice(ties, func(i, j int) bool { return ties[i].Name < ties[j].Name })
	b = appendInt(b, len(ties))
	for _, t := range ties {
		b = appendStr(b, t.Name)
		b = appendRect(b, t.Rect)
		b = appendRect(b, t.Reserve)
	}

	formTies := append([]domain.Rect(nil), spec.Geometry.FormTies...)
	sort.Slice(formTies, func(i, j int) bool { return rectLess(formTies[i], formTies[j]) })
	b = appendInt(b, len(formTies))
	for _, r := range formTies {
		b = appendRect(b, r)
	}

	layers := append([]domain.Layer(nil), spec.Geometry.Layers...)
	sort.Slice(layers, func(i, j int) bool { return layers[i].Number < layers[j].Number })
	b = appendInt(b, len(layers))
	for _, l := range layers {
		b = appendInt(b, l.Number)
		b = appendRect(b, l.Rect)
	}

	cells := append([]domain.Cell(nil), spec.Geometry.Cells...)
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].Layer != cells[j].Layer {
			return cells[i].Layer < cells[j].Layer
		}
		return cells[i].Seq < cells[j].Seq
	})
	b = appendInt(b, len(cells))
	for _, c := range cells {
		b = appendInt(b, c.Layer)
		b = appendInt(b, c.Seq)
		b = appendRect(b, c.Rect)
	}

	grid := append([]domain.Rect(nil), spec.Geometry.Grid...)
	sort.Slice(grid, func(i, j int) bool { return rectLess(grid[i], grid[j]) })
	b = appendInt(b, len(grid))
	for _, r := range grid {
		b = appendRect(b, r)
	}

	adj := append([][2]domain.CellRef(nil), spec.Geometry.PressureAdj...)
	sort.Slice(adj, func(i, j int) bool { return cellRefLess(adj[i][0], adj[j][0]) })
	b = appendInt(b, len(adj))
	for _, e := range adj {
		b = appendCellRef(b, e[0])
		b = appendCellRef(b, e[1])
	}

	batches := append([]domain.MaterialBatch(nil), spec.Batches...)
	sort.Slice(batches, func(i, j int) bool { return batches[i].ID < batches[j].ID })
	b = appendInt(b, len(batches))
	for _, m := range batches {
		b = appendStr(b, m.ID)
		b = appendStr(b, string(m.Component))
		b = appendStr(b, m.Source)
		b = AppendInt64(b, m.BalanceG)
	}

	b = AppendInt64(b, spec.Recipe.RawEarthG)
	b = AppendInt64(b, spec.Recipe.GravelG)
	b = AppendInt64(b, spec.Recipe.StabilizerG)
	b = AppendInt64(b, spec.Recipe.WaterG)
	b = AppendInt64(b, spec.TargetMoisture)

	b = AppendInt64(b, spec.Program.LooseThickness)
	b = appendInt(b, spec.Program.PassesPerCell)
	b = appendInt(b, spec.Program.BlowsPerPass)
	b = AppendInt64(b, spec.Program.RammerWeightG)
	b = AppendInt64(b, spec.Program.FallHeightMM)

	b = AppendInt64(b, spec.Thresholds.MinDryDensity)
	b = AppendInt64(b, spec.Thresholds.MaxDryDensity)
	b = AppendInt64(b, spec.Thresholds.MinCompaction)
	b = AppendInt64(b, spec.Thresholds.MinMoisture)
	b = AppendInt64(b, spec.Thresholds.MaxMoisture)
	b = AppendInt64(b, spec.Thresholds.MinShear)
	b = AppendInt64(b, spec.Thresholds.MaxErosion)
	b = AppendInt64(b, spec.Thresholds.MaxDeviation)

	b = appendInt(b, spec.Curing.HoursPerLayer)
	b = appendInt(b, spec.Curing.MinHours)

	b = appendInt(b, spec.MixPlan.PanCount)
	b = AppendInt64(b, spec.MixPlan.PanSizeG)
	b = AppendInt64(b, spec.MixPlan.UsableUnits)

	specimens := append([]domain.Specimen(nil), spec.Specimens...)
	sort.Slice(specimens, func(i, j int) bool { return specimens[i].ID < specimens[j].ID })
	b = appendInt(b, len(specimens))
	for _, s := range specimens {
		b = appendStr(b, s.ID)
		b = appendStr(b, s.Point)
	}

	return b
}

func appendStr(b []byte, s string) []byte {
	b = appendInt(b, len(s))
	return append(b, s...)
}

func appendInt(b []byte, v int) []byte {
	return AppendInt64(b, int64(v))
}

func appendRect(b []byte, r domain.Rect) []byte {
	b = AppendInt64(b, r.X)
	b = AppendInt64(b, r.Y)
	b = AppendInt64(b, r.W)
	b = AppendInt64(b, r.H)
	return b
}

func appendCellRef(b []byte, c domain.CellRef) []byte {
	b = appendInt(b, c.Layer)
	b = appendInt(b, c.Seq)
	return b
}

func rectLess(a, b domain.Rect) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	if a.W != b.W {
		return a.W < b.W
	}
	return a.H < b.H
}

func cellRefLess(a, b domain.CellRef) bool {
	if a.Layer != b.Layer {
		return a.Layer < b.Layer
	}
	return a.Seq < b.Seq
}
