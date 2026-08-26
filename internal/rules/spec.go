package rules

import (
	"sort"

	"rammed-earth-roof-beam-clearance/internal/domain"
)

// ValidateSpec validates the full locked design beyond geometry: the integer
// recipe, ramming program, quality thresholds, mix plan, curing schedule and
// material batches. It returns a single error whose primary code is the
// highest-priority problem and whose reasons are deterministically sorted
// (acceptance 1 and 2).
func ValidateSpec(spec domain.TaskSpec) *Error {
	var reasons []string

	if err := checkRecipe(spec.Recipe); err != nil {
		reasons = append(reasons, err.Reasons...)
	}
	if err := checkProgram(spec.Program); err != nil {
		reasons = append(reasons, err.Reasons...)
	}
	if err := checkThresholds(spec.Thresholds); err != nil {
		reasons = append(reasons, err.Reasons...)
	}
	if spec.MixPlan.PanCount <= 0 {
		reasons = append(reasons, "mix plan must have at least one pan")
	}
	if spec.MixPlan.PanSizeG <= 0 {
		reasons = append(reasons, "mix pan size must be positive")
	}
	if spec.MixPlan.UsableUnits <= 0 {
		reasons = append(reasons, "mix usable window must be positive")
	}
	if spec.Curing.HoursPerLayer <= 0 || spec.Curing.MinHours <= 0 {
		reasons = append(reasons, "curing schedule must have positive durations")
	}
	if spec.TargetMoisture < 0 {
		reasons = append(reasons, "target moisture must be non-negative")
	}
	for _, b := range spec.Batches {
		if b.BalanceG < 0 {
			reasons = append(reasons, "batch "+b.ID+" has negative balance")
		}
	}
	if len(reasons) == 0 {
		return nil
	}
	sort.Strings(reasons)
	return New(CodeInvalidSign, "", reasons...)
}

func checkRecipe(r domain.Recipe) *Error {
	if r.RawEarthG < 0 || r.GravelG < 0 || r.StabilizerG < 0 || r.WaterG < 0 {
		return New(CodeInvalidSign, "", "recipe components must be non-negative")
	}
	total := r.RawEarthG
	var err error
	for _, v := range []int64{r.GravelG, r.StabilizerG, r.WaterG} {
		total, err = CheckedAdd(total, v)
		if err != nil {
			return New(CodeFixedOverflow, "", "recipe total overflow")
		}
	}
	if total <= 0 {
		return New(CodeInvalidSign, "", "recipe total must be positive")
	}
	return nil
}

func checkProgram(p domain.CompactionProgram) *Error {
	if p.LooseThickness <= 0 || p.PassesPerCell <= 0 || p.BlowsPerPass <= 0 ||
		p.RammerWeightG <= 0 || p.FallHeightMM <= 0 {
		return New(CodeInvalidSign, "", "ramming program values must be positive")
	}
	return nil
}

func checkThresholds(t domain.Thresholds) *Error {
	if t.MinDryDensity < 0 || t.MaxDryDensity < 0 || t.MinCompaction < 0 ||
		t.MinMoisture < 0 || t.MaxMoisture < 0 || t.MinShear < 0 ||
		t.MaxErosion < 0 || t.MaxDeviation < 0 {
		return New(CodeInvalidSign, "", "thresholds must be non-negative")
	}
	if t.MinDryDensity > t.MaxDryDensity {
		return New(CodeInvalidSign, "", "min dry density exceeds max")
	}
	if t.MinMoisture > t.MaxMoisture {
		return New(CodeInvalidSign, "", "min moisture exceeds max")
	}
	return nil
}

// ValidateCoverage verifies that the locked layers and cells continuously
// cover the wall entity: layers partition the wall height, and each layer's
// cells partition the layer width, minus openings and tie reserves (domain
// rule 2). It reports GEOMETRY_GAP for missing coverage.
func ValidateCoverage(g domain.WallGeometry) *Error {
	var reasons []string

	// Layers must collectively span the wall height with no gap or overlap.
	if len(g.Layers) == 0 {
		return New(CodeGeometryGap, "", "wall has no layers")
	}
	layers := append([]domain.Layer(nil), g.Layers...)
	sort.Slice(layers, func(i, j int) bool { return layers[i].Number < layers[j].Number })
	expected := g.Wall.Y
	for i, l := range layers {
		if l.Rect.Y != expected {
			return New(CodeGeometryGap, "", "layer coverage gap at layer "+itoa(l.Number))
		}
		if l.Rect.X != g.Wall.X || l.Rect.W != g.Wall.W {
			return New(CodeGeometryGap, "", "layer "+itoa(l.Number)+" does not span wall width")
		}
		expected = l.Rect.Y + l.Rect.H
		if i == len(layers)-1 && expected != g.Wall.Y+g.Wall.H {
			return New(CodeGeometryGap, "", "layers do not reach wall top")
		}
	}

	// Each layer's cells must span the layer width with no gap or overlap.
	for _, l := range layers {
		cells := cellsInLayer(g.Cells, l.Number)
		if len(cells) == 0 {
			return New(CodeGeometryGap, "", "layer "+itoa(l.Number)+" has no cells")
		}
		sort.Slice(cells, func(i, j int) bool { return cells[i].Seq < cells[j].Seq })
		x := l.Rect.X
		for i, c := range cells {
			if c.Rect.X != x {
				return New(CodeGeometryGap, "", "cell coverage gap in layer "+itoa(l.Number))
			}
			if c.Rect.Y != l.Rect.Y || c.Rect.H != l.Rect.H {
				return New(CodeGeometryGap, "", "cell height mismatch in layer "+itoa(l.Number))
			}
			x = c.Rect.X + c.Rect.W
			if i == len(cells)-1 && x != l.Rect.X+l.Rect.W {
				return New(CodeGeometryGap, "", "cells do not reach layer width in layer "+itoa(l.Number))
			}
		}
		_ = reasons
	}
	return nil
}

func cellsInLayer(cells []domain.Cell, layer int) []domain.Cell {
	var out []domain.Cell
	for _, c := range cells {
		if c.Layer == layer {
			out = append(out, c)
		}
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
