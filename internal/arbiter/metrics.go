// Package arbiter implements the inspection, rebuild and terminal-verdict
// arbitrator: overflow-checked fixed-point quality metrics, threshold
// findings, the pure-function rebuild set, dual independent review and the
// single-write terminal verdict barrier.
package arbiter

import (
	"rammed-earth-roof-beam-clearance/internal/rules"
)

// Metric is a computed fixed-point quality value.
type Metric struct {
	Name  string
	Value int64
}

// MoistureContent computes water/dryMass as a percentage in fixed point
// (permille scale). All inputs are integer grams; a non-positive dry mass is a
// divide-by-zero style error.
func MoistureContent(waterG, dryMassG int64) (int64, error) {
	return rules.PercentOf(waterG, dryMassG)
}

// DryDensity computes dryMass/volume in fixed point (grams per litre scaled).
func DryDensity(dryMassG, volume int64) (int64, error) {
	return rules.ScaledDiv(dryMassG, volume)
}

// CompactionRatio computes dryDensity/maxDryDensity in fixed point.
func CompactionRatio(dryDensity, maxDryDensity int64) (int64, error) {
	if maxDryDensity == 0 {
		return 0, rules.New(rules.CodeDivideByZero, "", "max dry density is zero")
	}
	return rules.ScaledDiv(dryDensity, maxDryDensity)
}

// CompactiveEnergy computes (weight*fallHeight*blows)/volume in fixed point,
// with overflow checks on the intermediate product.
func CompactiveEnergy(weight, fallHeight, blows, volume int64) (int64, error) {
	if volume == 0 {
		return 0, rules.New(rules.CodeDivideByZero, "", "volume is zero")
	}
	work, err := rules.CheckedMul(weight, fallHeight)
	if err != nil {
		return 0, err
	}
	work, err = rules.CheckedMul(work, blows)
	if err != nil {
		return 0, err
	}
	return rules.HalfAwayFromZero(work, volume)
}

// ShearStrength computes shearLoad/contactArea in fixed point.
func ShearStrength(shearLoad, contactArea int64) (int64, error) {
	return rules.ScaledDiv(shearLoad, contactArea)
}

// ErosionLossRate computes massLoss/originalMass as a percentage in fixed
// point (permille scale).
func ErosionLossRate(massLoss, originalMass int64) (int64, error) {
	return rules.PercentOf(massLoss, originalMass)
}

// VerticalDeviation computes deviation/height in fixed point (permille scale),
// rejecting a zero height.
func VerticalDeviation(deviation, height int64) (int64, error) {
	if height == 0 {
		return 0, rules.New(rules.CodeDivideByZero, "", "height is zero")
	}
	return rules.ScaledDiv(deviation, height)
}
